// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type shutdownCleanupClient struct {
	clients.Ec2Client
	calls, terminations int
}

func (m *shutdownCleanupClient) TerminateInstances(context.Context, *ec2.TerminateInstancesInput, ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	m.terminations++
	return &ec2.TerminateInstancesOutput{}, nil
}
func (m *shutdownCleanupClient) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	m.calls++
	state := types.InstanceStateNameStopping
	if m.calls > 1 {
		state = types.InstanceStateNameTerminated
	}
	return &ec2.DescribeInstancesOutput{Reservations: []types.Reservation{{Instances: []types.Instance{{InstanceId: aws.String("i-test"), State: &types.InstanceState{Name: state}}}}}}, nil
}
func TestCleanupAfterConfirmedGuestShutdown(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		name := "default"
		if confirmed {
			name = "confirmed shutdown"
		}
		t.Run(name, func(t *testing.T) {
			client := new(shutdownCleanupClient)
			state := new(multistep.BasicStateBag)
			state.Put("ec2v2", client)
			state.Put("ui", new(packersdk.MockUi))
			if confirmed {
				state.Put(GuestShutdownConfirmedKey, true)
			}
			step := &StepRunSourceInstance{instanceId: "i-test", PollingConfig: &AWSPollingConfig{DelaySeconds: 1}}
			step.Cleanup(state)
			wantCalls := 1
			if confirmed {
				wantCalls = 2
			}
			if client.calls != wantCalls || client.terminations != 1 {
				t.Fatalf("describe calls=%d want %d, terminate calls=%d", client.calls, wantCalls, client.terminations)
			}
		})
	}
}
