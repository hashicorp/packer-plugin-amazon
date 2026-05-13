// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	confighelper "github.com/hashicorp/packer-plugin-sdk/template/config"
)

type mockEC2ClientStepNetworkTests struct {
	clients.Ec2Client

	describeInstanceTypeOfferings func(ctx context.Context, in *ec2.DescribeInstanceTypeOfferingsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error)
	describeVpcs                  func(ctx context.Context, in *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	describeSubnets               func(ctx context.Context, in *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
}

func (m *mockEC2ClientStepNetworkTests) DescribeInstanceTypeOfferings(ctx context.Context, in *ec2.DescribeInstanceTypeOfferingsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
	if m.describeInstanceTypeOfferings != nil {
		return m.describeInstanceTypeOfferings(ctx, in, optFns...)
	}

	return nil, fmt.Errorf("unimplemented: describeInstanceTypeOfferings")
}

func (m *mockEC2ClientStepNetworkTests) DescribeVpcs(ctx context.Context, in *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	if m.describeVpcs != nil {
		return m.describeVpcs(ctx, in, optFns...)
	}

	return nil, fmt.Errorf("unimplemented: describeVpcs")
}

func (m *mockEC2ClientStepNetworkTests) DescribeSubnets(ctx context.Context, in *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	if m.describeSubnets != nil {
		return m.describeSubnets(ctx, in, optFns...)
	}

	return nil, fmt.Errorf("unimplemented: describeSubnets")
}

func TestStepNetwork_GetFilterAZByMachineType(t *testing.T) {
	testcases := []struct {
		name         string
		describeImpl func(ctx context.Context, in *ec2.DescribeInstanceTypeOfferingsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error)
		machineType  ec2types.InstanceType
		inputAZs     []string
		expectedAZs  []string
		expectError  bool
	}{
		{
			name: "Fail: describe returns an error",
			describeImpl: func(ctx context.Context, in *ec2.DescribeInstanceTypeOfferingsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
				return nil, fmt.Errorf("STOP")
			},
			machineType: "t2.micro",
			inputAZs:    []string{"us-east-1a", "us-east-1b"},
			expectedAZs: nil,
			expectError: true,
		},
		{
			name: "Fail, no AZ match machine type",
			describeImpl: func(ctx context.Context, in *ec2.DescribeInstanceTypeOfferingsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
				return &ec2.DescribeInstanceTypeOfferingsOutput{
					InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
						{
							InstanceType: "t3.mini",
						},
						{
							InstanceType: "t2.mini",
						},
					},
				}, nil
			},
			machineType: "t2.micro",
			inputAZs:    []string{"us-east-1b", "us-east-1c"},
			expectedAZs: nil,
			expectError: true,
		},
		{
			name: "OK, found at least one AZ matching machine type",
			describeImpl: func(ctx context.Context, in *ec2.DescribeInstanceTypeOfferingsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
				return &ec2.DescribeInstanceTypeOfferingsOutput{
					InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
						{
							InstanceType: "t2.micro",
						},
					},
				}, nil
			},
			machineType: "t2.micro",
			inputAZs:    []string{"us-east-1a", "us-east-1b"},
			expectedAZs: []string{"us-east-1a", "us-east-1b"},
			expectError: false,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			conn := &mockEC2ClientStepNetworkTests{}
			conn.describeInstanceTypeOfferings = tt.describeImpl

			retAZ, err := filterAZByMachineType(t.Context(), tt.inputAZs, tt.machineType, conn)

			diff := cmp.Diff(retAZ, tt.expectedAZs)
			if diff != "" {
				t.Errorf("AZ mismatch between computed and expected: %s", diff)
			}

			if (err != nil) != tt.expectError {
				t.Errorf("Error mismatch, got %t, expected %t", err != nil, tt.expectError)
			}

			if err != nil {
				t.Logf("Got an error: %s", err)
			}
		})
	}
}

func TestStepNetwork_FilterSubnetsByAZ(t *testing.T) {
	testcases := []struct {
		name       string
		inSubnets  []ec2types.Subnet
		azs        []string
		outSubnets []ec2types.Subnet
	}{
		{
			name: "No subnet matching",
			inSubnets: []ec2types.Subnet{
				{
					AvailabilityZone: aws.String("us-east-1-c"),
				},
			},
			azs:        []string{"us-east-1a"},
			outSubnets: nil,
		},
		{
			name: "Found subnet matching",
			inSubnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet1"),
					AvailabilityZone: aws.String("us-east-1c"),
				},
				{
					SubnetId:         aws.String("subnet2"),
					AvailabilityZone: aws.String("us-east-1b"),
				},
			},
			azs: []string{"us-east-1c"},
			outSubnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet1"),
					AvailabilityZone: aws.String("us-east-1c"),
				},
			},
		},
		{
			name: "Found multiple subnets matching",
			inSubnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet1"),
					AvailabilityZone: aws.String("us-east-1c"),
				},
				{
					SubnetId:         aws.String("subnet2"),
					AvailabilityZone: aws.String("us-east-1c"),
				},
			},
			azs: []string{"us-east-1c"},
			outSubnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet1"),
					AvailabilityZone: aws.String("us-east-1c"),
				},
				{
					SubnetId:         aws.String("subnet2"),
					AvailabilityZone: aws.String("us-east-1c"),
				},
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			subnets := filterSubnetsByAZ(tt.inSubnets, tt.azs)
			diff := cmp.Diff(subnets, tt.outSubnets)
			if diff != "" {
				t.Errorf("subnet mismatch between computed and expected: %s", diff)
			}
		})
	}
}

func TestStepNetwork_WithPublicIPSetAndNoVPCOrSubnet(t *testing.T) {
	mockConn := &mockEC2ClientStepNetworkTests{
		describeVpcs: func(ctx context.Context, dvi *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
			ok := false
			for _, filter := range dvi.Filters {
				if *filter.Name == "is-default" {
					ok = true
					break
				}
			}

			if !ok {
				return nil, fmt.Errorf("expected to filter on default VPC = true, did not find that filter")
			}

			return &ec2.DescribeVpcsOutput{
				Vpcs: []ec2types.Vpc{
					{
						VpcId: aws.String("default-vpc"),
					},
				},
			}, nil
		},
		describeSubnets: func(ctx context.Context, dsi *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
			if dsi.SubnetIds != nil {
				sub := dsi.SubnetIds[0]
				if sub != "subnet1" {
					return nil, fmt.Errorf("expected selected subnet to be us-east-1a, but was %s", sub)
				}

				return &ec2.DescribeSubnetsOutput{
					Subnets: []ec2types.Subnet{
						{
							SubnetId:         aws.String("subnet1"),
							AvailabilityZone: aws.String("us-east-1a"),
							VpcId:            aws.String("default-vpc"),
						},
					},
				}, nil
			}

			vpcFilterFound := false
			for _, filter := range dsi.Filters {
				if *filter.Name != "vpc-id" {
					continue
				}
				filterVal := filter.Values[0]
				if filterVal != "default-vpc" {
					return nil, fmt.Errorf("expected vpc-id filter to be %q, got %q", "default-vpc", filterVal)
				}

				vpcFilterFound = true
			}

			if !vpcFilterFound {
				return nil, fmt.Errorf("expected to find vpc-id filter, but did not find it")
			}

			return &ec2.DescribeSubnetsOutput{
				Subnets: []ec2types.Subnet{
					{
						AvailabilityZone:        aws.String("us-east-1a"),
						SubnetId:                aws.String("subnet1"),
						AvailableIpAddressCount: aws.Int32(256),
					},
					{
						AvailabilityZone:        aws.String("us-east-1b"),
						SubnetId:                aws.String("subnet2"),
						AvailableIpAddressCount: aws.Int32(512),
					},
				},
			}, nil
		},
		describeInstanceTypeOfferings: func(ctx context.Context, in *ec2.DescribeInstanceTypeOfferingsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
			if in.LocationType != ec2types.LocationTypeAvailabilityZone {
				return nil, fmt.Errorf("called DescribeInstanceTypeOfferings with LocationType = %q, expected availability_zone", in.LocationType)
			}

			var machines []ec2types.InstanceTypeOffering

			foundLocation := false
			for _, filter := range in.Filters {
				if *filter.Name != "location" {
					continue
				}
				foundLocation = true

				filterVal := filter.Values[0]
				switch filterVal {
				case "us-east-1a":
					machines = []ec2types.InstanceTypeOffering{
						{
							InstanceType: "t2.mini",
						},
						{
							InstanceType: "t3.large",
						},
					}
				case "us-east-1b":
					machines = []ec2types.InstanceTypeOffering{
						{
							InstanceType: "t2.mini",
						},
						{
							InstanceType: "t2.micro",
						},
					}
				default:
					return nil, fmt.Errorf("error: location %q not expected", filterVal)
				}
			}

			if !foundLocation {
				return nil, fmt.Errorf("couldn't find location in filters")
			}

			return &ec2.DescribeInstanceTypeOfferingsOutput{
				InstanceTypeOfferings: machines,
			}, nil
		},
	}

	stepConfig := &StepNetworkInfo{
		AssociatePublicIpAddress: confighelper.TriTrue,
		RequestedMachineType:     "t3.large",
	}

	state := &multistep.BasicStateBag{}
	state.Put("ec2", mockConn)
	state.Put("ui", &packersdk.MockUi{})

	actRet := stepConfig.Run(context.Background(), state)
	if actRet == multistep.ActionHalt {
		t.Fatalf("running the step failed: %s", state.Get("error").(error))
	}

	vpcid, ok := state.GetOk("vpc_id")
	if !ok || vpcid != "default-vpc" {
		t.Errorf("error: vpc should be 'default-vpc', but is %q", vpcid)
	}
	t.Logf("set vpc is %q", vpcid)

	subnetid, ok := state.GetOk("subnet_id")
	if !ok || subnetid != "subnet1" {
		t.Errorf("error: subnet should be 'subnet1', but is %q", subnetid)
	}
	t.Logf("set subnet is %q", subnetid)

	az, ok := state.GetOk("availability_zone")
	if !ok || az != "us-east-1a" {
		t.Errorf("error: availability_zone should be 'us-east-1a', but is %q", az)
	}
	t.Logf("set AZ is %q", az)
}

func TestStepNetwork_GetDefaultVPCFailDueToPermissions(t *testing.T) {
	mockConn := &mockEC2ClientStepNetworkTests{
		describeVpcs: func(ctx context.Context, dvi *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
			return nil, fmt.Errorf("Insufficient permissions: missing ec2:DescribeVpcs")
		},
	}

	stepConfig := &StepNetworkInfo{
		AssociatePublicIpAddress: confighelper.TriTrue,
		RequestedMachineType:     "t3.large",
	}

	ui := &packersdk.MockUi{}

	state := &multistep.BasicStateBag{}
	state.Put("ec2", mockConn)
	state.Put("ui", ui)

	actRet := stepConfig.Run(context.Background(), state)
	if actRet == multistep.ActionHalt {
		t.Fatalf("running the step failed: %s", state.Get("error").(error))
	}

	vpcid, ok := state.GetOk("vpc_id")
	if !ok || vpcid != "" {
		t.Errorf("error: vpc should be empty, but is %q", vpcid)
	}

	subnetid, ok := state.GetOk("subnet_id")
	if !ok || subnetid != "" {
		t.Errorf("error: subnet should be empty, but is %q", subnetid)
	}

	az, ok := state.GetOk("availability_zone")
	if !ok || az != "" {
		t.Errorf("error: availability_zone should be empty, but is %q", az)
	}

	var foundMsg bool

	for _, msg := range ui.SayMessages {
		if msg.Message == "associate_public_ip_address is set without a subnet_id." {
			t.Log("found warning on associate_public_ip_address")
			foundMsg = true
		}
	}

	if !foundMsg {
		t.Errorf("failed to find a message that states that associate_public_ip_address will be ignored.")
	}
}

func TestStepNetwork_SetVPCAndSubnetWithoutAssociatePublicIP(t *testing.T) {
	mockConn := &mockEC2ClientStepNetworkTests{}

	stepConfig := &StepNetworkInfo{
		AssociatePublicIpAddress: confighelper.TriUnset,
		RequestedMachineType:     "t3.large",
		VpcId:                    "default-vpc",
		SubnetId:                 "subnet1",
		AvailabilityZone:         "us-east-1a",
	}

	ui := &packersdk.MockUi{}

	state := &multistep.BasicStateBag{}
	state.Put("ec2", mockConn)
	state.Put("ui", ui)

	actRet := stepConfig.Run(context.Background(), state)
	if actRet == multistep.ActionHalt {
		t.Fatalf("running the step failed: %s", state.Get("error").(error))
	}

	vpcid, ok := state.GetOk("vpc_id")
	if !ok || vpcid != "default-vpc" {
		t.Errorf("error: vpc should be 'default-vpc', but is %q", vpcid)
	}

	subnetid, ok := state.GetOk("subnet_id")
	if !ok || subnetid != "subnet1" {
		t.Errorf("error: subnet should be 'subnet_id', but is %q", subnetid)
	}

	az, ok := state.GetOk("availability_zone")
	if !ok || az != "us-east-1a" {
		t.Errorf("error: availability_zone should be 'us-east-1a', but is %q", az)
	}

	var foundDefaultVPCMsg bool
	for _, msg := range ui.SayMessages {
		if strings.Contains(msg.Message, "Setting public IP address to") {
			foundDefaultVPCMsg = true
		}
	}

	if foundDefaultVPCMsg {
		t.Errorf("Should not have found a message that stated that we need to process public IP address setting")
	}
}

func TestStepNetwork_SetPublicIPAddressWithoutSubnetAndMissingDescribeInstanceTypeOfferings(t *testing.T) {
	mockConn := &mockEC2ClientStepNetworkTests{
		describeVpcs: func(ctx context.Context, dvi *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
			ok := false
			for _, filter := range dvi.Filters {
				if *filter.Name == "is-default" {
					ok = true
					break
				}
			}

			if !ok {
				return nil, fmt.Errorf("expected to filter on default VPC = true, did not find that filter")
			}

			return &ec2.DescribeVpcsOutput{
				Vpcs: []ec2types.Vpc{
					{
						VpcId: aws.String("default-vpc"),
					},
				},
			}, nil
		},
		describeSubnets: func(ctx context.Context, dsi *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
			if dsi.SubnetIds != nil {
				sub := dsi.SubnetIds[0]
				if sub != "subnet1" {
					return nil, fmt.Errorf("expected selected subnet to be us-east-1a, but was %q", sub)
				}

				return &ec2.DescribeSubnetsOutput{
					Subnets: []ec2types.Subnet{
						{
							SubnetId:         aws.String("subnet1"),
							AvailabilityZone: aws.String("us-east-1a"),
							VpcId:            aws.String("default-vpc"),
						},
					},
				}, nil
			}

			vpcFilterFound := false
			for _, filter := range dsi.Filters {
				if *filter.Name != "vpc-id" {
					continue
				}
				filterVal := filter.Values[0]
				if filterVal != "default-vpc" {
					return nil, fmt.Errorf("expected vpc-id filter to be %q, got %q", "default-vpc", filterVal)
				}

				vpcFilterFound = true
			}

			if !vpcFilterFound {
				return nil, fmt.Errorf("expected to find vpc-id filter, but did not find it")
			}

			return &ec2.DescribeSubnetsOutput{
				Subnets: []ec2types.Subnet{
					{
						AvailabilityZone:        aws.String("us-east-1a"),
						SubnetId:                aws.String("subnet1"),
						AvailableIpAddressCount: aws.Int32(256),
					},
					{
						AvailabilityZone:        aws.String("us-east-1b"),
						SubnetId:                aws.String("subnet2"),
						AvailableIpAddressCount: aws.Int32(512),
					},
				},
			}, nil
		},
		describeInstanceTypeOfferings: func(ctx context.Context, in *ec2.DescribeInstanceTypeOfferingsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
			return nil, fmt.Errorf("Missing permission: ec2:DescribeInstanceTypeOfferings")
		},
	}

	stepConfig := &StepNetworkInfo{
		AssociatePublicIpAddress: confighelper.TriTrue,
		RequestedMachineType:     "t3.large",
	}

	ui := &packersdk.MockUi{}

	state := &multistep.BasicStateBag{}
	state.Put("ec2", mockConn)
	state.Put("ui", ui)

	actRet := stepConfig.Run(context.Background(), state)
	if actRet == multistep.ActionHalt {
		t.Fatalf("running the step failed: %s", state.Get("error").(error))
	}

	vpcid, ok := state.GetOk("vpc_id")
	if !ok || vpcid != "default-vpc" {
		t.Errorf("error: vpc should be 'default-vpc', but is %q", vpcid)
	}
	t.Logf("set vpc is %q", vpcid)

	subnetid, ok := state.GetOk("subnet_id")
	if !ok || subnetid != "subnet2" {
		t.Errorf("error: subnet should be 'subnet2', but is %q", subnetid)
	}
	t.Logf("set subnet is %q", subnetid)

	az, ok := state.GetOk("availability_zone")
	if !ok || az != "us-east-1b" {
		t.Errorf("error: availability_zone should be 'us-east-1a', but is %q", az)
	}
	t.Logf("set AZ is %q", az)
}
