// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebssurrogate

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

// StepSnapshotVolumes creates snapshots of the created volumes.
//
// Produces:
//
//	snapshot_ids map[string]string - IDs of the created snapshots
type StepSwapVolumes struct {
	PollingConfig *awscommon.AWSPollingConfig
	RootDevice    RootBlockDevice
	LaunchDevices []*ec2.BlockDeviceMapping
	LaunchOmitMap map[string]bool
	Ctx           interpolate.Context
}

func (s *StepSwapVolumes) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ec2conn := state.Get("ec2").(*ec2.EC2)
	ui := state.Get("ui").(packersdk.Ui)
	instance := state.Get("instance").(*ec2.Instance)

	// Describe the instance
	input := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{
			aws.String(*instance.InstanceId),
		},
	}

	result, err := ec2conn.DescribeInstances(input)
	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	deviceToVolumeMap := make(map[string]string)

	// Iterate through block device mappings and populate the map
	for _, reservation := range result.Reservations {
		for _, instance := range reservation.Instances {
			for _, blockDevice := range instance.BlockDeviceMappings {
				deviceToVolumeMap[*blockDevice.DeviceName] = *blockDevice.Ebs.VolumeId
			}
		}
	}

	for deviceName, volumeID := range deviceToVolumeMap {
		ui.Say(fmt.Sprintf("Detaching EBS Device Name: %s, Volume ID: %s\n", deviceName, volumeID))

		_, err := ec2conn.DetachVolume(&ec2.DetachVolumeInput{VolumeId: &volumeID})
		if err == nil {
			err = s.PollingConfig.WaitUntilVolumeDetached(ctx, ec2conn, volumeID)
		}

		if err != nil {
			err := fmt.Errorf("error detaching volume: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	for deviceName, volumeID := range deviceToVolumeMap {
		omit, ok := s.LaunchOmitMap[deviceName]
		if ok && omit {
			ui.Say(fmt.Sprintf("Skip Attaching Ommitted EBS Device Name: %s, Volume ID: %s\n", deviceName, volumeID))
			continue
		}

		if deviceName == s.RootDevice.DeviceName {
			ui.Say(fmt.Sprintf("Skip Attaching Root EBS Device Name: %s, Volume ID: %s\n", deviceName, volumeID))
			continue
		}
		ui.Say(fmt.Sprintf("Attaching EBS Device Name: %s, Volume ID: %s\n", deviceName, volumeID))

		attachVolume := deviceName
		if deviceName == s.RootDevice.SourceDeviceName {
			rootDeviceName := *aws.String(s.RootDevice.DeviceName)
			ui.Say(fmt.Sprintf("Rename EBS Device Name: %s as the root device name %s\n", deviceName, rootDeviceName))
			// For the API call, it expects "sd" prefixed devices.
			attachVolume = strings.Replace(rootDeviceName, "/xvd", "/sd", 1)
		}

		ui.Say(fmt.Sprintf("Attaching the volume to %s", attachVolume))
		_, err := ec2conn.AttachVolume(&ec2.AttachVolumeInput{
			InstanceId: instance.InstanceId,
			VolumeId:   &volumeID,
			Device:     &attachVolume,
		})
		if err != nil {
			err := fmt.Errorf("error attaching volume: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		// Wait for the volume to become attached
		err = s.PollingConfig.WaitUntilVolumeAttached(ctx, ec2conn, volumeID)
		if err != nil {
			err := fmt.Errorf("error waiting for volume: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	return multistep.ActionContinue
}

func (s *StepSwapVolumes) Cleanup(state multistep.StateBag) {}
