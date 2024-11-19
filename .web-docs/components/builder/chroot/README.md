Type: `amazon-chroot`
Artifact BuilderId: `mitchellh.amazon.chroot`

The `amazon-chroot` Packer builder is able to create Amazon AMIs backed by an
EBS volume as the root device. For more information on the difference between
instance storage and EBS-backed instances, see the ["storage for the root
device" section in the EC2
documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ComponentsAMIs.html#storage-for-the-root-device).

The difference between this builder and the `amazon-ebs` builder is that this
builder is able to build an EBS-backed AMI without launching a new EC2
instance. This can dramatically speed up AMI builds for organizations who need
the extra fast build.

~> **This is an advanced builder** If you're just getting started with
Packer, we recommend starting with the [amazon-ebs
builder](/packer/integrations/hashicorp/amazon/latest/components/builder/ebs), which is much easier to use.

The builder does _not_ manage AMIs. Once it creates an AMI and stores it in
your account, it is up to you to use, delete, etc., the AMI.

## How Does it Work?

This builder works by creating a new EBS volume from an existing source AMI and
attaching it into an already-running EC2 instance. Once attached, a
[chroot](https://en.wikipedia.org/wiki/Chroot) is used to provision the system
within that volume. After provisioning, the volume is detached, snapshotted,
and an AMI is made.

Using this process, minutes can be shaved off the AMI creation process because
a new EC2 instance doesn't need to be launched.

There are some restrictions, however. The host EC2 instance where the volume is
attached to must be a similar system (generally the same OS version, kernel
versions, etc.) as the AMI being built. Additionally, this process is much more
expensive because the EC2 instance must be kept running persistently in order
to build AMIs, whereas the other AMI builders start instances on-demand to
build AMIs as needed.

## Chroot Specific Configuration Reference

There are many configuration options available for the builder. In addition to
the items listed here, you will want to look at the general configuration
references for [AMI](#ami-configuration),
[BlockDevices](#block-devices-configuration) and
[Access](#access-config-configuration) configuration references, which are
necessary for this build to succeed and can be found further down the page.

### Required:

<!-- Code generated from the comments of the Config struct in builder/chroot/builder.go; DO NOT EDIT MANUALLY -->

- `source_ami` (string) - The source AMI whose root volume will be copied and provisioned on the
  currently running instance. This must be an EBS-backed AMI with a root
  volume snapshot that you have access to. Note: this is not used when
  from_scratch is set to true.

<!-- End of code generated from the comments of the Config struct in builder/chroot/builder.go; -->


### Optional:

<!-- Code generated from the comments of the Config struct in builder/chroot/builder.go; DO NOT EDIT MANUALLY -->

- `ami_block_device_mappings` (awscommon.BlockDevices) - Add one or more [block device
  mappings](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/block-device-mapping-concepts.html)
  to the AMI. If this field is populated, and you are building from an
  existing source image, the block device mappings in the source image
  will be overwritten. This means you must have a block device mapping
  entry for your root volume, `root_volume_size` and `root_device_name`.
  See the [BlockDevices](#block-devices-configuration) documentation for
  fields.

- `chroot_mounts` ([][]string) - This is a list of devices to mount into the chroot environment. This
  configuration parameter requires some additional documentation which is
  in the Chroot Mounts section. Please read that section for more
  information on how to use this.

- `command_wrapper` (string) - How to run shell commands. This defaults to `{{.Command}}`. This may be
  useful to set if you want to set environmental variables or perhaps run
  it with sudo or so on. This is a configuration template where the
  .Command variable is replaced with the command to be run. Defaults to
  `{{.Command}}`.

- `copy_files` ([]string) - Paths to files on the running EC2 instance that will be copied into the
  chroot environment prior to provisioning. Defaults to /etc/resolv.conf
  so that DNS lookups work. Pass an empty list to skip copying
  /etc/resolv.conf. You may need to do this if you're building an image
  that uses systemd.

- `device_path` (string) - The path to the device where the root volume of the source AMI will be
  attached. This defaults to "" (empty string), which forces Packer to
  find an open device automatically.

- `nvme_device_path` (string) - When we call the mount command (by default mount -o device dir), the
  string provided in nvme_mount_path will replace device in that command.
  When this option is not set, device in that command will be something
  like /dev/sdf1, mirroring the attached device name. This assumption
  works for most instances but will fail with c5 and m5 instances. In
  order to use the chroot builder with c5 and m5 instances, you must
  manually set nvme_device_path and device_path.

- `from_scratch` (bool) - Build a new volume instead of starting from an existing AMI root volume
  snapshot. Default false. If true, source_ami/source_ami_filter are no
  longer used and the following options become required:
  ami_virtualization_type, pre_mount_commands and root_volume_size.

- `mount_options` ([]string) - Options to supply the mount command when mounting devices. Each option
  will be prefixed with -o and supplied to the mount command ran by
  Packer. Because this command is ran in a shell, user discretion is
  advised. See this manual page for the mount command for valid file
  system specific options.

- `mount_partition` (string) - The partition number containing the / partition. By default this is the
  first partition of the volume, (for example, xvda1) but you can
  designate the entire block device by setting "mount_partition": "0" in
  your config, which will mount xvda instead.

- `mount_path` (string) - The path where the volume will be mounted. This is where the chroot
  environment will be. This defaults to
  `/mnt/packer-amazon-chroot-volumes/{{.Device}}`. This is a configuration
  template where the .Device variable is replaced with the name of the
  device where the volume is attached.

- `post_mount_commands` ([]string) - As pre_mount_commands, but the commands are executed after mounting the
  root device and before the extra mount and copy steps. The device and
  mount path are provided by `{{.Device}}` and `{{.MountPath}}`.

- `pre_mount_commands` ([]string) - A series of commands to execute after attaching the root volume and
  before mounting the chroot. This is not required unless using
  from_scratch. If so, this should include any partitioning and filesystem
  creation commands. The path to the device is provided by `{{.Device}}`.

- `root_device_name` (string) - The root device name. For example, xvda.

- `root_volume_size` (int64) - The size of the root volume in GB for the chroot environment and the
  resulting AMI. Default size is the snapshot size of the source_ami
  unless from_scratch is true, in which case this field must be defined.

- `root_volume_type` (string) - The type of EBS volume for the chroot environment and resulting AMI. The
  default value is the type of the source_ami, unless from_scratch is
  true, in which case the default value is gp2. You can only specify io1
  if building based on top of a source_ami which is also io1.

- `source_ami_filter` (awscommon.AmiFilterOptions) - Filters used to populate the source_ami field. Example:
  
  ```json
  {
  	 "source_ami_filter": {
  	 "filters": {
  	  "virtualization-type": "hvm",
  	  "name": "ubuntu/images/*ubuntu-xenial-16.04-amd64-server-*",
  	  "root-device-type": "ebs"
  	},
  	"owners": ["099720109477"],
  	"most_recent": true
  	 }
  }
  ```
  
  This selects the most recent Ubuntu 16.04 HVM EBS AMI from Canonical. NOTE:
  This will fail unless *exactly* one AMI is returned. In the above example,
  `most_recent` will cause this to succeed by selecting the newest image.
  
  -   `filters` (map[string,string] | multiple filters are allowed when seperated by commas) -
   filters used to select a `source_ami`.
  	NOTE: This will fail unless *exactly* one AMI is returned. Any filter
  	described in the docs for
  	[DescribeImages](http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeImages.html)
  	is valid.
  
  -   `owners` (array of strings) - Filters the images by their owner. You
  	may specify one or more AWS account IDs, "self" (which will use the
  	account whose credentials you are using to run Packer), or an AWS owner
  	alias: for example, "amazon", "aws-marketplace", or "microsoft". This
  	option is required for security reasons.
  
  -   `most_recent` (boolean) - Selects the newest created image when true.
  	This is most useful for selecting a daily distro build.
  
  You may set this in place of `source_ami` or in conjunction with it. If you
  set this in conjunction with `source_ami`, the `source_ami` will be added
  to the filter. The provided `source_ami` must meet all of the filtering
  criteria provided in `source_ami_filter`; this pins the AMI returned by the
  filter, but will cause Packer to fail if the `source_ami` does not exist.

- `root_volume_tags` (map[string]string) - Key/value pair tags to apply to the volumes that are *launched*. This is
  a [template engine](/packer/docs/templates/legacy_json_templates/engine), see [Build template
  data](#build-template-data) for more information.

- `root_volume_tag` ([]{key string, value string}) - Same as [`root_volume_tags`](#root_volume_tags) but defined as a
  singular block containing a `key` and a `value` field. In HCL2 mode the
  [`dynamic_block`](/packer/docs/templates/hcl_templates/expressions#dynamic-blocks)
  will allow you to create those programatically.

- `root_volume_encrypt_boot` (boolean) - Whether or not to encrypt the volumes that are *launched*. By default, Packer will keep
  the encryption setting to what it was in the source image when set to `false`. Setting true will
  always result in an encrypted one.

- `root_volume_kms_key_id` (string) - ID, alias or ARN of the KMS key to use for *launched* volumes encryption.
  
  Set this value if you select `root_volume_encrypt_boot`, but don't want to use the
  region's default KMS key.
  
  If you have a custom kms key you'd like to apply to the launch volume,
  and are only building in one region, it is more efficient to set this
  and `root_volume_encrypt_boot` to `true` and not use `encrypt_boot` and `kms_key_id`. This saves
  potentially many minutes at the end of the build by preventing Packer
  from having to copy and re-encrypt the image at the end of the build.
  
  For valid formats see *KmsKeyId* in the [AWS API docs -
  CopyImage](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CopyImage.html).
  This field is validated by Packer, when using an alias, you will have to
  prefix `kms_key_id` with `alias/`.

- `ami_architecture` (string) - what architecture to use when registering the final AMI; valid options
  are "arm64", "arm64_mac", "i386", "x86_64", or "x86_64_mac". Defaults to "x86_64".

- `boot_mode` (string) - The boot mode. Valid options are `legacy-bios` and `uefi`. See the documentation on
  [boot modes](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ami-boot.html) for
  more information. Defaults to `legacy-bios` when `ami_architecture` is `x86_64` and
  `uefi` when `ami_architecture` is `arm64`.

- `uefi_data` (string) - Base64 representation of the non-volatile UEFI variable store. For more information
  see [AWS documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/uefi-secure-boot-optionB.html).

- `tpm_support` (string) - NitroTPM Support. Valid options are `v2.0`. See the documentation on
  [NitroTPM Support](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/enable-nitrotpm-support-on-ami.html) for
  more information. Only enabled if a valid option is provided, otherwise ignored.

<!-- End of code generated from the comments of the Config struct in builder/chroot/builder.go; -->


## General Common Configuration Reference

Following will be a set of fields that are also settable for other aws
builders.

### AMI Configuration

#### Required:

<!-- Code generated from the comments of the AMIConfig struct in builder/common/ami_config.go; DO NOT EDIT MANUALLY -->

- `ami_name` (string) - The name of the resulting AMI that will appear when managing AMIs in the
  AWS console or via APIs. This must be unique. To help make this unique,
  use a function like timestamp (see [template
  engine](/packer/docs/templates/legacy_json_templates/engine) for more info).

<!-- End of code generated from the comments of the AMIConfig struct in builder/common/ami_config.go; -->


#### Optional:

<!-- Code generated from the comments of the AMIConfig struct in builder/common/ami_config.go; DO NOT EDIT MANUALLY -->

- `ami_description` (string) - The description to set for the resulting
  AMI(s). By default this description is empty.  This is a
  [template engine](/packer/docs/templates/legacy_json_templates/engine), see [Build template
  data](#build-template-data) for more information.

- `ami_virtualization_type` (string) - The type of virtualization for the AMI
  you are building. This option is required to register HVM images. Can be
  paravirtual (default) or hvm.

- `ami_users` ([]string) - A list of account IDs that have access to
  launch the resulting AMI(s). By default no additional users other than the
  user creating the AMI has permissions to launch it.

- `ami_groups` ([]string) - A list of groups that have access to
  launch the resulting AMI(s). By default no groups have permission to launch
  the AMI. `all` will make the AMI publicly accessible.
  AWS currently doesn't accept any value other than "all"

- `ami_org_arns` ([]string) - A list of Amazon Resource Names (ARN) of AWS Organizations that have access to
  launch the resulting AMI(s). By default no organizations have permission to launch
  the AMI.

- `ami_ou_arns` ([]string) - A list of Amazon Resource Names (ARN) of AWS Organizations organizational units (OU) that have access to
  launch the resulting AMI(s). By default no organizational units have permission to launch
  the AMI.

- `ami_product_codes` ([]string) - A list of product codes to
  associate with the AMI. By default no product codes are associated with the
  AMI.

- `ami_regions` ([]string) - A list of regions to copy the AMI to.
  Tags and attributes are copied along with the AMI. AMI copying takes time
  depending on the size of the AMI, but will generally take many minutes.

- `skip_region_validation` (bool) - Set to true if you want to skip
  validation of the ami_regions configuration option. Default false.

- `tags` (map[string]string) - Key/value pair tags applied to the AMI. This is a [template
  engine](/packer/docs/templates/legacy_json_templates/engine), see [Build template
  data](#build-template-data) for more information.
  
  The builder no longer adds a "Name": "Packer Builder" entry to the tags.

- `tag` ([]{key string, value string}) - Same as [`tags`](#tags) but defined as a singular repeatable block
  containing a `key` and a `value` field. In HCL2 mode the
  [`dynamic_block`](/packer/docs/templates/hcl_templates/expressions#dynamic-blocks)
  will allow you to create those programatically.

- `ena_support` (boolean) - Enable enhanced networking (ENA but not SriovNetSupport) on
  HVM-compatible AMIs. If set, add `ec2:ModifyInstanceAttribute` to your
  AWS IAM policy.
  
  Note: you must make sure enhanced networking is enabled on your
  instance. See [Amazon's documentation on enabling enhanced
  networking](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/enhanced-networking.html#enabling_enhanced_networking).

- `sriov_support` (bool) - Enable enhanced networking (SriovNetSupport but not ENA) on
  HVM-compatible AMIs. If true, add `ec2:ModifyInstanceAttribute` to your
  AWS IAM policy. Note: you must make sure enhanced networking is enabled
  on your instance. See [Amazon's documentation on enabling enhanced
  networking](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/enhanced-networking.html#enabling_enhanced_networking).
  Default `false`.

- `force_deregister` (bool) - Force Packer to first deregister an existing
  AMI if one with the same name already exists. Default false.

- `force_delete_snapshot` (bool) - Force Packer to delete snapshots
  associated with AMIs, which have been deregistered by force_deregister.
  Default false.

- `encrypt_boot` (boolean) - Whether or not to encrypt the resulting AMI when
  copying a provisioned instance to an AMI. By default, Packer will keep
  the encryption setting to what it was in the source image. Setting false
  will result in an unencrypted image, and true will result in an encrypted
  one.
  
  If you have used the `launch_block_device_mappings` to set an encryption
  key and that key is the same as the one you want the image encrypted with
  at the end, then you don't need to set this field; leaving it empty will
  prevent an unnecessary extra copy step and save you some time.
  
  Please note that if you are using an account with the global "Always
  encrypt new EBS volumes" option set to `true`, Packer will be unable to
  override this setting, and the final image will be encrypted whether
  you set this value or not.

- `kms_key_id` (string) - ID, alias or ARN of the KMS key to use for AMI encryption. This
  only applies to the main `region` -- any regions the AMI gets copied to
  will be encrypted by the default EBS KMS key for that region,
  unless you set region-specific keys in `region_kms_key_ids`.
  
  Set this value if you select `encrypt_boot`, but don't want to use the
  region's default KMS key.
  
  If you have a custom kms key you'd like to apply to the launch volume,
  and are only building in one region, it is more efficient to leave this
  and `encrypt_boot` empty and to instead set the key id in the
  launch_block_device_mappings (you can find an example below). This saves
  potentially many minutes at the end of the build by preventing Packer
  from having to copy and re-encrypt the image at the end of the build.
  
  For valid formats see *KmsKeyId* in the [AWS API docs -
  CopyImage](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CopyImage.html).
  This field is validated by Packer, when using an alias, you will have to
  prefix `kms_key_id` with `alias/`.

- `region_kms_key_ids` (map[string]string) - regions to copy the ami to, along with the custom kms key id (alias or
  arn) to use for encryption for that region. Keys must match the regions
  provided in `ami_regions`. If you just want to encrypt using a default
  ID, you can stick with `kms_key_id` and `ami_regions`. If you want a
  region to be encrypted with that region's default key ID, you can use an
  empty string `""` instead of a key id in this map. (e.g. `"us-east-1":
  ""`) However, you cannot use default key IDs if you are using this in
  conjunction with `snapshot_users` -- in that situation you must use
  custom keys. For valid formats see *KmsKeyId* in the [AWS API docs -
  CopyImage](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CopyImage.html).
  
  This option supercedes the `kms_key_id` option -- if you set both, and
  they are different, Packer will respect the value in
  `region_kms_key_ids` for your build region and silently disregard the
  value provided in `kms_key_id`.

- `skip_save_build_region` (bool) - If true, Packer will not check whether an AMI with the `ami_name` exists
  in the region it is building in. It will use an intermediary AMI name,
  which it will not convert to an AMI in the build region. It will copy
  the intermediary AMI into any regions provided in `ami_regions`, then
  delete the intermediary AMI. Default `false`.

- `imds_support` (string) - Enforce version of the Instance Metadata Service on the built AMI.
  Valid options are unset (legacy) and `v2.0`. See the documentation on
  [IMDS](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html)
  for more information. Defaults to legacy.

- `deprecate_at` (string) - The date and time to deprecate the AMI, in UTC, in the following format: YYYY-MM-DDTHH:MM:SSZ.
  If you specify a value for seconds, Amazon EC2 rounds the seconds to the nearest minute.
  You can’t specify a date in the past. The upper limit for DeprecateAt is 10 years from now.

- `deregistration_protection` (DeregistrationProtectionOptions) - Enable AMI deregistration protection. See
  [DeregistrationProtectionOptions](#deregistration-protection-options) below for more
  details on all of the options available, and for a usage example.

<!-- End of code generated from the comments of the AMIConfig struct in builder/common/ami_config.go; -->


<!-- Code generated from the comments of the SnapshotConfig struct in builder/common/snapshot_config.go; DO NOT EDIT MANUALLY -->

- `snapshot_tags` (map[string]string) - Key/value pair tags to apply to snapshot. They will override AMI tags if
  already applied to snapshot. This is a [template
  engine](/packer/docs/templates/legacy_json_templates/engine), see [Build template
  data](#build-template-data) for more information.

- `snapshot_tag` ([]{key string, value string}) - Same as [`snapshot_tags`](#snapshot_tags) but defined as a singular
  repeatable block containing a `key` and a `value` field. In HCL2 mode the
  [`dynamic_block`](/packer/docs/templates/hcl_templates/expressions#dynamic-blocks)
  will allow you to create those programatically.

- `snapshot_users` ([]string) - A list of account IDs that have
  access to create volumes from the snapshot(s). By default no additional
  users other than the user creating the AMI has permissions to create
  volumes from the backing snapshot(s).

- `snapshot_groups` ([]string) - A list of groups that have access to
  create volumes from the snapshot(s). By default no groups have permission
  to create volumes from the snapshot(s). all will make the snapshot
  publicly accessible.

<!-- End of code generated from the comments of the SnapshotConfig struct in builder/common/snapshot_config.go; -->


### Block Devices Configuration

Block devices can be nested in the
[ami_block_device_mappings](#ami_block_device_mappings) array.

<!-- Code generated from the comments of the BlockDevice struct in builder/common/block_device.go; DO NOT EDIT MANUALLY -->

These will be attached when launching your instance. Your
options here may vary depending on the type of VM you use.

Example use case:

The following mapping will tell Packer to encrypt the root volume of the
build instance at launch using a specific non-default kms key:

HCL2 example:

```hcl

	launch_block_device_mappings {
	    device_name = "/dev/sda1"
	    encrypted = true
	    kms_key_id = "1a2b3c4d-5e6f-1a2b-3c4d-5e6f1a2b3c4d"
	}

```

JSON example:
```json
"launch_block_device_mappings": [

	{
	   "device_name": "/dev/sda1",
	   "encrypted": true,
	   "kms_key_id": "1a2b3c4d-5e6f-1a2b-3c4d-5e6f1a2b3c4d"
	}

]
```

Please note that the kms_key_id option in this example exists for
launch_block_device_mappings but not ami_block_device_mappings.

Documentation for Block Devices Mappings can be found here:
https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/block-device-mapping-concepts.html

<!-- End of code generated from the comments of the BlockDevice struct in builder/common/block_device.go; -->


#### Optional:

<!-- Code generated from the comments of the BlockDevice struct in builder/common/block_device.go; DO NOT EDIT MANUALLY -->

- `delete_on_termination` (bool) - Indicates whether the EBS volume is deleted on instance termination.
  Default false. NOTE: If this value is not explicitly set to true and
  volumes are not cleaned up by an alternative method, additional volumes
  will accumulate after every build.

- `device_name` (string) - The device name exposed to the instance (for example, /dev/sdh or xvdh).
  Required for every device in the block device mapping.

- `encrypted` (boolean) - Indicates whether or not to encrypt the volume. By default, Packer will
  keep the encryption setting to what it was in the source image. Setting
  false will result in an unencrypted device, and true will result in an
  encrypted one.

- `iops` (\*int64) - The number of I/O operations per second (IOPS) that the volume supports.
  See the documentation on
  [IOPs](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_EbsBlockDevice.html)
  for more information

- `no_device` (bool) - Suppresses the specified device included in the block device mapping of
  the AMI.

- `snapshot_id` (string) - The ID of the snapshot.

- `throughput` (\*int64) - The throughput for gp3 volumes, only valid for gp3 types
  See the documentation on
  [Throughput](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_EbsBlockDevice.html)
  for more information

- `virtual_name` (string) - The virtual device name. See the documentation on
  [Block Device Mapping](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/block-device-mapping-concepts.html)
  for more information.
  
  Note: virtual_name only applies for ephemeral (instance) volumes. Any
  EBS-backed volume will have a `snapshot_id` instead.
  
  The volume virtual_name should be in the `ephemeral[0-23]` form, e.g. ephemeral1

- `volume_type` (string) - The volume type. gp2 & gp3 for General Purpose (SSD) volumes, io1 & io2
  for Provisioned IOPS (SSD) volumes, st1 for Throughput Optimized HDD,
  sc1 for Cold HDD, and standard for Magnetic volumes.

- `volume_size` (int64) - The size of the volume, in GiB. Required if not specifying a
  snapshot_id.

- `kms_key_id` (string) - ID, alias or ARN of the KMS key to use for boot volume encryption.
  This option exists for launch_block_device_mappings but not
  ami_block_device_mappings. The kms key id defined here only applies to
  the original build region; if the AMI gets copied to other regions, the
  volume in those regions will be encrypted by the default EBS KMS key.
  For valid formats see KmsKeyId in the [AWS API docs -
  CopyImage](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CopyImage.html)
  This field is validated by Packer. When using an alias, you will have to
  prefix kms_key_id with alias/.

<!-- End of code generated from the comments of the BlockDevice struct in builder/common/block_device.go; -->


### Access Config Configuration

#### Required:

<!-- Code generated from the comments of the AccessConfig struct in builder/common/access_config.go; DO NOT EDIT MANUALLY -->

- `access_key` (string) - The access key used to communicate with AWS. [Learn how  to set this](/packer/integrations/hashicorp/amazon#specifying-amazon-credentials).
  On EBS, this is not required if you are using `use_vault_aws_engine`
  for authentication instead.

- `region` (string) - The name of the region, such as `us-east-1`, in which
  to launch the EC2 instance to create the AMI.
  When chroot building, this value is guessed from environment.

- `secret_key` (string) - The secret key used to communicate with AWS. [Learn how to set
  this](/packer/integrations/hashicorp/amazon#specifying-amazon-credentials). This is not required
  if you are using `use_vault_aws_engine` for authentication instead.

<!-- End of code generated from the comments of the AccessConfig struct in builder/common/access_config.go; -->


#### Optional:

<!-- Code generated from the comments of the AccessConfig struct in builder/common/access_config.go; DO NOT EDIT MANUALLY -->

- `assume_role` (AssumeRoleConfig) - If provided with a role ARN, Packer will attempt to assume this role
  using the supplied credentials. See
  [AssumeRoleConfig](#assume-role-configuration) below for more
  details on all of the options available, and for a usage example.

- `custom_endpoint_ec2` (string) - This option is useful if you use a cloud
  provider whose API is compatible with aws EC2. Specify another endpoint
  like this https://ec2.custom.endpoint.com.

- `shared_credentials_file` (string) - Path to a credentials file to load credentials from

- `decode_authorization_messages` (bool) - Enable automatic decoding of any encoded authorization (error) messages
  using the `sts:DecodeAuthorizationMessage` API. Note: requires that the
  effective user/role have permissions to `sts:DecodeAuthorizationMessage`
  on resource `*`. Default `false`.

- `insecure_skip_tls_verify` (bool) - This allows skipping TLS
  verification of the AWS EC2 endpoint. The default is false.

- `max_retries` (int) - This is the maximum number of times an API call is retried, in the case
  where requests are being throttled or experiencing transient failures.
  The delay between the subsequent API calls increases exponentially.

- `mfa_code` (string) - The MFA
  [TOTP](https://en.wikipedia.org/wiki/Time-based_One-time_Password_Algorithm)
  code. This should probably be a user variable since it changes all the
  time.

- `profile` (string) - The profile to use in the shared credentials file for
  AWS. See Amazon's documentation on [specifying
  profiles](https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#specifying-profiles)
  for more details.

- `skip_metadata_api_check` (bool) - Skip Metadata Api Check

- `skip_credential_validation` (bool) - Set to true if you want to skip validating AWS credentials before runtime.

- `token` (string) - The access token to use. This is different from the
  access key and secret key. If you're not sure what this is, then you
  probably don't need it. This will also be read from the AWS_SESSION_TOKEN
  environmental variable.

- `vault_aws_engine` (VaultAWSEngineOptions) - Get credentials from HashiCorp Vault's aws secrets engine. You must
  already have created a role to use. For more information about
  generating credentials via the Vault engine, see the [Vault
  docs.](https://www.vaultproject.io/api/secret/aws#generate-credentials)
  If you set this flag, you must also set the below options:
  -   `name` (string) - Required. Specifies the name of the role to generate
      credentials against. This is part of the request URL.
  -   `engine_name` (string) - The name of the aws secrets engine. In the
      Vault docs, this is normally referred to as "aws", and Packer will
      default to "aws" if `engine_name` is not set.
  -   `role_arn` (string)- The ARN of the role to assume if credential\_type
      on the Vault role is assumed\_role. Must match one of the allowed role
      ARNs in the Vault role. Optional if the Vault role only allows a single
      AWS role ARN; required otherwise.
  -   `ttl` (string) - Specifies the TTL for the use of the STS token. This
      is specified as a string with a duration suffix. Valid only when
      credential\_type is assumed\_role or federation\_token. When not
      specified, the default\_sts\_ttl set for the role will be used. If that
      is also not set, then the default value of 3600s will be used. AWS
      places limits on the maximum TTL allowed. See the AWS documentation on
      the DurationSeconds parameter for AssumeRole (for assumed\_role
      credential types) and GetFederationToken (for federation\_token
      credential types) for more details.
  
  HCL2 example:
  
  ```hcl
  vault_aws_engine {
      name = "myrole"
      role_arn = "myarn"
      ttl = "3600s"
  }
  ```
  
  JSON example:
  
  ```json
  {
      "vault_aws_engine": {
          "name": "myrole",
          "role_arn": "myarn",
          "ttl": "3600s"
      }
  }
  ```

- `aws_polling` (\*AWSPollingConfig) - [Polling configuration](#polling-configuration) for the AWS waiter. Configures the waiter that checks
  resource state.

<!-- End of code generated from the comments of the AccessConfig struct in builder/common/access_config.go; -->


### Assume Role Configuration

<!-- Code generated from the comments of the AssumeRoleConfig struct in builder/common/access_config.go; DO NOT EDIT MANUALLY -->

AssumeRoleConfig lets users set configuration options for assuming a special
role when executing Packer.

Usage example:

HCL config example:

```HCL

	source "amazon-ebs" "example" {
		assume_role {
			role_arn     = "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
			session_name = "SESSION_NAME"
			external_id  = "EXTERNAL_ID"
		}
	}

```

JSON config example:

```json

	builder{
		"type": "amazon-ebs",
		"assume_role": {
			"role_arn"    :  "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME",
			"session_name":  "SESSION_NAME",
			"external_id" :  "EXTERNAL_ID"
		}
	}

```

<!-- End of code generated from the comments of the AssumeRoleConfig struct in builder/common/access_config.go; -->


<!-- Code generated from the comments of the AssumeRoleConfig struct in builder/common/access_config.go; DO NOT EDIT MANUALLY -->

- `role_arn` (string) - Amazon Resource Name (ARN) of the IAM Role to assume.

- `duration_seconds` (int) - Number of seconds to restrict the assume role session duration.

- `external_id` (string) - The external ID to use when assuming the role. If omitted, no external
  ID is passed to the AssumeRole call.

- `policy` (string) - IAM Policy JSON describing further restricting permissions for the IAM
  Role being assumed.

- `policy_arns` ([]string) - Set of Amazon Resource Names (ARNs) of IAM Policies describing further
  restricting permissions for the IAM Role being

- `session_name` (string) - Session name to use when assuming the role.

- `tags` (map[string]string) - Map of assume role session tags.

- `transitive_tag_keys` ([]string) - Set of assume role session tag keys to pass to any subsequent sessions.

<!-- End of code generated from the comments of the AssumeRoleConfig struct in builder/common/access_config.go; -->


### Polling Configuration

<!-- Code generated from the comments of the AWSPollingConfig struct in builder/common/state.go; DO NOT EDIT MANUALLY -->

Polling configuration for the AWS waiter. Configures the waiter for resources creation or actions like attaching
volumes or importing image.

HCL2 example:
```hcl

	aws_polling {
		 delay_seconds = 30
		 max_attempts = 50
	}

```

JSON example:
```json

	"aws_polling" : {
		 "delay_seconds": 30,
		 "max_attempts": 50
	}

```

<!-- End of code generated from the comments of the AWSPollingConfig struct in builder/common/state.go; -->


<!-- Code generated from the comments of the AWSPollingConfig struct in builder/common/state.go; DO NOT EDIT MANUALLY -->

- `max_attempts` (int) - Specifies the maximum number of attempts the waiter will check for resource state.
  This value can also be set via the AWS_MAX_ATTEMPTS.
  If both option and environment variable are set, the max_attempts will be considered over the AWS_MAX_ATTEMPTS.
  If none is set, defaults to AWS waiter default which is 40 max_attempts.

- `delay_seconds` (int) - Specifies the delay in seconds between attempts to check the resource state.
  This value can also be set via the AWS_POLL_DELAY_SECONDS.
  If both option and environment variable are set, the delay_seconds will be considered over the AWS_POLL_DELAY_SECONDS.
  If none is set, defaults to AWS waiter default which is 15 seconds.

<!-- End of code generated from the comments of the AWSPollingConfig struct in builder/common/state.go; -->


### Deregistration Protection Options

<!-- Code generated from the comments of the DeregistrationProtectionOptions struct in builder/common/ami_config.go; DO NOT EDIT MANUALLY -->

DeregistrationProtectionOptions lets users set AMI deregistration protection

HCL2 example:

```hcl

	source "amazon-ebs" "basic-example" {
	  deregistration_protection {
	    enabled = true
	    with_cooldown = true
	  }
	}

```

JSON Example:

```json
"builders" [

	{
	  "type": "amazon-ebs",
	  "deregistration_protection": {
	    "enabled": true,
	    "with_cooldown": true
	  }
	}

]
```

[Protect an AMI from deregistration](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ami-deregistration-protection.html)
When deregistration protection is enabled, the AMI cannot be deregistered.
To allow the AMI to be deregistered, you must first disable deregistration protection.

<!-- End of code generated from the comments of the DeregistrationProtectionOptions struct in builder/common/ami_config.go; -->


<!-- Code generated from the comments of the DeregistrationProtectionOptions struct in builder/common/ami_config.go; DO NOT EDIT MANUALLY -->

- `enabled` (bool) - Enable AMI deregistration protection.
  To allow the AMI to be deregistered, you must first disable deregistration protection.

- `with_cooldown` (bool) - When you turn on deregistration protection on an AMI, you have the option to include a 24-hour cooldown period.
  This cooldown period is the time during which deregistration protection remains in effect after you turn it off.
  During this cooldown period, the AMI can’t be deregistered.
  When the cooldown period ends, the AMI can be deregistered.

<!-- End of code generated from the comments of the DeregistrationProtectionOptions struct in builder/common/ami_config.go; -->


## Basic Example

Here is a basic example. It is completely valid except for the access keys:

**HCL2**

```hcl
// To make Packer read these variables from the environment into the var object,
// set the environment variables to have the same name as the declared
// variables, with the prefix PKR_VAR_.

// There are other ways to [set variables](/packer/docs/templates/hcl_templates/variables#assigning-values-to-build-variables), including from a var
// file or as a command argument.

// export PKR_VAR_aws_access_key=$YOURKEY
variable "aws_access_key" {
  type = string
  // default = "hardcoded_key" // Not recommended !
}

// export PKR_VAR_aws_secret_key=$YOURSECRETKEY
variable "aws_secret_key" {
  type = string
  // default = "hardcoded_secret_key" // Not recommended !
}

source "amazon-chroot" "basic-example" {
  access_key = var.aws_access_key
  secret_key =  var.aws_secret_key
  ami_name = "example-chroot"
  source_ami = "ami-e81d5881"
}

build {
  sources = [
    "source.amazon-chroot.basic-example"
  ]
}
```

**JSON**

```json
{
  "type": "amazon-chroot",
  "access_key": "YOUR KEY HERE",
  "secret_key": "YOUR SECRET KEY HERE",
  "source_ami": "ami-e81d5881",
  "ami_name": "packer-amazon-chroot {{timestamp}}"
}
```


## Chroot Mounts

The `chroot_mounts` configuration can be used to mount specific devices within
the chroot. By default, the following additional mounts are added into the
chroot by Packer:

- `/proc` (proc)
- `/sys` (sysfs)
- `/dev` (bind to real `/dev`)
- `/dev/pts` (devpts)
- `/proc/sys/fs/binfmt_misc` (binfmt_misc)

These default mounts are usually good enough for anyone and are reasonable
defaults. However, if you want to change or add the mount points, you may using
the `chroot_mounts` configuration. Here is an example configuration which only
mounts `/proc` and `/dev`:

**HCL2**

```hcl
source "amazon-chroot" "basic-example" {
  // ... other builder options
  chroot_mounts = [
    ["proc", "proc", "/proc"],
    ["bind", "/dev", "/dev"]
  ]
}
```

**JSON**

```json
...
"builders": [{
  "type": "amazon-chroot"
  ...
  "chroot_mounts": [
    ["proc", "proc", "/proc"],
    ["bind", "/dev", "/dev"]
  ]
}]
```


`chroot_mounts` is a list of a 3-tuples of strings. The three components of the
3-tuple, in order, are:

- The filesystem type. If this is "bind", then Packer will properly bind the
  filesystem to another mount point.

- The source device.

- The mount directory.

## Parallelism

A quick note on parallelism: it is perfectly safe to run multiple _separate_
Packer processes with the `amazon-chroot` builder on the same EC2 instance. In
fact, this is recommended as a way to push the most performance out of your AMI
builds.

Packer properly obtains a process lock for the parallelism-sensitive parts of
its internals such as finding an available device.

## Gotchas

### Unmounting the Filesystem

One of the difficulties with using the chroot builder is that your provisioning
scripts must not leave any processes running or packer will be unable to
unmount the filesystem.

For debian based distributions you can setup a
[policy-rc.d](http://people.debian.org/~hmh/invokerc.d-policyrc.d-specification.txt)
file which will prevent packages installed by your provisioners from starting
services:

**HCL2**

```hcl
// ...
build {
  sources = [
    "source.amazon-chroot.basic-example"
  ]

  // Set policy
  provisioner "shell" {
    inline = [
        "echo '#!/bin/sh' > /usr/sbin/policy-rc.d",
        "echo 'exit 101' >> /usr/sbin/policy-rc.d",
        "chmod a+x /usr/sbin/policy-rc.d"
    ]
  }

  // Un-set policy
  provisioner "shell" {
    inline = ["rm -f /usr/sbin/policy-rc.d"]
  }
}
```

**JSON**

```json
"provisioners": [
  {
    "type": "shell",
    "inline": [
      "echo '#!/bin/sh' > /usr/sbin/policy-rc.d",
      "echo 'exit 101' >> /usr/sbin/policy-rc.d",
      "chmod a+x /usr/sbin/policy-rc.d"
    ]
  },
  {
    "type": "shell",
    "inline": ["rm -f /usr/sbin/policy-rc.d"]
  }
]
```


### Ansible provisioner

Running ansible against `amazon-chroot` requires changing the Ansible
connection to chroot and running Ansible as root/sudo.

### Using Instances with NVMe block devices.

In C5, C5d, M5, and i3.metal instances, EBS volumes are exposed as NVMe block
devices
[reference](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/nvme-ebs-volumes.html).
In order to correctly mount these devices, you have to do some extra legwork,
involving the `nvme_device_path` option above. Read that for more information.

A working example for mounting an NVMe device is below:

**HCL2**

```hcl
// export PKR_VAR_aws_access_key=$YOURKEY
variable "aws_access_key" {
  type = string
}

// export PKR_VAR_aws_secret_key=$YOURSECRETKEY
variable "aws_secret_key" {
  type = string
}

data "amazon-ami" "example" {
  filters = {
    virtualization-type = "hvm"
    name                = "amzn-ami-hvm-*"
    root-device-type    = "ebs"
  }
  owners      = ["137112412989"]
  most_recent = true

  # Access Configuration
  region      = "us-east-1"
  access_key = var.aws_access_key
  secret_key = var.aws_secret_key
}

source "amazon-chroot" "basic-example" {
  access_key = var.aws_access_key
  secret_key = var.aws_secret_key
  region     = "us-east-1"
  source_ami = data.amazon-ami.example.id
  ena_support = true
  ami_name = "amazon-chroot-test-{{timestamp}}"
  nvme_device_path = "/dev/nvme1n1p"
  device_path = "/dev/sdf"
}

build {
  sources = [
    "source.amazon-chroot.basic-example"
  ]

  provisioner "shell" {
    inline = ["echo Test > /tmp/test.txt"]
  }
}
```

**JSON**

```json
{
  "variables": {
    "region": "us-east-2"
  },
  "builders": [
    {
      "type": "amazon-chroot",
      "region": "{{user `region`}}",
      "source_ami_filter": {
        "filters": {
          "virtualization-type": "hvm",
          "name": "amzn-ami-hvm-*",
          "root-device-type": "ebs"
        },
        "owners": ["137112412989"],
        "most_recent": true
      },
      "ena_support": true,
      "ami_name": "amazon-chroot-test-{{timestamp}}",
      "nvme_device_path": "/dev/nvme1n1p",
      "device_path": "/dev/sdf"
    }
  ],

  "provisioners": [
    {
      "type": "shell",
      "inline": ["echo Test > /tmp/test.txt"]
    }
  ]
}
```


Note that in the `nvme_device_path` you must end with the `p`; if you try to
define the partition in this path (e.g. `nvme_device_path`: `/dev/nvme1n1p1`)
and haven't also set the `"mount_partition": 0`, a `1` will be appended to the
`nvme_device_path` and Packer will fail.

## Building From Scratch

This example demonstrates the essentials of building an image from scratch. A
15G gp2 (SSD) device is created (overriding the default of standard/magnetic).
The device setup commands partition the device with one partition for use as an
HVM image and format it ext4. This builder block should be followed by
provisioning commands to install the os and bootloader.

**HCL2**

```hcl
// This example assumes that AWS_SECRET_ACCESS_KEY and AWS_ACCESS_KEY_ID are
// set in your environment, or a ~/.aws/credentials file is configured.
source "amazon-chroot" "basic-example" {
  region = "us-east-1"
  ami_name = "packer-from-scratch {{timestamp}}"
  from_scratch = true
  ami_virtualization_type = "hvm"
  pre_mount_commands = [
    "parted {{.Device}} mklabel msdos mkpart primary 1M 100% set 1 boot on print",
    "mkfs.ext4 {{.Device}}1"
  ]
  root_volume_size = 15
  root_device_name = "xvda"
  ami_block_device_mappings {
    device_name = "xvda"
    delete_on_termination = true
    volume_type = "gp2"
  }

}

build {
  sources = [
    "source.amazon-chroot.basic-example"
  ]

  provisioner "shell" {
    inline = [
        "echo '#!/bin/sh' > /usr/sbin/policy-rc.d",
        "echo 'exit 101' >> /usr/sbin/policy-rc.d",
        "chmod a+x /usr/sbin/policy-rc.d"
    ]
  }

  provisioner "shell" {
    inline = ["rm -f /usr/sbin/policy-rc.d"]
  }
}
```

**JSON**

```json
{
  "type": "amazon-chroot",
  "ami_name": "packer-from-scratch {{timestamp}}",
  "from_scratch": true,
  "ami_virtualization_type": "hvm",
  "pre_mount_commands": [
    "parted {{.Device}} mklabel msdos mkpart primary 1M 100% set 1 boot on print",
    "mkfs.ext4 {{.Device}}1"
  ],
  "root_volume_size": 15,
  "root_device_name": "xvda",
  "ami_block_device_mappings": [
    {
      "device_name": "xvda",
      "delete_on_termination": true,
      "volume_type": "gp2"
    }
  ]
}
```


## Build template data

In configuration directives marked as a template engine above, the following
variables are available:

- `BuildRegion` - The region (for example `eu-central-1`) where Packer is
  building the AMI.
- `SourceAMI` - The source AMI ID (for example `ami-a2412fcd`) used to build
  the AMI.
- `SourceAMICreationDate` - The source AMI creation date (for example `"2020-05-14T19:26:34.000Z"`).
- `SourceAMIName` - The source AMI Name (for example
  `ubuntu/images/ebs-ssd/ubuntu-xenial-16.04-amd64-server-20180306`) used to
  build the AMI.
- `SourceAMIOwner` - The source AMI owner ID.
- `SourceAMIOwnerName` - The source AMI owner alias/name (for example `amazon`).
- `SourceAMITags` - The source AMI Tags, as a `map[string]string` object.

## Build Shared Information Variables

This builder generates data that are shared with provisioner and post-processor via build function of [template engine](/packer/docs/templates/legacy_json_templates/engine) for JSON and [contextual variables](/packer/docs/templates/hcl_templates/contextual-variables) for HCL2.

The generated variables available for this builder are:

- `BuildRegion` - The region (for example `eu-central-1`) where Packer is
  building the AMI.
- `SourceAMI` - The source AMI ID (for example `ami-a2412fcd`) used to build
  the AMI.
- `SourceAMICreationDate` - The source AMI creation date (for example `"2020-05-14T19:26:34.000Z"`).
- `SourceAMIName` - The source AMI Name (for example
  `ubuntu/images/ebs-ssd/ubuntu-xenial-16.04-amd64-server-20180306`) used to
  build the AMI.
- `SourceAMIOwner` - The source AMI owner ID.
- `SourceAMIOwnerName` - The source AMI owner alias/name (for example `amazon`).
- `Device` - Root device path.
- `MountPath` - Device mounting path.

Usage example:

**HCL2**

```hcl
// When accessing one of these variables from inside the builder, you need to
// use the golang templating syntax. This is due to an architectural quirk that
// won't be easily resolvable until legacy json templates are deprecated:

{
source "amazon-ebs" "basic-example" {
  tags = {
        OS_Version = "Ubuntu"
        Release = "Latest"
        Base_AMI_ID = "{{ .SourceAMI }}"
        Base_AMI_Name = "{{ .SourceAMIName }}"
    }
}

// when accessing one of the variables from a provisioner or post-processor, use
// hcl-syntax
post-processor "manifest" {
    output = "manifest.json"
    strip_path = true
    custom_data = {
        source_ami_name = "${build.SourceAMIName}"
        device = "${build.Device}"
        mount_path = "${build.MountPath}"
    }
}
```

**JSON**

```json
"post-processors": [
  {
    "type": "manifest",
    "output": "manifest.json",
    "strip_path": true,
    "custom_data": {
      "source_ami_name": "{{ build `SourceAMIName` }}",
      "device": "{{ build `Device` }}",
      "mount_path": "{{ build `MountPath` }}"
    }
  }
]
```
