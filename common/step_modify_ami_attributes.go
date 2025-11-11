// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

type StepModifyAMIAttributes struct {
	AMISkipCreateImage bool

	Users          []string
	Groups         []string
	OrgArns        []string
	OuArns         []string
	SnapshotUsers  []string
	SnapshotGroups []string
	ProductCodes   []string
	IMDSSupport    string
	Description    string
	Ctx            interpolate.Context

	GeneratedData *packerbuilderdata.GeneratedData
}

func (s *StepModifyAMIAttributes) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	accessConfig := state.Get("access_config").(*AccessConfig)
	awsConfig := state.Get("aws_config").(*aws.Config)
	ui := state.Get("ui").(packersdk.Ui)

	if s.AMISkipCreateImage {
		ui.Say("Skipping AMI modify attributes...")
		return multistep.ActionContinue
	}

	amis := state.Get("amis").(map[string]string)
	snapshots := state.Get("snapshots").(map[string][]string)

	// Determine if there is any work to do.
	valid := false
	valid = valid || s.Description != ""
	valid = valid || len(s.Users) > 0
	valid = valid || len(s.Groups) > 0
	valid = valid || len(s.OrgArns) > 0
	valid = valid || len(s.OuArns) > 0
	valid = valid || len(s.ProductCodes) > 0
	valid = valid || len(s.SnapshotUsers) > 0
	valid = valid || len(s.SnapshotGroups) > 0
	valid = valid || s.IMDSSupport != ""

	if !valid {
		return multistep.ActionContinue
	}

	var err error
	s.Ctx.Data = extractBuildInfo(awsConfig.Region, state, s.GeneratedData)
	s.Description, err = interpolate.Render(s.Description, &s.Ctx)
	if err != nil {
		err = fmt.Errorf("Error interpolating AMI description: %s", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// Construct the modify image and snapshot attribute requests we're going
	// to make. We need to make each separately since the EC2 API only allows
	// changing one type at a kind currently.
	options := make(map[string]*ec2.ModifyImageAttributeInput)
	if s.Description != "" {
		options["description"] = &ec2.ModifyImageAttributeInput{
			Description: &ec2types.AttributeValue{Value: &s.Description},
		}
	}
	snapshotOptions := make(map[string]*ec2.ModifySnapshotAttributeInput)

	if len(s.Groups) > 0 {
		groups := make([]string, len(s.Groups))
		addsImage := make([]ec2types.LaunchPermission, len(s.Groups))
		addGroups := &ec2.ModifyImageAttributeInput{
			LaunchPermission: &ec2types.LaunchPermissionModifications{},
		}

		for i, g := range s.Groups {
			groups[i] = g
			addsImage[i] = ec2types.LaunchPermission{
				Group: ec2types.PermissionGroup(g),
			}
		}

		addGroups.UserGroups = groups
		addGroups.LaunchPermission.Add = addsImage
		options["groups"] = addGroups
	}

	if len(s.SnapshotGroups) > 0 {
		groups := make([]string, len(s.SnapshotGroups))
		addsSnapshot := make([]ec2types.CreateVolumePermission, len(s.SnapshotGroups))
		addSnapshotGroups := &ec2.ModifySnapshotAttributeInput{
			CreateVolumePermission: &ec2types.CreateVolumePermissionModifications{},
		}

		for i, g := range s.SnapshotGroups {
			groups[i] = g
			addsSnapshot[i] = ec2types.CreateVolumePermission{
				Group: ec2types.PermissionGroup(g),
			}
		}
		addSnapshotGroups.GroupNames = groups
		addSnapshotGroups.CreateVolumePermission.Add = addsSnapshot
		snapshotOptions["groups"] = addSnapshotGroups
	}

	if len(s.Users) > 0 {
		users := make([]string, len(s.Users))
		addsImage := make([]ec2types.LaunchPermission, len(s.Users))
		for i, u := range s.Users {
			users[i] = u
			addsImage[i] = ec2types.LaunchPermission{UserId: aws.String(u)}
		}

		options["users"] = &ec2.ModifyImageAttributeInput{
			UserIds: users,
			LaunchPermission: &ec2types.LaunchPermissionModifications{
				Add: addsImage,
			},
		}
	}

	if len(s.SnapshotUsers) > 0 {
		users := make([]string, len(s.SnapshotUsers))
		addsSnapshot := make([]ec2types.CreateVolumePermission, len(s.SnapshotUsers))
		for i, u := range s.SnapshotUsers {
			users[i] = u
			addsSnapshot[i] = ec2types.CreateVolumePermission{UserId: aws.String(u)}
		}

		snapshotOptions["users"] = &ec2.ModifySnapshotAttributeInput{
			UserIds: users,
			CreateVolumePermission: &ec2types.CreateVolumePermissionModifications{
				Add: addsSnapshot,
			},
		}
	}

	if len(s.OrgArns) > 0 {
		orgArns := make([]string, len(s.OrgArns))
		addsImage := make([]ec2types.LaunchPermission, len(s.OrgArns))
		for i, u := range s.OrgArns {
			orgArns[i] = u
			addsImage[i] = ec2types.LaunchPermission{OrganizationArn: aws.String(u)}
		}

		options["ami org arns"] = &ec2.ModifyImageAttributeInput{
			OrganizationArns: orgArns,
			LaunchPermission: &ec2types.LaunchPermissionModifications{
				Add: addsImage,
			},
		}
	}

	if len(s.OuArns) > 0 {
		ouArns := make([]string, len(s.OuArns))
		addsImage := make([]ec2types.LaunchPermission, len(s.OuArns))
		for i, u := range s.OuArns {
			ouArns[i] = u
			addsImage[i] = ec2types.LaunchPermission{OrganizationalUnitArn: aws.String(u)}
		}

		options["ami ou arns"] = &ec2.ModifyImageAttributeInput{
			OrganizationalUnitArns: ouArns,
			LaunchPermission: &ec2types.LaunchPermissionModifications{
				Add: addsImage,
			},
		}
	}

	if len(s.ProductCodes) > 0 {
		codes := make([]string, len(s.ProductCodes))
		copy(codes, s.ProductCodes)
		options["product codes"] = &ec2.ModifyImageAttributeInput{
			ProductCodes: codes,
		}
	}

	if s.IMDSSupport != "" {
		options["imds_support"] = &ec2.ModifyImageAttributeInput{
			ImdsSupport: &ec2types.AttributeValue{
				Value: &s.IMDSSupport,
			},
		}
	}

	// Modifying image attributes
	for region, ami := range amis {
		ui.Say(fmt.Sprintf("Modifying attributes on AMI (%s)...", ami))
		regionEc2Client, err := GetRegionConn(ctx, accessConfig, region)
		if err != nil {
			err := fmt.Errorf("Error getting region connection for modify AMI attributes: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
		for name, input := range options {
			ui.Say(fmt.Sprintf("Modifying: %s", name))
			input.ImageId = &ami
			_, err := regionEc2Client.ModifyImageAttribute(ctx, input)
			if err != nil {
				err := fmt.Errorf("Error modify AMI attributes: %s", err)
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
		}
	}

	// Modifying snapshot attributes
	for region, region_snapshots := range snapshots {
		for _, snapshot := range region_snapshots {
			ui.Say(fmt.Sprintf("Modifying attributes on snapshot (%s)...", snapshot))
			regionEc2Client := ec2.NewFromConfig(*awsConfig, func(o *ec2.Options) {
				o.Region = region
			})
			for name, input := range snapshotOptions {
				ui.Message(fmt.Sprintf("Modifying: %s", name))
				input.SnapshotId = &snapshot
				_, err := regionEc2Client.ModifySnapshotAttribute(ctx, input)
				if err != nil {
					err := fmt.Errorf("Error modify snapshot attributes: %s", err)
					state.Put("error", err)
					ui.Error(err.Error())
					return multistep.ActionHalt
				}
			}
		}
	}

	return multistep.ActionContinue
}

func (s *StepModifyAMIAttributes) Cleanup(state multistep.StateBag) {
	// No cleanup...
}
