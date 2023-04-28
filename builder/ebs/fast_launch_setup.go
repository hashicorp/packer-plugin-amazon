// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type FastLaunchConfig

package ebs

import "fmt"

// FastLaunchConfig is the configuration for setting up fast-launch for Windows AMIs
//
// NOTE: requires the Windows image to be sysprep'd to enable fast-launch. See the
// AWS docs for more information:
// https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/win-ami-config-fast-launch.html
type FastLaunchConfig struct {
	// Configure fast-launch for Windows AMIs
	UseFastLaunch bool `mapstructure:"enable_fast_launch"`
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

	if c.LaunchTemplateID != "" && c.LaunchTemplateName != "" {
		errs = append(errs, fmt.Errorf("both template ID and name cannot be specified at the same time"))
	}

	if c.LaunchTemplateVersion != 0 && c.LaunchTemplateVersion < 1 {
		errs = append(errs, fmt.Errorf("the launch template version must be >= 1, provided value is %d", c.LaunchTemplateVersion))
	}

	if c.LaunchTemplateVersion != 0 && c.LaunchTemplateID == "" && c.LaunchTemplateName == "" {
		errs = append(errs, fmt.Errorf("unsupported: launch template version specified without an ID or name"))
	}

	return errs
}
