// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebssurrogate

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
	"github.com/hashicorp/packer-plugin-amazon/builder/common/awserrors"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/random"
	"github.com/hashicorp/packer-plugin-sdk/retry"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

type StepCreateAMI struct {
	PollingConfig      *awscommon.AWSPollingConfig
	RootDevice         RootBlockDevice
	AMIDevices         []*ec2.BlockDeviceMapping
	LaunchDevices      []*ec2.BlockDeviceMapping
	LaunchOmitMap      map[string]bool
	image              *ec2.Image
	AMISkipBuildRegion bool
	AMISkipRunTags     bool
	IsRestricted       bool
	Ctx                interpolate.Context
	Tags               map[string]string
}

func (s *StepCreateAMI) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ec2conn := state.Get("ec2").(*ec2.EC2)
	instance := state.Get("instance").(*ec2.Instance)
	ui := state.Get("ui").(packersdk.Ui)

	blockDevices := s.combineDevices()

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
		BlockDeviceMappings: blockDevices,
	}

	if !s.IsRestricted {
		ec2Tags, err := awscommon.TagMap(s.Tags).EC2Tags(s.Ctx, *ec2conn.Config.Region, state)
		if err != nil {
			err := fmt.Errorf("Error tagging AMI: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
		if !s.AMISkipRunTags {
			ui.Say("Attaching run tags to AMI...")
			createOpts.TagSpecifications = ec2Tags.TagSpecifications(ec2.ResourceTypeImage, ec2.ResourceTypeSnapshot)
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
			return awserrors.Matches(err, "InvalidParameterValue", "Instance is not in state")
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(timeoutCtx, func(ctx context.Context) error {
		var err error
		createResp, err = ec2conn.CreateImage(createOpts)
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
	amis[*ec2conn.Config.Region] = *createResp.ImageId
	state.Put("amis", amis)

	// Wait for the image to become ready
	ui.Say("Waiting for AMI to become ready...")
	if waitErr := s.PollingConfig.WaitUntilAMIAvailable(ctx, ec2conn, *createResp.ImageId); waitErr != nil {
		// waitErr should get bubbled up if the issue is a wait timeout
		err := fmt.Errorf("Error waiting for AMI: %s", waitErr)
		imResp, imerr := ec2conn.DescribeImages(&ec2.DescribeImagesInput{ImageIds: []*string{createResp.ImageId}})
		if imerr != nil {
			// If there's a failure describing images, bubble that error up too, but don't erase the waitErr.
			log.Printf("DescribeImages call was unable to determine reason waiting for AMI failed: %s", imerr)
			err = fmt.Errorf("Unknown error waiting for AMI; %s. DescribeImages returned an error: %s", waitErr, imerr)
		}
		if imResp != nil && len(imResp.Images) > 0 {
			// Finally, if there's a stateReason, store that with the wait err
			image := imResp.Images[0]
			if image != nil {
				stateReason := image.StateReason
				if stateReason != nil {
					err = fmt.Errorf("Error waiting for AMI: %s. DescribeImages returned the state reason: %s", waitErr, stateReason)
				}
			}
		}
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	imagesResp, err := ec2conn.DescribeImages(&ec2.DescribeImagesInput{ImageIds: []*string{createResp.ImageId}})
	if err != nil {
		err := fmt.Errorf("Error searching for AMI: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	s.image = imagesResp.Images[0]

	snapshots := make(map[string][]string)
	for _, blockDeviceMapping := range imagesResp.Images[0].BlockDeviceMappings {
		if blockDeviceMapping.Ebs != nil && blockDeviceMapping.Ebs.SnapshotId != nil {

			snapshots[*ec2conn.Config.Region] = append(snapshots[*ec2conn.Config.Region], *blockDeviceMapping.Ebs.SnapshotId)
		}
	}
	state.Put("snapshots", snapshots)

	return multistep.ActionContinue
}

func (s *StepCreateAMI) combineDevices() []*ec2.BlockDeviceMapping {
	devices := map[string]*ec2.BlockDeviceMapping{}

	for _, device := range s.AMIDevices {
		devices[*device.DeviceName] = device
	}

	// Devices in launch_block_device_mappings override any with
	// the same name in ami_block_device_mappings
	// If launch device name is equal to Root SourceDeviceName
	// then set the device's deviceName to RootDevice.DeviceName from root ami_root_device
	// and overwrite / add the device in the devices map
	for _, device := range s.LaunchDevices {
		// Skip devices we've flagged for omission
		omit, ok := s.LaunchOmitMap[*device.DeviceName]
		if ok && omit {
			continue
		}

		// Use root device name for the new source root device
		if *device.DeviceName == s.RootDevice.SourceDeviceName {
			device.DeviceName = aws.String(s.RootDevice.DeviceName)
		}
		devices[*device.DeviceName] = device
	}

	blockDevices := []*ec2.BlockDeviceMapping{}
	for _, device := range devices {

		blockDevices = append(blockDevices, device)
	}
	return blockDevices
}

func (s *StepCreateAMI) Cleanup(state multistep.StateBag) {
	if s.image == nil {
		return
	}

	_, cancelled := state.GetOk(multistep.StateCancelled)
	_, halted := state.GetOk(multistep.StateHalted)
	if !cancelled && !halted {
		return
	}

	ec2conn := state.Get("ec2").(*ec2.EC2)
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Deregistering the AMI and deleting associated snapshots because " +
		"of cancellation, or error...")

	resp, err := ec2conn.DescribeImages(&ec2.DescribeImagesInput{
		ImageIds: []*string{s.image.ImageId},
	})

	if err != nil {
		err := fmt.Errorf("Error describing AMI: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return
	}

	// Deregister image by name.
	for _, i := range resp.Images {
		_, err := ec2conn.DeregisterImage(&ec2.DeregisterImageInput{
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
			if b.Ebs != nil && aws.StringValue(b.Ebs.SnapshotId) != "" {
				_, err := ec2conn.DeleteSnapshot(&ec2.DeleteSnapshotInput{
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
