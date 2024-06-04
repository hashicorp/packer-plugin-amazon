// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebssurrogate

import (
	"testing"

	"github.com/hashicorp/packer-plugin-amazon/builder/common"

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
	}
}

func TestBuilder_ImplementsBuilder(t *testing.T) {
	var raw interface{}
	raw = &Builder{}
	if _, ok := raw.(packersdk.Builder); !ok {
		t.Fatal("Builder should be a builder")
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
		t.Fatal("prepare should fail")
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

func TestBuilderPrepare_UefiData(t *testing.T) {
	tests := []struct {
		name         string
		bootMode     string
		uefiData     string
		architecture string
		expectError  bool
	}{
		{
			name:        "OK - boot mode set to uefi",
			uefiData:    "foo",
			bootMode:    "uefi",
			expectError: false,
		},
		{
			name:        "Error - boot mode set to legacy-bios",
			uefiData:    "foo",
			bootMode:    "legacy-bios",
			expectError: true,
		},
		{
			name:        "Error - default boot mode is legacy-bios",
			uefiData:    "foo",
			expectError: true,
		},
		{
			name:         "OK - default boot mode for arm64 is uefi",
			uefiData:     "foo",
			architecture: "arm64",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := testConfig()
			config["ami_name"] = "name"
			config["ami_virtualization_type"] = "kvm"
			config["uefi_data"] = tt.uefiData
			config["boot_mode"] = tt.bootMode
			config["ami_architecture"] = tt.architecture

			b := &Builder{}
			// Basic configuration
			b.config.RootDevice = RootBlockDevice{
				SourceDeviceName: "device name",
				DeviceName:       "device name",
			}
			b.config.LaunchMappings = BlockDevices{
				BlockDevice{
					BlockDevice: common.BlockDevice{
						DeviceName: "device name",
					},
					OmitFromArtifact: false,
				},
			}

			_, _, err := b.Prepare(config)
			if err != nil && !tt.expectError {
				t.Fatalf("got unexpected error: %s", err)
			}
			if err == nil && tt.expectError {
				t.Fatalf("expected an error, got a success instead")
			}

			if err != nil {
				t.Logf("OK: b.Prepare produced expected error: %s", err)
			}
		})
	}
}

func TestBuilderPrepare_ReturnGeneratedData(t *testing.T) {
	var b Builder
	// Basic configuration
	b.config.RootDevice = RootBlockDevice{
		SourceDeviceName: "device name",
		DeviceName:       "device name",
	}
	b.config.LaunchMappings = BlockDevices{
		BlockDevice{
			BlockDevice: common.BlockDevice{
				DeviceName: "device name",
			},
			OmitFromArtifact: false,
		},
	}
	b.config.AMIVirtType = "type"
	config := testConfig()
	config["ami_name"] = "name"

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

func TestBuilderPrepare_IMDSSupportValue(t *testing.T) {
	tests := []struct {
		name        string
		optValue    string
		expectError bool
	}{
		{
			name:        "OK - no value set",
			optValue:    "",
			expectError: false,
		},
		{
			name:        "OK - v2.0",
			optValue:    "v2.0",
			expectError: false,
		},
		{
			name:        "Error - bad value set",
			optValue:    "v3.0",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := testConfig()
			config["ami_name"] = "name"
			config["ami_virtualization_type"] = "kvm"
			config["imds_support"] = tt.optValue

			b := &Builder{}
			// Basic configuration
			b.config.RootDevice = RootBlockDevice{
				SourceDeviceName: "device name",
				DeviceName:       "device name",
			}
			b.config.LaunchMappings = BlockDevices{
				BlockDevice{
					BlockDevice: common.BlockDevice{
						DeviceName: "device name",
					},
					OmitFromArtifact: false,
				},
			}

			_, _, err := b.Prepare(config)
			if err != nil && !tt.expectError {
				t.Fatalf("got unexpected error: %s", err)
			}
			if err == nil && tt.expectError {
				t.Fatalf("expected an error, got a success instead")
			}

			if err != nil {
				t.Logf("OK: b.Prepare produced expected error: %s", err)
			}
		})
	}
}

func TestBuilderPrepare_TpmSupportValue(t *testing.T) {
	tests := []struct {
		name        string
		optValue    string
		expectError bool
	}{
		{
			name:        "OK - no value set",
			optValue:    "",
			expectError: false,
		},
		{
			name:        "OK - v2.0",
			optValue:    "v2.0",
			expectError: false,
		},
		{
			name:        "Error - bad value set",
			optValue:    "v3.0",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := testConfig()
			config["ami_name"] = "name"
			config["ami_virtualization_type"] = "kvm"

			config["tpm_support"] = tt.optValue

			b := &Builder{}
			// Basic configuration
			b.config.RootDevice = RootBlockDevice{
				SourceDeviceName: "device name",
				DeviceName:       "device name",
			}
			b.config.LaunchMappings = BlockDevices{
				BlockDevice{
					BlockDevice: common.BlockDevice{
						DeviceName: "device name",
					},
					OmitFromArtifact: false,
				},
			}

			_, _, err := b.Prepare(config)
			if err != nil && !tt.expectError {
				t.Fatalf("got unexpected error: %s", err)
			}
			if err == nil && tt.expectError {
				t.Fatalf("expected an error, got a success instead")
			}

			if err != nil {
				t.Logf("OK: b.Prepare produced expected error: %s", err)
			}
		})
	}
}
