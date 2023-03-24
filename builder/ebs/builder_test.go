package ebs

import (
	"fmt"
	"testing"
	"time"

	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func testConfig() map[string]interface{} {
	return map[string]interface{}{
		"access_key":    "foo",
		"secret_key":    "bar",
		"source_ami":    "foo",
		"instance_type": "foo",
		"region":        "us-east-1",
		"ssh_username":  "root",
		"ami_name":      "foo",
	}
}

func TestBuilder_ImplementsBuilder(t *testing.T) {
	var raw interface{}
	raw = &Builder{}
	if _, ok := raw.(packersdk.Builder); !ok {
		t.Fatalf("Builder should be a builder")
	}
}

func TestBuilder_Prepare_BadType(t *testing.T) {
	b := &Builder{}
	c := map[string]interface{}{
		"access_key": []string{},
	}

	_, warnings, err := b.Prepare(c)
	if len(warnings) > 0 {
		t.Fatalf("bad: %#v", warnings)
	}
	if err == nil {
		t.Fatalf("prepare should fail")
	}
}

func TestBuilderPrepare_AMIName(t *testing.T) {
	var b Builder
	config := testConfig()

	// Test good
	config["ami_name"] = "foo"
	_, warnings, err := b.Prepare(config)
	if len(warnings) > 0 {
		t.Fatalf("bad: %#v", warnings)
	}
	if err != nil {
		t.Fatalf("should not have error: %s", err)
	}

	// Test bad
	config["ami_name"] = "foo {{"
	b = Builder{}
	_, warnings, err = b.Prepare(config)
	if len(warnings) > 0 {
		t.Fatalf("bad: %#v", warnings)
	}
	if err == nil {
		t.Fatal("should have error")
	}

	// Test bad
	delete(config, "ami_name")
	b = Builder{}
	_, warnings, err = b.Prepare(config)
	if len(warnings) > 0 {
		t.Fatalf("bad: %#v", warnings)
	}
	if err == nil {
		t.Fatal("should have error")
	}
}

func TestBuilderPrepare_InvalidKey(t *testing.T) {
	var b Builder
	config := testConfig()

	// Add a random key
	config["i_should_not_be_valid"] = true
	_, warnings, err := b.Prepare(config)
	if len(warnings) > 0 {
		t.Fatalf("bad: %#v", warnings)
	}
	if err == nil {
		t.Fatal("should have error")
	}
}

func TestBuilderPrepare_InvalidShutdownBehavior(t *testing.T) {
	var b Builder
	config := testConfig()

	// Test good
	config["shutdown_behavior"] = "terminate"
	_, warnings, err := b.Prepare(config)
	if len(warnings) > 0 {
		t.Fatalf("bad: %#v", warnings)
	}
	if err != nil {
		t.Fatalf("should not have error: %s", err)
	}

	// Test good
	config["shutdown_behavior"] = "stop"
	_, warnings, err = b.Prepare(config)
	if len(warnings) > 0 {
		t.Fatalf("bad: %#v", warnings)
	}
	if err != nil {
		t.Fatalf("should not have error: %s", err)
	}

	// Test bad
	config["shutdown_behavior"] = "foobar"
	_, warnings, err = b.Prepare(config)
	if len(warnings) > 0 {
		t.Fatalf("bad: %#v", warnings)
	}
	if err == nil {
		t.Fatal("should have error")
	}
}

func TestBuilderPrepare_DeprecationTime(t *testing.T) {
	var b Builder
	config := testConfig()

	currentTime := time.Now().UTC()
	testcases := []struct {
		name            string
		deprecationTime string
		isErr           bool
	}{
		{"good", currentTime.Format(time.RFC3339), false},
		{"not in format (YYYY-MM-DDTHH:MM:SSZ)", currentTime.Format(time.ANSIC), true},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("%s_%s", tc.deprecationTime, tc.name), func(t *testing.T) {
			config["deprecate_at"] = tc.deprecationTime
			_, warnings, err := b.Prepare(config)
			if len(warnings) > 0 {
				t.Fatalf("bad: %#v", warnings)
			}
			if tc.isErr && err == nil {
				t.Error("Expect error")
			}
			if !tc.isErr && err != nil {
				t.Errorf("Expect no error. got %s", err)
			}
		})
	}
}

func TestBuilderPrepare_ReturnGeneratedData(t *testing.T) {
	var b Builder
	config := testConfig()

	generatedData, warnings, err := b.Prepare(config)
	if len(warnings) > 0 {
		t.Fatalf("bad: %#v", warnings)
	}
	if err != nil {
		t.Fatalf("should not have error: %s", err)
	}
	if len(generatedData) == 0 {
		t.Fatalf("Generated data should not be empty")
	}
	if len(generatedData) == 0 {
		t.Fatalf("Generated data should not be empty")
	}
	if generatedData[0] != "SourceAMIName" {
		t.Fatalf("Generated data should contain SourceAMIName")
	}
	if generatedData[1] != "BuildRegion" {
		t.Fatalf("Generated data should contain BuildRegion")
	}
	if generatedData[2] != "SourceAMI" {
		t.Fatalf("Generated data should contain SourceAMI")
	}
	if generatedData[3] != "SourceAMICreationDate" {
		t.Fatalf("Generated data should contain SourceAMICreationDate")
	}
	if generatedData[4] != "SourceAMIOwner" {
		t.Fatalf("Generated data should contain SourceAMIOwner")
	}
	if generatedData[5] != "SourceAMIOwnerName" {
		t.Fatalf("Generated data should contain SourceAMIOwnerName")
	}
}

func TestBuilerPrepare_IMDSSupport(t *testing.T) {
	testcases := []struct {
		name             string
		imdsSupportValue string
		isErr            bool
	}{
		{
			name:             "define valid IMDSv2 support",
			imdsSupportValue: "v2.0",
			isErr:            false,
		},
		{
			name:             "don't define IMDSv2 support",
			imdsSupportValue: "",
			isErr:            false,
		},
		{
			name:             "invalid IMDS support",
			imdsSupportValue: "v1.0",
			isErr:            true,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			var b Builder
			config := testConfig()

			config["imds_support"] = tt.imdsSupportValue
			_, warnings, err := b.Prepare(config)

			if len(warnings) > 0 {
				t.Fatalf("bad: %#v", warnings)
			}
			if (err != nil) != tt.isErr {
				t.Errorf("error mismatch, expected %t, got %t", tt.isErr, err != nil)
			}

			if err != nil {
				t.Logf("error: %s", err)
			}
		})
	}
}

func TestBuilderPrepare_FastLaunch(t *testing.T) {
	tests := []struct {
		name             string
		fastLaunchConfig map[string]interface{}
		expectError      bool
	}{
		{
			"OK - empty config",
			map[string]interface{}{},
			false,
		},
		{
			"OK - all specified, with template id",
			map[string]interface{}{
				"fast_launch": map[string]interface{}{
					"enable_fast_launch":    true,
					"template_id":           "id",
					"template_version":      2,
					"max_parallel_launches": 10,
					"target_resource_count": 20,
				},
			},
			false,
		},
		{
			"OK - all specified, with template name",
			map[string]interface{}{
				"fast_launch": map[string]interface{}{
					"enable_fast_launch":    true,
					"template_name":         "name",
					"template_version":      2,
					"max_parallel_launches": 10,
					"target_resource_count": 20,
				},
			},
			false,
		},
		{
			"Error - max parallel launches < 6",
			map[string]interface{}{
				"fast_launch": map[string]interface{}{
					"max_parallel_launches": 3,
				},
			},
			true,
		},
		{
			"Error - target resource count < 0",
			map[string]interface{}{
				"fast_launch": map[string]interface{}{
					"target_resource_count": -1,
				},
			},
			true,
		},
		{
			"Error - launch template ID & name specified",
			map[string]interface{}{
				"fast_launch": map[string]interface{}{
					"template_id":   "id",
					"template_name": "name",
				},
			},
			true,
		},
		{
			"Error - launch template version without name/id",
			map[string]interface{}{
				"fast_launch": map[string]interface{}{
					"template_version": 2,
				},
			},
			true,
		},
		{
			"Error - launch template version < 0",
			map[string]interface{}{
				"fast_launch": map[string]interface{}{
					"template_version": -1,
				},
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b Builder
			config := testConfig()

			for k, v := range tt.fastLaunchConfig {
				config[k] = v
			}

			_, warnings, err := b.Prepare(config)

			if len(warnings) > 0 {
				t.Errorf("got unexpected warnings: %#v", warnings)
			}
			if (err != nil) != tt.expectError {
				t.Errorf("error mismatch, expected %t, got %t", tt.expectError, err != nil)
			}

			if err != nil {
				t.Logf("got error: %s", err)
			}
		})
	}
}
