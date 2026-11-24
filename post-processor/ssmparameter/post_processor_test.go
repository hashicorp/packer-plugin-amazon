// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package ssmparameter

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/rpc"
)

func TestConfigSpecIncludesAWSAccess(t *testing.T) {
	spec := new(PostProcessor).ConfigSpec()
	for _, key := range []string{"parameter_name", "profile", "assume_role", "tags"} {
		if _, ok := spec[key]; !ok {
			t.Errorf("missing HCL configuration %q", key)
		}
	}
	for _, key := range []string{"name", "share_enabled", "share_name", "share_arn"} {
		if _, ok := spec[key]; ok {
			t.Errorf("unexpected HCL configuration %q", key)
		}
	}
}

func TestPostProcessRecognizesAMIArtifactOverRPC(t *testing.T) {
	client := startPostProcessorRPC(t, new(PostProcessor))
	_, _, _, err := client.PostProcess(context.Background(), new(packersdk.MockUi), &awscommon.Artifact{})
	if err == nil || !strings.Contains(err.Error(), "no AMIs found") {
		t.Fatalf("expected an empty AMI artifact error, got %v", err)
	}
}

func TestPostProcessPublishesAndRetainsAMIsOverRPC(t *testing.T) {
	for _, failWest := range []bool{false, true} {
		name := "success"
		if failWest {
			name = "partial failure"
		}
		t.Run(name, func(t *testing.T) {
			amis := map[string]string{"us-east-1": "ami-00000000000000001", "us-west-2": "ami-00000000000000002"}
			var puts, reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				region := "us-east-1"
				auth := r.Header.Get("Authorization")
				if strings.Contains(auth, "/us-west-2/ssm/") {
					region = "us-west-2"
				} else if !strings.Contains(auth, "/us-east-1/ssm/") {
					t.Errorf("unexpected signing region: %s", auth)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["Name"] != "/images/latest" {
					t.Errorf("unexpected parameter name: %v", body["Name"])
				}
				w.Header().Set("Content-Type", "application/x-amz-json-1.1")
				switch operation := r.Header.Get("X-Amz-Target"); operation {
				case "AmazonSSM.PutParameter":
					puts.Add(1)
					if body["Value"] != amis[region] || body["Type"] != "String" || body["DataType"] != "aws:ec2:image" || body["Overwrite"] != true {
						t.Errorf("unexpected request for %s: %v", region, body)
					}
					if failWest && region == "us-west-2" {
						w.WriteHeader(http.StatusBadRequest)
						fmt.Fprint(w, `{"__type":"AccessDeniedException","message":"test write rejected"}`)
						return
					}
					fmt.Fprint(w, `{"Version":4}`)
				case "AmazonSSM.GetParameter":
					reads.Add(1)
					fmt.Fprintf(w, `{"Parameter":{"ARN":"arn:aws:ssm:%s:000000000000:parameter/images/latest","Version":4,"Value":%q}}`, region, amis[region])
				default:
					t.Errorf("unexpected SSM operation: %q", operation)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			t.Setenv("AWS_ENDPOINT_URL_SSM", server.URL)
			client := startPostProcessorRPC(t, new(PostProcessor))
			if err := client.Configure(map[string]any{
				"parameter_name": "/images/latest", "overwrite": true,
				"access_key": "test-key", "secret_key": "test-secret", "skip_credential_validation": true,
			}); err != nil {
				t.Fatal(err)
			}
			// No EC2 config is needed: Destroy must not try to deregister these AMIs.
			input := &awscommon.Artifact{Amis: amis}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			output, keep, force, err := client.PostProcess(ctx, new(packersdk.MockUi), input)
			wantReads := int32(2)
			if failWest {
				wantReads = 1
				if err == nil || !strings.Contains(err.Error(), "us-west-2") || !strings.Contains(err.Error(), "test write rejected") {
					t.Fatalf("expected the western region failure, got %v", err)
				}
			} else {
				if err != nil || !keep || !force || output == nil {
					t.Fatalf("expected forced input retention, got keep=%t force=%t output=%v err=%v", keep, force, output, err)
				}
				if output.Id() != input.Id() {
					t.Fatalf("output artifact ID = %q, want %q", output.Id(), input.Id())
				}
				if err := output.Destroy(); err != nil {
					t.Fatalf("chained cleanup: %v", err)
				}
			}
			if puts.Load() != 2 || reads.Load() != wantReads {
				t.Fatalf("expected 2 writes and %d reads, got %d writes and %d reads", wantReads, puts.Load(), reads.Load())
			}
			if err := input.Destroy(); err != nil {
				t.Fatalf("builder artifact cleanup: %v", err)
			}
		})
	}
}

func TestPostProcessPublishesTagsOverRPC(t *testing.T) {
	tags := map[string]string{"environment": "production", "owner": "packer"}
	for _, test := range []struct {
		name          string
		overwrite     bool
		tags          map[string]string
		failTagRegion string
		wantTagCalls  int
		wantErrRegion string
	}{
		{
			name:         "create includes tags in PutParameter",
			tags:         tags,
			wantTagCalls: 0,
		},
		{
			name:         "overwrite adds tags after verification",
			overwrite:    true,
			tags:         tags,
			wantTagCalls: 2,
		},
		{
			name:          "tag failure includes region and retains AMIs",
			overwrite:     true,
			tags:          tags,
			failTagRegion: "us-west-2",
			wantTagCalls:  2,
			wantErrRegion: "us-west-2",
		},
		{
			name:         "omitted tags do not call AddTagsToResource",
			overwrite:    true,
			wantTagCalls: 0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			amis := map[string]string{"us-east-1": "ami-00000000000000001", "us-west-2": "ami-00000000000000002"}
			var puts, reads, tagCalls atomic.Int32
			regionOperations := map[string][]string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				region := "us-east-1"
				auth := r.Header.Get("Authorization")
				if strings.Contains(auth, "/us-west-2/ssm/") {
					region = "us-west-2"
				} else if !strings.Contains(auth, "/us-east-1/ssm/") {
					t.Errorf("unexpected signing region: %s", auth)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "application/x-amz-json-1.1")
				switch operation := r.Header.Get("X-Amz-Target"); operation {
				case "AmazonSSM.PutParameter":
					puts.Add(1)
					regionOperations[region] = append(regionOperations[region], "PutParameter")
					if body["Name"] != "/images/latest" || body["Value"] != amis[region] || body["Type"] != "String" || body["DataType"] != "aws:ec2:image" || body["Overwrite"] != test.overwrite {
						t.Errorf("unexpected PutParameter request for %s: %v", region, body)
					}
					if test.overwrite || len(test.tags) == 0 {
						if _, ok := body["Tags"]; ok {
							t.Errorf("PutParameter for overwrite=%t included Tags: %v", test.overwrite, body["Tags"])
						}
					} else {
						assertSSMRequestTags(t, body["Tags"], test.tags, "PutParameter", region)
					}
					fmt.Fprint(w, `{"Version":4}`)
				case "AmazonSSM.GetParameter":
					reads.Add(1)
					regionOperations[region] = append(regionOperations[region], "GetParameter")
					fmt.Fprintf(w, `{"Parameter":{"ARN":"arn:aws:ssm:%s:000000000000:parameter/images/latest","Version":4,"Value":%q}}`, region, amis[region])
				case "AmazonSSM.AddTagsToResource":
					tagCalls.Add(1)
					regionOperations[region] = append(regionOperations[region], "AddTagsToResource")
					if body["ResourceType"] != "Parameter" || body["ResourceId"] != "/images/latest" {
						t.Errorf("unexpected AddTagsToResource target for %s: %v", region, body)
					}
					assertSSMRequestTags(t, body["Tags"], test.tags, "AddTagsToResource", region)
					if region == test.failTagRegion {
						w.WriteHeader(http.StatusBadRequest)
						fmt.Fprint(w, `{"__type":"AccessDeniedException","message":"test tag rejected"}`)
						return
					}
					fmt.Fprint(w, `{}`)
				default:
					t.Errorf("unexpected SSM operation: %q", operation)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			t.Setenv("AWS_ENDPOINT_URL_SSM", server.URL)
			client := startPostProcessorRPC(t, new(PostProcessor))
			configuration := map[string]any{
				"parameter_name": "/images/latest", "overwrite": test.overwrite,
				"access_key": "test-key", "secret_key": "test-secret", "skip_credential_validation": true,
			}
			if test.tags != nil {
				configuration["tags"] = test.tags
			}
			if err := client.Configure(configuration); err != nil {
				t.Fatalf("Configure: %v", err)
			}

			input := &awscommon.Artifact{Amis: amis}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			output, keep, force, err := client.PostProcess(ctx, new(packersdk.MockUi), input)
			if test.wantErrRegion != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErrRegion) || !strings.Contains(err.Error(), "test tag rejected") {
					t.Fatalf("expected tag failure for %s, got %v", test.wantErrRegion, err)
				}
				if output != nil || keep || force {
					t.Fatalf("failed publication returned output=%v keep=%t force=%t", output, keep, force)
				}
				if len(input.Amis) != len(amis) || input.Amis["us-east-1"] != amis["us-east-1"] || input.Amis["us-west-2"] != amis["us-west-2"] {
					t.Fatalf("tag failure changed builder AMIs: %v", input.Amis)
				}
			} else {
				if err != nil || !keep || !force || output == nil {
					t.Fatalf("expected forced input retention, got keep=%t force=%t output=%v err=%v", keep, force, output, err)
				}
				if output.Id() != input.Id() {
					t.Fatalf("output artifact ID = %q, want %q", output.Id(), input.Id())
				}
				if err := output.Destroy(); err != nil {
					t.Fatalf("chained cleanup: %v", err)
				}
			}
			if puts.Load() != 2 || reads.Load() != 2 || int(tagCalls.Load()) != test.wantTagCalls {
				t.Fatalf("expected 2 puts, 2 reads, and %d tag calls; got %d puts, %d reads, %d tag calls", test.wantTagCalls, puts.Load(), reads.Load(), tagCalls.Load())
			}
			for _, region := range []string{"us-east-1", "us-west-2"} {
				wantOperations := []string{"PutParameter", "GetParameter"}
				if test.overwrite && len(test.tags) > 0 {
					wantOperations = append(wantOperations, "AddTagsToResource")
				}
				if got := strings.Join(regionOperations[region], ","); got != strings.Join(wantOperations, ",") {
					t.Errorf("operations in %s = %s, want %s", region, got, strings.Join(wantOperations, ","))
				}
			}
			if err := input.Destroy(); err != nil {
				t.Fatalf("builder artifact cleanup: %v", err)
			}
		})
	}
}

func assertSSMRequestTags(t *testing.T, raw any, want map[string]string, operation, region string) {
	t.Helper()
	values, ok := raw.([]any)
	if !ok {
		t.Errorf("%s tags for %s have type %T, want a list: %v", operation, region, raw, raw)
		return
	}
	got := make(map[string]string, len(values))
	for _, value := range values {
		tag, ok := value.(map[string]any)
		if !ok {
			t.Errorf("%s tag for %s has type %T, want an object", operation, region, value)
			continue
		}
		key, keyOK := tag["Key"].(string)
		val, valueOK := tag["Value"].(string)
		if !keyOK || !valueOK {
			t.Errorf("%s tag for %s missing Key or Value: %v", operation, region, tag)
			continue
		}
		got[key] = val
	}
	if len(got) != len(want) {
		t.Errorf("%s tags for %s = %v, want %v", operation, region, got, want)
		return
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s tag %q for %s = %q, want %q", operation, key, region, got[key], value)
		}
	}
}

func startPostProcessorRPC(t *testing.T, pp *PostProcessor) packersdk.PostProcessor {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close(); _ = clientConn.Close() })
	server, err := rpc.NewServer(serverConn)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.RegisterPostProcessor(pp); err != nil {
		t.Fatal(err)
	}
	go server.Serve()
	client, err := rpc.NewClient(clientConn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client.PostProcessor()
}
