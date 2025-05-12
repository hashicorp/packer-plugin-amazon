// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"

	"github.com/aws/smithy-go/middleware"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	stsv2 "github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
)

var encodedFailureMessagePattern = regexp.MustCompile(`(?i)(.*) Encoded authorization failure message: ([\w-]+) ?( .*)?`)

type stsDecoder interface {
	DecodeAuthorizationMessage(ctx context.Context, input *stsv2.DecodeAuthorizationMessageInput, optFns ...func(*stsv2.Options)) (*stsv2.DecodeAuthorizationMessageOutput, error)
}

func decodeAWSErrorV2(ctx context.Context, decoder stsDecoder, err error) error {

	groups := encodedFailureMessagePattern.FindStringSubmatch(err.Error())
	if len(groups) > 1 {
		result, decodeErr := decoder.DecodeAuthorizationMessage(ctx, &stsv2.DecodeAuthorizationMessageInput{
			EncodedMessage: awsv2.String(groups[2]),
		})
		if decodeErr == nil {
			msg := awsv2.ToString(result.DecodedMessage)
			return fmt.Errorf("%s Authorization failure message: '%s'%s", groups[1], msg, groups[3])
		}
		log.Printf("[WARN] Attempted to decode authorization message, but received: %v", decodeErr)
	}
	return err
}

// DecodeAuthZMessages enables automatic decoding of any
// encoded authorization messages
func DecodeAuthZMessages(cfg awsv2.Config) {
	authzMsgDecoder := &authZMessageDecoder{
		Decoder: stsv2.NewFromConfig(cfg),
	}

	// Add middleware to the config for error handling
	cfg.APIOptions = append(cfg.APIOptions, func(stack *middleware.Stack) error {
		return stack.Finalize.Add(
			middleware.FinalizeMiddlewareFunc("DecodeAuthZMessages", authzMsgDecoder.middleware),
			middleware.After,
		)
	})
}

type authZMessageDecoder struct {
	Decoder stsDecoder
}

func (a *authZMessageDecoder) middleware(
	ctx context.Context,
	in middleware.FinalizeInput,
	next middleware.FinalizeHandler,
) (middleware.FinalizeOutput, middleware.Metadata, error) {
	// Call the next middleware/handler
	out, metadata, err := next.HandleFinalize(ctx, in)

	// Check for UnauthorizedOperation error
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "UnauthorizedOperation" {
			// Decode the error and replace it
			decodedErr := decodeAWSErrorV2(ctx, a.Decoder, apiErr)
			return out, metadata, decodedErr
		}
	}

	return out, metadata, err
}
