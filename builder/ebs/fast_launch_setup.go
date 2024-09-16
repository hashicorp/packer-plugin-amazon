// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type FastLaunchConfig,FastLaunchTemplateConfig

package ebs

import (
	"fmt"
	"log"

	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

// FastLaunchTemplateConfig is the launch template configuration for a region.
//
// This must be used if the configuration has more than one region specified
// in the template, as each fast-launch enablement step occurs after the
// copy, and each region may pick their own launch template.
type FastLaunchTemplateConfig struct {
	// Enable fast launch allows you to disable fast launch settings on the region level.
	//
	// If unset, the default region behavior will be assumed - i.e. either use
	// the globally specified template ID/name (if specified), or AWS will set
	// it for you.
	//
	// Using other fast launch options, while unset, will imply enable_fast_launch to be true.
	//
	// If this is explicitly set to `false` fast-launch will be
	// disabled for the specified region and all other options besides region
	// will be ignored.
	EnableFalseLaunch config.Trilean `mapstructure:"enable_fast_launch"`
	// The region in which to find the launch template to use
	Region string `mapstructure:"region" required:"true"`
	// The ID of the launch template to use for the fast launch
	//
	// This cannot be specified in conjunction with the template name.
	//
	// If no template is specified, the default launch template will be used,
	// as specified in the AWS docs.
	LaunchTemplateID string `mapstructure:"template_id"`
	// The name of the launch template to use for fast launch
	//
	// This cannot be specified in conjunction with the template ID.
	//
	// If no template is specified, the default launch template will be used,
	// as specified in the AWS docs.
	LaunchTemplateName string `mapstructure:"template_name"`
	// The version of the launch template to use
	//
	// If unspecified, and a template is referenced, this will default to
	// the latest version available for the template.
	LaunchTemplateVersion int `mapstructure:"template_version"`
}

func (tc *FastLaunchTemplateConfig) Prepare() []error {
	var errs []error

	if tc.Region == "" {
		return append(errs, fmt.Errorf("region cannot be empty for a regional fast template config"))
	}

	// If we disabled fast-launch, we can immediately exit without validating
	// the other options.
	if tc.EnableFalseLaunch == config.TriFalse {
		return errs
	}

	if tc.LaunchTemplateID != "" && tc.LaunchTemplateName != "" {
		errs = append(errs, fmt.Errorf("fast_launch_template_config region %q: both template ID and name cannot be specified at the same time", tc.Region))
	}

	if tc.LaunchTemplateVersion != 0 && tc.LaunchTemplateVersion < 1 {
		errs = append(errs, fmt.Errorf("fast_launch_template_config region %q: the launch template version must be >= 1, provided value is %d", tc.Region, tc.LaunchTemplateVersion))
	}

	if tc.LaunchTemplateVersion != 0 && tc.LaunchTemplateID == "" && tc.LaunchTemplateName == "" {
		errs = append(errs, fmt.Errorf("fast_launch_template_config region %q: launch template version specified without an ID or name", tc.Region))
	}

	return errs
}

// FastLaunchConfig is the configuration for setting up fast-launch for Windows AMIs
//
// NOTE: requires the Windows image to be sysprep'd to enable fast-launch. See the
// AWS docs for more information:
// https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/win-ami-config-fast-launch.html
type FastLaunchConfig struct {
	// Configure fast-launch for Windows AMIs
	UseFastLaunch bool `mapstructure:"enable_fast_launch"`
	// The region in which the AMI will be built
	//
	// This is only set by the builder, and not by the client.
	// Its only use is to move the launch template configuration if it has
	// been specified into the map of region -> FastLaunchTemplateConfig
	// for easier processing later.
	defaultRegion string
	// The ID of the launch template to use for fast launch for the main AMI.
	//
	// This cannot be specified in conjunction with the template name.
	//
	// If no template is specified, the default launch template will be used,
	// as specified in the AWS docs.
	//
	// If you copy the AMI to other regions, this option should not
	// be used, use instead the `fast_launch_template_config` option.
	LaunchTemplateID string `mapstructure:"template_id"`
	// The name of the launch template to use for fast launch for the main AMI.
	//
	// This cannot be specified in conjunction with the template ID.
	//
	// If no template is specified, the default launch template will be used,
	// as specified in the AWS docs.
	//
	// If you copy the AMI to other regions, this option should not
	// be used, use instead the `fast_launch_template_config` option.
	LaunchTemplateName string `mapstructure:"template_name"`
	// The version of the launch template to use for fast launch for the main AMI.
	//
	// If unspecified, and a template is referenced, this will default to
	// the latest version available for the template.
	//
	// If you copy the AMI to other regions, this option should not
	// be used, use instead the `fast_launch_template_config` option.
	LaunchTemplateVersion int `mapstructure:"template_version"`
	// RegionLaunchTemplates is the list of launch templates per region.
	//
	// This should be specified if you want to use a custom launch
	// template for your fast-launched images, and you are copying
	// the image to other regions.
	//
	// All regions don't need a launch template configuration, but for
	// each that don't have a launch template specified, AWS will pick
	// a default one for that purpose.
	//
	// For information about each entry, refer to the
	// [Fast Launch Template Config](#fast-launch-template-config) documentation.
	RegionLaunchTemplates []FastLaunchTemplateConfig `mapstructure:"region_launch_templates"`
	// Maximum number of instances to launch for creating pre-provisioned snapshots
	//
	// If specified, must be a minimum of `6`
	MaxParallelLaunches int `mapstructure:"max_parallel_launches"`
	// The number of snapshots to pre-provision for later launching windows instances
	// from the resulting fast-launch AMI.
	//
	// If unspecified, this will create the default number of snapshots (as of
	// march 2023, this defaults to 5 on AWS)
	TargetResourceCount int `mapstructure:"target_resource_count"`
}

func (c FastLaunchConfig) isDefault() bool {
	if c.UseFastLaunch {
		return false
	}

	if c.MaxParallelLaunches != 0 {
		return false
	}

	if c.LaunchTemplateID != "" {
		return false
	}

	if c.LaunchTemplateName != "" {
		return false
	}

	if c.LaunchTemplateVersion != 0 {
		return false
	}

	if c.TargetResourceCount != 0 {
		return false
	}

	if len(c.RegionLaunchTemplates) != 0 {
		return false
	}

	return true
}

func (c *FastLaunchConfig) Prepare() []error {
	if c.isDefault() {
		return nil
	}

	// If any fast_launch option is set, we enable it.
	c.UseFastLaunch = true

	var errs []error

	if c.MaxParallelLaunches != 0 && c.MaxParallelLaunches < 6 {
		errs = append(errs, fmt.Errorf("max_parallel_launches must be >= 6, provided value is %d", c.MaxParallelLaunches))
	}

	if c.TargetResourceCount != 0 && c.TargetResourceCount < 1 {
		errs = append(errs, fmt.Errorf("target_resource_count must be >= 1, provided value is %d", c.TargetResourceCount))
	}

	if c.LaunchTemplateID != "" || c.LaunchTemplateName != "" || c.LaunchTemplateVersion != 0 {
		log.Printf("[INFO] fast_launch: setting default region %q for top-level template config", c.defaultRegion)
		c.RegionLaunchTemplates = append(c.RegionLaunchTemplates, FastLaunchTemplateConfig{
			Region:                c.defaultRegion,
			LaunchTemplateID:      c.LaunchTemplateID,
			LaunchTemplateName:    c.LaunchTemplateName,
			LaunchTemplateVersion: c.LaunchTemplateVersion,
		})
	}

	// Duplication check
	regions := map[string]struct{}{}

	for _, templateConfig := range c.RegionLaunchTemplates {
		_, ok := regions[templateConfig.Region]
		if ok {
			errs = append(errs, fmt.Errorf("fast launch: launch template specified twice for region %q, only once is supported", templateConfig.Region))
		}

		regions[templateConfig.Region] = struct{}{}

		err := templateConfig.Prepare()
		if err != nil {
			errs = append(errs, err...)
		}
	}

	return errs
}
