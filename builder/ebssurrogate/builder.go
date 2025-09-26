// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,RootBlockDevice,BlockDevice

// The ebssurrogate package contains a packersdk.Builder implementation that
// builds a new EBS-backed AMI using an ephemeral instance.
package ebssurrogate

import (
	"context"
	"errors"
	"fmt"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
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

const BuilderId = "mitchellh.amazon.ebssurrogate"

type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`
	awscommon.RunConfig    `mapstructure:",squash"`
	awscommon.AMIConfig    `mapstructure:",squash"`

	// The description for the snapshot.
	SnapshotDescription string `mapstructure:"snapshot_description" required:"false"`
	// Add one or more block device mappings to the AMI. These will be attached
	// when booting a new instance from your AMI. To add a block device during
	// the Packer build see `launch_block_device_mappings` below. Your options
	// here may vary depending on the type of VM you use. See the
	// [BlockDevices](#block-devices-configuration) documentation for fields.
	AMIMappings awscommon.BlockDevices `mapstructure:"ami_block_device_mappings" required:"false"`
	// If true will not propagate the run tags set on Packer created instance to the AMI created.
	AMISkipRunTags bool `mapstructure:"skip_ami_run_tags" required:"false"`

	// Add one or more block devices before the Packer build starts. If you add
	// instance store volumes or EBS volumes in addition to the root device
	// volume, the created AMI will contain block device mapping information
	// for those volumes. Amazon creates snapshots of the source instance's
	// root volume and any other EBS volumes described here. When you launch an
	// instance from this new AMI, the instance automatically launches with
	// these additional volumes, and will restore them from snapshots taken
	// from the source instance. See the
	// [BlockDevices](#block-devices-configuration) documentation for fields.
	LaunchMappings BlockDevices `mapstructure:"launch_block_device_mappings" required:"false"`
	// A block device mapping describing the root device of the AMI. This looks
	// like the mappings in `ami_block_device_mapping`, except with an
	// additional field:
	//
	// -   `source_device_name` (string) - The device name of the block device on
	//     the source instance to be used as the root device for the AMI. This
	//     must correspond to a block device in `launch_block_device_mapping`.
	RootDevice RootBlockDevice `mapstructure:"ami_root_device" required:"true"`
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
	// what architecture to use when registering the final AMI; valid options
	// are "arm64", "arm64_mac", "i386", "x86_64", or "x86_64_mac". Defaults to "x86_64".
	Architecture string `mapstructure:"ami_architecture" required:"false"`
	// The boot mode. Valid options are `legacy-bios` and `uefi`. See the documentation on
	// [boot modes](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ami-boot.html) for
	// more information. Defaults to `legacy-bios` when `ami_architecture` is `x86_64` and
	// `uefi` when `ami_architecture` is `arm64`.
	BootMode string `mapstructure:"boot_mode" required:"false"`
	// Base64 representation of the non-volatile UEFI variable store. For more information
	// see [AWS documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/uefi-secure-boot-optionB.html).
	UefiData string `mapstructure:"uefi_data" required:"false"`
	// NitroTPM Support. Valid options are `v2.0`. See the documentation on
	// [NitroTPM Support](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/enable-nitrotpm-support-on-ami.html) for
	// more information. Only enabled if a valid option is provided, otherwise ignored.
	TpmSupport string `mapstructure:"tpm_support" required:"false"`
	// Whether to use the CreateImage or RegisterImage API when creating the AMI.
	// When set to `true`, the CreateImage API is used and will create the image
	// from the instance itself, and inherit properties from the instance.
	// When set to `false`, the RegisterImage API is used and the image is created using
	// a snapshot of the specified EBS volume, and no properties are inherited from the instance.
	// Defaults to `false`.
	//Ref: https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CreateImage.html
	//     https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_RegisterImage.html
	UseCreateImage bool `mapstructure:"use_create_image" required:"false"`

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
				"snapshot_tags",
				"snapshot_tag",
				"spot_tags",
				"spot_tag",
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
	errs = packersdk.MultiErrorAppend(errs, b.config.RunConfig.Prepare(&b.config.ctx)...)
	errs = packersdk.MultiErrorAppend(errs,
		b.config.AMIConfig.Prepare(&b.config.AccessConfig, &b.config.ctx)...)
	errs = packersdk.MultiErrorAppend(errs, b.config.AMIMappings.Prepare(&b.config.ctx)...)
	errs = packersdk.MultiErrorAppend(errs, b.config.LaunchMappings.Prepare(&b.config.ctx)...)
	errs = packersdk.MultiErrorAppend(errs, b.config.RootDevice.Prepare(&b.config.ctx)...)

	if b.config.AMIVirtType == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("ami_virtualization_type is required."))
	}

	foundRootVolume := false
	for _, launchDevice := range b.config.LaunchMappings {
		if launchDevice.DeviceName == b.config.RootDevice.SourceDeviceName {
			foundRootVolume = true
			if launchDevice.OmitFromArtifact {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("You cannot set \"omit_from_artifact\": \"true\" for the root volume."))
			}
		}
	}

	if !foundRootVolume {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("no volume with name '%s' is found", b.config.RootDevice.SourceDeviceName))
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

	if b.config.Architecture == "" {
		b.config.Architecture = "x86_64"
	}
	valid := false
	for _, validArch := range []string{"arm64", "arm64_mac", "i386", "x86_64", "x86_64_mac"} {
		if validArch == b.config.Architecture {
			valid = true
			break
		}
	}
	if !valid {
		errs = packersdk.MultiErrorAppend(errs, errors.New(`The only valid ami_architecture values are "arm64", "arm64_mac", "i386", "x86_64", or "x86_64_mac"`))
	}

	if b.config.TpmSupport != "" && ec2types.TpmSupportValues(b.config.TpmSupport) != ec2types.TpmSupportValuesV20 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(`The only valid tpm_support value is %q`,
			ec2types.TpmSupportValuesV20))
	}

	if b.config.BootMode != "" {
		err := awscommon.IsValidBootMode(b.config.BootMode)
		if err != nil {
			errs = packersdk.MultiErrorAppend(errs, err)
		}
	}

	if b.config.UefiData != "" {
		if b.config.BootMode == "legacy-bios" {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(`You can't use uefi_data with boot_mode set to "legacy-bios".`))
		} else if b.config.BootMode == "" && b.config.Architecture != "arm64" {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(`You need boot_mode set to "uefi" to use uefi_data, `+
				`"%s" architecture defaults to "legacy-bios".`, b.config.Architecture))
		}
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
			HttpEndpoint:                      b.config.Metadata.HttpEndpoint,
			HttpTokens:                        b.config.Metadata.HttpTokens,
			HttpPutResponseHopLimit:           b.config.Metadata.HttpPutResponseHopLimit,
			InstanceMetadataTags:              b.config.Metadata.InstanceMetadataTags,
			InstanceInitiatedShutdownBehavior: b.config.InstanceInitiatedShutdownBehavior,
			InstanceType:                      b.config.InstanceType,
			FleetTags:                         b.config.FleetTags,
			Region:                            awsConfig.Region,
			SourceAMI:                         b.config.SourceAmi,
			SpotPrice:                         b.config.SpotPrice,
			SpotAllocationStrategy:            b.config.SpotAllocationStrategy,
			SpotInstanceTypes:                 b.config.SpotInstanceTypes,
			SpotTags:                          b.config.SpotTags,
			Tags:                              b.config.RunTags,
			UserData:                          b.config.UserData,
			UserDataFile:                      b.config.UserDataFile,
			VolumeTags:                        b.config.VolumeRunTags,
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
			IsBurstableInstanceType:           b.config.IsBurstableInstanceType(),
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
			Tenancy:                           tenancy,
			UserData:                          b.config.UserData,
			UserDataFile:                      b.config.UserDataFile,
			VolumeTags:                        b.config.VolumeRunTags,
		}
	}

	amiDevices := b.config.AMIMappings.BuildEC2BlockDeviceMappings()
	launchDevices := b.config.LaunchMappings.BuildEC2BlockDeviceMappings()

	var buildAmiStep multistep.Step
	var volumeStep multistep.Step

	if b.config.UseCreateImage {
		volumeStep = &StepSwapVolumes{
			PollingConfig: b.config.PollingConfig,
			RootDevice:    b.config.RootDevice,
			LaunchDevices: launchDevices,
			LaunchOmitMap: b.config.LaunchMappings.GetOmissions(),
			Ctx:           b.config.ctx,
		}

		buildAmiStep = &StepCreateAMI{
			AMISkipBuildRegion: b.config.AMISkipBuildRegion,
			AMISkipRunTags:     b.config.AMISkipRunTags,
			RootDevice:         b.config.RootDevice,
			AMIDevices:         amiDevices,
			LaunchDevices:      launchDevices,
			PollingConfig:      b.config.PollingConfig,
			IsRestricted:       b.config.IsChinaCloud() || b.config.IsGovCloud(),
			Tags:               b.config.RunTags,
			Ctx:                b.config.ctx,
		}
	} else {
		volumeStep = &StepSnapshotVolumes{
			PollingConfig:       b.config.PollingConfig,
			LaunchDevices:       launchDevices,
			SnapshotOmitMap:     b.config.LaunchMappings.GetOmissions(),
			SnapshotTags:        b.config.SnapshotTags,
			SnapshotDescription: b.config.SnapshotDescription,
			Ctx:                 b.config.ctx,
		}
		buildAmiStep = &StepRegisterAMI{
			RootDevice:               b.config.RootDevice,
			AMIDevices:               amiDevices,
			LaunchDevices:            launchDevices,
			EnableAMISriovNetSupport: b.config.AMISriovNetSupport,
			EnableAMIENASupport:      b.config.AMIENASupport,
			Architecture:             b.config.Architecture,
			LaunchOmitMap:            b.config.LaunchMappings.GetOmissions(),
			AMISkipBuildRegion:       b.config.AMISkipBuildRegion,
			PollingConfig:            b.config.PollingConfig,
			BootMode:                 b.config.BootMode,
			UefiData:                 b.config.UefiData,
			TpmSupport:               b.config.TpmSupport,
		}
	}

	// Build the steps
	steps := []multistep.Step{
		&awscommon.StepPreValidate{
			DestAmiName:        b.config.AMIName,
			ForceDeregister:    b.config.AMIForceDeregister,
			AMISkipBuildRegion: b.config.AMISkipBuildRegion,
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
			LaunchMappings: b.config.LaunchMappings.Common(),
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
			Skip:                     b.config.IsSpotInstance(),
			EnableAMISriovNetSupport: b.config.AMISriovNetSupport,
			EnableAMIENASupport:      b.config.AMIENASupport,
		},
		volumeStep,
		&awscommon.StepDeregisterAMI{
			AccessConfig:        &b.config.AccessConfig,
			ForceDeregister:     b.config.AMIForceDeregister,
			ForceDeleteSnapshot: b.config.AMIForceDeleteSnapshot,
			AMIName:             b.config.AMIName,
			Regions:             b.config.AMIRegions,
		},
		buildAmiStep,
		&awscommon.StepAMIRegionCopy{
			AccessConfig:                   &b.config.AccessConfig,
			Regions:                        b.config.AMIRegions,
			AMIKmsKeyId:                    b.config.AMIKmsKeyId,
			RegionKeyIds:                   b.config.AMIRegionKMSKeyIDs,
			EncryptBootVolume:              b.config.AMIEncryptBootVolume,
			Name:                           b.config.AMIName,
			OriginalRegion:                 awsConfig.Region,
			AMISkipBuildRegion:             b.config.AMISkipBuildRegion,
			AMISnapshotCopyDurationMinutes: b.config.AMISnapshotCopyDurationMinutes,
		},
		&awscommon.StepEnableDeprecation{
			AccessConfig:    &b.config.AccessConfig,
			DeprecationTime: b.config.DeprecationTime,
		},
		&awscommon.StepEnableDeregistrationProtection{
			AccessConfig:             &b.config.AccessConfig,
			DeregistrationProtection: &b.config.DeregistrationProtection,
		},
		&awscommon.StepModifyAMIAttributes{
			Description:    b.config.AMIDescription,
			Users:          b.config.AMIUsers,
			Groups:         b.config.AMIGroups,
			OrgArns:        b.config.AMIOrgArns,
			OuArns:         b.config.AMIOuArns,
			ProductCodes:   b.config.AMIProductCodes,
			SnapshotUsers:  b.config.SnapshotUsers,
			SnapshotGroups: b.config.SnapshotGroups,
			IMDSSupport:    b.config.AMIIMDSSupport,
			Ctx:            b.config.ctx,
			GeneratedData:  generatedData,
		},
		&awscommon.StepCreateTags{
			Tags:         b.config.AMITags,
			SnapshotTags: b.config.SnapshotTags,
			Ctx:          b.config.ctx,
		},
	}

	// Run!
	b.runner = commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)

	// If there was an error, return that
	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	if amis, ok := state.GetOk("amis"); ok {
		// Build the artifact and return it
		artifact := &awscommon.Artifact{
			Amis:           amis.(map[string]string),
			BuilderIdValue: BuilderId,
			Config:         awsConfig,
			StateData:      map[string]interface{}{"generated_data": state.Get("generated_data")},
		}

		return artifact, nil
	}

	return nil, nil
}
