// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package awserrors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

func TestIsCapacityError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"InsufficientInstanceCapacity is a capacity error", awserr.New("InsufficientInstanceCapacity", "no capacity", nil), true},
		{"InsufficientHostCapacity is a capacity error", awserr.New("InsufficientHostCapacity", "no host", nil), true},
		{"InsufficientReservedInstanceCapacity is a capacity error", awserr.New("InsufficientReservedInstanceCapacity", "no reserved", nil), true},
		{"InstanceLimitExceeded is not a capacity error", awserr.New("InstanceLimitExceeded", "quota", nil), false},
		{"Unsupported is not a capacity error", awserr.New("Unsupported", "unsupported", nil), false},
		{"AuthFailure is not a capacity error", awserr.New("AuthFailure", "denied", nil), false},
		{"InvalidParameterValue is not a capacity error", awserr.New("InvalidParameterValue", "bad", nil), false},
		{"non-awserr error is not a capacity error", errors.New("generic"), false},
		{"nil error is not a capacity error", nil, false},
		{"fmt-wrapped capacity error is still detected", fmt.Errorf("launch failed: %w", awserr.New("InsufficientInstanceCapacity", "no capacity", nil)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCapacityError(tt.err); got != tt.want {
				t.Errorf("IsCapacityError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
