// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"

	"github.com/hashicorp/packer-plugin-amazon/common/awserrors"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/retry"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

type StepRunSourceInstance struct {
	PollingConfig                     *AWSPollingConfig
	AssociatePublicIpAddress          config.Trilean
	LaunchMappings                    EC2BlockDeviceMappingsBuilder
	CapacityReservationPreference     string
	CapacityReservationId             string
	CapacityReservationGroupArn       string
	Comm                              *communicator.Config
	Ctx                               interpolate.Context
	Debug                             bool
	EbsOptimized                      bool
	EnableUnlimitedCredits            bool
	ExpectedRootDevice                string
	HttpEndpoint                      string
	HttpTokens                        string
	HttpPutResponseHopLimit           int32
	InstanceMetadataTags              string
	InstanceInitiatedShutdownBehavior string
	InstanceType                      string
	IsRestricted                      bool
	SourceAMI                         string
	Tags                              map[string]string
	LicenseSpecifications             []LicenseSpecification
	HostResourceGroupArn              string
	HostId                            string
	Tenancy                           string
	UserData                          string
	UserDataFile                      string
	VolumeTags                        map[string]string
	NoEphemeral                       bool
	EnableNitroEnclave                bool
	IsBurstableInstanceType           bool

	instanceId string
}

func (s *StepRunSourceInstance) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ec2Client := state.Get("ec2v2").(clients.Ec2Client)
	awsConfig := state.Get("aws_config").(*aws.Config)

	securityGroupIds := state.Get("securityGroupIds").([]string)
	iamInstanceProfile := aws.String(state.Get("iamInstanceProfile").(string))

	ui := state.Get("ui").(packersdk.Ui)

	userData := s.UserData
	if s.UserDataFile != "" {
		contents, err := os.ReadFile(s.UserDataFile)
		if err != nil {
			state.Put("error", fmt.Errorf("Problem reading user data file: %s", err))
			return multistep.ActionHalt
		}

		userData = string(contents)
	}

	// Test if it is encoded already, and if not, encode it
	if _, err := base64.StdEncoding.DecodeString(userData); err != nil {
		log.Printf("[DEBUG] base64 encoding user data...")
		userData = base64.StdEncoding.EncodeToString([]byte(userData))
	}

	ui.Say("Launching a source AWS instance...")
	image, ok := state.Get("source_image").(*ec2types.Image)
	if !ok {
		state.Put("error", fmt.Errorf("source_image type assertion failed"))
		return multistep.ActionHalt
	}
	s.SourceAMI = *image.ImageId

	if s.ExpectedRootDevice != "" && image.RootDeviceType != ec2types.DeviceType(s.ExpectedRootDevice) {
		state.Put("error", fmt.Errorf(
			"The provided source AMI has an invalid root device type.\n"+
				"Expected '%s', got '%s'.",
			s.ExpectedRootDevice, string(image.RootDeviceType)))
		return multistep.ActionHalt
	}

	var instanceId string

	ec2Tags, err := TagMap(s.Tags).EC2Tags(s.Ctx, awsConfig.Region, state)
	if err != nil {
		err := fmt.Errorf("Error tagging source instance: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	volTags, err := TagMap(s.VolumeTags).EC2Tags(s.Ctx, awsConfig.Region, state)
	if err != nil {
		err := fmt.Errorf("Error tagging volumes: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	enclaveOptions := ec2types.EnclaveOptionsRequest{
		Enabled: &s.EnableNitroEnclave,
	}

	az := state.Get("availability_zone").(string)
	runOpts := &ec2.RunInstancesInput{
		ImageId:             &s.SourceAMI,
		InstanceType:        ec2types.InstanceType(s.InstanceType),
		UserData:            &userData,
		MaxCount:            aws.Int32(1),
		MinCount:            aws.Int32(1),
		IamInstanceProfile:  &ec2types.IamInstanceProfileSpecification{Name: iamInstanceProfile},
		BlockDeviceMappings: s.LaunchMappings.BuildEC2BlockDeviceMappings(),
		Placement:           &ec2types.Placement{AvailabilityZone: &az},
		EbsOptimized:        &s.EbsOptimized,
		EnclaveOptions:      &enclaveOptions,
	}

	if s.NoEphemeral {
		// This is only relevant for windows guests. Ephemeral drives by
		// default are assigned to drive names xvdca-xvdcz.
		// When vms are launched from the AWS console, they're automatically
		// removed from the block devices if the user hasn't said to use them,
		// but the SDK does not perform this cleanup. The following code just
		// manually removes the ephemeral drives from the mapping so that they
		// don't clutter up console views and cause confusion.
		log.Printf("no_ephemeral was set, so creating drives xvdca-xvdcz as empty mappings")
		DefaultEphemeralDeviceLetters := "abcdefghijklmnopqrstuvwxyz"
		for _, letter := range DefaultEphemeralDeviceLetters {
			bd := ec2types.BlockDeviceMapping{
				DeviceName: aws.String("xvdc" + string(letter)),
				NoDevice:   aws.String(""),
			}
			runOpts.BlockDeviceMappings = append(runOpts.BlockDeviceMappings, bd)
		}
	}

	if s.IsBurstableInstanceType {
		runOpts.CreditSpecification = &ec2types.CreditSpecificationRequest{CpuCredits: aws.String(CPUCreditsStandard)}
	}

	if s.EnableUnlimitedCredits {
		runOpts.CreditSpecification = &ec2types.CreditSpecificationRequest{CpuCredits: aws.String(CPUCreditsUnlimited)}
	}

	if s.HttpEndpoint == "enabled" {
		runOpts.MetadataOptions = &ec2types.InstanceMetadataOptionsRequest{
			HttpEndpoint:            ec2types.InstanceMetadataEndpointState(s.HttpEndpoint),
			HttpTokens:              ec2types.HttpTokensState(s.HttpTokens),
			HttpPutResponseHopLimit: &s.HttpPutResponseHopLimit,
		}
	}

	if s.InstanceMetadataTags == "enabled" {
		runOpts.MetadataOptions.InstanceMetadataTags = ec2types.InstanceMetadataTagsState(s.InstanceMetadataTags)
	}

	// Collect tags for tagging on resource creation
	var tagSpecs []ec2types.TagSpecification

	if len(ec2Tags) > 0 {
		runTags := ec2types.TagSpecification{
			ResourceType: "instance",
			Tags:         ec2Tags,
		}

		tagSpecs = append(tagSpecs, runTags)

		networkInterfaceTags := ec2types.TagSpecification{
			ResourceType: "network-interface",
			Tags:         ec2Tags,
		}

		tagSpecs = append(tagSpecs, networkInterfaceTags)
	}

	if len(volTags) > 0 {
		runVolTags := ec2types.TagSpecification{
			ResourceType: "volume",
			Tags:         volTags,
		}

		tagSpecs = append(tagSpecs, runVolTags)
	}

	// If our region supports it, set tag specifications
	if len(tagSpecs) > 0 && !s.IsRestricted {
		runOpts.TagSpecifications = tagSpecs
		ec2Tags.Report(ui)
		volTags.Report(ui)
	}

	if s.Comm.SSHKeyPairName != "" {
		runOpts.KeyName = &s.Comm.SSHKeyPairName
	}

	subnetId := state.Get("subnet_id").(string)

	if subnetId != "" && s.AssociatePublicIpAddress != config.TriUnset {
		ui.Say(fmt.Sprintf("changing public IP address config to %t for instance on subnet %q",
			*s.AssociatePublicIpAddress.ToBoolPointer(),
			subnetId))
		runOpts.NetworkInterfaces = []ec2types.InstanceNetworkInterfaceSpecification{
			{
				DeviceIndex:              aws.Int32(0),
				AssociatePublicIpAddress: s.AssociatePublicIpAddress.ToBoolPointer(),
				SubnetId:                 aws.String(subnetId),
				Groups:                   securityGroupIds,
				DeleteOnTermination:      aws.Bool(true),
			},
		}
	} else {
		runOpts.SubnetId = aws.String(subnetId)
		runOpts.SecurityGroupIds = securityGroupIds
	}

	if s.ExpectedRootDevice == "ebs" {
		runOpts.InstanceInitiatedShutdownBehavior = ec2types.ShutdownBehavior(s.InstanceInitiatedShutdownBehavior)
	}

	if len(s.LicenseSpecifications) > 0 {
		for i := range s.LicenseSpecifications {
			licenseConfigurationArn := s.LicenseSpecifications[i].LicenseConfigurationRequest.LicenseConfigurationArn
			licenseSpecifications := []ec2types.LicenseConfigurationRequest{
				{
					LicenseConfigurationArn: aws.String(licenseConfigurationArn),
				},
			}
			runOpts.LicenseSpecifications = append(runOpts.LicenseSpecifications, licenseSpecifications...)
		}
	}

	if s.CapacityReservationPreference != "" {
		runOpts.CapacityReservationSpecification = &ec2types.CapacityReservationSpecification{
			CapacityReservationPreference: ec2types.CapacityReservationPreference(s.CapacityReservationPreference),
		}
	}

	if s.CapacityReservationId != "" || s.CapacityReservationGroupArn != "" {
		runOpts.CapacityReservationSpecification.CapacityReservationTarget = &ec2types.CapacityReservationTarget{}

		if s.CapacityReservationId != "" {
			runOpts.CapacityReservationSpecification.CapacityReservationTarget.CapacityReservationId = aws.String(s.CapacityReservationId)
		}

		if s.CapacityReservationGroupArn != "" {
			runOpts.CapacityReservationSpecification.CapacityReservationTarget.CapacityReservationResourceGroupArn = aws.String(s.CapacityReservationGroupArn)
		}
	}

	if s.HostResourceGroupArn != "" {
		runOpts.Placement.HostResourceGroupArn = aws.String(s.HostResourceGroupArn)
	}

	if s.HostId != "" {
		runOpts.Placement.HostId = aws.String(s.HostId)
	}

	if s.Tenancy != "" {
		runOpts.Placement.Tenancy = ec2types.Tenancy(s.Tenancy)
	}

	var runResp *ec2.RunInstancesOutput
	err = retry.Config{
		Tries: 11,
		ShouldRetry: func(err error) bool {
			return awserrors.Matches(err, "InvalidParameterValue", "iamInstanceProfile")
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		runResp, err = ec2Client.RunInstances(ctx, runOpts)
		return err
	})

	if awserrors.Matches(err, "VPCIdNotSpecified", "No default VPC for this user") && subnetId == "" {
		err := fmt.Errorf("Error launching source instance: a valid Subnet Id was not specified")
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	if err != nil {
		err := fmt.Errorf("Error launching source instance: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	instanceId = *runResp.Instances[0].InstanceId

	// Set the instance ID so that the cleanup works properly
	s.instanceId = instanceId
	if err := waitForInstanceReadiness(ctx, instanceId, ec2Client, ui, state, s.PollingConfig.WaitUntilInstanceRunning); err != nil {
		return multistep.ActionHalt
	}
	describeInstance := &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceId},
	}

	// there's a race condition that can happen because of AWS's eventual
	// consistency where even though the wait is complete, the describe call
	// will fail. Retry a couple of times to try to mitigate that race.

	var r *ec2.DescribeInstancesOutput
	err = retry.Config{
		Tries: 11,
		ShouldRetry: func(err error) bool {
			return awserrors.Matches(err, "InvalidInstanceID.NotFound", "")
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		r, err = ec2Client.DescribeInstances(ctx, describeInstance)
		return err
	})
	if err != nil || len(r.Reservations) == 0 || len(r.Reservations[0].Instances) == 0 {
		err := fmt.Errorf("Error finding source instance.")
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	instance := r.Reservations[0].Instances[0]

	if s.Debug {
		if instance.PublicDnsName != nil && *instance.PublicDnsName != "" {
			ui.Say(fmt.Sprintf("Public DNS: %s", *instance.PublicDnsName))
		}

		if instance.PublicIpAddress != nil && *instance.PublicIpAddress != "" {
			ui.Say(fmt.Sprintf("Public IP: %s", *instance.PublicIpAddress))
		}

		if instance.PrivateIpAddress != nil && *instance.PrivateIpAddress != "" {
			ui.Say(fmt.Sprintf("Private IP: %s", *instance.PrivateIpAddress))
		}
	}

	state.Put("instance", instance)
	// instance_id is the generic term used so that users can have access to the
	// instance id inside of the provisioners, used in step_provision.
	state.Put("instance_id", instance.InstanceId)

	// If we're in a region that doesn't support tagging on instance creation,
	// do that now.

	if s.IsRestricted {
		ec2Tags.Report(ui)
		// Retry creating tags for about 2.5 minutes
		err = retry.Config{Tries: 11, ShouldRetry: func(error) bool {
			if awserrors.Matches(err, "InvalidInstanceID.NotFound", "") {
				return true
			}
			return false
		},
			RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
		}.Run(ctx, func(ctx context.Context) error {
			if len(ec2Tags) > 0 {
				_, err := ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
					Tags:      ec2Tags,
					Resources: []string{*instance.InstanceId},
				})
				return err
			}
			return nil
		})

		if err != nil {
			err := fmt.Errorf("Error tagging source instance: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		if len(ec2Tags) > 0 {
			for _, networkInterface := range instance.NetworkInterfaces {
				log.Printf("Tagging network interface %s", *networkInterface.NetworkInterfaceId)
				_, err := ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
					Tags:      ec2Tags,
					Resources: []string{*networkInterface.NetworkInterfaceId},
				})
				if err != nil {
					ui.Error(fmt.Sprintf("Error tagging source instance's network interface %q: %s", *networkInterface.NetworkInterfaceId, err))
				}
			}
		}
		// Now tag volumes

		volumeIds := make([]string, 0)
		for _, v := range instance.BlockDeviceMappings {
			if ebs := v.Ebs; ebs != nil {
				volumeIds = append(volumeIds, *ebs.VolumeId)
			}
		}

		if len(volumeIds) > 0 && len(s.VolumeTags) > 0 {
			ui.Say("Adding tags to source EBS Volumes")

			volumeTags, err := TagMap(s.VolumeTags).EC2Tags(s.Ctx, awsConfig.Region, state)
			if err != nil {
				err := fmt.Errorf("Error tagging source EBS Volumes on %s: %s", *instance.InstanceId, err)
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
			volumeTags.Report(ui)

			_, err = ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
				Resources: volumeIds,
				Tags:      volumeTags,
			})

			if err != nil {
				err := fmt.Errorf("Error tagging source EBS Volumes on %s: %s", *instance.InstanceId, err)
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
		}
	}

	return multistep.ActionContinue
}

func waitForInstanceReadiness(
	ctx context.Context,
	instanceId string,
	ec2Client clients.Ec2Client,
	ui packersdk.Ui,
	state multistep.StateBag,
	waitUntilInstanceRunning func(context.Context, clients.Ec2Client, string) error,
) error {
	ui.Say(fmt.Sprintf("Instance ID: %s", instanceId))
	ui.Say(fmt.Sprintf("Waiting for instance (%v) to become ready...", instanceId))

	describeInstance := &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceId},
	}

	if err := waitUntilInstanceRunning(ctx, ec2Client, instanceId); err != nil {
		err := fmt.Errorf("Error waiting for instance (%s) to become ready: %s", instanceId, err)
		state.Put("error", err)
		ui.Error(err.Error())

		// try to get some context from AWS on why was instance
		// transitioned to the unexpected state
		if resp, e := ec2Client.DescribeInstances(ctx, describeInstance); e == nil {
			if len(resp.Reservations) > 0 && len(resp.Reservations[0].Instances) > 0 {
				instance := resp.Reservations[0].Instances[0]
				if instance.StateTransitionReason != nil && instance.StateReason != nil && instance.StateReason.Message != nil {
					ui.Error(fmt.Sprintf("Instance state change details: %s: %s",
						*instance.StateTransitionReason, *instance.StateReason.Message))
				}
			}
		}
		return err
	}
	return nil
}

func (s *StepRunSourceInstance) Cleanup(state multistep.StateBag) {

	ec2Client := state.Get("ec2v2").(clients.Ec2Client)
	ui := state.Get("ui").(packersdk.Ui)
	ctx := context.TODO()
	// Terminate the source instance if it exists
	if s.instanceId != "" {
		ui.Say("Terminating the source AWS instance...")
		if _, err := ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{s.
			instanceId}}); err != nil {
			ui.Error(fmt.Sprintf("Error terminating instance, may still be around: %s", err))
			return
		}

		if err := s.PollingConfig.WaitUntilInstanceTerminated(ctx, ec2Client, s.instanceId); err != nil {
			ui.Error(err.Error())
		}
	}
}
