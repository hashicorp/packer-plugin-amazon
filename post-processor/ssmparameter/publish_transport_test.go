// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package ssmparameter

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func TestPublishParameterHTTP(t *testing.T) {
	for _, tt := range []struct {
		name             string
		status           int
		body             string
		dropResponse     bool
		cancelAfterPut   bool
		indeterminate    bool
		wantServiceError error
		wantGets         int32
	}{
		{name: "connection drops after write", dropResponse: true, indeterminate: true},
		{name: "context canceled after write", cancelAfterPut: true, indeterminate: true},
		{name: "server error", status: http.StatusInternalServerError, body: `{"__type":"InternalServerError","message":"server failure"}`, indeterminate: true, wantServiceError: &types.InternalServerError{}},
		{name: "missing version", status: http.StatusOK, body: `{}`, indeterminate: true},
		{name: "invalid response body", status: http.StatusOK, body: `{`, indeterminate: true},
		{name: "parameter already exists", status: http.StatusBadRequest, body: `{"__type":"ParameterAlreadyExists","message":"already exists"}`, wantServiceError: &types.ParameterAlreadyExists{}},
		{name: "only Get retries", status: http.StatusOK, body: `{"Version":4}`, wantGets: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var puts, gets atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-amz-json-1.1")
				switch r.Header.Get("X-Amz-Target") {
				case "AmazonSSM.PutParameter":
					if _, err := io.Copy(io.Discard, r.Body); err != nil {
						t.Errorf("reading write request: %v", err)
						return
					}
					puts.Add(1)
					if tt.dropResponse {
						conn, _, err := http.NewResponseController(w).Hijack()
						if err != nil {
							t.Errorf("hijacking connection: %v", err)
							return
						}
						conn.Close()
						return
					}
					if tt.cancelAfterPut {
						cancel()
						<-r.Context().Done()
						return
					}
					w.WriteHeader(tt.status)
					io.WriteString(w, tt.body)
				case "AmazonSSM.GetParameter":
					if gets.Add(1) == 1 {
						w.WriteHeader(http.StatusInternalServerError)
						io.WriteString(w, `{"__type":"InternalServerError"}`)
						return
					}
					io.WriteString(w, `{"Parameter":{"Name":"/images/latest","Value":"ami-new","Version":4}}`)
				default:
					t.Errorf("unexpected X-Amz-Target: %q", r.Header.Get("X-Amz-Target"))
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()

			parameter, err := publishParameter(ctx, newPublishHTTPClient(server.URL, nil), &ssm.PutParameterInput{
				Name: aws.String("/images/latest"), Value: aws.String("ami-new"),
			}, time.Nanosecond)
			if puts.Load() != 1 || gets.Load() != tt.wantGets {
				t.Errorf("got %d Put and %d Get HTTP requests; want 1 Put and %d Gets", puts.Load(), gets.Load(), tt.wantGets)
			}
			switch {
			case tt.wantGets > 0:
				if err != nil || parameter == nil || parameter.Version != 4 {
					t.Fatalf("publishParameter() = (%v, %v), want version 4", parameter, err)
				}
			case err == nil:
				t.Fatal("publishParameter() succeeded, want an error")
			case tt.indeterminate:
				if !strings.Contains(err.Error(), "indeterminate") || !strings.Contains(err.Error(), "before retrying") {
					t.Errorf("publishParameter() error = %v, want indeterminate write with retry guidance", err)
				}
			default:
				if strings.Contains(err.Error(), "indeterminate") {
					t.Errorf("publishParameter() error = %v, want a definite service error", err)
				}
			}
			switch tt.wantServiceError.(type) {
			case *types.InternalServerError:
				var serviceErr *types.InternalServerError
				if !errors.As(err, &serviceErr) {
					t.Errorf("publishParameter() error = %v, want wrapped %T", err, tt.wantServiceError)
				}
			case *types.ParameterAlreadyExists:
				var serviceErr *types.ParameterAlreadyExists
				if !errors.As(err, &serviceErr) {
					t.Errorf("publishParameter() error = %v, want wrapped %T", err, tt.wantServiceError)
				}
			}
			if tt.cancelAfterPut && !errors.Is(err, context.Canceled) {
				t.Errorf("publishParameter() error = %v, want wrapped cancellation", err)
			}
		})
	}
}

func TestPublishParameterDoesNotReportIndeterminateWriteBeforeConnecting(t *testing.T) {
	dialErr := errors.New("test dial error")
	transport := &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) { return nil, dialErr },
	}
	_, err := publishParameter(context.Background(), newPublishHTTPClient("http://ssm.example.test", &http.Client{Transport: transport}), &ssm.PutParameterInput{
		Name: aws.String("/images/latest"), Value: aws.String("ami-new"),
	}, time.Nanosecond)
	if !errors.Is(err, dialErr) || strings.Contains(err.Error(), "indeterminate") {
		t.Fatalf("publishParameter() error = %v, want the definite dial error", err)
	}
}

func newPublishHTTPClient(endpoint string, client ssm.HTTPClient) *ssm.Client {
	return ssm.New(ssm.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
		HTTPClient:   client,
		Retryer: retry.NewStandard(func(o *retry.StandardOptions) {
			o.MaxAttempts = 3
			o.Backoff = retry.BackoffDelayerFunc(func(int, error) (time.Duration, error) { return 0, nil })
		}),
	})
}
