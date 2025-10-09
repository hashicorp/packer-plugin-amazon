// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type StepDeregisterAMI struct {
	AccessConfig        *AccessConfig
	ForceDeregister     bool
	ForceDeleteSnapshot bool
	AMIName             string
	Regions             []string
}

func (s *StepDeregisterAMI) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	// Check for force deregister
	if !s.ForceDeregister {
		return multistep.ActionContinue
	}

	ui := state.Get("ui").(packersdk.Ui)
	awsConfig := state.Get("aws_config").(*aws.Config)
	// Add the session region to list of regions will deregister AMIs in
	regions := append(s.Regions, awsConfig.Region)

	for _, region := range regions {
		// get new connection for each region in which we need to deregister vms

		regionEc2Client := ec2.NewFromConfig(*awsConfig, func(o *ec2.Options) {
			o.Region = region
		})

		resp, err := regionEc2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
			Owners: []string{"self"},
			Filters: []ec2types.Filter{
				{
					Name:   aws.String("name"),
					Values: []string{s.AMIName},
				},
			},
		})

		if err != nil {
			err := fmt.Errorf("Error describing AMI: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		// Deregister image(s) by name
		for _, i := range resp.Images {
			_, err := regionEc2Client.DeregisterImage(ctx, &ec2.DeregisterImageInput{
				ImageId: i.ImageId,
			})

			if err != nil {
				err := fmt.Errorf("Error deregistering existing AMI: %s", err)
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
			ui.Say(fmt.Sprintf("Deregistered AMI %s, id: %s", s.AMIName, *i.ImageId))

			// Delete snapshot(s) by image
			if s.ForceDeleteSnapshot {
				for _, b := range i.BlockDeviceMappings {
					if b.Ebs != nil && aws.ToString(b.Ebs.SnapshotId) != "" {
						_, err := regionEc2Client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
							SnapshotId: b.Ebs.SnapshotId,
						})

						if err != nil {
							err := fmt.Errorf("Error deleting existing snapshot: %s", err)
							state.Put("error", err)
							ui.Error(err.Error())
							return multistep.ActionHalt
						}
						ui.Say(fmt.Sprintf("Deleted snapshot: %s", *b.Ebs.SnapshotId))
					}
				}
			}
		}
	}

	return multistep.ActionContinue
}

func (s *StepDeregisterAMI) Cleanup(state multistep.StateBag) {
}
