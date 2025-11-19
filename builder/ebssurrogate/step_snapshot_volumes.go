// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebssurrogate

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	multierror "github.com/hashicorp/go-multierror"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

// StepSnapshotVolumes creates snapshots of the created volumes.
//
// Produces:
//
//	snapshot_ids map[string]string - IDs of the created snapshots
type StepSnapshotVolumes struct {
	PollingConfig       *awscommon.AWSPollingConfig
	LaunchDevices       []ec2types.BlockDeviceMapping
	snapshotIds         map[string]string
	snapshotMutex       sync.Mutex
	SnapshotOmitMap     map[string]bool
	SnapshotTags        map[string]string
	SnapshotDescription string
	Ctx                 interpolate.Context
}

func (s *StepSnapshotVolumes) snapshotVolume(ctx context.Context, deviceName string, state multistep.StateBag) error {
	ec2conn := state.Get("ec2v2").(clients.Ec2Client)
	awsConfig := state.Get("aws_config").(*aws.Config)
	ui := state.Get("ui").(packersdk.Ui)
	instance := state.Get("instance").(ec2types.Instance)

	var volumeId string
	for _, volume := range instance.BlockDeviceMappings {
		if *volume.DeviceName == deviceName {
			volumeId = *volume.Ebs.VolumeId
		}
	}
	if volumeId == "" {
		return fmt.Errorf("Volume ID for device %s not found", deviceName)
	}

	ui.Say("Creating snapshot tags")
	snapshotTags, err := awscommon.TagMap(s.SnapshotTags).EC2Tags(s.Ctx, awsConfig.Region, state)
	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return err
	}
	snapshotTags.Report(ui)

	ui.Say(fmt.Sprintf("Creating snapshot of EBS Volume %s...", volumeId))

	// Collect tags for tagging on resource creation
	var tagSpecs []ec2types.TagSpecification

	if len(snapshotTags) > 0 {
		snapTags := ec2types.TagSpecification{
			ResourceType: "snapshot",
			Tags:         snapshotTags,
		}
		tagSpecs = append(tagSpecs, snapTags)
	}
	description := s.SnapshotDescription
	if description == "" {
		description = fmt.Sprintf("Packer: %s", time.Now().String())
	}
	createSnapResp, err := ec2conn.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
		VolumeId:          &volumeId,
		Description:       aws.String(description),
		TagSpecifications: tagSpecs,
	})
	if err != nil {
		return err
	}

	// Set the snapshot ID so we can delete it later
	s.snapshotMutex.Lock()
	s.snapshotIds[deviceName] = *createSnapResp.SnapshotId
	s.snapshotMutex.Unlock()

	// Wait for snapshot to be created
	err = s.PollingConfig.WaitUntilSnapshotDone(ctx, ec2conn, *createSnapResp.SnapshotId)
	return err
}

func (s *StepSnapshotVolumes) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)

	s.snapshotIds = map[string]string{}

	var wg sync.WaitGroup
	var errs *multierror.Error
	for _, device := range s.LaunchDevices {
		// Skip devices we've flagged for omission
		omit, ok := s.SnapshotOmitMap[*device.DeviceName]
		if ok && omit {
			continue
		}

		wg.Add(1)
		go func(device ec2types.BlockDeviceMapping) {
			defer wg.Done()
			if err := s.snapshotVolume(ctx, *device.DeviceName, state); err != nil {
				errs = multierror.Append(errs, err)
			}
		}(device)
	}

	wg.Wait()

	if errs != nil {
		state.Put("error", errs)
		ui.Error(errs.Error())
		return multistep.ActionHalt
	}

	state.Put("snapshot_ids", s.snapshotIds)
	return multistep.ActionContinue
}

func (s *StepSnapshotVolumes) Cleanup(state multistep.StateBag) {
	if len(s.snapshotIds) == 0 {
		return
	}
	ctx := context.TODO()
	_, cancelled := state.GetOk(multistep.StateCancelled)
	_, halted := state.GetOk(multistep.StateHalted)

	if cancelled || halted {
		ec2conn := state.Get("ec2v2").(clients.Ec2Client)
		ui := state.Get("ui").(packersdk.Ui)
		ui.Say("Removing snapshots since we cancelled or halted...")
		s.snapshotMutex.Lock()
		for _, snapshotId := range s.snapshotIds {
			_, err := ec2conn.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: &snapshotId})
			if err != nil {
				ui.Error(fmt.Sprintf("Error: %s", err))
			}
		}
		s.snapshotMutex.Unlock()
	}
}
