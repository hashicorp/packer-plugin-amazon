// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package awserrors

import (
	"errors"
	"strings"

	"github.com/aws/smithy-go"
)

// Returns true if the err matches all these conditions:
//   - err is of type awserr.Error
//   - Error.Code() matches code
//   - Error.Message() contains message
func Matches(err error, code string, message string) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == code && strings.Contains(apiErr.ErrorMessage(), message)
	}
	return false
}
