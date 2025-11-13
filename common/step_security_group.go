// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/hashicorp/packer-plugin-sdk/uuid"
)

type StepSecurityGroup struct {
	PollingConfig             *AWSPollingConfig
	CommConfig                *communicator.Config
	SecurityGroupFilter       SecurityGroupFilterOptions
	SecurityGroupIds          []string
	TemporarySGSourceCidrs    []string
	TemporarySGSourcePublicIp bool
	SkipSSHRuleCreation       bool
	Ctx                       interpolate.Context
	IsRestricted              bool
	Tags                      map[string]string

	createdGroupId string
}

func (s *StepSecurityGroup) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ec2Client := state.Get("ec2v2").(clients.Ec2Client)
	awsConfig := state.Get("aws_config").(*aws.Config)
	ui := state.Get("ui").(packersdk.Ui)
	vpcId := state.Get("vpc_id").(string)

	if len(s.SecurityGroupIds) > 0 {
		_, err := ec2Client.DescribeSecurityGroups(ctx,
			&ec2.DescribeSecurityGroupsInput{
				GroupIds: s.SecurityGroupIds,
			},
		)
		if err != nil {
			err := fmt.Errorf("Couldn't find specified security group: %s", err)
			log.Printf("[DEBUG] %s", err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}
		log.Printf("Using specified security groups: %v", s.SecurityGroupIds)
		state.Put("securityGroupIds", s.SecurityGroupIds)
		return multistep.ActionContinue
	}

	if !s.SecurityGroupFilter.Empty() {

		params := &ec2.DescribeSecurityGroupsInput{}
		if vpcId != "" {
			s.SecurityGroupFilter.Filters["vpc-id"] = vpcId
		}
		securityGroupFilters, err := buildEc2Filters(s.SecurityGroupFilter.Filters)
		if err != nil {
			err := fmt.Errorf("Couldn't parse security groups filters: %s", err)
			log.Printf("[DEBUG] %s", err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}
		params.Filters = securityGroupFilters

		log.Printf("Using SecurityGroup Filters %s", prettyFilter(params.Filters))

		sgResp, err := ec2Client.DescribeSecurityGroups(ctx, params)
		if err != nil {
			err := fmt.Errorf("Couldn't find security groups for filter: %s", err)
			log.Printf("[DEBUG] %s", err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		securityGroupIds := []string{}
		for _, sg := range sgResp.SecurityGroups {
			securityGroupIds = append(securityGroupIds, *sg.GroupId)
		}

		ui.Say(fmt.Sprintf("Found Security Group(s): %s", strings.Join(securityGroupIds, ", ")))
		state.Put("securityGroupIds", securityGroupIds)

		return multistep.ActionContinue
	}

	// Create the group
	groupName := fmt.Sprintf("packer_%s", uuid.TimeOrderedUUID())
	ui.Say(fmt.Sprintf("Creating temporary security group for this instance: %s", groupName))
	group := &ec2.CreateSecurityGroupInput{
		GroupName:   &groupName,
		Description: aws.String("Temporary group for Packer"),
	}

	if !s.IsRestricted {
		ec2Tags, err := TagMap(s.Tags).EC2Tags(s.Ctx, awsConfig.Region, state)
		if err != nil {
			err := fmt.Errorf("Error tagging security group: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		group.TagSpecifications = ec2Tags.TagSpecifications(ec2types.ResourceTypeSecurityGroup)
	}

	group.VpcId = &vpcId

	groupResp, err := ec2Client.CreateSecurityGroup(ctx, group)
	if err != nil {
		ui.Error(err.Error())
		state.Put("error", err)
		return multistep.ActionHalt
	}

	// Set the group ID so we can delete it later
	s.createdGroupId = *groupResp.GroupId

	// Wait for the security group become available for authorizing
	log.Printf("[DEBUG] Waiting for temporary security group: %s", s.createdGroupId)
	err = s.PollingConfig.WaitUntilSecurityGroupExists(ctx, ec2Client, s.createdGroupId)
	if err != nil {
		err := fmt.Errorf("Timed out waiting for security group %s: %s", s.createdGroupId, err)
		log.Printf("[DEBUG] %s", err.Error())
		state.Put("error", err)
		return multistep.ActionHalt
	}

	log.Printf("[DEBUG] Found security group %s", s.createdGroupId)

	temporarySGSourceCidrs := s.TemporarySGSourceCidrs
	if len(temporarySGSourceCidrs) != 0 && s.TemporarySGSourcePublicIp {
		ui.Say("Using temporary_security_group_source_public_ip with" +
			" temporary_security_group_source_cidrs is unsupported," +
			" ignoring temporary_security_group_source_public_ip.")
	}
	if len(temporarySGSourceCidrs) == 0 && s.TemporarySGSourcePublicIp {
		ui.Say("Checking current host's public IP...")

		ip, err := CheckPublicIp()
		if err != nil {
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		// ensure 0.0.0.0 isn't used to configure the SG
		if ip.IsUnspecified() {
			err := fmt.Errorf("Current host's public IP is unspecified: %s", ip)
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		ui.Say(fmt.Sprintf("Current host's public IP: %s", ip))

		// ip.To4() attempts to parse the IP as IPv4, if this fails, we fallback to IPv6.
		bits := 128
		if tmp := ip.To4(); tmp != nil {
			ip = tmp
			bits = 32
		}
		temporarySGSourceCidrs = []string{fmt.Sprintf("%s/%d", ip, bits)}
	}

	// map the list of temporary security group CIDRs bundled with config to
	// types expected by EC2.
	groupIpRanges := []ec2types.IpRange{}
	for _, cidr := range temporarySGSourceCidrs {
		ipRange := ec2types.IpRange{
			CidrIp: aws.String(cidr),
		}
		groupIpRanges = append(groupIpRanges, ipRange)
	}

	// Set some state data for use in future steps
	state.Put("securityGroupIds", []string{s.createdGroupId})

	if s.SkipSSHRuleCreation {
		return multistep.ActionContinue
	}

	port := s.CommConfig.Port()
	// Authorize access for the provided port within the security group
	groupRules := &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: groupResp.GroupId,
		IpPermissions: []ec2types.IpPermission{
			{
				FromPort:   aws.Int32(int32(port)),
				ToPort:     aws.Int32(int32(port)),
				IpRanges:   groupIpRanges,
				IpProtocol: aws.String("tcp"),
			},
		},
	}

	ui.Say(fmt.Sprintf(
		"Authorizing access to port %d from %v in the temporary security groups...",
		port, temporarySGSourceCidrs),
	)
	_, err = ec2Client.AuthorizeSecurityGroupIngress(ctx, groupRules)
	if err != nil {
		err := fmt.Errorf("Error authorizing temporary security group: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	return multistep.ActionContinue
}

func (s *StepSecurityGroup) Cleanup(state multistep.StateBag) {
	if s.createdGroupId == "" {
		return
	}
	ctx := context.TODO()
	ec2conn := state.Get("ec2v2").(clients.Ec2Client)
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Deleting temporary security group...")

	var err error
	for i := 0; i < 5; i++ {
		_, err = ec2conn.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{GroupId: &s.createdGroupId})
		if err == nil {
			break
		}

		log.Printf("Error deleting security group: %s", err)
		time.Sleep(5 * time.Second)
	}

	if err != nil {
		ui.Error(fmt.Sprintf(
			"Error cleaning up security group. Please delete the group manually:"+
				" err: %s; security group ID: %s", err, s.createdGroupId))
	}
}
