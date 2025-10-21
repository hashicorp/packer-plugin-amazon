// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type AmiFilterOptions,SecurityGroupFilterOptions,SubnetFilterOptions,VpcFilterOptions,PolicyDocument,Statement,MetadataOptions,LicenseConfigurationRequest,LicenseSpecification,Placement

package common

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/hashicorp/packer-plugin-sdk/uuid"
)

const (
	// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CreditSpecificationRequest.html#API_CreditSpecificationRequest_Contents
	CPUCreditsStandard  = "standard"
	CPUCreditsUnlimited = "unlimited"
)

var reShutdownBehavior = regexp.MustCompile("^(stop|terminate)$")

type Statement struct {
	Effect   string   `mapstructure:"Effect" required:"false"`
	Action   []string `mapstructure:"Action" required:"false"`
	Resource []string `mapstructure:"Resource" required:"false"`
}

type PolicyDocument struct {
	Version   string      `mapstructure:"Version" required:"false"`
	Statement []Statement `mapstructure:"Statement" required:"false"`
}

type SecurityGroupFilterOptions struct {
	config.NameValueFilter `mapstructure:",squash"`
}

type SubnetFilterOptions struct {
	config.NameValueFilter `mapstructure:",squash"`
	MostFree               bool `mapstructure:"most_free"`
	Random                 bool `mapstructure:"random"`
}

type LicenseConfigurationRequest struct {
	// The Amazon Resource Name (ARN) of the license configuration.
	LicenseConfigurationArn string `mapstructure:"license_configuration_arn"`
}

type LicenseSpecification struct {
	// Describes a license configuration.
	LicenseConfigurationRequest LicenseConfigurationRequest `mapstructure:"license_configuration_request"`
}

type Placement struct {
	// The ARN of the host resource group in which to launch the instances.
	HostResourceGroupArn string `mapstructure:"host_resource_group_arn" required:"false"`
	// The ID of the host used when Packer launches an EC2 instance.
	HostId string `mapstructure:"host_id" required:"false"`
	// [Tenancy](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/dedicated-instance.html) used
	// when Packer launches the EC2 instance, allowing it to be launched on dedicated hardware.
	//
	// The default is "default", meaning shared tenancy. Allowed values are "default",
	// "dedicated" and "host".
	Tenancy string `mapstructure:"tenancy" required:"false"`
}

func (p Placement) Prepare() []error {
	var errs []error

	if p.HostId != "" && p.HostResourceGroupArn != "" {
		errs = append(errs, fmt.Errorf("The `host_id` and `host_resource_group_arn` settings cannot be specified at the same time."))
	}

	if p.HostId != "" || p.HostResourceGroupArn != "" {
		switch p.Tenancy {
		case "", "host":
		default:
			errs = append(errs, fmt.Errorf("The tenancy should be `host` if either the `host_id` or `host_resource_group_arn` attributes are specified."))
		}
	}

	return errs
}

type VpcFilterOptions struct {
	config.NameValueFilter `mapstructure:",squash"`
}

type MetadataOptions struct {
	// A string to enable or disable the IMDS endpoint for an instance. Defaults to enabled.
	// Accepts either "enabled" or "disabled"
	HttpEndpoint string `mapstructure:"http_endpoint" required:"false"`
	// A string to either set the use of IMDSv2 for the instance to optional or required. Defaults to "optional".
	// Accepts either "optional" or "required"
	HttpTokens string `mapstructure:"http_tokens" required:"false"`
	// A numerical value to set an upper limit for the amount of hops allowed when communicating with IMDS endpoints.
	// Defaults to 1.
	HttpPutResponseHopLimit int32 `mapstructure:"http_put_response_hop_limit" required:"false"`
	// A string to enable or disable access to instance tags from the instance metadata. Defaults to disabled.
	// Access to instance metadata tags is available for commercial regions. For non-commercial regions please check availability before enabling.
	// Accepts either "enabled" or "disabled"
	InstanceMetadataTags string `mapstructure:"instance_metadata_tags" required:"false"`
}

// RunConfig contains configuration for running an instance from a source
// AMI and details on how to access that launched image.
type RunConfig struct {
	// If using a non-default VPC,
	// public IP addresses are not provided by default. If this is true, your
	// new instance will get a Public IP. default: unset
	//
	// Note: when specifying this attribute without a `subnet_[id|filter]` or
	// `vpc_[id|filter]`, we will attempt to infer this information from the
	// default VPC/Subnet.
	// This operation may require some extra permissions to the IAM role that
	// runs the build:
	//
	// * ec2:DescribeVpcs
	// * ec2:DescribeSubnets
	//
	// Additionally, since we filter subnets/AZs by their capability to host
	// an instance of the selected type, you may also want to define the
	// `ec2:DescribeInstanceTypeOfferings` action to the role running the build.
	// Otherwise, Packer will pick the most available subnet in the VPC selected,
	// which may not be able to host the instance type you provided.
	AssociatePublicIpAddress config.Trilean `mapstructure:"associate_public_ip_address" required:"false"`
	// Destination availability zone to launch
	// instance in. Leave this empty to allow Amazon to auto-assign.
	AvailabilityZone string `mapstructure:"availability_zone" required:"false"`
	// Requires spot_price to be set. The
	// required duration for the Spot Instances (also known as Spot blocks). This
	// value must be a multiple of 60 (60, 120, 180, 240, 300, or 360). You can't
	// specify an Availability Zone group or a launch group if you specify a
	// duration. Note: This parameter is no longer available to new customers
	// from July 1, 2021. [See Amazon's
	//documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/spot-requests.html#fixed-duration-spot-instances).
	// Deprecated argument
	BlockDurationMinutes int64 `mapstructure:"block_duration_minutes" required:"false"`
	// Set the preference for using a capacity reservation if one exists.
	// Either will be `open` or `none`. Defaults to `none`
	CapacityReservationPreference string `mapstructure:"capacity_reservation_preference" required:"false"`
	// Provide the specific EC2 Capacity Reservation ID that will be used
	// by Packer.
	CapacityReservationId string `mapstructure:"capacity_reservation_id" required:"false"`
	// Provide the EC2 Capacity Reservation Group ARN that will be used by
	// Packer.
	CapacityReservationGroupArn string `mapstructure:"capacity_reservation_group_arn" required:"false"`

	// Packer normally stops the build instance after all provisioners have
	// run. For Windows instances, it is sometimes desirable to [run
	// Sysprep](https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/Creating_EBSbacked_WinAMI.html)
	// which will stop the instance for you. If this is set to `true`, Packer
	// *will not* stop the instance but will assume that you will send the stop
	// signal yourself through your final provisioner. You can do this with a
	// [windows-shell provisioner](/packer/plugins/provisioners/windows-shell). Note that
	// Packer will still wait for the instance to be stopped, and failing to
	// send the stop signal yourself, when you have set this flag to `true`,
	// will cause a timeout.
	//
	// An example of a valid windows shutdown command in a `windows-shell`
	// provisioner is :
	// ```shell-session
	//   ec2config.exe -sysprep
	// ```
	// or
	// ```sell-session
	//   "%programfiles%\amazon\ec2configservice\"ec2config.exe -sysprep""
	// ```
	// -> Note: The double quotation marks in the command are not required if
	// your CMD shell is already in the
	// `C:\Program Files\Amazon\EC2ConfigService\` directory.
	DisableStopInstance bool `mapstructure:"disable_stop_instance" required:"false"`
	// Mark instance as [EBS
	// Optimized](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/EBSOptimized.html).
	// Default `false`.
	EbsOptimized bool `mapstructure:"ebs_optimized" required:"false"`
	// Enable support for Nitro Enclaves on the instance.  Note that the instance type must
	// be able to [support Nitro Enclaves](https://aws.amazon.com/ec2/nitro/nitro-enclaves/faqs/).
	// This option is not supported for spot instances.
	EnableNitroEnclave bool `mapstructure:"enable_nitro_enclave" required:"false"`
	// Deprecated argument - please use "enable_unlimited_credits".
	// Enabling T2 Unlimited allows the source instance to burst additional CPU
	// beyond its available [CPU
	// Credits](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/t2-credits-baseline-concepts.html)
	// for as long as the demand exists. This is in contrast to the standard
	// configuration that only allows an instance to consume up to its
	// available CPU Credits. See the AWS documentation for [T2
	// Unlimited](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/t2-unlimited.html)
	// and the **T2 Unlimited Pricing** section of the [Amazon EC2 On-Demand
	// Pricing](https://aws.amazon.com/ec2/pricing/on-demand/) document for
	// more information. By default this option is disabled and Packer will set
	// up a [T2
	// Standard](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/t2-std.html)
	// instance instead.
	//
	// To use T2 Unlimited you must use a T2 instance type, e.g. `t2.micro`.
	// Additionally, T2 Unlimited cannot be used in conjunction with Spot
	// Instances, e.g. when the `spot_price` option has been configured.
	// Attempting to do so will cause an error.
	//
	// !&gt; **Warning!** Additional costs may be incurred by enabling T2
	// Unlimited - even for instances that would usually qualify for the
	// [AWS Free Tier](https://aws.amazon.com/free/).
	EnableT2Unlimited bool `mapstructure:"enable_t2_unlimited" required:"false"`
	// Enabling Unlimited credits allows the source instance to burst additional CPU
	// beyond its available [CPU
	// Credits](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-performance-instances-unlimited-mode-concepts.html#unlimited-mode-surplus-credits)
	// for as long as the demand exists. This is in contrast to the standard
	// configuration that only allows an instance to consume up to its
	// available CPU Credits. See the AWS documentation for [T2
	// Unlimited](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-performance-instances-unlimited-mode-concepts.html)
	// and the **Unlimited Pricing** section of the [Amazon EC2 On-Demand
	// Pricing](https://aws.amazon.com/ec2/pricing/on-demand/) document for
	// more information. By default this option is disabled and Packer will set
	// up a [Standard](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-performance-instances-standard-mode.html)
	// instance instead.
	//
	// To use Unlimited you must use a T2/T3/T3a/T4g instance type, e.g. (`t2.micro`, `t3.micro`).
	// Additionally, Unlimited cannot be used in conjunction with Spot
	// Instances for T2 type instances, e.g. when the `spot_price` option has been configured.
	// Attempting to do so will cause an error if the underlying instance type is a T2 type instance.
	// By default the supported burstable instance types (including t3/t3a/t4g) will be provisioned with its cpu credits set to standard,
	// only when `enable_unlimited_credits` is true will the instance be provisioned with unlimited cpu credits.
	EnableUnlimitedCredits bool `mapstructure:"enable_unlimited_credits" required:"false"`
	// The name of an [IAM instance
	// profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/instance-profiles.html)
	// to launch the EC2 instance with.
	IamInstanceProfile string `mapstructure:"iam_instance_profile" required:"false"`
	// Key/value pair tags to apply tags to the fleet that is issued.
	FleetTags map[string]string `mapstructure:"fleet_tags" required:"false"`
	// Same as [`fleet_tags`](#fleet_tags) but defined as a singular repeatable block
	// containing a `key` and a `value` field. In HCL2 mode the
	// [`dynamic_block`](/packer/docs/templates/hcl_templates/expressions#dynamic-blocks)
	// will allow you to create those programatically.
	FleetTag config.KeyValues `mapstructure:"fleet_tag" required:"false"`
	// Whether or not to check if the IAM instance profile exists. Defaults to false
	SkipProfileValidation bool `mapstructure:"skip_profile_validation" required:"false"`
	// Temporary IAM instance profile policy document
	// If IamInstanceProfile is specified it will be used instead.
	//
	// HCL2 example:
	// ```hcl
	//temporary_iam_instance_profile_policy_document {
	//	Statement {
	//		Action   = ["logs:*"]
	//		Effect   = "Allow"
	//		Resource = ["*"]
	//	}
	//	Version = "2012-10-17"
	//}
	// ```
	//
	// JSON example:
	// ```json
	//{
	//	"Version": "2012-10-17",
	//	"Statement": [
	//		{
	//			"Action": [
	//			"logs:*"
	//			],
	//			"Effect": "Allow",
	//			"Resource": ["*"]
	//		}
	//	]
	//}
	// ```
	//
	TemporaryIamInstanceProfilePolicyDocument *PolicyDocument `mapstructure:"temporary_iam_instance_profile_policy_document" required:"false"`
	// Automatically terminate instances on
	// shutdown in case Packer exits ungracefully. Possible values are stop and
	// terminate. Defaults to stop.
	InstanceInitiatedShutdownBehavior string `mapstructure:"shutdown_behavior" required:"false"`
	// The EC2 instance type to use while building the
	// AMI, such as t2.small.
	InstanceType string `mapstructure:"instance_type" required:"true"`
	// Filters used to populate the `security_group_ids` field.
	//
	// HCL2 Example:
	//
	// ```hcl
	//   security_group_filter {
	//     filters = {
	//       "tag:Class": "packer"
	//     }
	//   }
	// ```
	//
	// JSON Example:
	// ```json
	// {
	//   "security_group_filter": {
	//     "filters": {
	//       "tag:Class": "packer"
	//     }
	//   }
	// }
	// ```
	//
	// This selects the SG's with tag `Class` with the value `packer`.
	//
	// -   `filters` (map[string,string] | multiple filters are allowed when seperated by commas) - filters used to select a
	//     `security_group_ids`. Any filter described in the docs for
	//     [DescribeSecurityGroups](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeSecurityGroups.html)
	//     is valid.
	//
	// `security_group_ids` take precedence over this.
	SecurityGroupFilter SecurityGroupFilterOptions `mapstructure:"security_group_filter" required:"false"`
	// Key/value pair tags to apply to the generated key-pair, security group, iam profile and role, snapshot, network interfaces and instance
	// that is *launched* to create the EBS volumes. The resulting AMI will also inherit these tags.
	// This is a [template
	// engine](/packer/docs/templates/legacy_json_templates/engine), see [Build template
	// data](#build-template-data) for more information.
	RunTags map[string]string `mapstructure:"run_tags" required:"false"`
	// Same as [`run_tags`](#run_tags) but defined as a singular repeatable
	// block containing a `key` and a `value` field. In HCL2 mode the
	// [`dynamic_block`](/packer/docs/templates/hcl_templates/expressions#dynamic-blocks)
	// will allow you to create those programatically.
	RunTag config.KeyValues `mapstructure:"run_tag" required:"false"`
	// The ID (not the name) of the security
	// group to assign to the instance. By default this is not set and Packer will
	// automatically create a new temporary security group to allow SSH access.
	// Note that if this is specified, you must be sure the security group allows
	// access to the ssh_port given below.
	SecurityGroupId string `mapstructure:"security_group_id" required:"false"`
	// A list of security groups as
	// described above. Note that if this is specified, you must omit the
	// security_group_id.
	SecurityGroupIds []string `mapstructure:"security_group_ids" required:"false"`
	// The source AMI whose root volume will be copied and
	// provisioned on the currently running instance. This must be an EBS-backed
	// AMI with a root volume snapshot that you have access to.
	SourceAmi string `mapstructure:"source_ami" required:"true"`
	// Filters used to populate the `source_ami`
	// field.
	//
	// HCL2 example:
	// ```hcl
	// source "amazon-ebs" "basic-example" {
	//   source_ami_filter {
	//     filters = {
	//        virtualization-type = "hvm"
	//        name = "ubuntu/images/*ubuntu-xenial-16.04-amd64-server-*"
	//        root-device-type = "ebs"
	//     }
	//     owners = ["099720109477"]
	//     most_recent = true
	//   }
	// }
	// ```
	//
	// JSON Example:
	// ```json
	// "builders" [
	//   {
	//     "type": "amazon-ebs",
	//     "source_ami_filter": {
	//        "filters": {
	//        "virtualization-type": "hvm",
	//        "name": "ubuntu/images/*ubuntu-xenial-16.04-amd64-server-*",
	//        "root-device-type": "ebs"
	//        },
	//        "owners": ["099720109477"],
	//        "most_recent": true
	//     }
	//   }
	// ]
	// ```
	//
	//   This selects the most recent Ubuntu 16.04 HVM EBS AMI from Canonical. NOTE:
	//   This will fail unless *exactly* one AMI is returned. In the above example,
	//   `most_recent` will cause this to succeed by selecting the newest image.
	//
	//   -   `filters` (map[string,string] | multiple filters are allowed when seperated by commas) - filters used to select a `source_ami`.
	//       NOTE: This will fail unless *exactly* one AMI is returned. Any filter
	//       described in the docs for
	//       [DescribeImages](http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeImages.html)
	//       is valid.
	//
	//   -   `owners` (array of strings) - Filters the images by their owner. You
	//       may specify one or more AWS account IDs, "self" (which will use the
	//       account whose credentials you are using to run Packer), or an AWS owner
	//       alias: for example, `amazon`, `aws-marketplace`, or `microsoft`. This
	//       option is required for security reasons.
	//
	//   -   `most_recent` (boolean) - Selects the newest created image when true.
	//       This is most useful for selecting a daily distro build.
	//
	//   You may set this in place of `source_ami` or in conjunction with it. If you
	//   set this in conjunction with `source_ami`, the `source_ami` will be added
	//   to the filter. The provided `source_ami` must meet all of the filtering
	//   criteria provided in `source_ami_filter`; this pins the AMI returned by the
	//   filter, but will cause Packer to fail if the `source_ami` does not exist.
	SourceAmiFilter AmiFilterOptions `mapstructure:"source_ami_filter" required:"false"`
	// One of  `price-capacity-optimized`, `capacity-optimized`, `diversified` or `lowest-price`.
	// The strategy that determines how to allocate the target Spot Instance capacity
	// across the Spot Instance pools specified by the EC2 Fleet launch configuration.
	// If this option is not set, Packer will use default option provided by the SDK (currently `lowest-price`).
	// For more information, see [Amazon EC2 User Guide] (https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-fleet-allocation-strategy.html)
	SpotAllocationStrategy string `mapstructure:"spot_allocation_strategy" required:"false"`
	// a list of acceptable instance
	// types to run your build on. We will request a spot instance using the max
	// price of spot_price and the allocation strategy of "lowest price".
	// Your instance will be launched on an instance type of the lowest available
	// price that you have in your list.  This is used in place of instance_type.
	// You may only set either spot_instance_types or instance_type, not both.
	// This feature exists to help prevent situations where a Packer build fails
	// because a particular availability zone does not have capacity for the
	// specific instance_type requested in instance_type.
	SpotInstanceTypes []string `mapstructure:"spot_instance_types" required:"false"`
	// With Spot Instances, you pay the Spot price that's in effect for the
	// time period your instances are running. Spot Instance prices are set by
	// Amazon EC2 and adjust gradually based on long-term trends in supply and
	// demand for Spot Instance capacity.
	//
	// When this field is set, it represents the maximum hourly price you are
	// willing to pay for a spot instance. If you do not set this value, it
	// defaults to a maximum price equal to the on demand price of the
	// instance. In the situation where the current Amazon-set spot price
	// exceeds the value set in this field, Packer will not launch an instance
	// and the build will error. In the situation where the Amazon-set spot
	// price is less than the value set in this field, Packer will launch and
	// you will pay the Amazon-set spot price, not this maximum value.
	// For more information, see the Amazon docs on
	// [spot pricing](https://aws.amazon.com/ec2/spot/pricing/).
	SpotPrice string `mapstructure:"spot_price" required:"false"`
	// Required if spot_price is set to
	// auto. This tells Packer what sort of AMI you're launching to find the
	// best spot price. This must be one of: Linux/UNIX, SUSE Linux,
	// Windows, Linux/UNIX (Amazon VPC), SUSE Linux (Amazon VPC),
	// Windows (Amazon VPC)
	SpotPriceAutoProduct string `mapstructure:"spot_price_auto_product" required:"false" undocumented:"true"`
	// Requires spot_price to be set. Key/value pair tags to apply tags to the
	// spot request that is issued.
	SpotTags map[string]string `mapstructure:"spot_tags" required:"false"`
	// Same as [`spot_tags`](#spot_tags) but defined as a singular repeatable block
	// containing a `key` and a `value` field. In HCL2 mode the
	// [`dynamic_block`](/packer/docs/templates/hcl_templates/expressions#dynamic-blocks)
	// will allow you to create those programatically.
	SpotTag config.KeyValues `mapstructure:"spot_tag" required:"false"`
	// Filters used to populate the `subnet_id` field.
	//
	// HCL2 example:
	//
	// ```hcl
	// source "amazon-ebs" "basic-example" {
	//   subnet_filter {
	//     filters = {
	//           "tag:Class": "build"
	//     }
	//     most_free = true
	//     random = false
	//   }
	// }
	// ```
	//
	// JSON Example:
	// ```json
	// "builders" [
	//   {
	//     "type": "amazon-ebs",
	//     "subnet_filter": {
	//       "filters": {
	//         "tag:Class": "build"
	//       },
	//       "most_free": true,
	//       "random": false
	//     }
	//   }
	// ]
	// ```
	//
	//   This selects the Subnet with tag `Class` with the value `build`, which has
	//   the most free IP addresses. NOTE: This will fail unless *exactly* one
	//   Subnet is returned. By using `most_free` or `random` one will be selected
	//   from those matching the filter.
	//
	//   -   `filters` (map[string,string] | multiple filters are allowed when seperated by commas) - filters used to select a `subnet_id`.
	//       NOTE: This will fail unless *exactly* one Subnet is returned. Any
	//       filter described in the docs for
	//       [DescribeSubnets](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeSubnets.html)
	//       is valid.
	//
	//   -   `most_free` (boolean) - The Subnet with the most free IPv4 addresses
	//       will be used if multiple Subnets matches the filter.
	//
	//   -   `random` (boolean) - A random Subnet will be used if multiple Subnets
	//       matches the filter. `most_free` have precendence over this.
	//
	//   `subnet_id` take precedence over this.
	SubnetFilter SubnetFilterOptions `mapstructure:"subnet_filter" required:"false"`
	// If using VPC, the ID of the subnet, such as
	// subnet-12345def, where Packer will launch the EC2 instance. This field is
	// required if you are using an non-default VPC.
	SubnetId string `mapstructure:"subnet_id" required:"false"`
	// The license configurations.
	//
	// HCL2 example:
	// ```hcl
	// source "amazon-ebs" "basic-example" {
	//   license_specifications {
	//     license_configuration_request = {
	//       license_configuration_arn = "${var.license_configuration_arn}"
	//     }
	//   }
	// }
	// ```
	//
	// JSON example:
	// ```json
	// "builders" [
	//   {
	//     "type": "amazon-ebs",
	//     "license_specifications": [
	//       {
	//         "license_configuration_request": {
	//           "license_configuration_arn": "{{user `license_configuration_arn`}}"
	//         }
	//       }
	//     ]
	//   }
	// ]
	// ```
	//
	//   Each `license_configuration_request` describes a license configuration,
	//   the properties of which are:
	//
	//   - `license_configuration_arn` (string) - The Amazon Resource Name (ARN)
	//     of the license configuration.
	//
	LicenseSpecifications []LicenseSpecification `mapstructure:"license_specifications" required:"false"`
	// Describes the placement of an instance.
	//
	// HCL2 example:
	// ```hcl
	// source "amazon-ebs" "basic-example" {
	//   placement = {
	//     host_resource_group_arn = "${var.host_resource_group_arn}"
	//     tenancy                 = "${var.placement_tenancy}"
	//   }
	// }
	// ```
	//
	// JSON example:
	// ```json
	// "builders" [
	//   {
	//     "type": "amazon-ebs",
	//     "placement": {
	//       "host_resource_group_arn": "{{user `host_resource_group_arn`}}",
	//       "tenancy": "{{user `placement_tenancy`}}"
	//     }
	//   }
	// ]
	// ```
	//
	// Refer to the [Placement docs](#placement-configuration) for more information on the supported attributes for placement configuration.
	Placement Placement `mapstructure:"placement" required:"false"`
	// Deprecated: Use Placement Tenancy instead.
	Tenancy string `mapstructure:"tenancy" required:"false"`
	// A list of IPv4 CIDR blocks to be authorized access to the instance, when
	// packer is creating a temporary security group.
	//
	// The default is [`0.0.0.0/0`] (i.e., allow any IPv4 source).
	// Use `temporary_security_group_source_public_ip` to allow current host's
	// public IP instead of any IPv4 source.
	// This is only used when `security_group_id` or `security_group_ids` is not
	// specified.
	TemporarySGSourceCidrs []string `mapstructure:"temporary_security_group_source_cidrs" required:"false"`
	// When enabled, use public IP of the host (obtained from https://checkip.amazonaws.com)
	// as CIDR block to be authorized access to the instance, when packer
	// is creating a temporary security group. Defaults to `false`.
	//
	// This is only used when `security_group_id`, `security_group_ids`,
	// and `temporary_security_group_source_cidrs` are not specified.
	TemporarySGSourcePublicIp bool `mapstructure:"temporary_security_group_source_public_ip" required:"false"`
	// User data to apply when launching the instance. Note
	// that you need to be careful about escaping characters due to the templates
	// being JSON. It is often more convenient to use user_data_file, instead.
	// Packer will not automatically wait for a user script to finish before
	// shutting down the instance this must be handled in a provisioner.
	UserData string `mapstructure:"user_data" required:"false"`
	// Path to a file that will be used for the user
	// data when launching the instance.
	UserDataFile string `mapstructure:"user_data_file" required:"false"`
	// Filters used to populate the `vpc_id` field.
	//
	// HCL2 example:
	// ```hcl
	// source "amazon-ebs" "basic-example" {
	//   vpc_filter {
	//     filters = {
	//       "tag:Class": "build",
	//       "isDefault": "false",
	//       "cidr": "/24"
	//     }
	//   }
	// }
	// ```
	//
	// JSON Example:
	// ```json
	// "builders" [
	//   {
	//     "type": "amazon-ebs",
	//     "vpc_filter": {
	//       "filters": {
	//         "tag:Class": "build",
	//         "isDefault": "false",
	//         "cidr": "/24"
	//       }
	//     }
	//   }
	// ]
	// ```
	//
	// This selects the VPC with tag `Class` with the value `build`, which is not
	// the default VPC, and have a IPv4 CIDR block of `/24`. NOTE: This will fail
	// unless *exactly* one VPC is returned.
	//
	// -   `filters` (map[string,string] | multiple filters are allowed when seperated by commas) - filters used to select a `vpc_id`. NOTE:
	//     This will fail unless *exactly* one VPC is returned. Any filter
	//     described in the docs for
	//     [DescribeVpcs](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeVpcs.html)
	//     is valid.
	//
	// `vpc_id` take precedence over this.
	VpcFilter VpcFilterOptions `mapstructure:"vpc_filter" required:"false"`
	// If launching into a VPC subnet, Packer needs the VPC ID
	// in order to create a temporary security group within the VPC. Requires
	// subnet_id to be set. If this field is left blank, Packer will try to get
	// the VPC ID from the subnet_id.
	VpcId string `mapstructure:"vpc_id" required:"false"`
	// The timeout for waiting for a Windows
	// password for Windows instances. Defaults to 20 minutes. Example value:
	// 10m
	WindowsPasswordTimeout time.Duration `mapstructure:"windows_password_timeout" required:"false"`

	// [Metadata Settings](#metadata-settings)
	Metadata MetadataOptions `mapstructure:"metadata_options" required:"false"`

	// Communicator settings
	Comm communicator.Config `mapstructure:",squash"`

	// One of `public_ip`, `private_ip`, `public_dns`, `private_dns` or `session_manager`.
	//    If set, either the public IP address, private IP address, public DNS name
	//    or private DNS name will be used as the host for SSH. The default behaviour
	//    if inside a VPC is to use the public IP address if available, otherwise
	//    the private IP address will be used. If not in a VPC the public DNS name
	//    will be used. Also works for WinRM.
	//
	//    Where Packer is configured for an outbound proxy but WinRM traffic
	//    should be direct, `ssh_interface` must be set to `private_dns` and
	//    `<region>.compute.internal` included in the `NO_PROXY` environment
	//    variable.
	//
	//    When using `session_manager` the machine running Packer must have
	//	  the AWS Session Manager Plugin installed and within the users' system path.
	//    Connectivity via the `session_manager` interface establishes a secure tunnel
	//    between the local host and the remote host on an available local port to the specified `ssh_port`.
	//    See [Session Manager Connections](#session-manager-connections) for more information.
	//    - Session manager connectivity is currently only implemented for the SSH communicator, not the WinRM communicator.
	//    - Upon termination the secure tunnel will be terminated automatically, if however there is a failure in
	//    terminating the tunnel it will automatically terminate itself after 20 minutes of inactivity.
	SSHInterface string `mapstructure:"ssh_interface"`

	// The time to wait before establishing the Session Manager session.
	// The value of this should be a duration. Examples are
	// `5s` and `1m30s` which will cause Packer to wait five seconds and one
	// minute 30 seconds, respectively. If no set, defaults to 10 seconds.
	// This option is useful when the remote port takes longer to become available.
	PauseBeforeSSM time.Duration `mapstructure:"pause_before_ssm"`

	// Which port to connect the local end of the session tunnel to. If
	// left blank, Packer will choose a port for you from available ports.
	// This option is only used when `ssh_interface` is set `session_manager`.
	SessionManagerPort int `mapstructure:"session_manager_port"`
}

func (c *RunConfig) Prepare(ctx *interpolate.Context) []error {

	// If we are not given an explicit ssh_keypair_name or
	// ssh_private_key_file, then create a temporary one, but only if the
	// temporary_key_pair_name has not been provided and we are not using
	// ssh_password.
	if c.Comm.SSHKeyPairName == "" && c.Comm.SSHTemporaryKeyPairName == "" &&
		c.Comm.SSHPrivateKeyFile == "" && c.Comm.SSHPassword == "" {

		c.Comm.SSHTemporaryKeyPairName = fmt.Sprintf("packer_%s", uuid.TimeOrderedUUID())

	}

	if c.Comm.SSHTemporaryKeyPairType == "" {
		c.Comm.SSHTemporaryKeyPairType = "rsa"
	}

	if c.WindowsPasswordTimeout == 0 {
		c.WindowsPasswordTimeout = 20 * time.Minute
	}

	if c.RunTags == nil {
		c.RunTags = make(map[string]string)
	}

	// EnableT2Unlimited has been deprecated so we preserve any config settings.
	if c.EnableT2Unlimited && !c.EnableUnlimitedCredits {
		c.EnableUnlimitedCredits = c.EnableT2Unlimited
	}

	// Validation
	errs := c.Comm.Prepare(ctx)

	if c.Metadata.HttpEndpoint == "" {
		c.Metadata.HttpEndpoint = "enabled"
	}

	if c.Metadata.HttpTokens == "" {
		c.Metadata.HttpTokens = "optional"
	}

	if c.Metadata.HttpPutResponseHopLimit == 0 {
		c.Metadata.HttpPutResponseHopLimit = 1
	}

	if c.Metadata.InstanceMetadataTags == "" {
		c.Metadata.InstanceMetadataTags = "disabled"
	}

	if c.Comm.SSHTemporaryKeyPairType != "rsa" && c.Comm.SSHTemporaryKeyPairType != "ed25519" {
		msg := fmt.Errorf("temporary_key_pair_type requires either rsa or ed25519 as its value")
		errs = append(errs, msg)
	}

	if c.Metadata.HttpEndpoint != "enabled" && c.Metadata.HttpEndpoint != "disabled" {
		msg := fmt.Errorf("http_endpoint requires either disabled or enabled as its value")
		errs = append(errs, msg)
	}

	if c.Metadata.HttpTokens != "optional" && c.Metadata.HttpTokens != "required" {
		msg := fmt.Errorf("http_tokens requires either optional or required as its value")
		errs = append(errs, msg)
	}

	if c.Metadata.HttpPutResponseHopLimit < 1 || c.Metadata.HttpPutResponseHopLimit > 64 {
		msg := fmt.Errorf("http_put_response_hop_limit requires a number between 1 and 64")
		errs = append(errs, msg)
	}

	if c.Metadata.InstanceMetadataTags != "enabled" && c.Metadata.InstanceMetadataTags != "disabled" {
		msg := fmt.Errorf("instance_metadata_tags requires either disabled or enabled as its value")
		errs = append(errs, msg)
	}

	// Copy singular tag maps
	errs = append(errs, c.RunTag.CopyOn(&c.RunTags)...)
	errs = append(errs, c.SpotTag.CopyOn(&c.SpotTags)...)
	errs = append(errs, c.FleetTag.CopyOn(&c.FleetTags)...)

	for _, preparer := range []interface{ Prepare() []error }{
		&c.SecurityGroupFilter,
		&c.SubnetFilter,
		&c.VpcFilter,
	} {
		errs = append(errs, preparer.Prepare()...)
	}

	// Validating ssh_interface
	if c.SSHInterface != "public_ip" &&
		c.SSHInterface != "private_ip" &&
		c.SSHInterface != "public_dns" &&
		c.SSHInterface != "private_dns" &&
		c.SSHInterface != "session_manager" &&
		c.SSHInterface != "" {
		errs = append(errs, fmt.Errorf("Unknown interface type: %s", c.SSHInterface))
	}

	// Connectivity via Session Manager has a few requirements
	if c.SSHInterface == "session_manager" {
		if c.Comm.Type == "winrm" {
			msg := fmt.Errorf(`session_manager connectivity is not supported with the "winrm" communicator; please use "ssh"`)
			errs = append(errs, msg)
		}

		if c.IamInstanceProfile == "" && c.TemporaryIamInstanceProfilePolicyDocument == nil {
			msg := fmt.Errorf(`no iam_instance_profile defined; session_manager connectivity requires a valid instance profile with AmazonSSMManagedInstanceCore permissions. Alternatively a temporary_iam_instance_profile_policy_document can be used.`)
			errs = append(errs, msg)
		}
	}

	if c.Comm.SSHKeyPairName != "" {
		if c.Comm.Type == "winrm" && c.Comm.WinRMPassword == "" && c.Comm.SSHPrivateKeyFile == "" {
			errs = append(errs, fmt.Errorf("ssh_private_key_file must be provided to retrieve the winrm password when using ssh_keypair_name."))
		} else if c.Comm.SSHPrivateKeyFile == "" && !c.Comm.SSHAgentAuth {
			errs = append(errs, fmt.Errorf("ssh_private_key_file must be provided or ssh_agent_auth enabled when ssh_keypair_name is specified."))
		}
	}

	if c.SourceAmi == "" && c.SourceAmiFilter.Empty() {
		errs = append(errs, fmt.Errorf("A source_ami or source_ami_filter must be specified"))
	}

	if c.SourceAmi == "" && c.SourceAmiFilter.NoOwner() {
		errs = append(errs, fmt.Errorf("For security reasons, your source AMI filter must declare an owner."))
	}

	if c.InstanceType == "" && len(c.SpotInstanceTypes) == 0 {
		errs = append(errs, fmt.Errorf("either instance_type or "+
			"spot_instance_types must be specified"))
	}

	if c.InstanceType != "" && len(c.SpotInstanceTypes) > 0 {
		errs = append(errs, fmt.Errorf("either instance_type or "+
			"spot_instance_types must be specified, not both"))
	}

	if c.FleetTags != nil {
		if c.SpotPrice == "" || c.SpotPrice == "0" {
			errs = append(errs, fmt.Errorf(
				"fleet_tags should not be set when not requesting a spot instance"))
		}
	}

	if c.BlockDurationMinutes%60 != 0 {
		errs = append(errs, fmt.Errorf(
			"block_duration_minutes must be multiple of 60"))
	}

	if c.SpotTags != nil {
		if c.SpotPrice == "" || c.SpotPrice == "0" {
			errs = append(errs, fmt.Errorf(
				"spot_tags should not be set when not requesting a spot instance"))
		}
	}

	if c.SpotAllocationStrategy != "" {
		err := IsValidSpotAllocationStrategy(c.SpotAllocationStrategy)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if c.UserData != "" && c.UserDataFile != "" {
		errs = append(errs, fmt.Errorf("Only one of user_data or user_data_file can be specified."))
	} else if c.UserDataFile != "" {
		if _, err := os.Stat(c.UserDataFile); err != nil {
			errs = append(errs, fmt.Errorf("user_data_file not found: %s", c.UserDataFile))
		}
	}

	if c.SecurityGroupId != "" {
		if len(c.SecurityGroupIds) > 0 {
			errs = append(errs, fmt.Errorf("Only one of security_group_id or security_group_ids can be specified."))
		} else {
			c.SecurityGroupIds = []string{c.SecurityGroupId}
			c.SecurityGroupId = ""
		}
	}

	if len(c.TemporarySGSourceCidrs) == 0 && !c.TemporarySGSourcePublicIp {
		c.TemporarySGSourceCidrs = []string{"0.0.0.0/0"}
	} else {
		for _, cidr := range c.TemporarySGSourceCidrs {
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				errs = append(errs, fmt.Errorf("Error parsing CIDR in temporary_security_group_source_cidrs: %s", err.Error()))
			}
		}
	}

	if c.InstanceInitiatedShutdownBehavior == "" {
		c.InstanceInitiatedShutdownBehavior = "stop"
	} else if !reShutdownBehavior.MatchString(c.InstanceInitiatedShutdownBehavior) {
		errs = append(errs, fmt.Errorf("shutdown_behavior only accepts 'stop' or 'terminate' values."))
	}

	if c.EnableUnlimitedCredits {
		if !c.IsBurstableInstanceType() {
			errs = append(errs, fmt.Errorf("Error: Instance Type: %s is not within the supported types for Unlimited credits. Supported instance types are T2, T3, and T4g", c.InstanceType))
		}

		if c.SpotPrice != "" && regexp.MustCompile(`^t2\.`).MatchString(c.InstanceType) {
			errs = append(errs, fmt.Errorf("Error: Unlimited credits cannot be used in conjunction with Spot Instances"))
		}

	}

	capacityReservationTargetSet := false
	if c.CapacityReservationId != "" || c.CapacityReservationGroupArn != "" {
		capacityReservationTargetSet = true
	}

	if c.CapacityReservationGroupArn != "" && c.CapacityReservationId != "" {
		errs = append(errs, fmt.Errorf("capacity_reservation_id and capacity_reservation_group_arn are mutually exclusive, only one should be used"))
	}

	if capacityReservationTargetSet && c.CapacityReservationPreference != "" {
		errs = append(errs, fmt.Errorf("capacity_reservation_id, capacity_reservation_group_arn and capacity_reservation_preference are mutually exclusive, only one should be set"))
	}

	if c.CapacityReservationPreference == "" && c.CapacityReservationId == "" && c.CapacityReservationGroupArn == "" {
		c.CapacityReservationPreference = "none"
	}
	switch c.CapacityReservationPreference {
	case "", "none", "open":
	default:
		errs = append(errs, fmt.Errorf(`capacity_reservation_preference only accepts 'none' or 'open' values`))
	}

	var tenancy string
	tenancies := []string{c.Placement.Tenancy, c.Tenancy}

	for i := range tenancies {
		if tenancies[i] != "" {
			if tenancy != "" {
				errs = append(errs, fmt.Errorf("Error: Please set only one of placement.tenancy and tenancy (deprecated)"))
			}
			tenancy = tenancies[i]
		}
	}

	if tenancy != "" &&
		tenancy != "default" &&
		tenancy != "dedicated" &&
		tenancy != "host" {
		errs = append(errs, fmt.Errorf("Error: Unknown tenancy type %s", tenancy))
	}

	errs = append(errs, c.Placement.Prepare()...)

	if c.EnableNitroEnclave {
		if c.SpotPrice != "" {
			errs = append(errs, fmt.Errorf("Error: Nitro Enclave cannot be used in conjunction with Spot Instances"))
		}
		// check if we have an instance in the t-line (burstable instances)
		if strings.HasPrefix(c.InstanceType, "t") {
			errs = append(errs, fmt.Errorf("Error: Nitro Enclaves cannot be used in conjunction with burstable instance types: %s", c.InstanceType))
		}
	}

	return errs
}

func (c *RunConfig) IsSpotInstance() bool {
	return c.SpotPrice != "" && c.SpotPrice != "0"
}

func (c *RunConfig) SSMAgentEnabled() bool {
	hasIamInstanceProfile := c.IamInstanceProfile != "" || c.TemporaryIamInstanceProfilePolicyDocument != nil
	return c.SSHInterface == "session_manager" && hasIamInstanceProfile
}

// IsBurstableInstanceType checks if the InstanceType for the config is one
// of the following types T2, T3a, T3, T4g
func (c *RunConfig) IsBurstableInstanceType() bool {
	r := `^t(:?2|3a?|4g)\.`
	return regexp.MustCompile(r).MatchString(c.InstanceType)
}
