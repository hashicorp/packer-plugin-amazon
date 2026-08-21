// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package awserrors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/smithy-go"
)

func apiErr(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code + " message", Fault: smithy.FaultServer}
}

func TestIsCapacityError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"InsufficientInstanceCapacity is a capacity error", apiErr("InsufficientInstanceCapacity"), true},
		{"InsufficientHostCapacity is a capacity error", apiErr("InsufficientHostCapacity"), true},
		{"InsufficientReservedInstanceCapacity is a capacity error", apiErr("InsufficientReservedInstanceCapacity"), true},
		{"InstanceLimitExceeded is not a capacity error", apiErr("InstanceLimitExceeded"), false},
		{"Unsupported is not a capacity error", apiErr("Unsupported"), false},
		{"AuthFailure is not a capacity error", apiErr("AuthFailure"), false},
		{"InvalidParameterValue is not a capacity error", apiErr("InvalidParameterValue"), false},
		{"non-API error is not a capacity error", errors.New("some generic failure"), false},
		{"nil error is not a capacity error", nil, false},
		{"fmt-wrapped capacity error is still detected", fmt.Errorf("launch failed: %w", apiErr("InsufficientInstanceCapacity")), true},
		{"capacity error wrapped in OperationError is detected", &smithy.OperationError{ServiceID: "EC2", OperationName: "RunInstances", Err: apiErr("InsufficientInstanceCapacity")}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCapacityError(tt.err); got != tt.want {
				t.Errorf("IsCapacityError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
