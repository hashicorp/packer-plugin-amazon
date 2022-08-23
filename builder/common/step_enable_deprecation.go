package common

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
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

		session, err := s.AccessConfig.Session()
		if err != nil {
			err := fmt.Errorf("Error getting region %s connection for deprecation: %s", region, err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		regionconn := ec2.New(session.Copy(&aws.Config{
			Region: aws.String(region),
		}))

		ui.Say(fmt.Sprintf("Enabling deprecation on AMI (%s)...", ami))

		_, err = regionconn.EnableImageDeprecation(&ec2.EnableImageDeprecationInput{
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
