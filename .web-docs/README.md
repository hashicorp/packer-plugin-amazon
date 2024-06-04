The Amazon plugin can be used with HashiCorp Packer to create custom images on AWS. To achieve this, the plugin comes with
multiple builders, data sources, and a post-processor to build the AMI depending on the strategy you want to use.

### Installation

To install this plugin, copy and paste this code into your Packer configuration, then run [`packer init`](https://www.packer.io/docs/commands/init).

```hcl
packer {
  required_plugins {
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = "~> 1"
    }
  }
}
```

Alternatively, you can use `packer plugins install` to manage installation of this plugin.

```sh
$ packer plugins install github.com/hashicorp/amazon
```

### Components

**Don't know which builder to use?** If in doubt, use the [amazon-ebs builder](/packer/plugins/builders/amazon/ebs).
It is much easier to use and Amazon generally recommends EBS-backed images nowadays.

#### Builders
- [amazon-ebs](/packer/integrations/hashicorp/amazon/latest/components/builder/ebs) - Create EBS-backed AMIs by
  launching a source AMI and re-packaging it into a new AMI after
  provisioning. If in doubt, use this builder, which is the easiest to get
  started with.
- [amazon-instance](/packer/integrations/hashicorp/amazon/latest/components/builder/instance) - Create
  instance-store AMIs by launching and provisioning a source instance, then
  rebundling it and uploading it to S3.
- [amazon-chroot](/packer/integrations/hashicorp/amazon/latest/components/builder/chroot) - Create EBS-backed AMIs
  from an existing EC2 instance by mounting the root device and using a
  [Chroot](https://en.wikipedia.org/wiki/Chroot) environment to provision
  that device. This is an **advanced builder and should not be used by
  newcomers**. However, it is also the fastest way to build an EBS-backed AMI
  since no new EC2 instance needs to be launched.
- [amazon-ebssurrogate](/packer/integrations/hashicorp/amazon/latest/components/builder/ebssurrogate) - Create EBS
  -backed AMIs from scratch. Works similarly to the `chroot` builder but does
  not require running in AWS. This is an **advanced builder and should not be
  used by newcomers**.
- [amazon-ebs-volume](/packer/integrations/hashicorp/amazon/latest/components/builder/ebsvolume) - Create prepopulated
  EBS volumes by launching an instance and provisioning attached volumes.
  This is an **advanced builder and should not be used by newcomers**.

#### Data sources
- [amazon-ami](/packer/integrations/hashicorp/amazon/latest/components/data-source/ami) - Filter and fetch an Amazon AMI to output all the AMI information.
- [amazon-secretsmanager](/packer/integrations/hashicorp/amazon/latest/components/data-source/secretsmanager) - Retrieve information
  about a Secrets Manager secret version, including its secret value.
- [amazon-parameterstore](/packer/integrations/hashicorp/amazon/latest/components/data-source/parameterstore) - Retrieve information about a parameter in SSM.

#### Post-Processors
- [amazon-import](/packer/integrations/hashicorp/amazon/latest/components/post-processor/import) -  The Amazon Import post-processor takes an OVA artifact 
  from various builders and imports it to an AMI available to Amazon Web Services EC2.

### Authentication

The AWS provider offers a flexible means of providing credentials for
authentication. The following methods are supported, in this order, and
explained below:

- Static credentials
- Environment variables
- Shared credentials file
- EC2 Role

#### Static Credentials

Static credentials can be provided in the form of an access key id and secret.
These look like:

```json
"builders": {
  "type": "amazon-ebs",
  "access_key": "AKIAIOSFODNN7EXAMPLE",
  "secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
  "region": "us-east-1",
}
```

```hcl
source "amazon-ebs" "basic-example" {
  access_key = "AKIAIOSFODNN7EXAMPLE"
  secret_key = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
  region     = "us-east-1"
}
```

If you would like, you may also assume a role using the assume_role
configuration option. You must still have one of the valid credential resources
explained above, and your user must have permission to assume the role in
question. This is a way of running Packer with a more restrictive set of
permissions than your user.

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
"builders": [{
	"type": "amazon-ebs",
	"assume_role": {
		"role_arn"    :  "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME",
		"session_name":  "SESSION_NAME",
		"external_id" :  "EXTERNAL_ID"
	}
}]
```

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

#### Environment variables

You can provide your credentials via the `AWS_ACCESS_KEY_ID` and
`AWS_SECRET_ACCESS_KEY`, environment variables, representing your AWS Access
Key and AWS Secret Key, respectively. Note that setting your AWS credentials
using either these environment variables will override the use of
`AWS_SHARED_CREDENTIALS_FILE` and `AWS_PROFILE`. The `AWS_DEFAULT_REGION` and
`AWS_SESSION_TOKEN` environment variables are also used, if applicable:

Usage:

    $ export AWS_ACCESS_KEY_ID="anaccesskey"
    $ export AWS_SECRET_ACCESS_KEY="asecretkey"
    $ export AWS_DEFAULT_REGION="us-west-2"
    $ packer build template.pkr.hcl

#### Shared Credentials file

You can use an AWS credentials file to specify your credentials. The default
location is `$HOME/.aws/credentials` on Linux and OS X, or
`%USERPROFILE%.aws\credentials` for Windows users. If we fail to detect
credentials inline, or in the environment, the Amazon Plugin will check this location. You
can optionally specify a different location in the configuration by setting the
environment with the `AWS_SHARED_CREDENTIALS_FILE` variable.

The format for the credentials file is like so

    [default]
    aws_access_key_id=<your access key id>
    aws_secret_access_key=<your secret access key>

You may also configure the profile to use by setting the `profile`
configuration option, or setting the `AWS_PROFILE` environment variable:

```json
"builders": {
  "type": "amazon-ebs"
  "profile": "customprofile",
  "region": "us-east-1",
}
```

```hcl
source "amazon-ebs" "basic-example" {
  profile = "customprofile"
  region = "us-east-1"
}
```

#### IAM Task or Instance Role

Finally, the plugin will use credentials provided by the task's or instance's IAM
role, if it has one.

This is a preferred approach over any other when running in EC2 as you can
avoid hard coding credentials. Instead these are leased on-the-fly by the plugin,
which reduces the chance of leakage.

The following policy document provides the minimal set permissions necessary
for the Amazon plugin to work:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:AttachVolume",
        "ec2:AuthorizeSecurityGroupIngress",
        "ec2:CopyImage",
        "ec2:CreateImage",
        "ec2:CreateKeyPair",
        "ec2:CreateSecurityGroup",
        "ec2:CreateSnapshot",
        "ec2:CreateTags",
        "ec2:CreateVolume",
        "ec2:DeleteKeyPair",
        "ec2:DeleteSecurityGroup",
        "ec2:DeleteSnapshot",
        "ec2:DeleteVolume",
        "ec2:DeregisterImage",
        "ec2:DescribeImageAttribute",
        "ec2:DescribeImages",
        "ec2:DescribeInstances",
        "ec2:DescribeInstanceStatus",
        "ec2:DescribeRegions",
        "ec2:DescribeSecurityGroups",
        "ec2:DescribeSnapshots",
        "ec2:DescribeSubnets",
        "ec2:DescribeTags",
        "ec2:DescribeVolumes",
        "ec2:DetachVolume",
        "ec2:GetPasswordData",
        "ec2:ModifyImageAttribute",
        "ec2:ModifyInstanceAttribute",
        "ec2:ModifySnapshotAttribute",
        "ec2:RegisterImage",
        "ec2:RunInstances",
        "ec2:StopInstances",
        "ec2:TerminateInstances"
      ],
      "Resource": "*"
    }
  ]
}
```

Note that if you'd like to create a spot instance, you must also add:

    ec2:CreateLaunchTemplate,
    ec2:DeleteLaunchTemplate,
    ec2:CreateFleet

If you have the `spot_price` parameter set to `auto`, you must also add:

    ec2:DescribeSpotPriceHistory

If you are using the `vpc_filter` option, you must also add:

    ec2:DescribeVpcs

This permission may also be needed by the `associate_public_ip_address` option, if specified without a subnet.
In this case the plugin will invoke `DescribeVpcs` to find information about the default VPC.

When using `associate_public_ip_address` without a subnet, you will also benefit from having:

    ec2:DescribeInstanceTypeOfferings

This will ensure that the plugin will pick a subnet/AZ that can host the type of instance
you're requesting in your template.

If you are using the `deprecate_at` attribute in your templates, you will also need:

    ec2:EnableImageDeprecation

If you are using SSM to connect to the instance, and are specifying a private key file, you must also add:

    ec2-instance-connect:SendSSHPublicKey

If you are building a Windows AMI, and want to enable fast-launch, you will also need:

    ec2:EnableFastLaunch
    ec2:DescribeLaunchTemplates
    ec2:DescribeFastLaunchImages

### Troubleshooting

#### Attaching IAM Policies to Roles

IAM policies can be associated with users or roles. If you use the plugin with IAM
roles, you may encounter an error like this one:

    ==> amazon-ebs: Error launching source instance: You are not authorized to perform this operation.

You can read more about why this happens on the [Amazon Security
Blog](https://blogs.aws.amazon.com/security/post/Tx3M0IFB5XBOCQX/Granting-Permission-to-Launch-EC2-Instances-with-IAM-Roles-PassRole-Permission).
The example policy below may help the plugin work with IAM roles. Note that this
example provides more than the minimal set of permissions needed for the Amazon plugin to
work, but specifics will depend on your use-case.

```json
{
  "Sid": "PackerIAMPassRole",
  "Effect": "Allow",
  "Action": ["iam:PassRole", "iam:GetInstanceProfile"],
  "Resource": ["*"]
}
```

If using an existing instance profile with spot instances/spot pricing, the `iam:CreateServiceLinkedRole` action is also required:

```json
{
  "Sid": "PackerIAMPassRole",
  "Effect": "Allow",
  "Action": ["iam:PassRole", "iam:GetInstanceProfile", "iam:CreateServiceLinkedRole"],
  "Resource": ["*"]
}
```

In case when you're creating a temporary instance profile you will require to have following
IAM policies.

```json
{
  "Sid": "PackerIAMCreateRole",
  "Effect": "Allow",
  "Action": [
    "iam:PassRole",
    "iam:CreateInstanceProfile",
    "iam:DeleteInstanceProfile",
    "iam:GetRole",
    "iam:GetInstanceProfile",
    "iam:DeleteRolePolicy",
    "iam:RemoveRoleFromInstanceProfile",
    "iam:CreateRole",
    "iam:DeleteRole",
    "iam:PutRolePolicy",
    "iam:AddRoleToInstanceProfile"
  ],
  "Resource": "*"
}
```

In cases where you are using a KMS key for encryption, your key will need the
following policies at a minimum:

```json
{
  "Sid": "Allow use of the key",
  "Effect": "Allow",
  "Action": ["kms:ReEncrypt*", "kms:GenerateDataKey*"],
  "Resource": "*"
}
```

If you are using a key provided by a different account than the one you are
using to run the Packer build, your key will also need

```json
("kms:CreateGrant", "kms:DescribeKey")
```

#### Check System Time

Amazon uses the current time as part of the [request signing process](http://docs.aws.amazon.com/general/latest/gr/sigv4_signing.html). If
your system clock is too skewed from the current time, your requests might
fail. If that's the case, you might see an error like this:

    ==> amazon-ebs: Error querying AMI: AuthFailure: AWS was not able to validate the provided access credentials

If you suspect your system's date is wrong, you can compare it against
`http://www.time.gov/`. On Linux/OS X, you can run the `date` command to get the current time. If you're
on Linux, you can try setting the time with ntp by running `sudo ntpd -q`.

#### ResourceNotReady Error

This error generally appears as either `ResourceNotReady: exceeded wait attempts` or `ResourceNotReady: failed waiting for successful resource state`.

This opaque error gets returned from AWS's API for a number of reasons,
generally during image copy/encryption. Possible reasons for the error include:

- You aren't waiting long enough. This is where you'll see the `exceeded wait attempts` variety of this error message:
  We use the AWS SDK's built-in waiters to wait for longer-running tasks to
  complete. These waiters have default delays between queries and maximum
  number of queries that don't always work for our users.

  If you find that you are being rate-limited or have exceeded your max wait
  attempts, you can override the defaults by setting the following packer
  environment variables (note that these will apply to all AWS tasks that we
  have to wait for):

  - `AWS_MAX_ATTEMPTS` - This is how many times to re-send a status update
    request. Excepting tasks that we know can take an extremely long time, this
    defaults to 40 tries.

  - `AWS_POLL_DELAY_SECONDS` - How many seconds to wait in between status update
    requests. Generally defaults to 2 or 5 seconds, depending on the task.

  Alternatively, you can configure these settings in source section of the packer
  configuration file, for example:
  ```
  aws_polling {
    delay_seconds = 40
    max_attempts  = 5
  }
  ```

- You are using short-lived credentials that expired during the build. If this
  is the problem, you may also see `RequestExpired: Request has expired.`
  errors displayed in the Packer output:

  - If you are using STS credentials, make sure that they expire only after the
    build has completed

  - If you are chaining roles, make sure your build doesn't last more than an
    hour, since when you chain roles the maximum length of time your credentials
    will last is an hour:
    https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use.html

- Something is wrong with your KMS key. This is where you'll see the
  `ResourceNotReady: failed waiting for successful resource state` variety of
  this error message. Issues we've seen include:
  - Your KMS key is invalid, possibly because of a typo
  - Your KMS key is valid but does not have the necessary permissions (see
    above for the necessary key permissions)
  - Your KMS key is valid, but not in the region you've told us to use it in.
