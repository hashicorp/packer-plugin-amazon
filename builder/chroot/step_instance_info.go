// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package chroot

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// StepInstanceInfo verifies that this builder is running on an EC2 instance.
type StepInstanceInfo struct{}

func (s *StepInstanceInfo) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ec2conn := state.Get("ec2").(clients.Ec2Client)
	ui := state.Get("ui").(packersdk.Ui)
	config := state.Get("config").(*Config)

	awscfg, err := config.GetAWSConfig(ctx)
	if err != nil {
		err := fmt.Errorf("Error getting AWS config: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// Get our own instance ID
	ui.Say("Gathering information about this EC2 instance...")

	idmsconn := imds.NewFromConfig(*awscfg)
	identity, err := idmsconn.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		err := fmt.Errorf(
			"Error retrieving the ID of the instance Packer is running on.\n" +
				"Please verify Packer is running on a proper AWS EC2 instance.")
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	log.Printf("Instance ID: %s", identity.InstanceID)

	// Query the entire instance metadata
	instancesResp, err := ec2conn.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{identity.InstanceID}})
	if err != nil {
		err := fmt.Errorf("Error getting instance data: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	if len(instancesResp.Reservations) == 0 {
		err := fmt.Errorf("Error getting instance data: no instance found.")
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	instance := instancesResp.Reservations[0].Instances[0]
	state.Put("instance", instance)

	return multistep.ActionContinue
}

func (s *StepInstanceInfo) Cleanup(multistep.StateBag) {}
