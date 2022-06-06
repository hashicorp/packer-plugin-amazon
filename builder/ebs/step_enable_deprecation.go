package ebs

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type stepEnableDeprecation struct {
	DeprecationTime    string
	AMISkipCreateImage bool
}

func (s *stepEnableDeprecation) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	if s.AMISkipCreateImage || s.DeprecationTime == "" {
		ui.Say("Skipping Enable AMI deprecation...")
		return multistep.ActionContinue
	}

	ec2conn := state.Get("ec2").(*ec2.EC2)
	amis, ok := state.Get("amis").(map[string]string)
	if !ok {
		err := fmt.Errorf("no AMIs found in state to deprecate")
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	deprecationTime, _ := time.Parse(time.RFC3339, s.DeprecationTime)
	for _, ami := range amis {
		ui.Say(fmt.Sprintf("Enabling deprecation on AMI (%s)...", ami))

		_, err := ec2conn.EnableImageDeprecation(&ec2.EnableImageDeprecationInput{
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
func (s *stepEnableDeprecation) Cleanup(state multistep.StateBag) {
	// No cleanup...
}
