// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebssurrogate

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/random"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

// StepRegisterAMI creates the AMI.
type StepRegisterAMI struct {
	PollingConfig            *awscommon.AWSPollingConfig
	RootDevice               RootBlockDevice
	AMIDevices               []ec2types.BlockDeviceMapping
	LaunchDevices            []ec2types.BlockDeviceMapping
	EnableAMIENASupport      config.Trilean
	EnableAMISriovNetSupport bool
	Architecture             string
	image                    ec2types.Image
	LaunchOmitMap            map[string]bool
	AMISkipBuildRegion       bool
	BootMode                 string
	UefiData                 string
	TpmSupport               string
}

func (s *StepRegisterAMI) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ec2Client := state.Get("ec2v2").(clients.Ec2Client)
	awsConfig := state.Get("aws_config").(*aws.Config)
	snapshotIds := state.Get("snapshot_ids").(map[string]string)
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Registering the AMI...")

	blockDevices := s.combineDevices(snapshotIds)

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

	registerOpts := &ec2.RegisterImageInput{
		Name:                &amiName,
		Architecture:        ec2types.ArchitectureValues(s.Architecture),
		RootDeviceName:      aws.String(s.RootDevice.DeviceName),
		VirtualizationType:  aws.String(config.AMIVirtType),
		BlockDeviceMappings: blockDevices,
	}

	if s.EnableAMISriovNetSupport {
		// Set SriovNetSupport to "simple". See http://goo.gl/icuXh5
		// As of February 2017, this applies to C3, C4, D2, I2, R3, and M4 (excluding m4.16xlarge)
		registerOpts.SriovNetSupport = aws.String("simple")
	}
	if s.EnableAMIENASupport.True() {
		// Set EnaSupport to true
		// As of February 2017, this applies to C5, I3, P2, R4, X1, and m4.16xlarge
		registerOpts.EnaSupport = aws.Bool(true)
	}
	if s.BootMode != "" {
		registerOpts.BootMode = ec2types.BootModeValues(s.BootMode)
	}
	if s.UefiData != "" {
		registerOpts.UefiData = aws.String(s.UefiData)
	}
	if s.TpmSupport != "" {
		registerOpts.TpmSupport = ec2types.TpmSupportValues(s.TpmSupport)
	}
	registerResp, err := ec2Client.RegisterImage(ctx, registerOpts)
	if err != nil {
		state.Put("error", fmt.Errorf("Error registering AMI: %s", err))
		ui.Error(state.Get("error").(error).Error())
		return multistep.ActionHalt
	}

	// Set the AMI ID in the state
	ui.Say(fmt.Sprintf("AMI: %s", *registerResp.ImageId))
	amis := make(map[string]string)
	amis[awsConfig.Region] = *registerResp.ImageId
	state.Put("amis", amis)

	// Wait for the image to become ready
	ui.Say("Waiting for AMI to become ready...")
	if err := s.PollingConfig.WaitUntilAMIAvailable(ctx, ec2Client, *registerResp.ImageId); err != nil {
		err := fmt.Errorf("Error waiting for AMI: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	imagesResp, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{ImageIds: []string{*registerResp.
		ImageId}})
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

			snapshots[awsConfig.Region] = append(snapshots[awsConfig.Region], *blockDeviceMapping.Ebs.SnapshotId)
		}
	}
	state.Put("snapshots", snapshots)

	return multistep.ActionContinue
}

func (s *StepRegisterAMI) Cleanup(state multistep.StateBag) {
	if s.image.ImageId == nil {
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

	ui.Say("Deregistering the AMI because cancellation or error...")
	deregisterOpts := &ec2.DeregisterImageInput{ImageId: s.image.ImageId}
	if _, err := ec2Client.DeregisterImage(ctx, deregisterOpts); err != nil {
		ui.Error(fmt.Sprintf("Error deregistering AMI, may still be around: %s", err))
		return
	}
}

func (s *StepRegisterAMI) combineDevices(snapshotIds map[string]string) []ec2types.BlockDeviceMapping {
	devices := map[string]ec2types.BlockDeviceMapping{}

	for _, device := range s.AMIDevices {
		devices[*device.DeviceName] = device
	}

	// Devices in launch_block_device_mappings override any with
	// the same name in ami_block_device_mappings, except for the
	// one designated as the root device in ami_root_device
	for _, device := range s.LaunchDevices {
		// Skip devices we've flagged for omission
		omit, ok := s.LaunchOmitMap[*device.DeviceName]
		if ok && omit {
			continue
		}
		snapshotId, ok := snapshotIds[*device.DeviceName]
		if ok {
			device.Ebs.SnapshotId = aws.String(snapshotId)
			// Block devices with snapshot inherit
			// encryption settings from the snapshot
			device.Ebs.Encrypted = nil
			device.Ebs.KmsKeyId = nil
		}
		if *device.DeviceName == s.RootDevice.SourceDeviceName {
			device.DeviceName = aws.String(s.RootDevice.DeviceName)
		}
		devices[*device.DeviceName] = device
	}

	blockDevices := []ec2types.BlockDeviceMapping{}
	for _, device := range devices {
		blockDevices = append(blockDevices, device)
	}
	return blockDevices
}
