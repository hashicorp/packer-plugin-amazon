// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"fmt"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// IsValidBootMode checks that the bootmode is a value supported by AWS
func IsValidBootMode(bootmode string) error {
	validModes := []ec2types.BootModeValues{ec2types.BootModeValuesLegacyBios, ec2types.BootModeValuesUefi, ec2types.BootModeValuesUefiPreferred}

	for _, mode := range validModes {
		if bootmode == string(mode) {
			return nil
		}
	}

	return fmt.Errorf("invalid boot mode %q, valid values are either 'uefi', 'legacy-bios' or 'uefi-preferred'", bootmode)
}
