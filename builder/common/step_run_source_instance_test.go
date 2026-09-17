// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	confighelper "github.com/hashicorp/packer-plugin-sdk/template/config"
)

// fakeEC2Server spins up an httptest server that emulates just enough of the
// EC2 API (RunInstances and DescribeInstances) for StepRunSourceInstance.Run
// to complete successfully, while capturing the raw RunInstances request
// body so tests can assert on the parameters that were actually sent to AWS.
func fakeEC2Server(t *testing.T) (*ec2.EC2, *url.Values) {
	t.Helper()

	var runInstancesParams url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %s", err)
		}
		vals, err := url.ParseQuery(string(b))
		if err != nil {
			t.Fatalf("failed to parse request body: %s", err)
		}

		w.Header().Set("Content-Type", "text/xml")
		switch vals.Get("Action") {
		case "RunInstances":
			runInstancesParams = vals
			w.Write([]byte(`<RunInstancesResponse>
				<reservationId>r-123</reservationId>
				<instancesSet><item><instanceId>i-123</instanceId></item></instancesSet>
			</RunInstancesResponse>`))
		case "DescribeInstances":
			w.Write([]byte(`<DescribeInstancesResponse>
				<reservationSet><item><instancesSet><item>
					<instanceId>i-123</instanceId>
					<instanceState><code>16</code><name>running</name></instanceState>
				</item></instancesSet></item></reservationSet>
			</DescribeInstancesResponse>`))
		default:
			w.Write([]byte(`<Response></Response>`))
		}
	}))
	t.Cleanup(srv.Close)

	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String(srv.URL),
		Credentials: credentials.NewStaticCredentials("test-access-key", "test-secret-key", ""),
		MaxRetries:  aws.Int(0),
	}))

	return ec2.New(sess), &runInstancesParams
}

func tStateRunSourceInstance(conn *ec2.EC2) multistep.StateBag {
	state := new(multistep.BasicStateBag)
	state.Put("ui", packersdk.TestUi(new(testing.T)))
	state.Put("ec2", conn)
	state.Put("availability_zone", "us-east-1a")
	state.Put("securityGroupIds", []string{"sg-0b8984db72f213dc3"})
	state.Put("iamInstanceProfile", "")
	state.Put("subnet_id", "")
	state.Put("source_image", testImage())
	return state
}

// TestStepRunSourceInstance_HostId verifies that when Placement.HostId is
// configured, it is propagated through to the RunInstances API call. This
// guards against regressions like the one fixed for the ebssurrogate,
// ebsvolume, and instance builders, which previously failed to forward the
// host_id setting to AWS despite accepting and validating it.
func TestStepRunSourceInstance_HostId(t *testing.T) {
	conn, runInstancesParams := fakeEC2Server(t)

	state := tStateRunSourceInstance(conn)
	state.Put("ui", packersdk.TestUi(t))

	step := &StepRunSourceInstance{
		PollingConfig:            &AWSPollingConfig{},
		AssociatePublicIpAddress: confighelper.TriUnset,
		LaunchMappings:           BlockDevices{},
		ExpectedRootDevice:       "ebs",
		InstanceType:             "t2.micro",
		Comm:                     &communicator.Config{},
		HostId:                   "h-0123456789abcdef0",
	}

	action := step.Run(context.Background(), state)
	if err := state.Get("error"); err != nil {
		t.Fatalf("should not have errored, but got: %s", err)
	}
	if action != multistep.ActionContinue {
		t.Fatalf("expected action to continue, got: %v", action)
	}

	got := runInstancesParams.Get("Placement.HostId")
	if got != step.HostId {
		t.Fatalf("expected Placement.HostId to be %q, got %q", step.HostId, got)
	}
}

// TestStepRunSourceInstance_HostResourceGroupArn is the equivalent test for
// the sibling HostResourceGroupArn field, kept alongside HostId for
// symmetry and to make it clear both placement options are wired the same
// way.
func TestStepRunSourceInstance_HostResourceGroupArn(t *testing.T) {
	conn, runInstancesParams := fakeEC2Server(t)

	state := tStateRunSourceInstance(conn)
	state.Put("ui", packersdk.TestUi(t))

	step := &StepRunSourceInstance{
		PollingConfig:            &AWSPollingConfig{},
		AssociatePublicIpAddress: confighelper.TriUnset,
		LaunchMappings:           BlockDevices{},
		ExpectedRootDevice:       "ebs",
		InstanceType:             "t2.micro",
		Comm:                     &communicator.Config{},
		HostResourceGroupArn:     "arn:aws:resource-groups:us-east-1:123456789012:group/test-group",
	}

	action := step.Run(context.Background(), state)
	if err := state.Get("error"); err != nil {
		t.Fatalf("should not have errored, but got: %s", err)
	}
	if action != multistep.ActionContinue {
		t.Fatalf("expected action to continue, got: %v", action)
	}

	got := runInstancesParams.Get("Placement.HostResourceGroupArn")
	if got != step.HostResourceGroupArn {
		t.Fatalf("expected Placement.HostResourceGroupArn to be %q, got %q", step.HostResourceGroupArn, got)
	}
}
