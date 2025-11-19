// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebs

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

type stepPrepareFastLaunchTemplate struct {
	AccessConfig       *common.AccessConfig
	AMISkipCreateImage bool
	EnableFastLaunch   bool
	RegionTemplates    []FastLaunchTemplateConfig
}

type TemplateSpec struct {
	TemplateID string
	Version    int
	// Since this is what gets forwarded to the step that enables fast launch
	// for each region, we have to also forward if the option should be disabled
	// for a particular region
	Enabled bool
}

func (s *stepPrepareFastLaunchTemplate) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)

	if s.AMISkipCreateImage {
		ui.Say("Skipping fast-launch template setup...")
		return multistep.ActionContinue
	}

	if !s.EnableFastLaunch {
		log.Printf("fast-boot disabled, no launch-template to set")
		return multistep.ActionContinue
	}

	if len(s.RegionTemplates) == 0 {
		log.Printf("[INFO] no launch-template configured, will use defaults.")
		return multistep.ActionContinue
	}

	templateIDsByRegion := map[string]TemplateSpec{}
	for _, templateSpec := range s.RegionTemplates {
		region := templateSpec.Region

		if templateSpec.EnableFalseLaunch == config.TriFalse {
			log.Printf("[INFO] fast-launch explicitly disabled for region %q", region)
			templateIDsByRegion[region] = TemplateSpec{
				Enabled: false,
			}
			continue
		}

		if templateSpec.LaunchTemplateID == "" && templateSpec.LaunchTemplateName == "" {
			log.Printf("[INFO] No fast-launch template specified for region %q", region)
			continue
		}

		regionEc2Client, err := common.GetRegionConn(ctx, s.AccessConfig, region)
		if err != nil {
			state.Put("error", fmt.Errorf("Failed to get connection to region %q: %s", region, err))
			return multistep.ActionHalt
		}

		tmpl, err := getTemplate(ctx, regionEc2Client, templateSpec)
		if err != nil {
			ui.Error(fmt.Sprintf("Failed to get launch template from region %q: %s", region, err))
			state.Put("error", err)
			return multistep.ActionHalt
		}

		ts := TemplateSpec{
			TemplateID: *tmpl.LaunchTemplateId,
			Version:    templateSpec.LaunchTemplateVersion,
			Enabled:    true,
		}

		log.Printf("found template in region %q: ID %q, name %q", region, *tmpl.LaunchTemplateId, *tmpl.LaunchTemplateName)

		if templateSpec.LaunchTemplateVersion == 0 {
			ts.Version = int(*tmpl.LatestVersionNumber)
		}

		if *tmpl.LatestVersionNumber < int64(templateSpec.LaunchTemplateVersion) {
			err := fmt.Errorf("specified version (%d) is higher than the latest launch template version (%d) for region %q",
				templateSpec.LaunchTemplateVersion,
				tmpl.LatestVersionNumber,
				region)
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		templateIDsByRegion[region] = ts
	}

	state.Put("launch_template_version", templateIDsByRegion)

	return multistep.ActionContinue
}

func getTemplate(ctx context.Context, ec2Client clients.Ec2Client, templateSpec FastLaunchTemplateConfig) (*ec2types.
	LaunchTemplate, error) {
	requestInput := &ec2.DescribeLaunchTemplatesInput{}

	if templateSpec.LaunchTemplateID != "" {
		requestInput.LaunchTemplateIds = []string{templateSpec.LaunchTemplateID}
	}
	if templateSpec.LaunchTemplateName != "" {
		requestInput.LaunchTemplateNames = []string{templateSpec.LaunchTemplateName}
	}

	lts, err := ec2Client.DescribeLaunchTemplates(ctx, requestInput)
	if err != nil {
		return nil, err
	}

	tmpls := lts.LaunchTemplates
	if len(tmpls) != 1 {
		return nil, fmt.Errorf("failed to get launch template %q; received %d responses, expected only one to match",
			templateSpec.LaunchTemplateID,
			len(tmpls))
	}

	tmpl := tmpls[0]

	return &tmpl, nil
}

func (s *stepPrepareFastLaunchTemplate) Cleanup(state multistep.StateBag) {}
