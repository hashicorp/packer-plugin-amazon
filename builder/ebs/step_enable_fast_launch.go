// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/retry"
)

type stepEnableFastLaunch struct {
	PollingConfig      *awscommon.AWSPollingConfig
	AMISkipCreateImage bool
	EnableFastLaunch   bool
	MaxInstances       int
	ResourceCount      int
}

func (s *stepEnableFastLaunch) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	if !s.EnableFastLaunch {
		// Nothing to do if fast boot is disabled
		return multistep.ActionContinue
	}

	ui := state.Get("ui").(packersdk.Ui)

	if s.AMISkipCreateImage {
		ui.Say("Skipping fast-launch setup...")
		return multistep.ActionContinue
	}

	ec2conn := state.Get("ec2").(*ec2.EC2)
	amis := state.Get("amis").(map[string]string)

	for _, ami := range amis {
		ui.Say(fmt.Sprintf("Enabling fast boot for AMI %s", ami))

		fastLaunchInput := s.prepareFastLaunchRequest(ami, state)

		// Create a timeout for the EnableFastLaunch call.
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		err := retry.Config{
			Tries: 5,
			ShouldRetry: func(err error) bool {
				log.Printf("Enabling fast launch failed: %s", err)
				return true
			},
			RetryDelay: (&retry.Backoff{InitialBackoff: 500 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
		}.Run(timeoutCtx, func(ctx context.Context) error {
			var err error
			_, err = ec2conn.EnableFastLaunch(fastLaunchInput)
			return err
		})
		if err != nil {
			err := fmt.Errorf("Error enabling fast boot for AMI: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		// Wait for the image to become ready
		ui.Say("Waiting for fast launch to become ready...")
		waitErr := s.PollingConfig.WaitUntilFastLaunchEnabled(ctx, ec2conn, ami)
		if waitErr != nil {
			err := fmt.Errorf("Failed to enable fast launch: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	return multistep.ActionContinue
}

func (s *stepEnableFastLaunch) prepareFastLaunchRequest(ami string, state multistep.StateBag) *ec2.EnableFastLaunchInput {
	fastLaunchInput := &ec2.EnableFastLaunchInput{
		ImageId: &ami,
	}

	if s.MaxInstances > 0 {
		mi := int64(s.MaxInstances)
		fastLaunchInput.MaxParallelLaunches = &mi
	}

	if s.ResourceCount > 0 {
		rc := int64(s.ResourceCount)
		fastLaunchInput.SnapshotConfiguration = &ec2.FastLaunchSnapshotConfigurationRequest{
			TargetResourceCount: &rc,
		}
	}

	templateID, ok := state.Get("launch_template_id").(string)
	if !ok {
		return fastLaunchInput
	}
	version := fmt.Sprintf("%d", state.Get("launch_template_version").(int))

	fastLaunchInput.LaunchTemplate = &ec2.FastLaunchLaunchTemplateSpecificationRequest{
		LaunchTemplateId: &templateID,
		Version:          &version,
	}

	return fastLaunchInput
}

func (s *stepEnableFastLaunch) Cleanup(state multistep.StateBag) {}
