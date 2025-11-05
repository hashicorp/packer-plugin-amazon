// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package ssmparameter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type ssmClient interface {
	PutParameter(context.Context, *ssm.PutParameterInput, ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// publishParameter writes a parameter once, then waits until the latest value
// returned by SSM confirms both the version and value from that write.
func publishParameter(ctx context.Context, client ssmClient, input *ssm.PutParameterInput, pollInterval time.Duration) (*types.Parameter, error) {
	if input == nil {
		return nil, fmt.Errorf("put SSM parameter: input is nil")
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	name := aws.ToString(input.Name)
	value := aws.ToString(input.Value)

	var putMayHaveReachedSSM atomic.Bool
	putOutput, err := client.PutParameter(ctx, input, func(options *ssm.Options) {
		// PutParameter has no idempotency token. A retry after a lost response
		// can create another version. Do not change the retryer for GetParameter.
		options.Retryer = aws.NopRetryer{}
		options.HTTPClient = singlePutHTTPClient(options.HTTPClient, &putMayHaveReachedSSM)
	})
	if err != nil {
		if isIndeterminatePut(err, putMayHaveReachedSSM.Load()) {
			return nil, indeterminatePutError(name, err)
		}
		return nil, fmt.Errorf("put SSM parameter %q: %w", name, err)
	}
	if putOutput == nil {
		return nil, indeterminatePutError(name, errors.New("empty response"))
	}
	if putOutput.Version <= 0 {
		return nil, indeterminatePutError(name, fmt.Errorf("invalid version %d", putOutput.Version))
	}

	confirmationError := func(err error) error {
		return fmt.Errorf("waiting for SSM parameter %q version %d: %w", name, putOutput.Version, err)
	}
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, confirmationError(ctxErr)
		}

		getOutput, getErr := client.GetParameter(ctx, &ssm.GetParameterInput{Name: input.Name})
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, confirmationError(ctxErr)
		}
		if getErr != nil {
			if !isParameterPublicationPending(getErr) {
				return nil, fmt.Errorf("get SSM parameter %q after publishing version %d: %w", name, putOutput.Version, getErr)
			}
		} else if getOutput != nil && getOutput.Parameter != nil {
			parameter := getOutput.Parameter
			switch {
			case parameter.Version > putOutput.Version:
				return nil, fmt.Errorf("SSM parameter %q was changed by another writer: published version %d but latest version is %d", name, putOutput.Version, parameter.Version)
			case parameter.Version == putOutput.Version && parameter.Value != nil && *parameter.Value == value:
				return parameter, nil
			}
		}

		// A missing parameter, an older version, or a mismatched value is not confirmation.
		select {
		case <-ctx.Done():
			return nil, confirmationError(ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

func singlePutHTTPClient(client ssm.HTTPClient, mayHaveReachedSSM *atomic.Bool) ssm.HTTPClient {
	// Go's standard transport reports GotConn before attempting to write the
	// HTTP request. This distinguishes dial/TLS failures from potential writes,
	// including when the SDK replaces a send failure with context cancellation.
	_, traced := client.(*awshttp.BuildableClient)
	if c, ok := client.(*http.Client); ok {
		transport := c.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		_, traced = transport.(*http.Transport)
	}
	return smithyhttp.ClientDoFunc(func(request *http.Request) (*http.Response, error) {
		ctx := request.Context()
		if traced {
			ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
				GotConn: func(httptrace.GotConnInfo) { mayHaveReachedSSM.Store(true) },
			})
		} else {
			// An opaque HTTP client may not emit net/http trace events.
			mayHaveReachedSSM.Store(true)
		}
		request = request.WithContext(ctx)
		// Keep the non-idempotent POST body single-use at the HTTP layer too.
		request.GetBody = nil
		return client.Do(request)
	})
}

func isIndeterminatePut(err error, mayHaveReachedSSM bool) bool {
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPStatusCode() != 0 {
		status := responseErr.HTTPStatusCode()
		// A failed decode of a successful response or a server failure does not
		// establish whether the write committed. A client rejection does.
		return status >= 500 || (status >= 200 && status < 300)
	}
	return mayHaveReachedSSM
}

func indeterminatePutError(name string, err error) error {
	return fmt.Errorf("put SSM parameter %q: write outcome is indeterminate; check the current parameter value and version before retrying: %w", name, err)
}

func isParameterPublicationPending(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "ParameterNotFound", "ParameterVersionNotFound", "ParameterNotReady", "ParameterNotValidated":
		return true
	default:
		return false
	}
}
