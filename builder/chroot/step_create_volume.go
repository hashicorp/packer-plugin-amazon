// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package chroot

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

// StepCreateVolume creates a new volume from the snapshot of the root
// device of the AMI.
//
// Produces:
//
//	volume_id string - The ID of the created volume
type StepCreateVolume struct {
	PollingConfig         *awscommon.AWSPollingConfig
	volumeId              string
	RootVolumeSize        int32
	RootVolumeType        ec2types.VolumeType
	RootVolumeTags        map[string]string
	RootVolumeEncryptBoot config.Trilean
	RootVolumeKmsKeyId    string
	Ctx                   interpolate.Context
}

func (s *StepCreateVolume) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ec2conn := state.Get("ec2").(clients.Ec2Client)
	instance := state.Get("instance").(*ec2types.Instance)
	ui := state.Get("ui").(packersdk.Ui)

	awscfg, err := config.GetAWSConfig(ctx)
	if err != nil {
		err := fmt.Errorf("Error getting AWS config: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	volTags, err := awscommon.TagMap(s.RootVolumeTags).EC2Tags(s.Ctx, awscfg.Region, state)
	if err != nil {
		err := fmt.Errorf("Error tagging volumes: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// Collect tags for tagging on resource creation
	var tagSpecs []ec2types.TagSpecification

	if len(volTags) > 0 {
		runVolTags := ec2types.TagSpecification{
			ResourceType: ec2types.ResourceTypeVolume,
			Tags:         volTags,
		}

		tagSpecs = append(tagSpecs, runVolTags)
	}

	var createVolume *ec2.CreateVolumeInput
	if config.FromScratch {
		rootVolumeType := ec2types.VolumeTypeGp2
		if s.RootVolumeType == ec2types.VolumeTypeIo1 {
			err := errors.New("Cannot use io1 volume when building from scratch")
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		} else if s.RootVolumeType != "" {
			rootVolumeType = s.RootVolumeType
		}
		createVolume = &ec2.CreateVolumeInput{
			AvailabilityZone: instance.Placement.AvailabilityZone,
			Size:             aws.Int32(s.RootVolumeSize),
			VolumeType:       rootVolumeType,
		}

	} else {
		// Determine the root device snapshot
		image := state.Get("source_image").(*ec2types.Image)
		log.Printf("Searching for root device of the image (%s)", *image.RootDeviceName)
		var rootDevice *ec2types.BlockDeviceMapping
		for _, device := range image.BlockDeviceMappings {
			if aws.ToString(device.DeviceName) == aws.ToString(image.RootDeviceName) {
				rootDevice = &device
				break
			}
		}

		ui.Say("Creating the root volume...")
		createVolume, err = s.buildCreateVolumeInput(*instance.Placement.AvailabilityZone, rootDevice)
		if err != nil {
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	if len(tagSpecs) > 0 {
		createVolume.TagSpecifications = tagSpecs
		volTags.Report(ui)
	}
	log.Printf("Create args: %+v", createVolume)

	createVolumeResp, err := ec2conn.CreateVolume(ctx, createVolume)
	if err != nil {
		err := fmt.Errorf("Error creating root volume: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// Set the volume ID so we remember to delete it later
	s.volumeId = *createVolumeResp.VolumeId
	log.Printf("Volume ID: %s", s.volumeId)

	// Wait for the volume to become ready
	err = s.PollingConfig.WaitUntilVolumeAvailable(ctx, ec2conn, s.volumeId)
	if err != nil {
		err := fmt.Errorf("Error waiting for volume: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	state.Put("context", ctx)
	state.Put("volume_id", s.volumeId)
	return multistep.ActionContinue
}

func (s *StepCreateVolume) Cleanup(state multistep.StateBag) {
	if s.volumeId == "" {
		return
	}

	ec2conn := state.Get("ec2").(clients.Ec2Client)
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Deleting the created EBS volume...")
	ctx := state.Get("context").(context.Context)
	_, err := ec2conn.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: &s.volumeId})
	if err != nil {
		ui.Error(fmt.Sprintf("Error deleting EBS volume: %s", err))
	}
}

func (s *StepCreateVolume) buildCreateVolumeInput(az string, rootDevice *ec2types.BlockDeviceMapping) (*ec2.CreateVolumeInput, error) {
	if rootDevice == nil {
		return nil, fmt.Errorf("Couldn't find root device!")
	}
	createVolumeInput := &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(az),
		Size:             rootDevice.Ebs.VolumeSize,
		SnapshotId:       rootDevice.Ebs.SnapshotId,
		VolumeType:       rootDevice.Ebs.VolumeType,
		Iops:             rootDevice.Ebs.Iops,
		Encrypted:        rootDevice.Ebs.Encrypted,
		KmsKeyId:         rootDevice.Ebs.KmsKeyId,
	}
	if s.RootVolumeSize > *rootDevice.Ebs.VolumeSize {
		createVolumeInput.Size = aws.Int32(s.RootVolumeSize)
	}

	if s.RootVolumeEncryptBoot.True() {
		createVolumeInput.Encrypted = aws.Bool(true)
	}

	if s.RootVolumeKmsKeyId != "" {
		createVolumeInput.KmsKeyId = aws.String(s.RootVolumeKmsKeyId)
	}

	if s.RootVolumeType == "" || s.RootVolumeType == rootDevice.Ebs.VolumeType {
		return createVolumeInput, nil
	}

	if s.RootVolumeType == "io1" {
		return nil, fmt.Errorf("Root volume type cannot be io1, because existing root volume type was %s", rootDevice.Ebs.VolumeType)
	}

	createVolumeInput.VolumeType = s.RootVolumeType
	// non io1 cannot set iops
	createVolumeInput.Iops = nil

	return createVolumeInput, nil
}
