// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type StepEnableDeprecation struct {
	AccessConfig       *AccessConfig
	DeprecationTime    string
	AMISkipCreateImage bool
}

func (s *StepEnableDeprecation) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	if s.AMISkipCreateImage || s.DeprecationTime == "" {
		ui.Say("Skipping Enable AMI deprecation...")
		return multistep.ActionContinue
	}

	amis, ok := state.Get("amis").(map[string]string)
	if !ok {
		err := fmt.Errorf("no AMIs found in state to deprecate")
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	deprecationTime, _ := time.Parse(time.RFC3339, s.DeprecationTime)
	for region, ami := range amis {
		ui.Say(fmt.Sprintf("Enabling deprecation on AMI (%s) in region %q ...", ami, region))

		conn, err := GetRegionConn(s.AccessConfig, region)
		if err != nil {
			err := fmt.Errorf("failed to connect to region %s: %s", region, err)
			state.Put("error", err.Error())
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		_, err = conn.EnableImageDeprecation(&ec2.EnableImageDeprecationInput{
			ImageId:     &ami,
			DeprecateAt: &deprecationTime,
		})
		if err != nil {
			err := fmt.Errorf("Error enable AMI deprecation: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}
	return multistep.ActionContinue
}
func (s *StepEnableDeprecation) Cleanup(state multistep.StateBag) {
	// No cleanup...
}
