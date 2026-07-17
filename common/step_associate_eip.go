// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// StepAssociateEIP associates a pre-existing Elastic IP with the build instance
// immediately after launch. Cleanup disassociates (but does not release) the EIP.
type StepAssociateEIP struct {
	AllocationId  string
	associationId string
}

func (s *StepAssociateEIP) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	if s.AllocationId == "" {
		return multistep.ActionContinue
	}
	ec2Client := state.Get("ec2v2").(clients.Ec2Client)
	ui := state.Get("ui").(packersdk.Ui)
	instance := state.Get("instance").(ec2types.Instance)

	ui.Say(fmt.Sprintf("Associating Elastic IP %s with instance %s...", s.AllocationId, *instance.InstanceId))
	resp, err := ec2Client.AssociateAddress(ctx, &ec2.AssociateAddressInput{
		AllocationId: aws.String(s.AllocationId),
		InstanceId:   instance.InstanceId,
	})
	if err != nil {
		err = fmt.Errorf("error associating EIP %s: %w", s.AllocationId, err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	s.associationId = aws.ToString(resp.AssociationId)
	ui.Say(fmt.Sprintf("EIP associated (association ID: %s)", s.associationId))

	// Refresh instance in state so SSHHost reads the EIP as PublicIpAddress.
	descResp, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{*instance.InstanceId},
	})
	if err != nil || len(descResp.Reservations) == 0 || len(descResp.Reservations[0].Instances) == 0 {
		err = fmt.Errorf("error refreshing instance after EIP association: %w", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	state.Put("instance", descResp.Reservations[0].Instances[0])
	return multistep.ActionContinue
}

func (s *StepAssociateEIP) Cleanup(state multistep.StateBag) {
	if s.associationId == "" {
		return
	}
	ec2Client := state.Get("ec2v2").(clients.Ec2Client)
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say(fmt.Sprintf("Disassociating Elastic IP (association ID: %s)...", s.associationId))
	_, err := ec2Client.DisassociateAddress(context.Background(), &ec2.DisassociateAddressInput{
		AssociationId: aws.String(s.associationId),
	})
	if err != nil {
		ui.Error(fmt.Sprintf("Error disassociating EIP %s: %s", s.associationId, err))
		return
	}
	ui.Say("Elastic IP disassociated.")
}
