// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebs

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type stepPrepareFastLaunchTemplate struct {
	AMISkipCreateImage bool
	EnableFastLaunch   bool
	TemplateID         string
	TemplateName       string
	TemplateVersion    int
}

func (s *stepPrepareFastLaunchTemplate) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ec2conn := state.Get("ec2").(*ec2.EC2)
	ui := state.Get("ui").(packersdk.Ui)

	if !s.EnableFastLaunch {
		log.Printf("fast-boot disabled, no launch-template to set")
		return multistep.ActionContinue
	}

	if s.TemplateID == "" && s.TemplateName == "" {
		ui.Say("No fast-launch template specified")
		return multistep.ActionContinue
	}

	if s.AMISkipCreateImage {
		ui.Say("Skipping fast-launch template setup...")
		return multistep.ActionContinue
	}

	tmpl, err := s.getTemplate(ec2conn)
	if err != nil {
		ui.Error(err.Error())
		state.Put("error", err)
		return multistep.ActionHalt
	}

	log.Printf("found template ID %q, name is %q", *tmpl.LaunchTemplateId, *tmpl.LaunchTemplateName)

	state.Put("launch_template_id", *tmpl.LaunchTemplateId)
	if s.TemplateVersion == 0 {
		log.Printf("latest launch template version is %d", *tmpl.LatestVersionNumber)
		state.Put("launch_template_version", int(*tmpl.LatestVersionNumber))
		return multistep.ActionContinue
	}

	if *tmpl.LatestVersionNumber < int64(s.TemplateVersion) {
		err := fmt.Errorf("specified version (%d) is higher than the latest launch template version (%d)",
			s.TemplateVersion,
			tmpl.LatestVersionNumber)
		ui.Error(err.Error())
		state.Put("error", err)
		return multistep.ActionHalt
	}

	log.Printf("setting launch template version to %d", s.TemplateVersion)
	state.Put("launch_template_version", int(s.TemplateVersion))

	return multistep.ActionContinue
}

func (s *stepPrepareFastLaunchTemplate) getTemplate(ec2conn *ec2.EC2) (*ec2.LaunchTemplate, error) {
	requestInput := &ec2.DescribeLaunchTemplatesInput{}

	if s.TemplateID != "" {
		requestInput.LaunchTemplateIds = []*string{&s.TemplateID}
	}
	if s.TemplateName != "" {
		requestInput.LaunchTemplateNames = []*string{&s.TemplateName}
	}

	lts, err := ec2conn.DescribeLaunchTemplates(requestInput)
	if err != nil {
		return nil, err
	}

	tmpls := lts.LaunchTemplates
	if len(tmpls) != 1 {
		return nil, fmt.Errorf("failed to get launch template %q; received %d responses, expected only one to match",
			s.TemplateID,
			len(tmpls))
	}

	tmpl := tmpls[0]

	return tmpl, nil
}

func (s *stepPrepareFastLaunchTemplate) Cleanup(state multistep.StateBag) {}
