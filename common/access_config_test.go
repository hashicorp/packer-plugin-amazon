// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/hashicorp/packer-plugin-sdk/common"
)

func TestAccessConfigPrepare_Region(t *testing.T) {
	c := FakeAccessConfig()

	ctx := context.TODO()
	c.RawRegion = "us-east-12"
	err := c.ValidateRegion(ctx, c.RawRegion)
	if err == nil {
		t.Fatalf("should have region validation err: %s", c.RawRegion)
	}

	c.RawRegion = "us-east-1"
	err = c.ValidateRegion(ctx, c.RawRegion)
	if err != nil {
		t.Fatalf("shouldn't have region validation err: %s", c.RawRegion)
	}

	c.RawRegion = "custom"
	err = c.ValidateRegion(ctx, c.RawRegion)
	if err == nil {
		t.Fatalf("should have region validation err: %s", c.RawRegion)
	}
}

func TestAccessConfigPrepare_RegionRestricted(t *testing.T) {
	c := FakeAccessConfig()

	c.config = mustLoadConfig(config.WithRegion("us-gov-west-1"))

	packerConfig := &common.PackerConfig{
		PackerCoreVersion: "0.0.0",
	}
	if err := c.Prepare(packerConfig); err != nil {
		t.Fatalf("shouldn't have err: %s", err)
	}

	if !c.IsGovCloud() {
		t.Fatal("We should be in gov region.")
	}
}

func TestAccessConfigPrepare_UnknownPackerCoreVersion(t *testing.T) {
	c := FakeAccessConfig()

	// Create a Session with a custom region
	c.config = mustLoadConfig(config.WithRegion("us-east-1"))

	if err := c.Prepare(nil); err != nil {
		t.Fatalf("shouldn't have err: %s", err)
	}

	if c.packerConfig.PackerCoreVersion != "unknown" {
		t.Fatalf("packer core version should be unknown, but got %s", c.packerConfig.PackerCoreVersion)
	}
}

func TestAccessConfig_SkipCredsValidationDoesNotDisableIMDS(t *testing.T) {
	// This test specifically verifies that SkipCredsValidation does not disable IMDS
	// which was the bug that was fixed in the commit that moved AMI datasource from
	// builder/common to common
	tests := []struct {
		name                 string
		skipMetadataApiCheck bool
		skipCredsValidation  bool
		shouldPass           bool
		description          string
	}{
		{
			name:                 "IMDS enabled when skip_credential_validation is true but skip_metadata_api_check is false",
			skipMetadataApiCheck: false,
			skipCredsValidation:  true,
			shouldPass:           true,
			description:          "This is the key scenario that was broken - IMDS should be enabled for credential resolution",
		},
		{
			name:                 "IMDS disabled when skip_metadata_api_check is true",
			skipMetadataApiCheck: true,
			skipCredsValidation:  true,
			shouldPass:           true,
			description:          "IMDS should be disabled when explicitly requested",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AccessConfig{
				SkipMetadataApiCheck: tt.skipMetadataApiCheck,
				SkipCredsValidation:  tt.skipCredsValidation,
				RawRegion:            "us-east-1",
				// Set credentials to avoid credential resolution errors in tests
				AccessKey: "test-key",
				SecretKey: "test-secret",
			}

			cfg, err := c.getBaseAwsConfig(context.Background())
			if tt.shouldPass && err != nil {
				t.Fatalf("getBaseAwsConfig() should not fail for %s, got error: %v", tt.description, err)
			}

			if tt.shouldPass && cfg.Region != "us-east-1" {
				t.Errorf("Expected region us-east-1, got %s", cfg.Region)
			}
		})
	}
}
