Type: `amazon-instance`
Artifact BuilderId: `mitchellh.amazon.instance`

The `amazon-instance` Packer builder is able to create Amazon AMIs backed by
instance storage as the root device. For more information on the difference
between instance storage and EBS-backed instances, see the ["storage for the
root device" section in the EC2
documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ComponentsAMIs.html#storage-for-the-root-device).

This builder builds an AMI by launching an EC2 instance from an existing
instance-storage backed AMI, provisioning that running machine, and then
bundling and creating a new AMI from that machine. This is all done in your own
AWS account. This builder will create temporary key pairs, security group
rules, etc. that provide it temporary access to the instance while the image is
being created. This simplifies configuration quite a bit.

This builder does _not_ manage AMIs. Once it creates an AMI and stores it in
your account, it is up to you to use, delete, etc. the AMI.

-> **Note:** Temporary resources are, by default, all created with the
prefix `packer`. This can be useful if you want to restrict the security groups
and key pairs packer is able to operate on.

-> **Note:** This builder requires that the [Amazon EC2 AMI
Tools](https://aws.amazon.com/developertools/368) are installed onto the
machine. This can be done within a provisioner, but must be done before the
builder finishes running.

~> Instance builds are not supported for Windows. Use
[`amazon-ebs`](/packer/integrations/hashicorp/amazon/latest/components/builder/ebs) instead.

## Configuration Reference

There are many configuration options available for the builder. In addition to
the items listed here, you will want to look at the general configuration
references for [AMI](#ami-configuration),
[BlockDevices](#block-devices-configuration),
[Access](#access-configuration),
[Run](#run-configuration) and
[Communicator](#communicator-configuration)
configuration references, which are
necessary for this build to succeed and can be found further down the page.

### Required:

<!-- Code generated from the comments of the Config struct in builder/instance/builder.go; DO NOT EDIT MANUALLY -->

- `account_id` (string) - Your AWS account ID. This is required for bundling the AMI. This is not
  the same as the access key. You can find your account ID in the security
  credentials page of your AWS account.

- `s3_bucket` (string) - The name of the S3 bucket to upload the AMI. This bucket will be created
  if it doesn't exist.

- `x509_cert_path` (string) - The local path to a valid X509 certificate for your AWS account. This is
  used for bundling the AMI. This X509 certificate must be registered with
  your account from the security credentials page in the AWS console.

- `x509_key_path` (string) - The local path to the private key for the X509 certificate specified by
  x509_cert_path. This is used for bundling the AMI.

<!-- End of code generated from the comments of the Config struct in builder/instance/builder.go; -->


### Optional:

<!-- Code generated from the comments of the Config struct in builder/instance/builder.go; DO NOT EDIT MANUALLY -->

- `ami_block_device_mappings` (awscommon.BlockDevices) - Add one or more block device mappings to the AMI. These will be attached
  when booting a new instance from your AMI. To add a block device during
  the Packer build see `launch_block_device_mappings` below. Your options
  here may vary depending on the type of VM you use. See the
  [BlockDevices](#block-devices-configuration) documentation for fields.

- `launch_block_device_mappings` (awscommon.BlockDevices) - Add one or more block devices before the Packer build starts. If you add
  instance store volumes or EBS volumes in addition to the root device
  volume, the created AMI will contain block device mapping information
  for those volumes. Amazon creates snapshots of the source instance's
  root volume and any other EBS volumes described here. When you launch an
  instance from this new AMI, the instance automatically launches with
  these additional volumes, and will restore them from snapshots taken
  from the source instance. See the
  [BlockDevices](#block-devices-configuration) documentation for fields.

- `bundle_destination` (string) - The directory on the running instance where the bundled AMI will be
  saved prior to uploading. By default this is /tmp. This directory must
  exist and be writable.

- `bundle_prefix` (string) - The prefix for files created from bundling the root volume. By default
  this is `image-{{timestamp}}`. The timestamp variable should be used to
  make sure this is unique, otherwise it can collide with other created
  AMIs by Packer in your account.

- `bundle_upload_command` (string) - The command to use to upload the bundled volume. See the "custom bundle
  commands" section below for more information.

- `bundle_vol_command` (string) - The command to use to bundle the volume. See the "custom bundle
  commands" section below for more information.

- `x509_upload_path` (string) - The path on the remote machine where the X509 certificate will be
  uploaded. This path must already exist and be writable. X509
  certificates are uploaded after provisioning is run, so it is perfectly
  okay to create this directory as part of the provisioning process.
  Defaults to /tmp.

- `tpm_support` (string) - NitroTPM Support. Valid options are `v2.0`. See the documentation on
  [NitroTPM Support](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/enable-nitrotpm-support-on-ami.html) for
  more information. Only enabled if a valid option is provided, otherwise ignored.

<!-- End of code generated from the comments of the Config struct in builder/instance/builder.go; -->


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

<!-- End of code generated from the comments of the AMIConfig struct in builder/common/ami_config.go; -->


### Access Configuration

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


### Run Configuration

#### Required:

<!-- Code generated from the comments of the RunConfig struct in builder/common/run_config.go; DO NOT EDIT MANUALLY -->

- `instance_type` (string) - The EC2 instance type to use while building the
  AMI, such as t2.small.

- `source_ami` (string) - The source AMI whose root volume will be copied and
  provisioned on the currently running instance. This must be an EBS-backed
  AMI with a root volume snapshot that you have access to.

<!-- End of code generated from the comments of the RunConfig struct in builder/common/run_config.go; -->


#### Optional:

<!-- Code generated from the comments of the RunConfig struct in builder/common/run_config.go; DO NOT EDIT MANUALLY -->

- `associate_public_ip_address` (confighelper.Trilean) - If using a non-default VPC,
  public IP addresses are not provided by default. If this is true, your
  new instance will get a Public IP. default: unset
  
  Note: when specifying this attribute without a `subnet_[id|filter]` or
  `vpc_[id|filter]`, we will attempt to infer this information from the
  default VPC/Subnet.
  This operation may require some extra permissions to the IAM role that
  runs the build:
  
  * ec2:DescribeVpcs
  * ec2:DescribeSubnets
  
  Additionally, since we filter subnets/AZs by their capability to host
  an instance of the selected type, you may also want to define the
  `ec2:DescribeInstanceTypeOfferings` action to the role running the build.
  Otherwise, Packer will pick the most available subnet in the VPC selected,
  which may not be able to host the instance type you provided.

- `availability_zone` (string) - Destination availability zone to launch
  instance in. Leave this empty to allow Amazon to auto-assign.

- `block_duration_minutes` (int64) - Requires spot_price to be set. The
  required duration for the Spot Instances (also known as Spot blocks). This
  value must be a multiple of 60 (60, 120, 180, 240, 300, or 360). You can't
  specify an Availability Zone group or a launch group if you specify a
  duration. Note: This parameter is no longer available to new customers
  from July 1, 2021. [See Amazon's
  documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/spot-requests.html#fixed-duration-spot-instances).

- `capacity_reservation_preference` (string) - Set the preference for using a capacity reservation if one exists.
  Either will be `open` or `none`. Defaults to `none`

- `capacity_reservation_id` (string) - Provide the specific EC2 Capacity Reservation ID that will be used
  by Packer.

- `capacity_reservation_group_arn` (string) - Provide the EC2 Capacity Reservation Group ARN that will be used by
  Packer.

- `disable_stop_instance` (bool) - Packer normally stops the build instance after all provisioners have
  run. For Windows instances, it is sometimes desirable to [run
  Sysprep](https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/Creating_EBSbacked_WinAMI.html)
  which will stop the instance for you. If this is set to `true`, Packer
  *will not* stop the instance but will assume that you will send the stop
  signal yourself through your final provisioner. You can do this with a
  [windows-shell provisioner](/packer/integrations/hashicorp/windows-shell). Note that
  Packer will still wait for the instance to be stopped, and failing to
  send the stop signal yourself, when you have set this flag to `true`,
  will cause a timeout.
  
  An example of a valid windows shutdown command in a `windows-shell`
  provisioner is :
  ```shell-session
    ec2config.exe -sysprep
  ```
  or
  ```sell-session
    "%programfiles%\amazon\ec2configservice\"ec2config.exe -sysprep""
  ```
  -> Note: The double quotation marks in the command are not required if
  your CMD shell is already in the
  `C:\Program Files\Amazon\EC2ConfigService\` directory.

- `ebs_optimized` (bool) - Mark instance as [EBS
  Optimized](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/EBSOptimized.html).
  Default `false`.

- `enable_nitro_enclave` (bool) - Enable support for Nitro Enclaves on the instance.  Note that the instance type must
  be able to [support Nitro Enclaves](https://aws.amazon.com/ec2/nitro/nitro-enclaves/faqs/).
  This option is not supported for spot instances.

- `enable_t2_unlimited` (bool) - Deprecated argument - please use "enable_unlimited_credits".
  Enabling T2 Unlimited allows the source instance to burst additional CPU
  beyond its available [CPU
  Credits](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/t2-credits-baseline-concepts.html)
  for as long as the demand exists. This is in contrast to the standard
  configuration that only allows an instance to consume up to its
  available CPU Credits. See the AWS documentation for [T2
  Unlimited](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/t2-unlimited.html)
  and the **T2 Unlimited Pricing** section of the [Amazon EC2 On-Demand
  Pricing](https://aws.amazon.com/ec2/pricing/on-demand/) document for
  more information. By default this option is disabled and Packer will set
  up a [T2
  Standard](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/t2-std.html)
  instance instead.
  
  To use T2 Unlimited you must use a T2 instance type, e.g. `t2.micro`.
  Additionally, T2 Unlimited cannot be used in conjunction with Spot
  Instances, e.g. when the `spot_price` option has been configured.
  Attempting to do so will cause an error.
  
  !&gt; **Warning!** Additional costs may be incurred by enabling T2
  Unlimited - even for instances that would usually qualify for the
  [AWS Free Tier](https://aws.amazon.com/free/).

- `enable_unlimited_credits` (bool) - Enabling Unlimited credits allows the source instance to burst additional CPU
  beyond its available [CPU
  Credits](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-performance-instances-unlimited-mode-concepts.html#unlimited-mode-surplus-credits)
  for as long as the demand exists. This is in contrast to the standard
  configuration that only allows an instance to consume up to its
  available CPU Credits. See the AWS documentation for [T2
  Unlimited](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-performance-instances-unlimited-mode-concepts.html)
  and the **Unlimited Pricing** section of the [Amazon EC2 On-Demand
  Pricing](https://aws.amazon.com/ec2/pricing/on-demand/) document for
  more information. By default this option is disabled and Packer will set
  up a [Standard](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-performance-instances-standard-mode.html)
  instance instead.
  
  To use Unlimited you must use a T2/T3/T3a/T4g instance type, e.g. (`t2.micro`, `t3.micro`).
  Additionally, Unlimited cannot be used in conjunction with Spot
  Instances for T2 type instances, e.g. when the `spot_price` option has been configured.
  Attempting to do so will cause an error if the underlying instance type is a T2 type instance.
  By default the supported burstable instance types (including t3/t3a/t4g) will be provisioned with its cpu credits set to standard,
  only when `enable_unlimited_credits` is true will the instance be provisioned with unlimited cpu credits.

- `iam_instance_profile` (string) - The name of an [IAM instance
  profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/instance-profiles.html)
  to launch the EC2 instance with.

- `fleet_tags` (map[string]string) - Key/value pair tags to apply tags to the fleet that is issued.

- `fleet_tag` ([]{key string, value string}) - Same as [`fleet_tags`](#fleet_tags) but defined as a singular repeatable block
  containing a `key` and a `value` field. In HCL2 mode the
  [`dynamic_block`](/packer/docs/templates/hcl_templates/expressions#dynamic-blocks)
  will allow you to create those programatically.

- `skip_profile_validation` (bool) - Whether or not to check if the IAM instance profile exists. Defaults to false

- `temporary_iam_instance_profile_policy_document` (\*PolicyDocument) - Temporary IAM instance profile policy document
  If IamInstanceProfile is specified it will be used instead.
  
  HCL2 example:
  ```hcl
  temporary_iam_instance_profile_policy_document {
  	Statement {
  		Action   = ["logs:*"]
  		Effect   = "Allow"
  		Resource = ["*"]
  	}
  	Version = "2012-10-17"
  }
  ```
  
  JSON example:
  ```json
  {
  	"Version": "2012-10-17",
  	"Statement": [
  		{
  			"Action": [
  			"logs:*"
  			],
  			"Effect": "Allow",
  			"Resource": ["*"]
  		}
  	]
  }
  ```

- `shutdown_behavior` (string) - Automatically terminate instances on
  shutdown in case Packer exits ungracefully. Possible values are stop and
  terminate. Defaults to stop.

- `security_group_filter` (SecurityGroupFilterOptions) - Filters used to populate the `security_group_ids` field.
  
  HCL2 Example:
  
  ```hcl
    security_group_filter {
      filters = {
        "tag:Class": "packer"
      }
    }
  ```
  
  JSON Example:
  ```json
  {
    "security_group_filter": {
      "filters": {
        "tag:Class": "packer"
      }
    }
  }
  ```
  
  This selects the SG's with tag `Class` with the value `packer`.
  
  -   `filters` (map[string,string] | multiple filters are allowed when seperated by commas) - filters used to select a
      `security_group_ids`. Any filter described in the docs for
      [DescribeSecurityGroups](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeSecurityGroups.html)
      is valid.
  
  `security_group_ids` take precedence over this.

- `run_tags` (map[string]string) - Key/value pair tags to apply to the generated key-pair, security group, iam profile and role, snapshot, network interfaces and instance
  that is *launched* to create the EBS volumes. The resulting AMI will also inherit these tags.
  This is a [template
  engine](/packer/docs/templates/legacy_json_templates/engine), see [Build template
  data](#build-template-data) for more information.

- `run_tag` ([]{key string, value string}) - Same as [`run_tags`](#run_tags) but defined as a singular repeatable
  block containing a `key` and a `value` field. In HCL2 mode the
  [`dynamic_block`](/packer/docs/templates/hcl_templates/expressions#dynamic-blocks)
  will allow you to create those programatically.

- `security_group_id` (string) - The ID (not the name) of the security
  group to assign to the instance. By default this is not set and Packer will
  automatically create a new temporary security group to allow SSH access.
  Note that if this is specified, you must be sure the security group allows
  access to the ssh_port given below.

- `security_group_ids` ([]string) - A list of security groups as
  described above. Note that if this is specified, you must omit the
  security_group_id.

- `source_ami_filter` (AmiFilterOptions) - Filters used to populate the `source_ami`
  field.
  
  HCL2 example:
  ```hcl
  source "amazon-ebs" "basic-example" {
    source_ami_filter {
      filters = {
         virtualization-type = "hvm"
         name = "ubuntu/images/*ubuntu-xenial-16.04-amd64-server-*"
         root-device-type = "ebs"
      }
      owners = ["099720109477"]
      most_recent = true
    }
  }
  ```
  
  JSON Example:
  ```json
  "builders" [
    {
      "type": "amazon-ebs",
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
  ]
  ```
  
    This selects the most recent Ubuntu 16.04 HVM EBS AMI from Canonical. NOTE:
    This will fail unless *exactly* one AMI is returned. In the above example,
    `most_recent` will cause this to succeed by selecting the newest image.
  
    -   `filters` (map[string,string] | multiple filters are allowed when seperated by commas) - filters used to select a `source_ami`.
        NOTE: This will fail unless *exactly* one AMI is returned. Any filter
        described in the docs for
        [DescribeImages](http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeImages.html)
        is valid.
  
    -   `owners` (array of strings) - Filters the images by their owner. You
        may specify one or more AWS account IDs, "self" (which will use the
        account whose credentials you are using to run Packer), or an AWS owner
        alias: for example, `amazon`, `aws-marketplace`, or `microsoft`. This
        option is required for security reasons.
  
    -   `most_recent` (boolean) - Selects the newest created image when true.
        This is most useful for selecting a daily distro build.
  
    You may set this in place of `source_ami` or in conjunction with it. If you
    set this in conjunction with `source_ami`, the `source_ami` will be added
    to the filter. The provided `source_ami` must meet all of the filtering
    criteria provided in `source_ami_filter`; this pins the AMI returned by the
    filter, but will cause Packer to fail if the `source_ami` does not exist.

- `spot_instance_types` ([]string) - a list of acceptable instance
  types to run your build on. We will request a spot instance using the max
  price of spot_price and the allocation strategy of "lowest price".
  Your instance will be launched on an instance type of the lowest available
  price that you have in your list.  This is used in place of instance_type.
  You may only set either spot_instance_types or instance_type, not both.
  This feature exists to help prevent situations where a Packer build fails
  because a particular availability zone does not have capacity for the
  specific instance_type requested in instance_type.

- `spot_price` (string) - With Spot Instances, you pay the Spot price that's in effect for the
  time period your instances are running. Spot Instance prices are set by
  Amazon EC2 and adjust gradually based on long-term trends in supply and
  demand for Spot Instance capacity.
  
  When this field is set, it represents the maximum hourly price you are
  willing to pay for a spot instance. If you do not set this value, it
  defaults to a maximum price equal to the on demand price of the
  instance. In the situation where the current Amazon-set spot price
  exceeds the value set in this field, Packer will not launch an instance
  and the build will error. In the situation where the Amazon-set spot
  price is less than the value set in this field, Packer will launch and
  you will pay the Amazon-set spot price, not this maximum value.
  For more information, see the Amazon docs on
  [spot pricing](https://aws.amazon.com/ec2/spot/pricing/).

- `spot_tags` (map[string]string) - Requires spot_price to be set. Key/value pair tags to apply tags to the
  spot request that is issued.

- `spot_tag` ([]{key string, value string}) - Same as [`spot_tags`](#spot_tags) but defined as a singular repeatable block
  containing a `key` and a `value` field. In HCL2 mode the
  [`dynamic_block`](/packer/docs/templates/hcl_templates/expressions#dynamic-blocks)
  will allow you to create those programatically.

- `subnet_filter` (SubnetFilterOptions) - Filters used to populate the `subnet_id` field.
  
  HCL2 example:
  
  ```hcl
  source "amazon-ebs" "basic-example" {
    subnet_filter {
      filters = {
            "tag:Class": "build"
      }
      most_free = true
      random = false
    }
  }
  ```
  
  JSON Example:
  ```json
  "builders" [
    {
      "type": "amazon-ebs",
      "subnet_filter": {
        "filters": {
          "tag:Class": "build"
        },
        "most_free": true,
        "random": false
      }
    }
  ]
  ```
  
    This selects the Subnet with tag `Class` with the value `build`, which has
    the most free IP addresses. NOTE: This will fail unless *exactly* one
    Subnet is returned. By using `most_free` or `random` one will be selected
    from those matching the filter.
  
    -   `filters` (map[string,string] | multiple filters are allowed when seperated by commas) - filters used to select a `subnet_id`.
        NOTE: This will fail unless *exactly* one Subnet is returned. Any
        filter described in the docs for
        [DescribeSubnets](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeSubnets.html)
        is valid.
  
    -   `most_free` (boolean) - The Subnet with the most free IPv4 addresses
        will be used if multiple Subnets matches the filter.
  
    -   `random` (boolean) - A random Subnet will be used if multiple Subnets
        matches the filter. `most_free` have precendence over this.
  
    `subnet_id` take precedence over this.

- `subnet_id` (string) - If using VPC, the ID of the subnet, such as
  subnet-12345def, where Packer will launch the EC2 instance. This field is
  required if you are using an non-default VPC.

- `license_specifications` ([]LicenseSpecification) - The license configurations.
  
  HCL2 example:
  ```hcl
  source "amazon-ebs" "basic-example" {
    license_specifications {
      license_configuration_request = {
        license_configuration_arn = "${var.license_configuration_arn}"
      }
    }
  }
  ```
  
  JSON example:
  ```json
  "builders" [
    {
      "type": "amazon-ebs",
      "license_specifications": [
        {
          "license_configuration_request": {
            "license_configuration_arn": "{{user `license_configuration_arn`}}"
          }
        }
      ]
    }
  ]
  ```
  
    Each `license_configuration_request` describes a license configuration,
    the properties of which are:
  
    - `license_configuration_arn` (string) - The Amazon Resource Name (ARN)
      of the license configuration.

- `placement` (Placement) - Describes the placement of an instance.
  
  HCL2 example:
  ```hcl
  source "amazon-ebs" "basic-example" {
    placement = {
      host_resource_group_arn = "${var.host_resource_group_arn}"
      tenancy                 = "${var.placement_tenancy}"
    }
  }
  ```
  
  JSON example:
  ```json
  "builders" [
    {
      "type": "amazon-ebs",
      "placement": {
        "host_resource_group_arn": "{{user `host_resource_group_arn`}}",
        "tenancy": "{{user `placement_tenancy`}}"
      }
    }
  ]
  ```
  
    - `host_resource_group_arn` (string) - The ARN of the host resource
      group in which to launch the instances. If you specify a host
      resource group ARN, omit the Tenancy parameter or set it to `host`.
    - `tenancy` (string) - The tenancy of the instance (if the instance is
      running in a VPC). An instance with a tenancy of `dedicated` runs on
      single-tenant hardware. The default is `default`, meaning shared
      tenancy. Allowed values are `default`, `dedicated` and `host`.

- `tenancy` (string) - Deprecated: Use Placement Tenancy instead.

- `temporary_security_group_source_cidrs` ([]string) - A list of IPv4 CIDR blocks to be authorized access to the instance, when
  packer is creating a temporary security group.
  
  The default is [`0.0.0.0/0`] (i.e., allow any IPv4 source).
  Use `temporary_security_group_source_public_ip` to allow current host's
  public IP instead of any IPv4 source.
  This is only used when `security_group_id` or `security_group_ids` is not
  specified.

- `temporary_security_group_source_public_ip` (bool) - When enabled, use public IP of the host (obtained from https://checkip.amazonaws.com)
  as CIDR block to be authorized access to the instance, when packer
  is creating a temporary security group. Defaults to `false`.
  
  This is only used when `security_group_id`, `security_group_ids`,
  and `temporary_security_group_source_cidrs` are not specified.

- `user_data` (string) - User data to apply when launching the instance. Note
  that you need to be careful about escaping characters due to the templates
  being JSON. It is often more convenient to use user_data_file, instead.
  Packer will not automatically wait for a user script to finish before
  shutting down the instance this must be handled in a provisioner.

- `user_data_file` (string) - Path to a file that will be used for the user
  data when launching the instance.

- `vpc_filter` (VpcFilterOptions) - Filters used to populate the `vpc_id` field.
  
  HCL2 example:
  ```hcl
  source "amazon-ebs" "basic-example" {
    vpc_filter {
      filters = {
        "tag:Class": "build",
        "isDefault": "false",
        "cidr": "/24"
      }
    }
  }
  ```
  
  JSON Example:
  ```json
  "builders" [
    {
      "type": "amazon-ebs",
      "vpc_filter": {
        "filters": {
          "tag:Class": "build",
          "isDefault": "false",
          "cidr": "/24"
        }
      }
    }
  ]
  ```
  
  This selects the VPC with tag `Class` with the value `build`, which is not
  the default VPC, and have a IPv4 CIDR block of `/24`. NOTE: This will fail
  unless *exactly* one VPC is returned.
  
  -   `filters` (map[string,string] | multiple filters are allowed when seperated by commas) - filters used to select a `vpc_id`. NOTE:
      This will fail unless *exactly* one VPC is returned. Any filter
      described in the docs for
      [DescribeVpcs](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeVpcs.html)
      is valid.
  
  `vpc_id` take precedence over this.

- `vpc_id` (string) - If launching into a VPC subnet, Packer needs the VPC ID
  in order to create a temporary security group within the VPC. Requires
  subnet_id to be set. If this field is left blank, Packer will try to get
  the VPC ID from the subnet_id.

- `windows_password_timeout` (duration string | ex: "1h5m2s") - The timeout for waiting for a Windows
  password for Windows instances. Defaults to 20 minutes. Example value:
  10m

- `metadata_options` (MetadataOptions) - [Metadata Settings](#metadata-settings)

- `ssh_interface` (string) - One of `public_ip`, `private_ip`, `public_dns`, `private_dns` or `session_manager`.
     If set, either the public IP address, private IP address, public DNS name
     or private DNS name will be used as the host for SSH. The default behaviour
     if inside a VPC is to use the public IP address if available, otherwise
     the private IP address will be used. If not in a VPC the public DNS name
     will be used. Also works for WinRM.
  
     Where Packer is configured for an outbound proxy but WinRM traffic
     should be direct, `ssh_interface` must be set to `private_dns` and
     `<region>.compute.internal` included in the `NO_PROXY` environment
     variable.
  
     When using `session_manager` the machine running Packer must have
  	  the AWS Session Manager Plugin installed and within the users' system path.
     Connectivity via the `session_manager` interface establishes a secure tunnel
     between the local host and the remote host on an available local port to the specified `ssh_port`.
     See [Session Manager Connections](#session-manager-connections) for more information.
     - Session manager connectivity is currently only implemented for the SSH communicator, not the WinRM communicator.
     - Upon termination the secure tunnel will be terminated automatically, if however there is a failure in
     terminating the tunnel it will automatically terminate itself after 20 minutes of inactivity.

- `pause_before_ssm` (duration string | ex: "1h5m2s") - The time to wait before establishing the Session Manager session.
  The value of this should be a duration. Examples are
  `5s` and `1m30s` which will cause Packer to wait five seconds and one
  minute 30 seconds, respectively. If no set, defaults to 10 seconds.
  This option is useful when the remote port takes longer to become available.

- `session_manager_port` (int) - Which port to connect the local end of the session tunnel to. If
  left blank, Packer will choose a port for you from available ports.
  This option is only used when `ssh_interface` is set `session_manager`.

<!-- End of code generated from the comments of the RunConfig struct in builder/common/run_config.go; -->


### Session Manager Connections

Support for the AWS Systems Manager session manager lets users manage EC2 instances without the need to open inbound ports, or maintain bastion hosts. Session manager connectivity relies on the use of the [session manager plugin](#session-manager-plugin) to open a secure tunnel between the local machine and the remote instance. Once the tunnel has been created all SSH communication will be tunneled through SSM to the remote instance.

-> Note: Session manager connectivity is currently only implemented for the SSH communicator, not the WinRM Communicator.

To use the session manager as the connection interface for the SSH communicator you need to add the following configuration options to the Amazon builder options:

- `ssh_interface`: The ssh interface must be set to "session_manager". When using this option the builder will create an SSM tunnel to the configured `ssh_port` (defaults to 22) on the remote host.
- `iam_instance_profile`: A valid instance profile granting Systems Manager permissions to manage the remote instance is required in order for the aws ssm-agent to start and stop session connections.
  See below for more details on [IAM instance profile for Systems Manager](#iam-instance-profile-for-systems-manager).

#### Optional

- `session_manager_port`: A local port on the host machine that should be used as the local end of the session tunnel to the remote host. If not specified Packer will find an available port to use.
- `temporary_iam_instance_profile_policy_document`: Creates a temporary instance profile policy document to grant Systems Manager permissions to the Ec2 instance. This is an alternative to using an existing `iam_instance_profile`.

HCL2 example:

```hcl
# file: example.pkr.hcl

# In order to get these variables to read from the environment,
# set the environment variables to have the same name as the declared
# variables, with the prefix PKR_VAR_.
# You could also hardcode them into the file, but we do not recommend that.

data "amazon-ami" "example" {
  filters = {
    virtualization-type = "hvm"
    name                = "ubuntu/images/*ubuntu-xenial-16.04-amd64-server-*"
    root-device-type    = "ebs"
  }
  owners      = ["099720109477"]
  most_recent = true
  region      = "us-east-1"
}

source "amazon-ebs" "ssm-example" {
  ami_name             = "packer_AWS {{timestamp}}"
  instance_type        = "t2.micro"
  region               = "us-east-1"
  source_ami           = data.amazon-ami.example.id
  ssh_username         = "ubuntu"
  ssh_interface        = "session_manager"
  communicator         = "ssh"
  iam_instance_profile = "myinstanceprofile"
}

build {
  sources = ["source.amazon-ebs.ssm-example"]

  provisioner "shell" {
    inline = ["echo Connected via SSM at '${build.User}@${build.Host}:${build.Port}'"]
  }
}
```

JSON example:

```json
{
  "builders": [
    {
      "type": "amazon-ebs",
      "ami_name": "packer-ami-{{timestamp}}",
      "instance_type": "t2.micro",
      "source_ami_filter": {
        "filters": {
          "virtualization-type": "hvm",
          "name": "ubuntu/images/*ubuntu-xenial-16.04-amd64-server-*",
          "root-device-type": "ebs"
        },
        "owners": ["099720109477"],
        "most_recent": true
      },
      "ssh_username": "ubuntu",
      "ssh_interface": "session_manager",
      "communicator": "ssh",
      "iam_instance_profile": "{{user `iam_instance_profile`}}"
    }
  ],
  "provisioners": [
    {
      "type": "shell",
      "inline": [
        "echo Connected via SSM at '{{build `User`}}@{{build `Host`}}:{{build `Port`}}'"
      ]
    }
  ]
}
```

#### Session Manager Plugin

Connectivity via the session manager requires the use of a session-manger-plugin, which needs to be installed alongside Packer, and an instance AMI that is capable of running the AWS ssm-agent - see [About SSM Agent](https://docs.aws.amazon.com/systems-manager/latest/userguide/prereqs-ssm-agent.html) for details on supported AMIs.

In order for Packer to start and end sessions that connect you to your managed instances, you must first install the Session Manager plugin on your local machine. The plugin can be installed on supported versions of Microsoft Windows, macOS, Linux, and Ubuntu Server.
[Installation instructions for the session-manager-plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html)

#### IAM instance profile for Systems Manager

By default Systems Manager doesn't have permission to perform actions on created instances so SSM access must be granted by creating an instance profile with the `AmazonSSMManagedInstanceCore` policy. The instance profile can then be attached to any instance you wish to manage via the session-manager-plugin. See [Adding System Manager instance profile](https://docs.aws.amazon.com/systems-manager/latest/userguide/setup-instance-profile.html#instance-profile-add-permissions) for details on creating the required instance profile.

#### Permissions for Closing the Tunnel

To close the SSM tunnels created, this plugin relies on being able to call
[DescribeInstanceStatus](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeInstanceStatus.html).
In case this is not possible you might see a `Bad exit status` message in the logs.

The absence of this permission won't prevent you from building the AMI, and the error only means that packer is not able to close the tunnel gracefully.


### Block Devices Configuration

Block devices can be nested in the
[ami_block_device_mappings](#ami_block_device_mappings) or the
[launch_block_device_mappings](#launch_block_device_mappings) array.

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

- `virtual_name` (string) - The virtual device name. See the documentation on Block Device Mapping
  for more information.

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


### Communicator Configuration

#### Optional:

<!-- Code generated from the comments of the Config struct in communicator/config.go; DO NOT EDIT MANUALLY -->

- `communicator` (string) - Packer currently supports three kinds of communicators:
  
  -   `none` - No communicator will be used. If this is set, most
      provisioners also can't be used.
  
  -   `ssh` - An SSH connection will be established to the machine. This
      is usually the default.
  
  -   `winrm` - A WinRM connection will be established.
  
  In addition to the above, some builders have custom communicators they
  can use. For example, the Docker builder has a "docker" communicator
  that uses `docker exec` and `docker cp` to execute scripts and copy
  files.

- `pause_before_connecting` (duration string | ex: "1h5m2s") - We recommend that you enable SSH or WinRM as the very last step in your
  guest's bootstrap script, but sometimes you may have a race condition
  where you need Packer to wait before attempting to connect to your
  guest.
  
  If you end up in this situation, you can use the template option
  `pause_before_connecting`. By default, there is no pause. For example if
  you set `pause_before_connecting` to `10m` Packer will check whether it
  can connect, as normal. But once a connection attempt is successful, it
  will disconnect and then wait 10 minutes before connecting to the guest
  and beginning provisioning.

<!-- End of code generated from the comments of the Config struct in communicator/config.go; -->


<!-- Code generated from the comments of the SSH struct in communicator/config.go; DO NOT EDIT MANUALLY -->

- `ssh_host` (string) - The address to SSH to. This usually is automatically configured by the
  builder.

- `ssh_port` (int) - The port to connect to SSH. This defaults to `22`.

- `ssh_username` (string) - The username to connect to SSH with. Required if using SSH.

- `ssh_password` (string) - A plaintext password to use to authenticate with SSH.

- `ssh_ciphers` ([]string) - This overrides the value of ciphers supported by default by Golang.
  The default value is [
    "aes128-gcm@openssh.com",
    "chacha20-poly1305@openssh.com",
    "aes128-ctr", "aes192-ctr", "aes256-ctr",
  ]
  
  Valid options for ciphers include:
  "aes128-ctr", "aes192-ctr", "aes256-ctr", "aes128-gcm@openssh.com",
  "chacha20-poly1305@openssh.com",
  "arcfour256", "arcfour128", "arcfour", "aes128-cbc", "3des-cbc",

- `ssh_clear_authorized_keys` (bool) - If true, Packer will attempt to remove its temporary key from
  `~/.ssh/authorized_keys` and `/root/.ssh/authorized_keys`. This is a
  mostly cosmetic option, since Packer will delete the temporary private
  key from the host system regardless of whether this is set to true
  (unless the user has set the `-debug` flag). Defaults to "false";
  currently only works on guests with `sed` installed.

- `ssh_key_exchange_algorithms` ([]string) - If set, Packer will override the value of key exchange (kex) algorithms
  supported by default by Golang. Acceptable values include:
  "curve25519-sha256@libssh.org", "ecdh-sha2-nistp256",
  "ecdh-sha2-nistp384", "ecdh-sha2-nistp521",
  "diffie-hellman-group14-sha1", and "diffie-hellman-group1-sha1".

- `ssh_certificate_file` (string) - Path to user certificate used to authenticate with SSH.
  The `~` can be used in path and will be expanded to the
  home directory of current user.

- `ssh_pty` (bool) - If `true`, a PTY will be requested for the SSH connection. This defaults
  to `false`.

- `ssh_timeout` (duration string | ex: "1h5m2s") - The time to wait for SSH to become available. Packer uses this to
  determine when the machine has booted so this is usually quite long.
  Example value: `10m`.
  This defaults to `5m`, unless `ssh_handshake_attempts` is set.

- `ssh_disable_agent_forwarding` (bool) - If true, SSH agent forwarding will be disabled. Defaults to `false`.

- `ssh_handshake_attempts` (int) - The number of handshakes to attempt with SSH once it can connect.
  This defaults to `10`, unless a `ssh_timeout` is set.

- `ssh_bastion_host` (string) - A bastion host to use for the actual SSH connection.

- `ssh_bastion_port` (int) - The port of the bastion host. Defaults to `22`.

- `ssh_bastion_agent_auth` (bool) - If `true`, the local SSH agent will be used to authenticate with the
  bastion host. Defaults to `false`.

- `ssh_bastion_username` (string) - The username to connect to the bastion host.

- `ssh_bastion_password` (string) - The password to use to authenticate with the bastion host.

- `ssh_bastion_interactive` (bool) - If `true`, the keyboard-interactive used to authenticate with bastion host.

- `ssh_bastion_private_key_file` (string) - Path to a PEM encoded private key file to use to authenticate with the
  bastion host. The `~` can be used in path and will be expanded to the
  home directory of current user.

- `ssh_bastion_certificate_file` (string) - Path to user certificate used to authenticate with bastion host.
  The `~` can be used in path and will be expanded to the
  home directory of current user.

- `ssh_file_transfer_method` (string) - `scp` or `sftp` - How to transfer files, Secure copy (default) or SSH
  File Transfer Protocol.
  
  **NOTE**: Guests using Windows with Win32-OpenSSH v9.1.0.0p1-Beta, scp
  (the default protocol for copying data) returns a a non-zero error code since the MOTW
  cannot be set, which cause any file transfer to fail. As a workaround you can override the transfer protocol
  with SFTP instead `ssh_file_transfer_protocol = "sftp"`.

- `ssh_proxy_host` (string) - A SOCKS proxy host to use for SSH connection

- `ssh_proxy_port` (int) - A port of the SOCKS proxy. Defaults to `1080`.

- `ssh_proxy_username` (string) - The optional username to authenticate with the proxy server.

- `ssh_proxy_password` (string) - The optional password to use to authenticate with the proxy server.

- `ssh_keep_alive_interval` (duration string | ex: "1h5m2s") - How often to send "keep alive" messages to the server. Set to a negative
  value (`-1s`) to disable. Example value: `10s`. Defaults to `5s`.

- `ssh_read_write_timeout` (duration string | ex: "1h5m2s") - The amount of time to wait for a remote command to end. This might be
  useful if, for example, packer hangs on a connection after a reboot.
  Example: `5m`. Disabled by default.

- `ssh_remote_tunnels` ([]string) - 

- `ssh_local_tunnels` ([]string) - 

<!-- End of code generated from the comments of the SSH struct in communicator/config.go; -->


<!-- Code generated from the comments of the SSHTemporaryKeyPair struct in communicator/config.go; DO NOT EDIT MANUALLY -->

- `temporary_key_pair_type` (string) - `dsa` | `ecdsa` | `ed25519` | `rsa` ( the default )
  
  Specifies the type of key to create. The possible values are 'dsa',
  'ecdsa', 'ed25519', or 'rsa'.
  
  NOTE: DSA is deprecated and no longer recognized as secure, please
  consider other alternatives like RSA or ED25519.

- `temporary_key_pair_bits` (int) - Specifies the number of bits in the key to create. For RSA keys, the
  minimum size is 1024 bits and the default is 4096 bits. Generally, 3072
  bits is considered sufficient. DSA keys must be exactly 1024 bits as
  specified by FIPS 186-2. For ECDSA keys, bits determines the key length
  by selecting from one of three elliptic curve sizes: 256, 384 or 521
  bits. Attempting to use bit lengths other than these three values for
  ECDSA keys will fail. Ed25519 keys have a fixed length and bits will be
  ignored.
  
  NOTE: DSA is deprecated and no longer recognized as secure as specified
  by FIPS 186-5, please consider other alternatives like RSA or ED25519.

<!-- End of code generated from the comments of the SSHTemporaryKeyPair struct in communicator/config.go; -->


- `ssh_keypair_name` (string) - If specified, this is the key that will be used for SSH with the
  machine. The key must match a key pair name loaded up into the remote.
  By default, this is blank, and Packer will generate a temporary keypair
  unless [`ssh_password`](#ssh_password) is used.
  [`ssh_private_key_file`](#ssh_private_key_file) or
  [`ssh_agent_auth`](#ssh_agent_auth) must be specified when
  [`ssh_keypair_name`](#ssh_keypair_name) is utilized.


- `ssh_private_key_file` (string) - Path to a PEM encoded private key file to use to authenticate with SSH.
  The `~` can be used in path and will be expanded to the home directory
  of current user.


- `ssh_agent_auth` (bool) - If true, the local SSH agent will be used to authenticate connections to
  the source instance. No temporary keypair will be created, and the
  values of [`ssh_password`](#ssh_password) and
  [`ssh_private_key_file`](#ssh_private_key_file) will be ignored. The
  environment variable `SSH_AUTH_SOCK` must be set for this option to work
  properly.


## Basic Example

Here is a basic example. It is completely valid except for the access keys:

**HCL2**

```hcl
source "amazon-instance" "basic-example" {
  region = "us-east-1"
  source_ami = "ami-d9d6a6b0"
  instance_type = "m1.small"
  ssh_username = "ubuntu"

  account_id = "0123-4567-0890"
  s3_bucket = "packer-images"
  x509_cert_path = "x509.cert"
  x509_key_path = "x509.key"
  x509_upload_path = "/tmp"
}

build {
  source "sources.amazon-instance.basic-example" {
    ami_name = "packer-quick-start {{timestamp}}"
  }
}
```

**JSON**

```json
{
  "type": "amazon-instance",
  "access_key": "YOUR KEY HERE",
  "secret_key": "YOUR SECRET KEY HERE",
  "region": "us-east-1",
  "source_ami": "ami-d9d6a6b0",
  "instance_type": "m1.small",
  "ssh_username": "ubuntu",

  "account_id": "0123-4567-0890",
  "s3_bucket": "packer-images",
  "x509_cert_path": "x509.cert",
  "x509_key_path": "x509.key",
  "x509_upload_path": "/tmp",

  "ami_name": "packer-quick-start {{timestamp}}"
}
```


-> **Note:** Packer can also read the access key and secret access key from
environmental variables. See the configuration reference in the section above
for more information on what environmental variables Packer will look for.

## Accessing the Instance to Debug

If you need to access the instance to debug for some reason, run this builder
with the `-debug` flag. In debug mode, the Amazon builder will save the private
key in the current directory and will output the DNS or IP information as well.
You can use this information to access the instance as it is running.

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
      "source_ami_name": "{{ build `SourceAMIName` }}"
    }
  }
]
```


## Custom Bundle Commands

A lot of the process required for creating an instance-store backed AMI
involves commands being run on the actual source instance. Specifically, the
`ec2-bundle-vol` and `ec2-upload-bundle` commands must be used to bundle the
root filesystem and upload it, respectively.

Each of these commands have a lot of available flags. Instead of exposing each
possible flag as a template configuration option, the instance-store AMI
builder for Packer lets you customize the entire command used to bundle and
upload the AMI.

These are configured with `bundle_vol_command` and `bundle_upload_command`.
Both of these configurations are [configuration
templates](/packer/docs/templates/legacy_json_templates/engine) and have support for their own set of
template variables.

### Bundle Volume Command

The default value for `bundle_vol_command` is shown below. It is split across
multiple lines for convenience of reading. The bundle volume command is
responsible for executing `ec2-bundle-vol` in order to store and image of the
root filesystem to use to create the AMI.

```shell-session
$ sudo -i -n ec2-bundle-vol \
  -k {{.KeyPath}}  \
  -u {{.AccountId}} \
  -c {{.CertPath}} \
  -r {{.Architecture}} \
  -e {{.PrivatePath}}/* \
  -d {{.Destination}} \
  -p {{.Prefix}} \
  --batch \
  --no-filter
```

The available template variables should be self-explanatory based on the
parameters they're used to satisfy the `ec2-bundle-vol` command.

~> **Warning!** Some versions of ec2-bundle-vol silently ignore all .pem
and .gpg files during the bundling of the AMI, which can cause problems on some
systems, such as Ubuntu. You may want to customize the bundle volume command to
include those files (see the `--no-filter` option of `ec2-bundle-vol`).

### Bundle Upload Command

The default value for `bundle_upload_command` is shown below. It is split
across multiple lines for convenience of reading. Access key and secret key are
omitted if using instance profile. The bundle upload command is responsible for
taking the bundled volume and uploading it to S3.

```shell-session
$ sudo -i -n ec2-upload-bundle \
  -b {{.BucketName}} \
  -m {{.ManifestPath}} \
  -a {{.AccessKey}} \
  -s {{.SecretKey}} \
  -d {{.BundleDirectory}} \
  --batch \
  --region {{.Region}} \
  --retry
```

The available template variables should be self-explanatory based on the
parameters they're used to satisfy the `ec2-upload-bundle` command.
Additionally, `{{.Token}}` is available when overriding this command. You must
create your own bundle command with the addition of `-t {{.Token}}` if you are
assuming a role.

#### Bundle Upload Permissions

The `ec2-upload-bundle` requires a policy document that looks something like
this:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:GetObject",
        "s3:ListBucket",
        "s3:GetBucketLocation",
        "s3:PutObjectAcl"
      ],
      "Resource": "*"
    }
  ]
}
```

You may wish to constrain the resource to a specific bucket.

## Which SSH Options to use:

This chart breaks down what Packer does if you set any of the below SSH options:

| ssh_password | ssh_private_key_file | ssh_keypair_name | temporary_key_pair_name | Packer will...                                                                             |
| ------------ | -------------------- | ---------------- | ----------------------- | ------------------------------------------------------------------------------------------ |
| X            | -                    | -                | -                       | ssh authenticating with username and given password                                        |
| -            | X                    | -                | -                       | ssh authenticating with private key file                                                   |
| -            | X                    | X                | -                       | ssh authenticating with given private key file and "attaching" the keypair to the instance |
| -            | -                    | -                | X                       | Create a temporary ssh keypair with a particular name, clean it up                         |
| -            | -                    | -                | -                       | Create a temporary ssh keypair with a default name, clean it up                            |
