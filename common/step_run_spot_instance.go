// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/awserrors"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/random"
	"github.com/hashicorp/packer-plugin-sdk/retry"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

type EC2BlockDeviceMappingsBuilder interface {
	BuildEC2BlockDeviceMappings() []ec2types.BlockDeviceMapping
}

type StepRunSpotInstance struct {
	PollingConfig                     *AWSPollingConfig
	AssociatePublicIpAddress          config.Trilean
	LaunchMappings                    EC2BlockDeviceMappingsBuilder
	BlockDurationMinutes              int64
	Debug                             bool
	Comm                              *communicator.Config
	EbsOptimized                      bool
	ExpectedRootDevice                string
	FleetTags                         map[string]string
	HttpEndpoint                      string
	HttpTokens                        string
	HttpPutResponseHopLimit           int32
	InstanceMetadataTags              string
	InstanceInitiatedShutdownBehavior string
	InstanceType                      string
	Region                            string
	SourceAMI                         string
	SpotAllocationStrategy            string
	SpotPrice                         string
	SpotTags                          map[string]string
	SpotInstanceTypes                 []string
	Tags                              map[string]string
	VolumeTags                        map[string]string
	UserData                          string
	UserDataFile                      string
	Ctx                               interpolate.Context
	NoEphemeral                       bool
	IsBurstableInstanceType           bool
	EnableUnlimitedCredits            bool

	instanceId string
}

// The EbsBlockDevice and LaunchTemplateEbsBlockDeviceRequest structs are
// nearly identical except for the struct's name and one extra field in
// EbsBlockDeviceResuest, which unfortunately means you can't just cast one
// into the other. THANKS AMAZON.
func castBlockDeviceToRequest(bd *ec2types.EbsBlockDevice) *ec2types.LaunchTemplateEbsBlockDeviceRequest {
	out := &ec2types.LaunchTemplateEbsBlockDeviceRequest{
		DeleteOnTermination: bd.DeleteOnTermination,
		Encrypted:           bd.Encrypted,
		Iops:                bd.Iops,
		KmsKeyId:            bd.KmsKeyId,
		SnapshotId:          bd.SnapshotId,
		Throughput:          bd.Throughput,
		VolumeSize:          bd.VolumeSize,
		VolumeType:          bd.VolumeType,
	}
	return out
}

func (s *StepRunSpotInstance) CreateTemplateData(userData *string, az string,
	state multistep.StateBag, marketOptions *ec2types.LaunchTemplateInstanceMarketOptionsRequest) *ec2types.
	RequestLaunchTemplateData {
	blockDeviceMappings := s.LaunchMappings.BuildEC2BlockDeviceMappings()
	// Convert the BlockDeviceMapping into a
	// LaunchTemplateBlockDeviceMappingRequest. These structs are identical,
	// except for the EBS field -- on one, that field contains a
	// LaunchTemplateEbsBlockDeviceRequest, and on the other, it contains an
	// EbsBlockDevice.
	var launchMappingRequests []ec2types.LaunchTemplateBlockDeviceMappingRequest
	for _, mapping := range blockDeviceMappings {
		launchRequest := ec2types.LaunchTemplateBlockDeviceMappingRequest{
			DeviceName:  mapping.DeviceName,
			Ebs:         castBlockDeviceToRequest(mapping.Ebs),
			VirtualName: mapping.VirtualName,
		}
		launchMappingRequests = append(launchMappingRequests, launchRequest)
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
			launchRequest := ec2types.LaunchTemplateBlockDeviceMappingRequest{
				DeviceName: aws.String("xvdc" + string(letter)),
				NoDevice:   aws.String(""),
			}
			launchMappingRequests = append(launchMappingRequests, launchRequest)
		}

	}

	ui := state.Get("ui").(packersdk.Ui)

	iamInstanceProfile := aws.String(state.Get("iamInstanceProfile").(string))

	// Create a launch template.
	templateData := ec2types.RequestLaunchTemplateData{
		BlockDeviceMappings:   launchMappingRequests,
		DisableApiTermination: aws.Bool(false),
		EbsOptimized:          &s.EbsOptimized,
		IamInstanceProfile:    &ec2types.LaunchTemplateIamInstanceProfileSpecificationRequest{Name: iamInstanceProfile},
		ImageId:               &s.SourceAMI,
		InstanceMarketOptions: marketOptions,
		Placement: &ec2types.LaunchTemplatePlacementRequest{
			AvailabilityZone: &az,
		},
		UserData: userData,
	}
	// Create a network interface
	securityGroupIds := state.Get("securityGroupIds").([]string)
	subnetId := state.Get("subnet_id").(string)

	if subnetId != "" {
		// Set up a full network interface
		networkInterface := ec2types.LaunchTemplateInstanceNetworkInterfaceSpecificationRequest{
			Groups:              securityGroupIds,
			DeleteOnTermination: aws.Bool(true),
			DeviceIndex:         aws.Int32(0),
			SubnetId:            aws.String(subnetId),
		}
		if s.AssociatePublicIpAddress != config.TriUnset {
			ui.Say(fmt.Sprintf("changing public IP address config to %t for instance on subnet %q",
				*s.AssociatePublicIpAddress.ToBoolPointer(),
				subnetId))
			networkInterface.AssociatePublicIpAddress = s.AssociatePublicIpAddress.ToBoolPointer()
		}
		templateData.NetworkInterfaces = []ec2types.LaunchTemplateInstanceNetworkInterfaceSpecificationRequest{networkInterface}
	} else {
		templateData.SecurityGroupIds = securityGroupIds

	}

	if s.IsBurstableInstanceType {
		templateData.CreditSpecification = &ec2types.CreditSpecificationRequest{CpuCredits: aws.String(CPUCreditsStandard)}
	}

	if s.EnableUnlimitedCredits {
		templateData.CreditSpecification = &ec2types.CreditSpecificationRequest{CpuCredits: aws.String(
			CPUCreditsUnlimited)}
	}

	if s.HttpEndpoint == "enabled" {
		templateData.MetadataOptions = &ec2types.LaunchTemplateInstanceMetadataOptionsRequest{
			HttpEndpoint:            ec2types.LaunchTemplateInstanceMetadataEndpointState(s.HttpEndpoint),
			HttpTokens:              ec2types.LaunchTemplateHttpTokensState(s.HttpTokens),
			HttpPutResponseHopLimit: &s.HttpPutResponseHopLimit,
		}
	}

	if s.InstanceMetadataTags == "enabled" {
		templateData.MetadataOptions.InstanceMetadataTags = ec2types.LaunchTemplateInstanceMetadataTagsState(
			s.InstanceMetadataTags)
	}

	// If instance type is not set, we'll just pick the lowest priced instance
	// available.
	if s.InstanceType != "" {
		templateData.InstanceType = ec2types.InstanceType(s.InstanceType)
	}

	if s.Comm.SSHKeyPairName != "" {
		templateData.KeyName = &s.Comm.SSHKeyPairName
	}

	return &templateData
}

func (s *StepRunSpotInstance) LoadUserData() (string, error) {
	userData := s.UserData
	if s.UserDataFile != "" {
		contents, err := os.ReadFile(s.UserDataFile)
		if err != nil {
			return "", fmt.Errorf("Problem reading user data file: %s", err)
		}

		userData = string(contents)
	}

	// Test if it is encoded already, and if not, encode it
	if _, err := base64.StdEncoding.DecodeString(userData); err != nil {
		log.Printf("[DEBUG] base64 encoding user data...")
		userData = base64.StdEncoding.EncodeToString([]byte(userData))
	}
	return userData, nil
}

func (s *StepRunSpotInstance) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ec2Client := state.Get("ec2v2").(clients.Ec2Client)
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Launching a spot AWS instance...")

	// Get and validate the source AMI
	image, ok := state.Get("source_image").(*ec2types.Image)
	if !ok {
		state.Put("error", fmt.Errorf("source_image type assertion failed"))
		return multistep.ActionHalt
	}
	s.SourceAMI = *image.ImageId

	if s.ExpectedRootDevice != "" && string(image.RootDeviceType) != s.ExpectedRootDevice {
		state.Put("error", fmt.Errorf(
			"The provided source AMI has an invalid root device type.\n"+
				"Expected '%s', got '%s'.",
			s.ExpectedRootDevice, string(image.RootDeviceType)))
		return multistep.ActionHalt
	}

	azConfig := ""
	if azRaw, ok := state.GetOk("availability_zone"); ok {
		azConfig = azRaw.(string)
	}
	az := azConfig

	var instanceId string

	ui.Say("Interpolating tags for spot instance...")

	// Convert tags from the tag map provided by the user into *ec2.Tag s
	ec2Tags, err := TagMap(s.Tags).EC2Tags(s.Ctx, s.Region, state)
	if err != nil {
		err := fmt.Errorf("Error generating tags for source instance: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	// This prints the tags to the ui; it doesn't actually add them to the
	// instance yet
	ec2Tags.Report(ui)

	volumeTags, err := TagMap(s.VolumeTags).EC2Tags(s.Ctx, s.Region, state)
	if err != nil {
		err := fmt.Errorf("Error generating volume tags: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	volumeTags.Report(ui)

	spotOptions := ec2types.LaunchTemplateSpotMarketOptionsRequest{}
	// The default is to set the maximum price to the OnDemand price.
	if s.SpotPrice != "auto" {
		spotOptions.MaxPrice = &s.SpotPrice
	}
	// Block Duration Minutes is not supported in aws sdk go v2
	/*
		if s.BlockDurationMinutes != 0 {
			spotOptions.BlockDurationMinutes = &s.BlockDurationMinutes
		}
	*/
	marketOptions := &ec2types.LaunchTemplateInstanceMarketOptionsRequest{
		SpotOptions: &spotOptions,
	}
	marketOptions.MarketType = ec2types.MarketTypeSpot

	spotTags, err := TagMap(s.SpotTags).EC2Tags(s.Ctx, s.Region, state)
	if err != nil {
		err := fmt.Errorf("Error generating tags for spot request: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// Create a launch template for the instance
	ui.Say("Loading User Data File...")

	// Generate a random name to avoid conflicting with other
	// instances of packer running in this AWS account
	launchTemplateName := fmt.Sprintf(
		"packer-fleet-launch-template-%s",
		random.AlphaNum(7))
	state.Put("launchTemplateName", launchTemplateName) // For the cleanup step

	userData, err := s.LoadUserData()
	if err != nil {
		state.Put("error", err)
		return multistep.ActionHalt
	}
	ui.Say("Creating Spot Fleet launch template...")
	templateData := s.CreateTemplateData(&userData, az, state, marketOptions)
	launchTemplate := &ec2.CreateLaunchTemplateInput{
		LaunchTemplateData: templateData,
		LaunchTemplateName: aws.String(launchTemplateName),
		VersionDescription: aws.String("template generated by packer for launching spot instances"),
	}
	if len(spotTags) > 0 {
		launchTemplate.TagSpecifications = append(
			launchTemplate.TagSpecifications,
			ec2types.TagSpecification{
				ResourceType: "launch-template",
				Tags:         spotTags,
			},
		)
	}

	if len(ec2Tags) > 0 {
		launchTemplate.LaunchTemplateData.TagSpecifications = append(
			launchTemplate.LaunchTemplateData.TagSpecifications,
			ec2types.LaunchTemplateTagSpecificationRequest{
				ResourceType: "instance",
				Tags:         ec2Tags,
			},
		)

		launchTemplate.LaunchTemplateData.TagSpecifications = append(
			launchTemplate.LaunchTemplateData.TagSpecifications,
			ec2types.LaunchTemplateTagSpecificationRequest{
				ResourceType: "network-interface",
				Tags:         ec2Tags,
			},
		)
	}

	if len(volumeTags) > 0 {
		launchTemplate.LaunchTemplateData.TagSpecifications = append(
			launchTemplate.LaunchTemplateData.TagSpecifications,
			ec2types.LaunchTemplateTagSpecificationRequest{
				ResourceType: "volume",
				Tags:         volumeTags,
			},
		)
	}

	// Tell EC2 to create the template
	createLaunchTemplateOutput, err := ec2Client.CreateLaunchTemplate(ctx, launchTemplate)
	if err != nil {
		err := fmt.Errorf("Error creating launch template for spot instance: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	launchTemplateId := createLaunchTemplateOutput.LaunchTemplate.LaunchTemplateId
	ui.Say(fmt.Sprintf("Created Spot Fleet launch template: %s", *launchTemplateId))

	// Add overrides for each user-provided instance type
	var overrides []ec2types.FleetLaunchTemplateOverridesRequest
	for _, instanceType := range s.SpotInstanceTypes {
		override := ec2types.FleetLaunchTemplateOverridesRequest{
			InstanceType: ec2types.InstanceType(instanceType),
		}
		overrides = append(overrides, override)
	}

	createFleetInput := &ec2.CreateFleetInput{
		LaunchTemplateConfigs: []ec2types.FleetLaunchTemplateConfigRequest{
			{
				LaunchTemplateSpecification: &ec2types.FleetLaunchTemplateSpecificationRequest{
					LaunchTemplateName: aws.String(launchTemplateName),
					Version:            aws.String("1"),
				},
				Overrides: overrides,
			},
		},
		ReplaceUnhealthyInstances: aws.Bool(false),
		TargetCapacitySpecification: &ec2types.TargetCapacitySpecificationRequest{
			TotalTargetCapacity:       aws.Int32(1),
			DefaultTargetCapacityType: "spot",
		},
		Type: "instant",
	}

	if s.SpotAllocationStrategy != "" {
		createFleetInput.SpotOptions = &ec2types.SpotOptionsRequest{
			AllocationStrategy: ec2types.SpotAllocationStrategy(s.SpotAllocationStrategy),
		}
	}

	fleetTags, err := TagMap(s.FleetTags).EC2Tags(s.Ctx, s.Region, state)
	if err != nil {
		err := fmt.Errorf("Error generating fleet tags: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	fleetTags.Report(ui)

	if len(fleetTags) > 0 {
		createFleetInput.TagSpecifications = append(
			createFleetInput.TagSpecifications,
			ec2types.TagSpecification{
				ResourceType: "fleet",
				Tags:         fleetTags,
			},
		)
	}
	var createOutput *ec2.CreateFleetOutput
	err = retry.Config{
		Tries: 11,
		ShouldRetry: func(err error) bool {
			if strings.Contains(err.Error(), "Invalid IAM Instance Profile name") {
				// eventual consistency of the profile. PutRolePolicy &
				// AddRoleToInstanceProfile are eventually consistent and once
				// we can wait on those operations, this can be removed.
				return true
			}
			if err.Error() == "InsufficientInstanceCapacity" {
				return true
			}
			return false
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 500 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		createOutput, err = ec2Client.CreateFleet(ctx, createFleetInput)
		if err == nil && createOutput.Errors != nil {
			var sb strings.Builder
			for _, fleetErr := range createOutput.Errors {
				sb.WriteString(fmt.Sprintf("Error code: %v, Error Message: %v\n", *fleetErr.ErrorCode,
					*fleetErr.ErrorMessage))
			}
			err = fmt.Errorf("errors: %v", sb.String())
		}
		// We can end up with errors because one of the allowed availability
		// zones doesn't have one of the allowed instance types; as long as
		// an instance is launched, these errors aren't important.
		if len(createOutput.Instances) > 0 {
			if err != nil {
				log.Printf("create request failed for some instances %v", err.Error())
			}
			return nil
		}
		if err != nil {
			log.Printf("create request failed %v", err)
		}
		// We can end up unavailable Spot capacity, we keep retrying
		for _, err := range createOutput.Errors {
			if err.ErrorCode != nil && *err.ErrorCode == "InsufficientInstanceCapacity" {
				return fmt.Errorf("%s", *err.ErrorCode)
			}
		}
		return err
	})

	if err != nil {
		if createOutput.FleetId != nil {
			err = fmt.Errorf("Error waiting for fleet request (%s): %s", *createOutput.FleetId, err)
		}
		if len(createOutput.Errors) > 0 {
			errString := fmt.Sprintf("Error waiting for fleet request (%s) to become ready:", *createOutput.FleetId)
			for _, outErr := range createOutput.Errors {
				errString = errString + aws.ToString(outErr.ErrorMessage)
			}
			err = errors.New(errString)
		}
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	instanceId = createOutput.Instances[0].InstanceIds[0]
	// Set the instance ID so that the cleanup works properly
	s.instanceId = instanceId
	if err := waitForInstanceReadiness(ctx, instanceId, ec2Client, ui, state, s.PollingConfig.WaitUntilInstanceRunning); err != nil {
		return multistep.ActionHalt
	}

	// Get information about the created instance
	var describeOutput *ec2.DescribeInstancesOutput
	err = retry.Config{
		Tries:      11,
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		describeOutput, err = ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: []string{instanceId},
		})
		if len(describeOutput.Reservations) > 0 && len(describeOutput.Reservations[0].Instances) > 0 {
			if len(s.LaunchMappings.BuildEC2BlockDeviceMappings()) > 0 && len(describeOutput.Reservations[0].Instances[0].BlockDeviceMappings) == 0 {
				return fmt.Errorf("Instance has no block devices")
			}
		}
		return err
	})
	if err != nil || len(describeOutput.Reservations) == 0 || len(describeOutput.Reservations[0].Instances) == 0 {
		err := fmt.Errorf("Error finding source instance.")
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	instance := describeOutput.Reservations[0].Instances[0]

	// Tag the spot instance request (not the eventual spot instance)
	if len(spotTags) > 0 && len(s.SpotTags) > 0 {
		spotTags.Report(ui)
		// Use the instance ID to find out the SIR, so that we can tag the spot
		// request associated with this instance.
		sir := describeOutput.Reservations[0].Instances[0].SpotInstanceRequestId

		// Apply tags to the spot request.
		err = retry.Config{
			Tries:       11,
			ShouldRetry: func(error) bool { return false },
			RetryDelay:  (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
		}.Run(ctx, func(ctx context.Context) error {
			_, err := ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
				Tags:      spotTags,
				Resources: []string{*sir},
			})
			return err
		})
		if err != nil {
			err := fmt.Errorf("Error tagging spot request: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	// Retry creating tags for about 2.5 minutes
	err = retry.Config{Tries: 11, ShouldRetry: func(err error) bool {
		return awserrors.Matches(err, "InvalidInstanceID.NotFound", "")
	},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		if len(ec2Tags) == 0 {
			return nil
		}

		_, err := ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
			Tags:      ec2Tags,
			Resources: []string{*instance.InstanceId},
		})
		return err
	})

	if len(ec2Tags) > 0 && err != nil {
		err := fmt.Errorf("Error tagging source instance: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	volumeIds := make([]string, 0)
	for _, v := range instance.BlockDeviceMappings {
		if ebs := v.Ebs; ebs != nil {
			volumeIds = append(volumeIds, *ebs.VolumeId)
		}
	}

	if len(volumeIds) > 0 && len(s.VolumeTags) > 0 {
		ui.Say("Adding tags to source EBS Volumes")
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

	return multistep.ActionContinue
}

func (s *StepRunSpotInstance) Cleanup(state multistep.StateBag) {
	ec2Client := state.Get("ec2v2").(clients.Ec2Client)
	ui := state.Get("ui").(packersdk.Ui)
	launchTemplateName := state.Get("launchTemplateName").(string)

	// Terminate the source instance if it exists
	if s.instanceId != "" {
		ui.Say("Terminating the source AWS instance...")
		if _, err := ec2Client.TerminateInstances(context.Background(), &ec2.TerminateInstancesInput{InstanceIds: []string{s.
			instanceId}}); err != nil {
			ui.Error(fmt.Sprintf("Error terminating instance, may still be around: %s", err))
			return
		}

		if err := s.PollingConfig.WaitUntilInstanceTerminated(context.Background(), ec2Client, s.instanceId); err != nil {
			ui.Error(err.Error())
		}
	}

	// Delete the launch template used to create the spot fleet
	deleteInput := &ec2.DeleteLaunchTemplateInput{
		LaunchTemplateName: aws.String(launchTemplateName),
	}
	if _, err := ec2Client.DeleteLaunchTemplate(context.Background(), deleteInput); err != nil {
		ui.Error(err.Error())
	}
}
