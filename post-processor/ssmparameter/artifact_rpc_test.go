// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package ssmparameter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/packer-plugin-amazon/builder/chroot"
	legacy "github.com/hashicorp/packer-plugin-amazon/builder/common"
	"github.com/hashicorp/packer-plugin-amazon/builder/instance"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func TestArtifactRPCUsesReservedStateAndRetainsAMIsOnPartialFailure(t *testing.T) {
	const eastAMI = "ami-00000000000000001"
	const westAMI = "ami-00000000000000002"
	amis := map[string]string{"us-east-1": eastAMI, "us-west-2": westAMI}
	state := map[string]interface{}{
		awscommon.AMIIDsStateKey:     map[string]string{"us-east-1": westAMI},
		awscommon.RetainAMIsStateKey: false,
	}
	for _, test := range []struct {
		name  string
		input packersdk.Artifact
	}{
		{"modern", &awscommon.Artifact{Amis: amis, StateData: state}},
		{"chroot", &legacy.Artifact{Amis: amis, StateData: state, BuilderIdValue: chroot.BuilderId}},
		{"instance", &legacy.Artifact{Amis: amis, StateData: state, BuilderIdValue: instance.BuilderId}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var eastWrites, westWrites atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				region := "us-east-1"
				if auth := r.Header.Get("Authorization"); strings.Contains(auth, "/us-west-2/ssm/") {
					region = "us-west-2"
				} else if !strings.Contains(auth, "/us-east-1/ssm/") {
					t.Errorf("unexpected signing region: %s", auth)
				}
				w.Header().Set("Content-Type", "application/x-amz-json-1.1")
				switch operation := r.Header.Get("X-Amz-Target"); operation {
				case "AmazonSSM.PutParameter":
					var body struct{ Value string }
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.Value != amis[region] {
						t.Errorf("AMI written in %s = %q, want %q", region, body.Value, amis[region])
					}
					if region == "us-west-2" {
						westWrites.Add(1)
						w.WriteHeader(http.StatusBadRequest)
						fmt.Fprint(w, `{"__type":"AccessDeniedException","message":"test write rejected"}`)
						return
					}
					eastWrites.Add(1)
					fmt.Fprint(w, `{"Version":4}`)
				case "AmazonSSM.GetParameter":
					fmt.Fprintf(w, `{"Parameter":{"ARN":"arn:aws:ssm:%s:000000000000:parameter/images/latest","Version":4,"Value":%q}}`, region, amis[region])
				default:
					t.Errorf("unexpected SSM operation: %q", operation)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			t.Cleanup(server.Close)
			t.Setenv("AWS_ENDPOINT_URL_SSM", server.URL)
			client := startPostProcessorRPC(t, new(PostProcessor))
			if err := client.Configure(map[string]interface{}{
				"parameter_name": "/images/latest", "overwrite": true,
				"access_key": "test-key", "secret_key": "test-secret", "skip_credential_validation": true,
			}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, keep, force, err := client.PostProcess(ctx, new(packersdk.MockUi), test.input)
			if err == nil || !strings.Contains(err.Error(), "us-west-2") || !strings.Contains(err.Error(), "test write rejected") {
				t.Fatalf("expected the western region failure, got %v", err)
			}
			if out != nil || keep || force {
				t.Fatalf("failed publication returned artifact=%v keep=%t force=%t", out, keep, force)
			}
			if eastWrites.Load() != 1 || westWrites.Load() != 1 {
				t.Fatalf("writes east=%d west=%d, want one per region", eastWrites.Load(), westWrites.Load())
			}
			// Simulate core cleanup after the error. No EC2 config is needed when retention was set remotely.
			if err := test.input.Destroy(); err != nil {
				t.Fatalf("cleanup after a partial publication: %v", err)
			}
		})
	}
}
