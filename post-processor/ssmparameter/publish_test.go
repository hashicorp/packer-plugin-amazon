// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package ssmparameter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

type publishTestClient struct {
	putOutput *ssm.PutParameterOutput
	putErr    error
	get       func(context.Context, *ssm.GetParameterInput) (*ssm.GetParameterOutput, error)
	putCalls  int
	getCalls  int
}

func (c *publishTestClient) PutParameter(context.Context, *ssm.PutParameterInput, ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	c.putCalls++
	return c.putOutput, c.putErr
}

func (c *publishTestClient) GetParameter(ctx context.Context, input *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	c.getCalls++
	return c.get(ctx, input)
}

func TestPublishParameterWaitsForWrittenVersionAndValue(t *testing.T) {
	client := &publishTestClient{
		putOutput: &ssm.PutParameterOutput{Version: 4},
	}
	responses := []*types.Parameter{
		{Name: aws.String("/images/latest"), Value: aws.String("ami-old"), Version: 3},
		{Name: aws.String("/images/latest"), Value: aws.String("ami-new"), Version: 4},
	}
	client.get = func(_ context.Context, input *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
		if aws.ToString(input.Name) != "/images/latest" {
			t.Fatalf("GetParameter name = %q", aws.ToString(input.Name))
		}
		parameter := responses[0]
		responses = responses[1:]
		return &ssm.GetParameterOutput{Parameter: parameter}, nil
	}

	input := &ssm.PutParameterInput{Name: aws.String("/images/latest"), Value: aws.String("ami-new")}
	got, err := publishParameter(context.Background(), client, input, time.Nanosecond)
	if err != nil {
		t.Fatalf("publishParameter() error = %v", err)
	}
	if got.Version != 4 || aws.ToString(got.Value) != "ami-new" {
		t.Fatalf("publishParameter() = %#v, want version 4 and ami-new", got)
	}
	if client.putCalls != 1 {
		t.Errorf("PutParameter calls = %d, want 1", client.putCalls)
	}
	if client.getCalls != 2 {
		t.Errorf("GetParameter calls = %d, want 2", client.getCalls)
	}
}

func TestPublishParameterRetriesNotFoundAndUnvalidatedValue(t *testing.T) {
	client := &publishTestClient{putOutput: &ssm.PutParameterOutput{Version: 2}}
	responses := []struct {
		output *ssm.GetParameterOutput
		err    error
	}{
		{err: &smithy.GenericAPIError{Code: "ParameterNotFound", Message: "not found"}},
		{output: &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String("ami-old"), Version: 2}}},
		{output: &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String("ami-new"), Version: 2}}},
	}
	client.get = func(context.Context, *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
		response := responses[0]
		responses = responses[1:]
		return response.output, response.err
	}

	got, err := publishParameter(context.Background(), client, &ssm.PutParameterInput{
		Name: aws.String("/images/latest"), Value: aws.String("ami-new"),
	}, time.Nanosecond)
	if err != nil {
		t.Fatalf("publishParameter() error = %v", err)
	}
	if got.Version != 2 || client.getCalls != 3 {
		t.Fatalf("got version %d after %d reads; want version 2 after 3 reads", got.Version, client.getCalls)
	}
}

func TestPublishParameterFailsWhenNewerVersionCompetes(t *testing.T) {
	client := &publishTestClient{
		putOutput: &ssm.PutParameterOutput{Version: 2},
		get: func(context.Context, *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String("ami-other"), Version: 3}}, nil
		},
	}

	_, err := publishParameter(context.Background(), client, &ssm.PutParameterInput{
		Name: aws.String("/images/latest"), Value: aws.String("ami-new"),
	}, time.Nanosecond)
	if err == nil || !strings.Contains(err.Error(), "version is 3") {
		t.Fatalf("publishParameter() error = %v, want useful competing-version error", err)
	}
}

func TestPublishParameterReturnsNonretryableGetError(t *testing.T) {
	wantErr := errors.New("access denied")
	client := &publishTestClient{
		putOutput: &ssm.PutParameterOutput{Version: 2},
		get: func(context.Context, *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
			return nil, wantErr
		},
	}

	_, err := publishParameter(context.Background(), client, &ssm.PutParameterInput{
		Name: aws.String("/images/latest"), Value: aws.String("ami-new"),
	}, time.Nanosecond)
	if !errors.Is(err, wantErr) {
		t.Fatalf("publishParameter() error = %v, want wrapped access denied", err)
	}
	if client.getCalls != 1 {
		t.Errorf("GetParameter calls = %d, want 1", client.getCalls)
	}
}

func TestPublishParameterReturnsDefinitePutError(t *testing.T) {
	wantErr := errors.New("put failed")
	client := &publishTestClient{putErr: wantErr}
	client.get = func(context.Context, *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
		t.Fatal("GetParameter called after PutParameter failed")
		return nil, nil
	}

	_, err := publishParameter(context.Background(), client, &ssm.PutParameterInput{
		Name: aws.String("/images/latest"), Value: aws.String("ami-new"),
	}, time.Nanosecond)
	if !errors.Is(err, wantErr) {
		t.Fatalf("publishParameter() error = %v, want wrapped put failure", err)
	}
	if strings.Contains(err.Error(), "indeterminate") {
		t.Errorf("publishParameter() error = %v, want a definite put failure", err)
	}
}

func TestPublishParameterRespectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &publishTestClient{
		putOutput: &ssm.PutParameterOutput{Version: 2},
		get: func(context.Context, *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
			cancel()
			return nil, &smithy.GenericAPIError{Code: "ParameterNotFound", Message: "not found"}
		},
	}

	_, err := publishParameter(ctx, client, &ssm.PutParameterInput{
		Name: aws.String("/images/latest"), Value: aws.String("ami-new"),
	}, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("publishParameter() error = %v, want context.Canceled", err)
	}
}
