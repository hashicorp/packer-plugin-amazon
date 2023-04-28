// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import "fmt"

// IsValidBootMode checks that the bootmode is a value supported by AWS
func IsValidBootMode(bootmode string) error {
	validModes := []string{"legacy-bios", "uefi", "uefi-preferred"}

	for _, mode := range validModes {
		if bootmode == mode {
			return nil
		}
	}

	return fmt.Errorf("invalid boot mode %q, valid values are either 'uefi', 'legacy-bios' or 'uefi-preferred'", bootmode)
}
