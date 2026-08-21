// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package awserrors

import (
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

// Returns true if the err matches all these conditions:
//   - err is of type awserr.Error
//   - Error.Code() matches code
//   - Error.Message() contains message
func Matches(err error, code string, message string) bool {
	if err, ok := err.(awserr.Error); ok {
		return err.Code() == code && strings.Contains(err.Message(), message)
	}
	return false
}

// capacityErrorCodes are the EC2 API error codes that indicate a launch failed
// purely because the requested instance type has no available capacity, and so
// an equivalent alternate type may still launch. Configuration, permission, and
// quota errors are deliberately excluded so they surface immediately.
var capacityErrorCodes = map[string]struct{}{
	"InsufficientInstanceCapacity":         {},
	"InsufficientHostCapacity":             {},
	"InsufficientReservedInstanceCapacity": {},
}

// IsCapacityError reports whether err is an EC2 insufficient-capacity error.
// It uses errors.As, so a wrapped capacity error is still recognized.
func IsCapacityError(err error) bool {
	var apiErr awserr.Error
	if errors.As(err, &apiErr) {
		_, ok := capacityErrorCodes[apiErr.Code()]
		return ok
	}
	return false
}
