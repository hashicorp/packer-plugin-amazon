// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type mockEIPClient struct {
	clients.Ec2Client

	associateAddressFunc    func(ctx context.Context, params *ec2.AssociateAddressInput, optFns ...func(*ec2.Options)) (*ec2.AssociateAddressOutput, error)
	disassociateAddressFunc func(ctx context.Context, params *ec2.DisassociateAddressInput, optFns ...func(*ec2.Options)) (*ec2.DisassociateAddressOutput, error)
	describeInstancesFunc   func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)

	associateCalls    int
	disassociateCalls int
	describeCalls     int
}

func (m *mockEIPClient) AssociateAddress(ctx context.Context, params *ec2.AssociateAddressInput, optFns ...func(*ec2.Options)) (*ec2.AssociateAddressOutput, error) {
	m.associateCalls++
	return m.associateAddressFunc(ctx, params, optFns...)
}

func (m *mockEIPClient) DisassociateAddress(ctx context.Context, params *ec2.DisassociateAddressInput, optFns ...func(*ec2.Options)) (*ec2.DisassociateAddressOutput, error) {
	m.disassociateCalls++
	return m.disassociateAddressFunc(ctx, params, optFns...)
}

func (m *mockEIPClient) DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	m.describeCalls++
	return m.describeInstancesFunc(ctx, params, optFns...)
}

func testEIPState(client clients.Ec2Client) multistep.StateBag {
	state := new(multistep.BasicStateBag)
	state.Put("ec2v2", client)
	state.Put("ui", &packersdk.BasicUi{Writer: new(bytes.Buffer)})
	state.Put("instance", ec2types.Instance{
		InstanceId: aws.String("i-1234567890abcdef0"),
	})
	return state
}

func TestStepAssociateEIP_NoOp(t *testing.T) {
	mock := &mockEIPClient{}
	state := testEIPState(mock)

	step := &StepAssociateEIP{AllocationId: ""}
	action := step.Run(context.Background(), state)
	if action != multistep.ActionContinue {
		t.Fatalf("expected ActionContinue, got %v", action)
	}
	if mock.associateCalls != 0 {
		t.Errorf("expected no AssociateAddress calls, got %d", mock.associateCalls)
	}
}

func TestStepAssociateEIP_Run_Success(t *testing.T) {
	refreshedInstance := ec2types.Instance{
		InstanceId:      aws.String("i-1234567890abcdef0"),
		PublicIpAddress: aws.String("1.2.3.4"),
	}
	mock := &mockEIPClient{
		associateAddressFunc: func(ctx context.Context, params *ec2.AssociateAddressInput, optFns ...func(*ec2.Options)) (*ec2.AssociateAddressOutput, error) {
			return &ec2.AssociateAddressOutput{AssociationId: aws.String("eipassoc-abc123")}, nil
		},
		describeInstancesFunc: func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{
				Reservations: []ec2types.Reservation{
					{Instances: []ec2types.Instance{refreshedInstance}},
				},
			}, nil
		},
	}
	state := testEIPState(mock)

	step := &StepAssociateEIP{AllocationId: "eipalloc-0123456789abcdef0"}
	action := step.Run(context.Background(), state)
	if action != multistep.ActionContinue {
		t.Fatalf("expected ActionContinue, got %v", action)
	}
	if step.associationId != "eipassoc-abc123" {
		t.Errorf("unexpected associationId: %s", step.associationId)
	}
	inst := state.Get("instance").(ec2types.Instance)
	if aws.ToString(inst.PublicIpAddress) != "1.2.3.4" {
		t.Errorf("expected refreshed instance in state, got PublicIpAddress=%v", inst.PublicIpAddress)
	}
}

func TestStepAssociateEIP_Run_AssociateError(t *testing.T) {
	mock := &mockEIPClient{
		associateAddressFunc: func(ctx context.Context, params *ec2.AssociateAddressInput, optFns ...func(*ec2.Options)) (*ec2.AssociateAddressOutput, error) {
			return nil, fmt.Errorf("access denied")
		},
	}
	state := testEIPState(mock)

	step := &StepAssociateEIP{AllocationId: "eipalloc-0123456789abcdef0"}
	action := step.Run(context.Background(), state)
	if action != multistep.ActionHalt {
		t.Fatalf("expected ActionHalt, got %v", action)
	}
	if state.Get("error") == nil {
		t.Error("expected error in state")
	}
}

func TestStepAssociateEIP_Run_DescribeError(t *testing.T) {
	mock := &mockEIPClient{
		associateAddressFunc: func(ctx context.Context, params *ec2.AssociateAddressInput, optFns ...func(*ec2.Options)) (*ec2.AssociateAddressOutput, error) {
			return &ec2.AssociateAddressOutput{AssociationId: aws.String("eipassoc-abc123")}, nil
		},
		describeInstancesFunc: func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return nil, fmt.Errorf("describe failed")
		},
	}
	state := testEIPState(mock)

	step := &StepAssociateEIP{AllocationId: "eipalloc-0123456789abcdef0"}
	action := step.Run(context.Background(), state)
	if action != multistep.ActionHalt {
		t.Fatalf("expected ActionHalt, got %v", action)
	}
	if state.Get("error") == nil {
		t.Error("expected error in state")
	}
}

func TestStepAssociateEIP_Cleanup_Disassociates(t *testing.T) {
	mock := &mockEIPClient{
		disassociateAddressFunc: func(ctx context.Context, params *ec2.DisassociateAddressInput, optFns ...func(*ec2.Options)) (*ec2.DisassociateAddressOutput, error) {
			return &ec2.DisassociateAddressOutput{}, nil
		},
	}
	state := testEIPState(mock)

	step := &StepAssociateEIP{
		AllocationId:  "eipalloc-0123456789abcdef0",
		associationId: "eipassoc-abc123",
	}
	step.Cleanup(state)
	if mock.disassociateCalls != 1 {
		t.Errorf("expected 1 DisassociateAddress call, got %d", mock.disassociateCalls)
	}
}

func TestStepAssociateEIP_Cleanup_NoopWhenEmpty(t *testing.T) {
	mock := &mockEIPClient{}
	state := testEIPState(mock)

	step := &StepAssociateEIP{AllocationId: "eipalloc-0123456789abcdef0", associationId: ""}
	step.Cleanup(state)
	if mock.disassociateCalls != 0 {
		t.Errorf("expected no DisassociateAddress calls, got %d", mock.disassociateCalls)
	}
}

func TestStepAssociateEIP_Cleanup_DisassociateError(t *testing.T) {
	mock := &mockEIPClient{
		disassociateAddressFunc: func(ctx context.Context, params *ec2.DisassociateAddressInput, optFns ...func(*ec2.Options)) (*ec2.DisassociateAddressOutput, error) {
			return nil, fmt.Errorf("network error")
		},
	}
	state := testEIPState(mock)

	step := &StepAssociateEIP{
		AllocationId:  "eipalloc-0123456789abcdef0",
		associationId: "eipassoc-abc123",
	}
	// Should not panic
	step.Cleanup(state)
	if mock.disassociateCalls != 1 {
		t.Errorf("expected 1 DisassociateAddress call, got %d", mock.disassociateCalls)
	}
}
