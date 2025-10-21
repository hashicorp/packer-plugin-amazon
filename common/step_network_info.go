// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

// StepNetworkInfo queries AWS for information about
// VPC's and Subnets that is used throughout the AMI creation process.
//
// Produces (adding them to the state bag):
//
//	vpc_id string - the VPC ID
//	subnet_id string - the Subnet ID
//	availability_zone string - the AZ name
type StepNetworkInfo struct {
	VpcId                    string
	VpcFilter                VpcFilterOptions
	SubnetId                 string
	SubnetFilter             SubnetFilterOptions
	AssociatePublicIpAddress config.Trilean
	AvailabilityZone         string
	SecurityGroupIds         []string
	SecurityGroupFilter      SecurityGroupFilterOptions
	// RequestedMachineType is the machine type of the instance we want to create.
	// This is used for selecting a subnet/AZ which supports the type of instance
	// selected, and not just the most available / random one.
	RequestedMachineType string
}

type subnetsSort []ec2types.Subnet

func (a subnetsSort) Len() int      { return len(a) }
func (a subnetsSort) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a subnetsSort) Less(i, j int) bool {
	return *a[i].AvailableIpAddressCount < *a[j].AvailableIpAddressCount
}

// Returns the most recent AMI out of a slice of images.
func mostFreeSubnet(subnets []ec2types.Subnet) ec2types.Subnet {
	sortedSubnets := subnets
	sort.Sort(subnetsSort(sortedSubnets))
	return sortedSubnets[len(sortedSubnets)-1]
}

func (s *StepNetworkInfo) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ec2Client := state.Get("ec2v2").(clients.Ec2Client)
	ui := state.Get("ui").(packersdk.Ui)

	// Set VpcID if none was specified but filters are defined in the template.
	if s.VpcId == "" && !s.VpcFilter.Empty() {
		params := &ec2.DescribeVpcsInput{}
		vpcFilters, err := buildEc2Filters(s.VpcFilter.Filters)
		if err != nil {
			err := fmt.Errorf("Couldn't parse vpc filters: %s", err)
			log.Printf("[DEBUG] %s", err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}
		params.Filters = vpcFilters
		s.VpcFilter.Filters["state"] = "available"

		log.Printf("Using VPC Filters %s", prettyFilter(params.Filters))

		vpcResp, err := ec2Client.DescribeVpcs(ctx, params)
		if err != nil {
			err := fmt.Errorf("Error querying VPCs: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		if len(vpcResp.Vpcs) != 1 {
			err := fmt.Errorf("Exactly one VPC should match the filter, but %d VPC's was found matching filters: %v", len(vpcResp.Vpcs), params)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		s.VpcId = *vpcResp.Vpcs[0].VpcId
		ui.Say(fmt.Sprintf("Found VPC ID: %s", s.VpcId))
	}

	// Set SubnetID if none was specified but filters are defined in the template.
	if s.SubnetId == "" && !s.SubnetFilter.Empty() {
		params := &ec2.DescribeSubnetsInput{}
		s.SubnetFilter.Filters["state"] = "available"

		if s.VpcId != "" {
			s.SubnetFilter.Filters["vpc-id"] = s.VpcId
		}
		if s.AvailabilityZone != "" {
			s.SubnetFilter.Filters["availabilityZone"] = s.AvailabilityZone
		}
		subnetFilters, err := buildEc2Filters(s.SubnetFilter.Filters)
		if err != nil {
			err := fmt.Errorf("Couldn't parse subnet filters: %s", err)
			log.Printf("[DEBUG] %s", err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}
		params.Filters = subnetFilters
		log.Printf("Using Subnet Filters %s", prettyFilter(params.Filters))

		subnetsResp, err := ec2Client.DescribeSubnets(ctx, params)
		if err != nil {
			err := fmt.Errorf("Error querying Subnets: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		if len(subnetsResp.Subnets) == 0 {
			err := fmt.Errorf("No Subnets was found matching filters: %v", params)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		if len(subnetsResp.Subnets) > 1 && !s.SubnetFilter.Random && !s.SubnetFilter.MostFree {
			err := fmt.Errorf("Your filter matched %d Subnets. Please try a more specific search, or set random or most_free to true.", len(subnetsResp.Subnets))
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		var subnet ec2types.Subnet
		switch {
		case s.SubnetFilter.MostFree:
			subnet = mostFreeSubnet(subnetsResp.Subnets)
		case s.SubnetFilter.Random:
			subnet = subnetsResp.Subnets[rand.Intn(len(subnetsResp.Subnets))]
		default:
			subnet = subnetsResp.Subnets[0]
		}
		s.SubnetId = *subnet.SubnetId
		ui.Say(fmt.Sprintf("Found Subnet ID: %s", s.SubnetId))
	}

	// Set VPC/Subnet if we explicitely enable or disable public IP assignment to the instance
	// and we did not set or get a subnet ID before
	if s.AssociatePublicIpAddress != config.TriUnset && s.SubnetId == "" {
		err := s.GetDefaultVPCAndSubnet(ctx, ui, ec2Client, state)
		if err != nil {
			ui.Say("associate_public_ip_address is set without a subnet_id.")
			ui.Say(fmt.Sprintf("Packer attempted to infer a subnet from default VPC (if unspecified), but failed due to: %s", err))
			ui.Say("The associate_public_ip_address will be ignored for the remainder of the build, and a public IP will only be associated if the VPC chosen enables it by default.")
		}
	}

	// Try to find AZ and VPC Id from Subnet if they are not yet found/given
	if s.SubnetId != "" && (s.AvailabilityZone == "" || s.VpcId == "") {
		log.Printf("[INFO] Finding AZ and VpcId for the given subnet '%s'", s.SubnetId)
		resp, err := ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{SubnetIds: []string{s.SubnetId}})
		if err != nil {
			err := fmt.Errorf("Describing the subnet: %s returned error: %s.", s.SubnetId, err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
		if s.AvailabilityZone == "" {
			s.AvailabilityZone = *resp.Subnets[0].AvailabilityZone
			log.Printf("[INFO] AvailabilityZone found: '%s'", s.AvailabilityZone)
		}
		if s.VpcId == "" {
			s.VpcId = *resp.Subnets[0].VpcId
			log.Printf("[INFO] VpcId found: '%s'", s.VpcId)
		}
	}

	state.Put("vpc_id", s.VpcId)
	state.Put("availability_zone", s.AvailabilityZone)
	state.Put("subnet_id", s.SubnetId)
	return multistep.ActionContinue
}

func (s *StepNetworkInfo) GetDefaultVPCAndSubnet(ctx context.Context, ui packersdk.Ui, ec2Client clients.Ec2Client,
	state multistep.StateBag) error {
	ui.Say(fmt.Sprintf("Setting public IP address to %t on instance without a subnet ID",
		*s.AssociatePublicIpAddress.ToBoolPointer()))

	var vpc = s.VpcId
	if vpc == "" {
		ui.Say("No VPC ID provided, Packer will use the default VPC")
		vpcs, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
			Filters: []ec2types.Filter{
				{
					Name:   aws.String("is-default"),
					Values: []string{"true"},
				},
			},
		})
		if err != nil {
			return fmt.Errorf("Failed to describe VPCs: %s", err)
		}

		if len(vpcs.Vpcs) != 1 {
			return fmt.Errorf("No default VPC found")
		}
		vpc = *vpcs.Vpcs[0].VpcId
	}

	var err error

	ui.Say(fmt.Sprintf("Inferring subnet from the selected VPC %q", vpc))
	params := &ec2.DescribeSubnetsInput{}
	filters := map[string]string{
		"vpc-id": vpc,
		"state":  "available",
	}
	params.Filters, err = buildEc2Filters(filters)
	if err != nil {
		return fmt.Errorf("Failed to prepare subnet filters: %s", err)
	}
	subnetOut, err := ec2Client.DescribeSubnets(ctx, params)
	if err != nil {
		return fmt.Errorf("Failed to describe subnets: %s", err)
	}

	subnets := subnetOut.Subnets

	// Filter by AZ with support for machine type
	azs := getAZFromSubnets(subnets)
	azs, err = filterAZByMachineType(ctx, azs, s.RequestedMachineType, ec2Client)
	if err == nil {
		subnets = filterSubnetsByAZ(subnets, azs)
		if subnets == nil {
			return fmt.Errorf("Failed to get subnets for the filtered AZs")
		}
	} else {
		ui.Say(fmt.Sprintf(
			"Failed to filter subnets/AZ for the requested machine type %q: %s",
			s.RequestedMachineType, err))
		ui.Say("This may result in Packer picking a subnet/AZ that can't host the requested machine type")
		ui.Say("Please check that you have the permissions required to run DescribeInstanceTypeOfferings and try again.")
	}

	subnet := mostFreeSubnet(subnets)

	s.SubnetId = *subnet.SubnetId
	s.VpcId = vpc
	s.AvailabilityZone = *subnet.AvailabilityZone

	ui.Say(fmt.Sprintf("Set subnet as %q", s.SubnetId))

	return nil
}

func getAZFromSubnets(subnets []ec2types.Subnet) []string {
	azs := map[string]struct{}{}
	for _, sub := range subnets {
		azs[*sub.AvailabilityZone] = struct{}{}
	}

	retAZ := make([]string, 0, len(azs))
	for az := range azs {
		retAZ = append(retAZ, az)
	}

	return retAZ
}

func filterAZByMachineType(ctx context.Context, azs []string, machineType string, ec2Client clients.Ec2Client) ([]string,
	error) {
	var retAZ []string

	for _, az := range azs {
		resp, err := ec2Client.DescribeInstanceTypeOfferings(ctx, &ec2.DescribeInstanceTypeOfferingsInput{
			LocationType: "availability-zone",
			Filters: []ec2types.Filter{
				{
					Name:   aws.String("location"),
					Values: []string{az},
				},
				{
					Name:   aws.String("instance-type"),
					Values: []string{machineType},
				},
			},
		})
		if err != nil {
			err = fmt.Errorf("failed to get offerings for AZ %q: %s", az, err)
			return nil, err
		}

		for _, off := range resp.InstanceTypeOfferings {
			if off.InstanceType == ec2types.InstanceType(machineType) {
				retAZ = append(retAZ, az)
				break
			}
		}
	}

	if retAZ == nil {
		return nil, fmt.Errorf("no AZ match the requested machine type %q", machineType)
	}

	return retAZ, nil
}

func filterSubnetsByAZ(subnets []ec2types.Subnet, azs []string) []ec2types.Subnet {
	var retSubs []ec2types.Subnet

outLoop:
	for _, sub := range subnets {
		for _, az := range azs {
			if *sub.AvailabilityZone == az {
				retSubs = append(retSubs, sub)
				continue outLoop
			}
		}
	}

	return retSubs
}

func (s *StepNetworkInfo) Cleanup(multistep.StateBag) {}
