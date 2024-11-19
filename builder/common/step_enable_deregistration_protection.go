// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type StepEnableDeregistrationProtection struct {
	AccessConfig             *AccessConfig
	AMISkipCreateImage       bool
	DeregistrationProtection *DeregistrationProtectionOptions
}

func (s *StepEnableDeregistrationProtection) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	if !s.DeregistrationProtection.Enabled {
		ui.Say("Skipping Enable AMI deregistration protection...")
		return multistep.ActionContinue
	}

	if s.AMISkipCreateImage {
		ui.Say("skip_create_ami was set. Skipping AMI deregistration protection...")
		return multistep.ActionContinue
	}

	amis, ok := state.Get("amis").(map[string]string)
	if !ok {
		err := fmt.Errorf("no AMIs found in state to enable deregistration protection")
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	for region, ami := range amis {
		log.Printf("Enabling deregistration protection on AMI (%s) in region %q ...", ami, region)

		conn, err := GetRegionConn(s.AccessConfig, region)
		if err != nil {
			err := fmt.Errorf("failed to connect to region %s: %s", region, err)
			state.Put("error", err.Error())
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		_, err = conn.EnableImageDeregistrationProtection(&ec2.EnableImageDeregistrationProtectionInput{
			ImageId:      &ami,
			WithCooldown: &s.DeregistrationProtection.WithCooldown,
		})
		if err != nil {
			err := fmt.Errorf("failed to enable AMI deregistration protection: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}
	return multistep.ActionContinue
}
func (s *StepEnableDeregistrationProtection) Cleanup(state multistep.StateBag) {
	// No cleanup...
}
