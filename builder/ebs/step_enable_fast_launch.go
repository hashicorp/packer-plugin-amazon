// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
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

	wg := &sync.WaitGroup{}
	wg.Add(len(amis))

	errWg := &sync.WaitGroup{}
	errWg.Add(1)

	errCh := make(chan (error), 1)
	var errs []error
	go func() {
		defer errWg.Done()
		for err := range errCh {
			errs = append(errs, err)
		}
	}()

	for region, ami := range amis {
		regionEc2Client, err := common.GetRegionConn(ctx, s.AccessConfig, region)
		if err != nil {
			state.Put("error", fmt.Errorf("Failed to get connection to region %q: %w", region, err))
			return multistep.ActionHalt
		}

		go func(region, ami string) {
			defer wg.Done()

			// Immediately return if we don't want fast-launch for a
			// particular region
			templateIDsByRegion, ok := state.GetOk("launch_template_version")
			if ok && !templateIDsByRegion.(map[string]TemplateSpec)[region].Enabled {
				log.Printf("skipping fast launch for region %q", region)
				return
			}

			// Casting is somewhat unsafe, but since the retryer below only
			// accepts this type, and not ec2iface.EC2API, we can safely
			// do this here, unless the `GetRegionConn` function evolves
			// later, in which case this will fail.
			// todo check this
			//ec2conn := regionEc2Client.(*ec2.EC2)

			ui.Say(fmt.Sprintf("Enabling fast boot for AMI %s in region %s", ami, region))

			fastLaunchInput := s.prepareFastLaunchRequest(ami, region, state)

			// Create a timeout for the EnableFastLaunch call.
			timeoutCtx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()

			err = retry.Config{
				Tries: 20,
				ShouldRetry: func(err error) bool {
					log.Printf("Enabling fast launch failed: %s", err)
					return true
				},
				RetryDelay: (&retry.Backoff{InitialBackoff: 500 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
			}.Run(timeoutCtx, func(ctx context.Context) error {
				var err error
				_, err = regionEc2Client.EnableFastLaunch(ctx, fastLaunchInput)
				return err
			})
			if err != nil {
				errCh <- fmt.Errorf("Error enabling fast boot for AMI in region %s: %w", region, err)
				return
			}

			// Wait for the image to become ready
			ui.Say(fmt.Sprintf("Waiting for fast launch to become ready on AMI %q in region %s...", ami, region))
			waitErr := s.PollingConfig.WaitUntilFastLaunchEnabled(ctx, regionEc2Client, ami)
			if waitErr != nil {
				errCh <- fmt.Errorf("Failed to enable fast launch: %w", waitErr)
				return
			}

			flStatus, err := regionEc2Client.DescribeFastLaunchImages(ctx, &ec2.DescribeFastLaunchImagesInput{
				ImageIds: []string{
					ami,
				},
			})
			if err != nil {
				errCh <- fmt.Errorf("Failed to get fast-launch status for AMI %q: %w", ami, err)
				return
			}

			for _, img := range flStatus.FastLaunchImages {
				if img.State != "enabled" {
					errCh <- fmt.Errorf("Failed to enable fast-launch for AMI %q: %s", ami, *img.StateTransitionReason)
					return
				}
			}
		}(region, ami)
	}

	wg.Wait()
	close(errCh)
	// Wait until we finished queueing the errors before continuing
	//
	// Otherwise we may end-up queueing an error and leaving the routine
	// that processes with an error, and the error check below may execute
	// before the error gets queued up, which may result in a build
	// succeeding even if there was an error with the fast-launch on one AMI.
	errWg.Wait()

	for _, err := range errs {
		ui.Error(err.Error())
	}

	if errs != nil {
		err := errors.New("Failed to enable fast-launch because of errors")
		ui.Error(err.Error())
		state.Put("error", err)
		return multistep.ActionHalt
	}

	return multistep.ActionContinue
}

func (s *stepEnableFastLaunch) prepareFastLaunchRequest(ami string, region string, state multistep.StateBag) *ec2.EnableFastLaunchInput {
	fastLaunchInput := &ec2.EnableFastLaunchInput{
		ImageId: &ami,
	}

	if s.MaxInstances > 0 {
		mi := int32(s.MaxInstances)
		fastLaunchInput.MaxParallelLaunches = &mi
	}

	if s.ResourceCount > 0 {
		rc := int32(s.ResourceCount)
		fastLaunchInput.SnapshotConfiguration = &ec2types.FastLaunchSnapshotConfigurationRequest{
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

	fastLaunchInput.LaunchTemplate = &ec2types.FastLaunchLaunchTemplateSpecificationRequest{
		LaunchTemplateId: &templateConfig.TemplateID,
		Version:          aws.String(fmt.Sprintf("%d", templateConfig.Version)),
	}

	return fastLaunchInput
}

func (s *stepEnableFastLaunch) Cleanup(state multistep.StateBag) {}
