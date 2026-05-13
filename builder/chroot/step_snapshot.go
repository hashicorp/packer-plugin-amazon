// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package chroot

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// StepSnapshot creates a snapshot of the created volume.
//
// Produces:
//
//	snapshot_id string - ID of the created snapshot
type StepSnapshot struct {
	PollingConfig *awscommon.AWSPollingConfig
	snapshotId    string
}

func (s *StepSnapshot) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ec2conn := state.Get("ec2").(clients.Ec2Client)
	awscfg := state.Get("awsConfig").(*aws.Config)
	ui := state.Get("ui").(packersdk.Ui)
	volumeId := state.Get("volume_id").(string)

	ui.Say("Creating snapshot...")
	description := fmt.Sprintf("Packer: %s", time.Now().String())

	createSnapResp, err := ec2conn.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
		VolumeId:    &volumeId,
		Description: &description,
	})
	if err != nil {
		err := fmt.Errorf("Error creating snapshot: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// Set the snapshot ID so we can delete it later
	s.snapshotId = *createSnapResp.SnapshotId
	ui.Message(fmt.Sprintf("Snapshot ID: %s", s.snapshotId))

	// Wait for the snapshot to be ready
	err = s.PollingConfig.WaitUntilSnapshotDone(ctx, ec2conn, s.snapshotId)
	if err != nil {
		err := fmt.Errorf("Error waiting for snapshot: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	state.Put("snapshot_id", s.snapshotId)

	snapshots := map[string][]string{
		awscfg.Region: {s.snapshotId},
	}
	state.Put("snapshots", snapshots)

	return multistep.ActionContinue
}

func (s *StepSnapshot) Cleanup(state multistep.StateBag) {
	if s.snapshotId == "" {
		return
	}

	ctx := state.Get("context").(context.Context)
	_, cancelled := state.GetOk(multistep.StateCancelled)
	_, halted := state.GetOk(multistep.StateHalted)

	if cancelled || halted {
		ec2conn := state.Get("ec2").(clients.Ec2Client)
		ui := state.Get("ui").(packersdk.Ui)
		ui.Say("Removing snapshot since we cancelled or halted...")
		_, err := ec2conn.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: &s.snapshotId})
		if err != nil {
			ui.Error(fmt.Sprintf("Error: %s", err))
		}
	}
}
