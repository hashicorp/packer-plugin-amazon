// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebssurrogate

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

// StepSwapVolumes detaches omitted volumes and original root volume and reattaches
// the new root volume specified by ami_root_device.source_device_name.
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
	volumeToDeleteMap := make(map[string]*bool)

	// Iterate through block device mappings and populate the map
	for _, reservation := range result.Reservations {
		for _, instance := range reservation.Instances {
			for _, blockDevice := range instance.BlockDeviceMappings {
				deviceToVolumeMap[*blockDevice.DeviceName] = *blockDevice.Ebs.VolumeId
				volumeToDeleteMap[*blockDevice.Ebs.VolumeId] = blockDevice.Ebs.DeleteOnTermination
			}
		}
	}
	for deviceName, volumeID := range deviceToVolumeMap {
		omit, ok := s.LaunchOmitMap[deviceName]
		if ok && omit {
			ui.Say(fmt.Sprintf("Detaching Ommitted EBS Device Name: %s, Volume ID: %s\n", deviceName, volumeID))
			err = s.detachVolume(ctx, ec2conn, deviceName, volumeID)
		} else if deviceName == s.RootDevice.DeviceName || deviceName == s.RootDevice.SourceDeviceName || deviceName == "/dev/sda1" {
			ui.Say(fmt.Sprintf("Detaching Root EBS Device Name: %s, Volume ID: %s\n", deviceName, volumeID))
			err = s.detachVolume(ctx, ec2conn, deviceName, volumeID)
		} else {
			ui.Say(fmt.Sprintf("Skip Detach of EBS Device Name: %s, Volume ID: %s\n", deviceName, volumeID))
		}

		if err != nil {
			err := fmt.Errorf("error detaching volume: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

	}

	rootVolumeId := aws.String(deviceToVolumeMap[s.RootDevice.SourceDeviceName])
	rootDeviceName := aws.String(s.RootDevice.DeviceName)
	ui.Say(fmt.Sprintf("Attaching Root EBS Device Name %s, Volume ID: %s", *rootDeviceName, *rootVolumeId))

	_, err = ec2conn.AttachVolume(&ec2.AttachVolumeInput{
		InstanceId: instance.InstanceId,
		VolumeId:   rootVolumeId,
		Device:     rootDeviceName,
	})

	if err != nil {
		err := fmt.Errorf("error attaching volume: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// Restore the DeleteOnTermination attribute for the root volume
	// When detaching and reattaching volumes, the original BlockDeviceMapping attributes are lost
	// This explicitly sets the DeleteOnTermination flag back to its original value
	_, err = ec2conn.ModifyInstanceAttribute(&ec2.ModifyInstanceAttributeInput{
		InstanceId: instance.InstanceId,
		BlockDeviceMappings: []*ec2.InstanceBlockDeviceMappingSpecification{
			{
				DeviceName: rootDeviceName,
				Ebs: &ec2.EbsInstanceBlockDeviceSpecification{
					DeleteOnTermination: volumeToDeleteMap[*rootVolumeId],
				},
			},
		},
	})
	if err != nil {
		err := fmt.Errorf("error setting the delete_on_termination attribute block device mapping: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// Wait for the volume to become attached
	err = s.PollingConfig.WaitUntilVolumeAttached(ctx, ec2conn, *rootVolumeId)
	if err != nil {
		err := fmt.Errorf("error waiting for volume: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	state.Put("volume_delete_map", volumeToDeleteMap)

	return multistep.ActionContinue
}

func (s *StepSwapVolumes) detachVolume(ctx context.Context, ec2conn *ec2.EC2, deviceName string, volumeId string) error {
	_, err := ec2conn.DetachVolume(&ec2.DetachVolumeInput{VolumeId: &volumeId})
	if err == nil {
		return s.PollingConfig.WaitUntilVolumeDetached(ctx, ec2conn, volumeId)
	}

	return err
}

func (s *StepSwapVolumes) Cleanup(state multistep.StateBag) {

	ec2conn := state.Get("ec2").(*ec2.EC2)
	ui := state.Get("ui").(packersdk.Ui)
	instance := state.Get("instance").(*ec2.Instance)
	ui.Say("Cleaning up any detached volumes with delete_on_termination set to true...")
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
		return
	}

	attachedVolumes := getAttachedVolumes(result)

	volumeToDeleteMap := state.Get("volume_delete_map").(map[string]*bool)

	volumesToDelete := filterVolumesToDelete(volumeToDeleteMap, attachedVolumes)
	log.Printf("Found %v volumes to delete", len(volumesToDelete))

	for _, volumeId := range volumesToDelete {
		ui.Say(fmt.Sprintf("Deleting EBS Volume ID: %s", volumeId))
		_, err := ec2conn.DeleteVolume(&ec2.DeleteVolumeInput{VolumeId: &volumeId})
		if err != nil {
			err := fmt.Errorf("error deleting volume: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return
		}
		ui.Say(fmt.Sprintf("Deleted EBS Volume ID: %s", volumeId))
	}

}

func getAttachedVolumes(result *ec2.DescribeInstancesOutput) map[string]struct{} {
	volumes := make(map[string]struct{})
	for _, reservation := range result.Reservations {
		for _, inst := range reservation.Instances {
			for _, bd := range inst.BlockDeviceMappings {
				if bd.Ebs != nil && bd.Ebs.VolumeId != nil {
					volumes[*bd.Ebs.VolumeId] = struct{}{}
				}
			}
		}
	}
	return volumes
}

func filterVolumesToDelete(volumesMap map[string]*bool, attached map[string]struct{}) []string {
	var volumesToDelete []string
	for volumeId, shouldDelete := range volumesMap {
		if shouldDelete != nil && *shouldDelete {
			if _, attached := attached[volumeId]; !attached {
				volumesToDelete = append(volumesToDelete, volumeId)
			}
		}
	}
	return volumesToDelete
}
