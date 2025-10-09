// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config

// The amazonebs package contains a packersdk.Builder implementation that
// builds AMIs for Amazon EC2.
//
// In general, there are two types of AMIs that can be created: ebs-backed or
// instance-store. This builder _only_ builds ebs-backed images.
package ebs

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/hashicorp/hcl/v2/hcldec"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

// The unique ID for this builder
const BuilderId = "mitchellh.amazonebs"

type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`
	awscommon.AMIConfig    `mapstructure:",squash"`
	awscommon.RunConfig    `mapstructure:",squash"`
	// If true, Packer will not create the AMI. Useful for setting to `true`
	// during a build test stage. Default `false`.
	AMISkipCreateImage bool `mapstructure:"skip_create_ami" required:"false"`

	// If true will not propagate the run tags set on Packer created instance to the AMI created.
	AMISkipRunTags bool `mapstructure:"skip_ami_run_tags" required:"false"`

	// Add one or more block device mappings to the AMI. These will be attached
	// when booting a new instance from your AMI. To add a block device during
	// the Packer build see `launch_block_device_mappings` below. Your options
	// here may vary depending on the type of VM you use. See the
	// [BlockDevices](#block-devices-configuration) documentation for fields.
	AMIMappings awscommon.BlockDevices `mapstructure:"ami_block_device_mappings" required:"false"`
	// Add one or more block devices before the Packer build starts. If you add
	// instance store volumes or EBS volumes in addition to the root device
	// volume, the created AMI will contain block device mapping information
	// for those volumes. Amazon creates snapshots of the source instance's
	// root volume and any other EBS volumes described here. When you launch an
	// instance from this new AMI, the instance automatically launches with
	// these additional volumes, and will restore them from snapshots taken
	// from the source instance. See the
	// [BlockDevices](#block-devices-configuration) documentation for fields.
	LaunchMappings awscommon.BlockDevices `mapstructure:"launch_block_device_mappings" required:"false"`
	// Tags to apply to the volumes that are *launched* to create the AMI.
	// These tags are *not* applied to the resulting AMI unless they're
	// duplicated in `tags`. This is a [template
	// engine](/packer/docs/templates/legacy_json_templates/engine), see [Build template
	// data](#build-template-data) for more information.
	VolumeRunTags map[string]string `mapstructure:"run_volume_tags"`
	// Same as [`run_volume_tags`](#run_volume_tags) but defined as a singular
	// block containing a `name` and a `value` field. In HCL2 mode the
	// [`dynamic_block`](https://packer.io/docs/templates/hcl_templates/expressions.html#dynamic-blocks)
	// will allow you to create those programatically.
	VolumeRunTag config.NameValues `mapstructure:"run_volume_tag" required:"false"`
	// Relevant only to Windows guests: If you set this flag, we'll add clauses
	// to the launch_block_device_mappings that make sure ephemeral drives
	// don't show up in the EC2 console. If you launched from the EC2 console,
	// you'd get this automatically, but the SDK does not provide this service.
	// For more information, see
	// https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/InstanceStorage.html.
	// Because we don't validate the OS type of your guest, it is up to you to
	// make sure you don't set this for *nix guests; behavior may be
	// unpredictable.
	NoEphemeral bool `mapstructure:"no_ephemeral" required:"false"`
	// The configuration for fast launch support.
	//
	// Fast launch is only relevant for Windows AMIs, and should not be used
	// for other OSes.
	// See the [Fast Launch Configuration](#fast-launch-config) section for
	// information on the attributes supported for this block.
	FastLaunch FastLaunchConfig `mapstructure:"fast_launch" required:"false"`

	ctx interpolate.Context
}

type Builder struct {
	config Config
	runner multistep.Runner
}

func (b *Builder) ConfigSpec() hcldec.ObjectSpec { return b.config.FlatMapstructure().HCL2Spec() }

func (b *Builder) Prepare(raws ...interface{}) ([]string, []string, error) {
	b.config.ctx.Funcs = awscommon.TemplateFuncs
	err := config.Decode(&b.config, &config.DecodeOpts{
		PluginType:         BuilderId,
		Interpolate:        true,
		InterpolateContext: &b.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{
				"ami_description",
				"fleet_tags",
				"fleet_tag",
				"run_tags",
				"run_tag",
				"run_volume_tags",
				"run_volume_tag",
				"spot_tags",
				"spot_tag",
				"snapshot_tags",
				"snapshot_tag",
				"tags",
				"tag",
			},
		},
	}, raws...)
	if err != nil {
		return nil, nil, err
	}

	if b.config.PackerConfig.PackerForce {
		b.config.AMIForceDeregister = true
	}

	// Accumulate any errors
	var errs *packersdk.MultiError
	var warns []string

	errs = packersdk.MultiErrorAppend(errs, b.config.VolumeRunTag.CopyOn(&b.config.VolumeRunTags)...)

	errs = packersdk.MultiErrorAppend(errs, b.config.AccessConfig.Prepare(&b.config.PackerConfig)...)
	errs = packersdk.MultiErrorAppend(errs,
		b.config.AMIConfig.Prepare(&b.config.AccessConfig, &b.config.ctx)...)
	errs = packersdk.MultiErrorAppend(errs, b.config.AMIMappings.Prepare(&b.config.ctx)...)
	errs = packersdk.MultiErrorAppend(errs, b.config.LaunchMappings.Prepare(&b.config.ctx)...)
	errs = packersdk.MultiErrorAppend(errs, b.config.RunConfig.Prepare(&b.config.ctx)...)

	b.config.FastLaunch.defaultRegion = b.config.RawRegion
	errs = packersdk.MultiErrorAppend(errs, b.config.FastLaunch.Prepare()...)
	for _, templateConfig := range b.config.FastLaunch.RegionLaunchTemplates {
		exists := false
		for _, cpRegion := range b.config.AMIRegions {
			if cpRegion == templateConfig.Region {
				exists = true
				break
			}
		}
		if b.config.RawRegion == templateConfig.Region {
			exists = true
		}

		if !exists {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("Launch template specified for enabling fast-launch on region %q, but the AMI won't be copied there.", templateConfig.Region))
		}
	}

	if b.config.IsSpotInstance() && (b.config.AMIENASupport.True() || b.config.AMISriovNetSupport) {
		errs = packersdk.MultiErrorAppend(errs,
			fmt.Errorf("Spot instances do not support modification, which is required "+
				"when either `ena_support` or `sriov_support` are set. Please ensure "+
				"you use an AMI that already has either SR-IOV or ENA enabled."))
	}

	if b.config.RunConfig.SpotPriceAutoProduct != "" {
		warns = append(warns, "spot_price_auto_product is deprecated and no "+
			"longer necessary for Packer builds. In future versions of "+
			"Packer, inclusion of spot_price_auto_product will error your "+
			"builds. Please take a look at our current documentation to "+
			"understand how Packer requests Spot instances.")
	}

	if b.config.RunConfig.EnableT2Unlimited {
		warns = append(warns, "enable_t2_unlimited is deprecated please use "+
			"enable_unlimited_credits. In future versions of "+
			"Packer, inclusion of enable_t2_unlimited will error your builds.")
	}

	if errs != nil && len(errs.Errors) > 0 {
		return nil, warns, errs
	}

	packersdk.LogSecretFilter.Set(b.config.AccessKey, b.config.SecretKey, b.config.Token)

	generatedData := awscommon.GetGeneratedDataList()
	return generatedData, warns, nil
}

func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {

	client, err := b.config.NewEC2Client(ctx)
	if err != nil {
		return nil, err
	}

	awsConfig, err := b.config.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating config: %w", err)
	}
	iamClient := iam.NewFromConfig(*awsConfig)

	// Setup the state bag and initial state for the steps
	state := new(multistep.BasicStateBag)
	state.Put("config", &b.config)
	state.Put("access_config", &b.config.AccessConfig)
	state.Put("ami_config", &b.config.AMIConfig)
	state.Put("ec2v2", client)
	state.Put("iam", iamClient)
	state.Put("hook", hook)
	state.Put("ui", ui)
	state.Put("region", awsConfig.Region)
	state.Put("aws_config", awsConfig)
	generatedData := &packerbuilderdata.GeneratedData{State: state}

	var instanceStep multistep.Step

	if b.config.IsSpotInstance() {
		instanceStep = &awscommon.StepRunSpotInstance{
			PollingConfig:                     b.config.PollingConfig,
			AssociatePublicIpAddress:          b.config.AssociatePublicIpAddress,
			LaunchMappings:                    b.config.LaunchMappings,
			BlockDurationMinutes:              b.config.BlockDurationMinutes,
			Ctx:                               b.config.ctx,
			Comm:                              &b.config.RunConfig.Comm,
			Debug:                             b.config.PackerDebug,
			EbsOptimized:                      b.config.EbsOptimized,
			IsBurstableInstanceType:           b.config.RunConfig.IsBurstableInstanceType(),
			EnableUnlimitedCredits:            b.config.EnableUnlimitedCredits,
			ExpectedRootDevice:                "ebs",
			FleetTags:                         b.config.FleetTags,
			HttpEndpoint:                      b.config.Metadata.HttpEndpoint,
			HttpTokens:                        b.config.Metadata.HttpTokens,
			HttpPutResponseHopLimit:           b.config.Metadata.HttpPutResponseHopLimit,
			InstanceMetadataTags:              b.config.Metadata.InstanceMetadataTags,
			InstanceInitiatedShutdownBehavior: b.config.InstanceInitiatedShutdownBehavior,
			InstanceType:                      b.config.InstanceType,
			Region:                            awsConfig.Region,
			SourceAMI:                         b.config.SourceAmi,
			SpotPrice:                         b.config.SpotPrice,
			SpotTags:                          b.config.SpotTags,
			Tags:                              b.config.RunTags,
			SpotInstanceTypes:                 b.config.SpotInstanceTypes,
			SpotAllocationStrategy:            b.config.SpotAllocationStrategy,
			UserData:                          b.config.UserData,
			UserDataFile:                      b.config.UserDataFile,
			VolumeTags:                        b.config.VolumeRunTags,
			NoEphemeral:                       b.config.NoEphemeral,
		}
	} else {
		var tenancy string
		tenancies := []string{b.config.Placement.Tenancy, b.config.Tenancy}

		for i := range tenancies {
			if tenancies[i] != "" {
				tenancy = tenancies[i]
				break
			}
		}

		instanceStep = &awscommon.StepRunSourceInstance{
			PollingConfig:                     b.config.PollingConfig,
			AssociatePublicIpAddress:          b.config.AssociatePublicIpAddress,
			LaunchMappings:                    b.config.LaunchMappings,
			CapacityReservationPreference:     b.config.CapacityReservationPreference,
			CapacityReservationId:             b.config.CapacityReservationId,
			CapacityReservationGroupArn:       b.config.CapacityReservationGroupArn,
			Comm:                              &b.config.RunConfig.Comm,
			Ctx:                               b.config.ctx,
			Debug:                             b.config.PackerDebug,
			EbsOptimized:                      b.config.EbsOptimized,
			EnableNitroEnclave:                b.config.EnableNitroEnclave,
			IsBurstableInstanceType:           b.config.RunConfig.IsBurstableInstanceType(),
			EnableUnlimitedCredits:            b.config.EnableUnlimitedCredits,
			ExpectedRootDevice:                "ebs",
			HttpEndpoint:                      b.config.Metadata.HttpEndpoint,
			HttpTokens:                        b.config.Metadata.HttpTokens,
			HttpPutResponseHopLimit:           b.config.Metadata.HttpPutResponseHopLimit,
			InstanceMetadataTags:              b.config.Metadata.InstanceMetadataTags,
			InstanceInitiatedShutdownBehavior: b.config.InstanceInitiatedShutdownBehavior,
			InstanceType:                      b.config.InstanceType,
			IsRestricted:                      b.config.IsChinaCloud(),
			SourceAMI:                         b.config.SourceAmi,
			Tags:                              b.config.RunTags,
			LicenseSpecifications:             b.config.LicenseSpecifications,
			HostResourceGroupArn:              b.config.Placement.HostResourceGroupArn,
			HostId:                            b.config.Placement.HostId,
			Tenancy:                           tenancy,
			UserData:                          b.config.UserData,
			UserDataFile:                      b.config.UserDataFile,
			VolumeTags:                        b.config.VolumeRunTags,
			NoEphemeral:                       b.config.NoEphemeral,
		}
	}

	// Build the steps
	steps := []multistep.Step{
		&awscommon.StepPreValidate{
			DestAmiName:        b.config.AMIName,
			ForceDeregister:    b.config.AMIForceDeregister,
			AMISkipBuildRegion: b.config.AMISkipBuildRegion,
			AMISkipCreateImage: b.config.AMISkipCreateImage,
			VpcId:              b.config.VpcId,
			SubnetId:           b.config.SubnetId,
			HasSubnetFilter:    !b.config.SubnetFilter.Empty(),
		},
		&awscommon.StepSourceAMIInfo{
			SourceAmi:                b.config.SourceAmi,
			EnableAMISriovNetSupport: b.config.AMISriovNetSupport,
			EnableAMIENASupport:      b.config.AMIENASupport,
			AmiFilters:               b.config.SourceAmiFilter,
			AMIVirtType:              b.config.AMIVirtType,
		},
		&awscommon.StepNetworkInfo{
			VpcId:                    b.config.VpcId,
			VpcFilter:                b.config.VpcFilter,
			SecurityGroupIds:         b.config.SecurityGroupIds,
			SecurityGroupFilter:      b.config.SecurityGroupFilter,
			SubnetId:                 b.config.SubnetId,
			SubnetFilter:             b.config.SubnetFilter,
			AvailabilityZone:         b.config.AvailabilityZone,
			AssociatePublicIpAddress: b.config.AssociatePublicIpAddress,
			RequestedMachineType:     b.config.InstanceType,
		},
		&awscommon.StepKeyPair{
			Debug:        b.config.PackerDebug,
			Comm:         &b.config.RunConfig.Comm,
			IsRestricted: b.config.IsChinaCloud(),
			DebugKeyPath: fmt.Sprintf("ec2_%s.pem", b.config.PackerBuildName),
			Tags:         b.config.RunTags,
			Ctx:          b.config.ctx,
		},
		&awscommon.StepSecurityGroup{
			PollingConfig:             b.config.PollingConfig,
			SecurityGroupFilter:       b.config.SecurityGroupFilter,
			SecurityGroupIds:          b.config.SecurityGroupIds,
			CommConfig:                &b.config.RunConfig.Comm,
			TemporarySGSourceCidrs:    b.config.TemporarySGSourceCidrs,
			TemporarySGSourcePublicIp: b.config.TemporarySGSourcePublicIp,
			SkipSSHRuleCreation:       b.config.SSMAgentEnabled(),
			IsRestricted:              b.config.IsChinaCloud(),
			Tags:                      b.config.RunTags,
			Ctx:                       b.config.ctx,
		},
		&awscommon.StepIamInstanceProfile{
			PollingConfig:                             b.config.PollingConfig,
			IamInstanceProfile:                        b.config.IamInstanceProfile,
			SkipProfileValidation:                     b.config.SkipProfileValidation,
			TemporaryIamInstanceProfilePolicyDocument: b.config.TemporaryIamInstanceProfilePolicyDocument,
			Tags: b.config.RunTags,
			Ctx:  b.config.ctx,
		},
		&awscommon.StepCleanupVolumes{
			LaunchMappings: b.config.LaunchMappings,
		},
		instanceStep,
		&awscommon.StepGetPassword{
			Debug:     b.config.PackerDebug,
			Comm:      &b.config.RunConfig.Comm,
			Timeout:   b.config.WindowsPasswordTimeout,
			BuildName: b.config.PackerBuildName,
		},
		&awscommon.StepCreateSSMTunnel{
			AwsConfig:        *awsConfig,
			Region:           awsConfig.Region,
			PauseBeforeSSM:   b.config.PauseBeforeSSM,
			LocalPortNumber:  b.config.SessionManagerPort,
			RemotePortNumber: b.config.Comm.Port(),
			SSMAgentEnabled:  b.config.SSMAgentEnabled(),
			SSHConfig:        &b.config.Comm.SSH,
		},
		&communicator.StepConnect{
			Config: &b.config.RunConfig.Comm,
			Host: awscommon.SSHHost(
				ctx,
				client,
				b.config.SSHInterface,
				b.config.Comm.Host(),
			),
			SSHPort: awscommon.Port(
				b.config.SSHInterface,
				b.config.Comm.Port(),
			),
			SSHConfig: b.config.RunConfig.Comm.SSHConfigFunc(),
		},
		&awscommon.StepSetGeneratedData{
			GeneratedData: generatedData,
		},
		&commonsteps.StepProvision{},
		&commonsteps.StepCleanupTempKeys{
			Comm: &b.config.RunConfig.Comm,
		},
		&awscommon.StepStopEBSBackedInstance{
			PollingConfig:       b.config.PollingConfig,
			Skip:                b.config.IsSpotInstance(),
			DisableStopInstance: b.config.DisableStopInstance,
		},
		&awscommon.StepModifyEBSBackedInstance{
			EnableAMISriovNetSupport: b.config.AMISriovNetSupport,
			EnableAMIENASupport:      b.config.AMIENASupport,
		},
		&awscommon.StepDeregisterAMI{
			AccessConfig:        &b.config.AccessConfig,
			ForceDeregister:     b.config.AMIForceDeregister,
			ForceDeleteSnapshot: b.config.AMIForceDeleteSnapshot,
			AMIName:             b.config.AMIName,
			Regions:             b.config.AMIRegions,
		},
		&stepCreateAMI{
			AMISkipCreateImage: b.config.AMISkipCreateImage,
			AMISkipBuildRegion: b.config.AMISkipBuildRegion,
			AMISkipRunTags:     b.config.AMISkipRunTags,
			PollingConfig:      b.config.PollingConfig,
			IsRestricted:       b.config.IsChinaCloud() || b.config.IsGovCloud(),
			Tags:               b.config.RunTags,
			Ctx:                b.config.ctx,
		},
		&awscommon.StepAMIRegionCopy{
			AccessConfig:                   &b.config.AccessConfig,
			Regions:                        b.config.AMIRegions,
			AMIKmsKeyId:                    b.config.AMIKmsKeyId,
			RegionKeyIds:                   b.config.AMIRegionKMSKeyIDs,
			EncryptBootVolume:              b.config.AMIEncryptBootVolume,
			Name:                           b.config.AMIName,
			OriginalRegion:                 awsConfig.Region,
			AMISkipCreateImage:             b.config.AMISkipCreateImage,
			AMISkipBuildRegion:             b.config.AMISkipBuildRegion,
			AMISnapshotCopyDurationMinutes: b.config.AMISnapshotCopyDurationMinutes,
		},
		&stepPrepareFastLaunchTemplate{
			AccessConfig:       &b.config.AccessConfig,
			AMISkipCreateImage: b.config.AMISkipCreateImage,
			EnableFastLaunch:   b.config.FastLaunch.UseFastLaunch,
			RegionTemplates:    b.config.FastLaunch.RegionLaunchTemplates,
		},
		&stepEnableFastLaunch{
			AccessConfig:       &b.config.AccessConfig,
			PollingConfig:      b.config.PollingConfig,
			ResourceCount:      b.config.FastLaunch.TargetResourceCount,
			AMISkipCreateImage: b.config.AMISkipCreateImage,
			EnableFastLaunch:   b.config.FastLaunch.UseFastLaunch,
			MaxInstances:       b.config.FastLaunch.MaxParallelLaunches,
		},
		&awscommon.StepEnableDeprecation{
			AccessConfig:       &b.config.AccessConfig,
			DeprecationTime:    b.config.DeprecationTime,
			AMISkipCreateImage: b.config.AMISkipCreateImage,
		},
		&awscommon.StepEnableDeregistrationProtection{
			AccessConfig:             &b.config.AccessConfig,
			AMISkipCreateImage:       b.config.AMISkipCreateImage,
			DeregistrationProtection: &b.config.DeregistrationProtection,
		},
		&awscommon.StepModifyAMIAttributes{
			AMISkipCreateImage: b.config.AMISkipCreateImage,
			Description:        b.config.AMIDescription,
			Users:              b.config.AMIUsers,
			Groups:             b.config.AMIGroups,
			OrgArns:            b.config.AMIOrgArns,
			OuArns:             b.config.AMIOuArns,
			ProductCodes:       b.config.AMIProductCodes,
			SnapshotUsers:      b.config.SnapshotUsers,
			SnapshotGroups:     b.config.SnapshotGroups,
			IMDSSupport:        b.config.AMIIMDSSupport,
			Ctx:                b.config.ctx,
			GeneratedData:      generatedData,
		},
		&awscommon.StepCreateTags{
			AMISkipCreateImage: b.config.AMISkipCreateImage,
			Tags:               b.config.AMITags,
			SnapshotTags:       b.config.SnapshotTags,
			Ctx:                b.config.ctx,
		},
	}

	// Run!
	b.runner = commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)
	// If there was an error, return that
	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	// If there are no AMIs, then just return
	if _, ok := state.GetOk("amis"); !ok {
		return nil, nil
	}

	// Build the artifact and return it
	artifact := &awscommon.Artifact{
		Amis:           state.Get("amis").(map[string]string),
		BuilderIdValue: BuilderId,
		Config:         awsConfig,
		StateData:      map[string]interface{}{"generated_data": state.Get("generated_data")},
	}

	return artifact, nil
}
