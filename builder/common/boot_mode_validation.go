package common

import "fmt"

// IsValidBootMode checks that the bootmode is a value supported by AWS
func IsValidBootMode(bootmode string) error {
	validModes := []string{"legacy-bios", "uefi"}

	for _, mode := range validModes {
		if bootmode == mode {
			return nil
		}
	}

	return fmt.Errorf("invalid boot mode %q, valid values are either 'uefi' or 'legacy-bios'", bootmode)
}
