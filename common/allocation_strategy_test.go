package common

import "testing"

func TestIsValidSpotAllocationStrategy(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		expectError bool
	}{
		{
			"Valid value lowest-price",
			"lowest-price",
			false,
		},
		{
			"Valid value diversified",
			"diversified",
			false,
		},
		{
			"Valid value capacity-optimized",
			"capacity-optimized",
			false,
		},
		{
			"Invalid value notavalidstrategy",
			"notavalidstrategy",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidSpotAllocationStrategy(tt.value)
			if (err != nil) != tt.expectError {
				t.Errorf("error mismatch, expected %t, got %t", tt.expectError, err != nil)
				if err != nil {
					t.Logf("got error: %s", err)
				}
			}
		})
	}
}
