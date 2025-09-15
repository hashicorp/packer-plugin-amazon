// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/hashicorp/packer-plugin-sdk/uuid"
)

type StepSecurityGroup struct {
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
	ec2conn := state.Get("ec2").(*ec2.EC2)
	ui := state.Get("ui").(packersdk.Ui)
	vpcId := state.Get("vpc_id").(string)

	if len(s.SecurityGroupIds) > 0 {
		_, err := ec2conn.DescribeSecurityGroups(
			&ec2.DescribeSecurityGroupsInput{
				GroupIds: aws.StringSlice(s.SecurityGroupIds),
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

		log.Printf("Using SecurityGroup Filters %v", params)

		sgResp, err := ec2conn.DescribeSecurityGroups(params)
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

		ui.Message(fmt.Sprintf("Found Security Group(s): %s", strings.Join(securityGroupIds, ", ")))
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
		ec2Tags, err := TagMap(s.Tags).EC2Tags(s.Ctx, *ec2conn.Config.Region, state)
		if err != nil {
			err := fmt.Errorf("Error tagging security group: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		group.TagSpecifications = ec2Tags.TagSpecifications(ec2.ResourceTypeSecurityGroup)
	}

	group.VpcId = &vpcId

	groupResp, err := ec2conn.CreateSecurityGroup(group)
	if err != nil {
		ui.Error(err.Error())
		state.Put("error", err)
		return multistep.ActionHalt
	}

	// Set the group ID so we can delete it later
	s.createdGroupId = *groupResp.GroupId

	// Wait for the security group become available for authorizing
	log.Printf("[DEBUG] Waiting for temporary security group: %s", s.createdGroupId)
	err = waitUntilSecurityGroupExists(ec2conn,
		&ec2.DescribeSecurityGroupsInput{
			GroupIds: []*string{aws.String(s.createdGroupId)},
		},
	)
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
	groupIpRanges := []*ec2.IpRange{}
	groupIpv6Ranges := []*ec2.Ipv6Range{}
	for _, cidr := range temporarySGSourceCidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}
		if ipNet.IP.To4() != nil {
			// IPv4 CIDR
			groupIpRanges = append(groupIpRanges, &ec2.IpRange{
				CidrIp: aws.String(cidr),
			})
		} else {
			// IPv6 CIDR
			groupIpv6Ranges = append(groupIpv6Ranges, &ec2.Ipv6Range{
				CidrIpv6: aws.String(cidr),
			})
		}

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
		IpPermissions: []*ec2.IpPermission{
			{
				FromPort:   aws.Int64(int64(port)),
				ToPort:     aws.Int64(int64(port)),
				Ipv6Ranges: groupIpv6Ranges,
				IpRanges:   groupIpRanges,
				IpProtocol: aws.String("tcp"),
			},
		},
	}

	ui.Say(fmt.Sprintf(
		"Authorizing access to port %d from %v in the temporary security groups...",
		port, temporarySGSourceCidrs),
	)
	_, err = ec2conn.AuthorizeSecurityGroupIngress(groupRules)
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

	ec2conn := state.Get("ec2").(*ec2.EC2)
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Deleting temporary security group...")

	var err error
	for i := 0; i < 5; i++ {
		_, err = ec2conn.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{GroupId: &s.createdGroupId})
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

func waitUntilSecurityGroupExists(c *ec2.EC2, input *ec2.DescribeSecurityGroupsInput) error {
	ctx := aws.BackgroundContext()
	w := request.Waiter{
		Name:        "DescribeSecurityGroups",
		MaxAttempts: 40,
		Delay:       request.ConstantWaiterDelay(5 * time.Second),
		Acceptors: []request.WaiterAcceptor{
			{
				State:    request.SuccessWaiterState,
				Matcher:  request.PathWaiterMatch,
				Argument: "length(SecurityGroups[]) > `0`",
				Expected: true,
			},
			{
				State:    request.RetryWaiterState,
				Matcher:  request.ErrorWaiterMatch,
				Argument: "",
				Expected: "InvalidGroup.NotFound",
			},
			{
				State:    request.RetryWaiterState,
				Matcher:  request.ErrorWaiterMatch,
				Argument: "",
				Expected: "InvalidSecurityGroupID.NotFound",
			},
		},
		Logger: c.Config.Logger,
		NewRequest: func(opts []request.Option) (*request.Request, error) {
			var inCpy *ec2.DescribeSecurityGroupsInput
			if input != nil {
				tmp := *input
				inCpy = &tmp
			}
			req, _ := c.DescribeSecurityGroupsRequest(inCpy)
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}
	return w.WaitWithContext(ctx)
}
