// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-amazon/common/awserrors"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/random"
	"github.com/hashicorp/packer-plugin-sdk/retry"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

type stepCreateAMI struct {
	PollingConfig      *awscommon.AWSPollingConfig
	image              *ec2types.Image
	AMISkipCreateImage bool
	AMISkipBuildRegion bool
	AMISkipRunTags     bool
	IsRestricted       bool
	Ctx                interpolate.Context
	Tags               map[string]string
}

func (s *stepCreateAMI) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ec2Client := state.Get("ec2v2").(clients.Ec2Client)
	awsConfig := state.Get("aws_config").(*aws.Config)
	instance := state.Get("instance").(ec2types.Instance)
	ui := state.Get("ui").(packersdk.Ui)

	if s.AMISkipCreateImage {
		ui.Say("Skipping AMI creation...")
		return multistep.ActionContinue
	}

	// Create the image
	amiName := config.AMIName
	state.Put("intermediary_image", false)
	if config.AMIEncryptBootVolume.True() || s.AMISkipBuildRegion {
		state.Put("intermediary_image", true)

		// From AWS SDK docs: You can encrypt a copy of an unencrypted snapshot,
		// but you cannot use it to create an unencrypted copy of an encrypted
		// snapshot. Your default CMK for EBS is used unless you specify a
		// non-default key using KmsKeyId.

		// If encrypt_boot is nil or true, we need to create a temporary image
		// so that in step_region_copy, we can copy it with the correct
		// encryption
		amiName = random.AlphaNum(7)
	}

	ui.Say(fmt.Sprintf("Creating AMI %s from instance %s", amiName, *instance.InstanceId))
	createOpts := &ec2.CreateImageInput{
		InstanceId:          instance.InstanceId,
		Name:                &amiName,
		BlockDeviceMappings: config.AMIMappings.BuildEC2BlockDeviceMappings(),
	}

	if !s.IsRestricted {
		ec2Tags, err := awscommon.TagMap(s.Tags).EC2Tags(s.Ctx, awsConfig.Region, state)
		if err != nil {
			err := fmt.Errorf("Error tagging AMI: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		if !s.AMISkipRunTags {
			ui.Say("Attaching run tags to AMI...")
			createOpts.TagSpecifications = ec2Tags.TagSpecifications(ec2types.ResourceTypeImage,
				ec2types.ResourceTypeSnapshot)
		} else {
			ui.Say("Skipping attaching run tags to AMI...")
		}
	}

	var createResp *ec2.CreateImageOutput

	// Create a timeout for the CreateImage call.
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Minute*15)
	defer cancel()

	err := retry.Config{
		Tries: 0,
		ShouldRetry: func(err error) bool {
			if awserrors.Matches(err, "InvalidParameterValue", "Instance is not in state") {
				return true
			}
			return false
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(timeoutCtx, func(ctx context.Context) error {
		var err error
		createResp, err = ec2Client.CreateImage(ctx, createOpts)
		return err
	})
	if err != nil {
		err := fmt.Errorf("Error creating AMI: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// Set the AMI ID in the state
	ui.Message(fmt.Sprintf("AMI: %s", *createResp.ImageId))
	amis := make(map[string]string)
	amis[awsConfig.Region] = *createResp.ImageId
	state.Put("amis", amis)

	// Wait for the image to become ready
	ui.Say("Waiting for AMI to become ready...")
	if waitErr := s.PollingConfig.WaitUntilAMIAvailable(ctx, ec2Client, *createResp.ImageId); waitErr != nil {
		// waitErr should get bubbled up if the issue is a wait timeout
		err := fmt.Errorf("Error waiting for AMI: %s", waitErr)
		imResp, imerr := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{ImageIds: []string{*createResp.
			ImageId}})
		if imerr != nil {
			// If there's a failure describing images, bubble that error up too, but don't erase the waitErr.
			log.Printf("DescribeImages call was unable to determine reason waiting for AMI failed: %s", imerr)
			err = fmt.Errorf("Unknown error waiting for AMI; %s. DescribeImages returned an error: %s", waitErr, imerr)
		}
		if imResp != nil && len(imResp.Images) > 0 {
			// Finally, if there's a stateReason, store that with the wait err
			image := imResp.Images[0]
			stateReason := image.StateReason
			if stateReason != nil {
				err = fmt.Errorf("Error waiting for AMI: %s. DescribeImages returned the state reason: %+v", waitErr,
					stateReason)
			}

		}
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	imagesResp, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds: []string{*createResp.ImageId}})
	if err != nil {
		err := fmt.Errorf("Error searching for AMI: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	s.image = &imagesResp.Images[0]

	snapshots := make(map[string][]string)
	for _, blockDeviceMapping := range imagesResp.Images[0].BlockDeviceMappings {
		if blockDeviceMapping.Ebs != nil && blockDeviceMapping.Ebs.SnapshotId != nil {

			snapshots[awsConfig.Region] = append(snapshots[awsConfig.Region], *blockDeviceMapping.Ebs.SnapshotId)
		}
	}
	state.Put("snapshots", snapshots)

	return multistep.ActionContinue
}

func (s *stepCreateAMI) Cleanup(state multistep.StateBag) {
	if s.image == nil {
		return
	}
	ctx := context.TODO()

	_, cancelled := state.GetOk(multistep.StateCancelled)
	_, halted := state.GetOk(multistep.StateHalted)
	if !cancelled && !halted {
		return
	}

	ec2Client := state.Get("ec2v2").(clients.Ec2Client)
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Deregistering the AMI and deleting associated snapshots because " +
		"of cancellation, or error...")

	resp, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds: []string{*s.image.ImageId},
	})

	if err != nil {
		err := fmt.Errorf("Error describing AMI: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return
	}

	// Deregister image by name.
	for _, i := range resp.Images {
		_, err := ec2Client.DeregisterImage(ctx, &ec2.DeregisterImageInput{
			ImageId: i.ImageId,
		})

		if err != nil {
			err := fmt.Errorf("Error deregistering existing AMI: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return
		}
		ui.Say(fmt.Sprintf("Deregistered AMI id: %s", *i.ImageId))

		// Delete snapshot(s) by image
		for _, b := range i.BlockDeviceMappings {
			if b.Ebs != nil && aws.ToString(b.Ebs.SnapshotId) != "" {
				_, err := ec2Client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
					SnapshotId: b.Ebs.SnapshotId,
				})

				if err != nil {
					err := fmt.Errorf("Error deleting existing snapshot: %s", err)
					state.Put("error", err)
					ui.Error(err.Error())
					return
				}
				ui.Say(fmt.Sprintf("Deleted snapshot: %s", *b.Ebs.SnapshotId))
			}
		}
	}
}
