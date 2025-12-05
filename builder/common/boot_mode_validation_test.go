// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import "testing"

func TestIsValidBuildMode(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		expectError bool
	}{
		{
			"Valid value legacy-bios",
			"legacy-bios",
			false,
		},
		{
			"Valid value uefi",
			"uefi",
			false,
		},
		{
			"Valid value uefi-preferred",
			"uefi-preferred",
			false,
		},
		{
			"Invalid value uefipreferred",
			"uefipreferred",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidBootMode(tt.value)
			if (err != nil) != tt.expectError {
				t.Errorf("error mismatch, expected %t, got %t", tt.expectError, err != nil)
				if err != nil {
					t.Logf("got error: %s", err)
				}
			}
		})
	}
}
