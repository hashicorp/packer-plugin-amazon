// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/packer-plugin-amazon/builder/common"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/retry"
)

type stepEnableFastLaunch struct {
	AccessConfig       *common.AccessConfig
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

	amis := state.Get("amis").(map[string]string)

	if len(amis) == 0 {
		ui.Say("No AMI found in state, skipping fast-launch setup...")
		return multistep.ActionContinue
	}

	for region, ami := range amis {
		ec2connif, err := common.GetRegionConn(s.AccessConfig, region)
		if err != nil {
			state.Put("error", fmt.Errorf("Failed to get connection to region %q: %s", region, err))
			return multistep.ActionHalt
		}

		// Casting is somewhat unsafe, but since the retryer below only
		// accepts this type, and not ec2iface.EC2API, we can safely
		// do this here, unless the `GetRegionConn` function evolves
		// later, in which case this will fail.
		ec2conn := ec2connif.(*ec2.EC2)

		ui.Say(fmt.Sprintf("Enabling fast boot for AMI %s in region %s", ami, region))

		fastLaunchInput := s.prepareFastLaunchRequest(ami, region, state)

		// Create a timeout for the EnableFastLaunch call.
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		err = retry.Config{
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
			err := fmt.Errorf("Error enabling fast boot for AMI in region %s: %s", region, err)
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

		flStatus, err := ec2conn.DescribeFastLaunchImages(&ec2.DescribeFastLaunchImagesInput{
			ImageIds: []*string{
				&ami,
			},
		})
		if err != nil {
			err := fmt.Errorf("Failed to get fast-launch status for AMI %q: %s", ami, err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		for _, img := range flStatus.FastLaunchImages {
			if *img.State != "enabled" {
				err := fmt.Errorf("Failed to enable fast-launch for AMI %q: %s", ami, *img.StateTransitionReason)
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
		}
	}

	return multistep.ActionContinue
}

func (s *stepEnableFastLaunch) prepareFastLaunchRequest(ami string, region string, state multistep.StateBag) *ec2.EnableFastLaunchInput {
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

	templateIDsByRegion, ok := state.GetOk("launch_template_version")
	if !ok {
		log.Printf("[TRACE] no template specified for region %q", region)
		return fastLaunchInput
	}

	templateConfig, ok := templateIDsByRegion.(map[string]TemplateSpec)[region]
	if !ok {
		log.Printf("[TRACE] no template specified for region %q", region)
		return fastLaunchInput
	}

	fastLaunchInput.LaunchTemplate = &ec2.FastLaunchLaunchTemplateSpecificationRequest{
		LaunchTemplateId: &templateConfig.TemplateID,
		Version:          aws.String(fmt.Sprintf("%d", templateConfig.Version)),
	}

	return fastLaunchInput
}

func (s *stepEnableFastLaunch) Cleanup(state multistep.StateBag) {}
