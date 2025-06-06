// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,BlockDevices,BlockDevice

// The chroot package is able to create an Amazon AMI without requiring the
// launch of a new instance for every build. It does this by attaching and
// mounting the root volume of another AMI and chrooting into that directory.
// It then creates an AMI from that attached drive.
package chroot

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/hcl/v2/hcldec"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
	"github.com/hashicorp/packer-plugin-sdk/chroot"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

// The unique ID for this builder
const BuilderId = "mitchellh.amazon.chroot"

// Config is the configuration that is chained through the steps and settable
// from the template.
type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AMIConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`
	// Add one or more [block device
	// mappings](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/block-device-mapping-concepts.html)
	// to the AMI. If this field is populated, and you are building from an
	// existing source image, the block device mappings in the source image
	// will be overwritten. This means you must have a block device mapping
	// entry for your root volume, `root_volume_size` and `root_device_name`.
	// See the [BlockDevices](#block-devices-configuration) documentation for
	// fields.
	AMIMappings awscommon.BlockDevices `mapstructure:"ami_block_device_mappings" hcl2-schema-generator:"ami_block_device_mappings,direct" required:"false"`
	// This is a list of devices to mount into the chroot environment. This
	// configuration parameter requires some additional documentation which is
	// in the Chroot Mounts section. Please read that section for more
	// information on how to use this.
	ChrootMounts [][]string `mapstructure:"chroot_mounts" required:"false"`
	// How to run shell commands. This defaults to `{{.Command}}`. This may be
	// useful to set if you want to set environmental variables or perhaps run
	// it with sudo or so on. This is a configuration template where the
	// .Command variable is replaced with the command to be run. Defaults to
	// `{{.Command}}`.
	CommandWrapper string `mapstructure:"command_wrapper" required:"false"`
	// Paths to files on the running EC2 instance that will be copied into the
	// chroot environment prior to provisioning. Defaults to /etc/resolv.conf
	// so that DNS lookups work. Pass an empty list to skip copying
	// /etc/resolv.conf. You may need to do this if you're building an image
	// that uses systemd.
	CopyFiles []string `mapstructure:"copy_files" required:"false"`
	// The path to the device where the root volume of the source AMI will be
	// attached. This defaults to "" (empty string), which forces Packer to
	// find an open device automatically.
	DevicePath string `mapstructure:"device_path" required:"false"`
	// When we call the mount command (by default mount -o device dir), the
	// string provided in nvme_mount_path will replace device in that command.
	// When this option is not set, device in that command will be something
	// like /dev/sdf1, mirroring the attached device name. This assumption
	// works for most instances but will fail with c5 and m5 instances. In
	// order to use the chroot builder with c5 and m5 instances, you must
	// manually set nvme_device_path and device_path.
	NVMEDevicePath string `mapstructure:"nvme_device_path" required:"false"`
	// Build a new volume instead of starting from an existing AMI root volume
	// snapshot. Default false. If true, source_ami/source_ami_filter are no
	// longer used and the following options become required:
	// ami_virtualization_type, pre_mount_commands and root_volume_size.
	FromScratch bool `mapstructure:"from_scratch" required:"false"`
	// Options to supply the mount command when mounting devices. Each option
	// will be prefixed with -o and supplied to the mount command ran by
	// Packer. Because this command is ran in a shell, user discretion is
	// advised. See this manual page for the mount command for valid file
	// system specific options.
	MountOptions []string `mapstructure:"mount_options" required:"false"`
	// The partition number containing the / partition. By default this is the
	// first partition of the volume, (for example, xvda1) but you can
	// designate the entire block device by setting "mount_partition": "0" in
	// your config, which will mount xvda instead.
	MountPartition string `mapstructure:"mount_partition" required:"false"`
	// The path where the volume will be mounted. This is where the chroot
	// environment will be. This defaults to
	// `/mnt/packer-amazon-chroot-volumes/{{.Device}}`. This is a configuration
	// template where the .Device variable is replaced with the name of the
	// device where the volume is attached.
	MountPath string `mapstructure:"mount_path" required:"false"`
	// As pre_mount_commands, but the commands are executed after mounting the
	// root device and before the extra mount and copy steps. The device and
	// mount path are provided by `{{.Device}}` and `{{.MountPath}}`.
	PostMountCommands []string `mapstructure:"post_mount_commands" required:"false"`
	// A series of commands to execute after attaching the root volume and
	// before mounting the chroot. This is not required unless using
	// from_scratch. If so, this should include any partitioning and filesystem
	// creation commands. The path to the device is provided by `{{.Device}}`.
	PreMountCommands []string `mapstructure:"pre_mount_commands" required:"false"`
	// The root device name. For example, xvda.
	RootDeviceName string `mapstructure:"root_device_name" required:"false"`
	// The size of the root volume in GB for the chroot environment and the
	// resulting AMI. Default size is the snapshot size of the source_ami
	// unless from_scratch is true, in which case this field must be defined.
	RootVolumeSize int64 `mapstructure:"root_volume_size" required:"false"`
	// The type of EBS volume for the chroot environment and resulting AMI. The
	// default value is the type of the source_ami, unless from_scratch is
	// true, in which case the default value is gp2. You can only specify io1
	// if building based on top of a source_ami which is also io1.
	RootVolumeType string `mapstructure:"root_volume_type" required:"false"`
	// The source AMI whose root volume will be copied and provisioned on the
	// currently running instance. This must be an EBS-backed AMI with a root
	// volume snapshot that you have access to. Note: this is not used when
	// from_scratch is set to true.
	SourceAmi string `mapstructure:"source_ami" required:"true"`
	// Filters used to populate the source_ami field. Example:
	//
	//```json
	//{
	//	 "source_ami_filter": {
	//	 "filters": {
	//	  "virtualization-type": "hvm",
	//	  "name": "ubuntu/images/*ubuntu-xenial-16.04-amd64-server-*",
	//	  "root-device-type": "ebs"
	//	},
	//	"owners": ["099720109477"],
	//	"most_recent": true
	//	 }
	//}
	//```
	//
	//This selects the most recent Ubuntu 16.04 HVM EBS AMI from Canonical. NOTE:
	//This will fail unless *exactly* one AMI is returned. In the above example,
	//`most_recent` will cause this to succeed by selecting the newest image.
	//
	//-   `filters` (map[string,string] | multiple filters are allowed when seperated by commas) -
	//  filters used to select a `source_ami`.
	//	NOTE: This will fail unless *exactly* one AMI is returned. Any filter
	//	described in the docs for
	//	[DescribeImages](http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeImages.html)
	//	is valid.
	//
	//-   `owners` (array of strings) - Filters the images by their owner. You
	//	may specify one or more AWS account IDs, "self" (which will use the
	//	account whose credentials you are using to run Packer), or an AWS owner
	//	alias: for example, "amazon", "aws-marketplace", or "microsoft". This
	//	option is required for security reasons.
	//
	//-   `most_recent` (boolean) - Selects the newest created image when true.
	//	This is most useful for selecting a daily distro build.
	//
	//You may set this in place of `source_ami` or in conjunction with it. If you
	//set this in conjunction with `source_ami`, the `source_ami` will be added
	//to the filter. The provided `source_ami` must meet all of the filtering
	//criteria provided in `source_ami_filter`; this pins the AMI returned by the
	//filter, but will cause Packer to fail if the `source_ami` does not exist.
	SourceAmiFilter awscommon.AmiFilterOptions `mapstructure:"source_ami_filter" required:"false"`
	// Key/value pair tags to apply to the volumes that are *launched*. This is
	// a [template engine](/packer/docs/templates/legacy_json_templates/engine), see [Build template
	// data](#build-template-data) for more information.
	RootVolumeTags map[string]string `mapstructure:"root_volume_tags" required:"false"`
	// Same as [`root_volume_tags`](#root_volume_tags) but defined as a
	// singular block containing a `key` and a `value` field. In HCL2 mode the
	// [`dynamic_block`](/packer/docs/templates/hcl_templates/expressions#dynamic-blocks)
	// will allow you to create those programatically.
	RootVolumeTag config.KeyValues `mapstructure:"root_volume_tag" required:"false"`
	// Whether or not to encrypt the volumes that are *launched*. By default, Packer will keep
	// the encryption setting to what it was in the source image when set to `false`. Setting true will
	// always result in an encrypted one.
	RootVolumeEncryptBoot config.Trilean `mapstructure:"root_volume_encrypt_boot" required:"false"`
	// ID, alias or ARN of the KMS key to use for *launched* volumes encryption.
	//
	// Set this value if you select `root_volume_encrypt_boot`, but don't want to use the
	// region's default KMS key.
	//
	// If you have a custom kms key you'd like to apply to the launch volume,
	// and are only building in one region, it is more efficient to set this
	// and `root_volume_encrypt_boot` to `true` and not use `encrypt_boot` and `kms_key_id`. This saves
	// potentially many minutes at the end of the build by preventing Packer
	// from having to copy and re-encrypt the image at the end of the build.
	//
	// For valid formats see *KmsKeyId* in the [AWS API docs -
	// CopyImage](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CopyImage.html).
	// This field is validated by Packer, when using an alias, you will have to
	// prefix `kms_key_id` with `alias/`.
	RootVolumeKmsKeyId string `mapstructure:"root_volume_kms_key_id" required:"false"`
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

	ctx interpolate.Context
}

func (c *Config) GetContext() interpolate.Context {
	return c.ctx
}

type wrappedCommandTemplate struct {
	Command string
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
				"snapshot_tags",
				"snapshot_tag",
				"tags",
				"tag",
				"root_volume_tags",
				"root_volume_tag",
				"command_wrapper",
				"post_mount_commands",
				"pre_mount_commands",
				"mount_path",
			},
		},
	}, raws...)
	if err != nil {
		return nil, nil, err
	}

	if b.config.Architecture == "" {
		b.config.Architecture = "x86_64"
	}

	if b.config.PackerConfig.PackerForce {
		b.config.AMIForceDeregister = true
	}

	// Defaults
	if b.config.ChrootMounts == nil {
		b.config.ChrootMounts = make([][]string, 0)
	}

	if len(b.config.ChrootMounts) == 0 {
		b.config.ChrootMounts = [][]string{
			{"proc", "proc", "/proc"},
			{"sysfs", "sysfs", "/sys"},
			{"bind", "/dev", "/dev"},
			{"devpts", "devpts", "/dev/pts"},
			{"binfmt_misc", "binfmt_misc", "/proc/sys/fs/binfmt_misc"},
		}
	}

	// set default copy file if we're not giving our own
	if b.config.CopyFiles == nil {
		if !b.config.FromScratch {
			b.config.CopyFiles = []string{"/etc/resolv.conf"}
		}
	}

	if b.config.CommandWrapper == "" {
		b.config.CommandWrapper = "{{.Command}}"
	}

	if b.config.MountPath == "" {
		b.config.MountPath = "/mnt/packer-amazon-chroot-volumes/{{.Device}}"
	}

	if b.config.MountPartition == "" {
		b.config.MountPartition = "1"
	}

	// Accumulate any errors or warnings
	var errs *packersdk.MultiError
	var warns []string

	errs = packersdk.MultiErrorAppend(errs, b.config.RootVolumeTag.CopyOn(&b.config.RootVolumeTags)...)
	errs = packersdk.MultiErrorAppend(errs, b.config.AccessConfig.Prepare(&b.config.PackerConfig)...)
	errs = packersdk.MultiErrorAppend(errs,
		b.config.AMIConfig.Prepare(&b.config.AccessConfig, &b.config.ctx)...)

	for _, mounts := range b.config.ChrootMounts {
		if len(mounts) != 3 {
			errs = packersdk.MultiErrorAppend(
				errs, errors.New("Each chroot_mounts entry should be three elements."))
			break
		}
	}

	if b.config.FromScratch {
		if b.config.SourceAmi != "" || !b.config.SourceAmiFilter.Empty() {
			warns = append(warns, "source_ami and source_ami_filter are unused when from_scratch is true")
		}
		if b.config.RootVolumeSize == 0 {
			errs = packersdk.MultiErrorAppend(
				errs, errors.New("root_volume_size is required with from_scratch."))
		}
		if len(b.config.PreMountCommands) == 0 {
			errs = packersdk.MultiErrorAppend(
				errs, errors.New("pre_mount_commands is required with from_scratch."))
		}
		if b.config.AMIVirtType == "" {
			errs = packersdk.MultiErrorAppend(
				errs, errors.New("ami_virtualization_type is required with from_scratch."))
		}
		if b.config.RootDeviceName == "" {
			errs = packersdk.MultiErrorAppend(
				errs, errors.New("root_device_name is required with from_scratch."))
		}
		if len(b.config.AMIMappings) == 0 {
			errs = packersdk.MultiErrorAppend(
				errs, errors.New("ami_block_device_mappings is required with from_scratch."))
		}
	} else {
		if b.config.SourceAmi == "" && b.config.SourceAmiFilter.Empty() {
			errs = packersdk.MultiErrorAppend(
				errs, errors.New("source_ami or source_ami_filter is required."))
		}
		if len(b.config.AMIMappings) > 0 && b.config.RootDeviceName != "" {
			if b.config.RootVolumeSize == 0 {
				// Although, they can specify the device size in the block
				// device mapping, it's easier to be specific here.
				errs = packersdk.MultiErrorAppend(
					errs, errors.New("root_volume_size is required if ami_block_device_mappings is specified"))
			}
			warns = append(warns, "ami_block_device_mappings from source image will be completely overwritten")
		} else if len(b.config.AMIMappings) > 0 {
			errs = packersdk.MultiErrorAppend(
				errs, errors.New("If ami_block_device_mappings is specified, root_device_name must be specified"))
		} else if b.config.RootDeviceName != "" {
			errs = packersdk.MultiErrorAppend(
				errs, errors.New("If root_device_name is specified, ami_block_device_mappings must be specified"))
		}

		if b.config.RootVolumeKmsKeyId != "" {
			if b.config.RootVolumeEncryptBoot.False() {
				errs = packersdk.MultiErrorAppend(
					errs, errors.New("If you have set root_volume_kms_key_id, root_volume_encrypt_boot must also be true."))
			} else if b.config.RootVolumeEncryptBoot.True() && !awscommon.ValidateKmsKey(b.config.RootVolumeKmsKeyId) {
				errs = packersdk.MultiErrorAppend(
					errs, fmt.Errorf("%q is not a valid KMS Key Id.", b.config.RootVolumeKmsKeyId))
			}
		}

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

	if b.config.TpmSupport != "" && b.config.TpmSupport != ec2.TpmSupportValuesV20 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(`The only valid tpm_support value is %q`, ec2.TpmSupportValuesV20))
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
	generatedData = append(generatedData, "Device", "MountPath")

	return generatedData, warns, nil
}

func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("The amazon-chroot builder only works on Linux environments.")
	}

	session, err := b.config.Session()
	if err != nil {
		return nil, err
	}
	ec2conn := ec2.New(session)

	wrappedCommand := func(command string) (string, error) {
		ictx := b.config.ctx
		ictx.Data = &wrappedCommandTemplate{Command: command}
		return interpolate.Render(b.config.CommandWrapper, &ictx)
	}

	// Setup the state bag and initial state for the steps
	state := new(multistep.BasicStateBag)
	state.Put("config", &b.config)
	state.Put("access_config", &b.config.AccessConfig)
	state.Put("ami_config", &b.config.AMIConfig)
	state.Put("ec2", ec2conn)
	state.Put("awsSession", session)
	state.Put("hook", hook)
	state.Put("ui", ui)
	state.Put("wrappedCommand", common.CommandWrapper(wrappedCommand))
	generatedData := &packerbuilderdata.GeneratedData{State: state}

	// Build the steps
	steps := []multistep.Step{
		&awscommon.StepPreValidate{
			DestAmiName:     b.config.AMIName,
			ForceDeregister: b.config.AMIForceDeregister,
		},
		&StepInstanceInfo{},
	}

	if !b.config.FromScratch {
		steps = append(steps,
			&awscommon.StepSourceAMIInfo{
				SourceAmi:                b.config.SourceAmi,
				EnableAMISriovNetSupport: b.config.AMISriovNetSupport,
				EnableAMIENASupport:      b.config.AMIENASupport,
				AmiFilters:               b.config.SourceAmiFilter,
				AMIVirtType:              b.config.AMIVirtType,
			},
			&StepCheckRootDevice{},
		)
	}

	steps = append(steps,
		&StepFlock{},
		&StepPrepareDevice{
			GeneratedData: generatedData,
		},
		&StepCreateVolume{
			PollingConfig:         b.config.PollingConfig,
			RootVolumeType:        b.config.RootVolumeType,
			RootVolumeSize:        b.config.RootVolumeSize,
			RootVolumeTags:        b.config.RootVolumeTags,
			RootVolumeEncryptBoot: b.config.RootVolumeEncryptBoot,
			RootVolumeKmsKeyId:    b.config.RootVolumeKmsKeyId,
			Ctx:                   b.config.ctx,
		},
		&StepAttachVolume{
			PollingConfig: b.config.PollingConfig,
		},
		&StepEarlyUnflock{},
		&chroot.StepPreMountCommands{
			Commands: b.config.PreMountCommands,
		},
		&StepMountDevice{
			MountOptions:   b.config.MountOptions,
			MountPartition: b.config.MountPartition,
			GeneratedData:  generatedData,
		},
		&chroot.StepPostMountCommands{
			Commands: b.config.PostMountCommands,
		},
		&chroot.StepMountExtra{
			ChrootMounts: b.config.ChrootMounts,
		},
		&chroot.StepCopyFiles{
			Files: b.config.CopyFiles,
		},
		&awscommon.StepSetGeneratedData{
			GeneratedData: generatedData,
		},
		&chroot.StepChrootProvision{},
		&chroot.StepEarlyCleanup{},
		&StepSnapshot{
			PollingConfig: b.config.PollingConfig,
		},
		&awscommon.StepDeregisterAMI{
			AccessConfig:        &b.config.AccessConfig,
			ForceDeregister:     b.config.AMIForceDeregister,
			ForceDeleteSnapshot: b.config.AMIForceDeleteSnapshot,
			AMIName:             b.config.AMIName,
			Regions:             b.config.AMIRegions,
		},
		&StepRegisterAMI{
			RootVolumeSize:           b.config.RootVolumeSize,
			EnableAMISriovNetSupport: b.config.AMISriovNetSupport,
			EnableAMIENASupport:      b.config.AMIENASupport,
			AMISkipBuildRegion:       b.config.AMISkipBuildRegion,
			PollingConfig:            b.config.PollingConfig,
			BootMode:                 b.config.BootMode,
			UefiData:                 b.config.UefiData,
			TpmSupport:               b.config.TpmSupport,
		},
		&awscommon.StepAMIRegionCopy{
			AccessConfig:                   &b.config.AccessConfig,
			Regions:                        b.config.AMIRegions,
			AMIKmsKeyId:                    b.config.AMIKmsKeyId,
			RegionKeyIds:                   b.config.AMIRegionKMSKeyIDs,
			EncryptBootVolume:              b.config.AMIEncryptBootVolume,
			Name:                           b.config.AMIName,
			OriginalRegion:                 *ec2conn.Config.Region,
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
	)

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
		Session:        session,
		StateData:      map[string]interface{}{"generated_data": state.Get("generated_data")},
	}

	return artifact, nil
}
