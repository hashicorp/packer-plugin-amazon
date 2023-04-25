Type: `amazon-ami`

The Amazon AMI data source will filter and fetch an Amazon AMI, and output all the AMI information that will
be then available to use in the [Amazon builders](/packer/integrations/BrandonRomano/amazon).

-> **Note:** Data sources is a feature exclusively available to HCL2 templates.

Basic example of usage:

```hcl
data "amazon-ami" "basic-example" {
    filters = {
        virtualization-type = "hvm"
        name = "ubuntu/images/*ubuntu-xenial-16.04-amd64-server-*"
        root-device-type = "ebs"
    }
    owners = ["099720109477"]
    most_recent = true
}
```
This selects the most recent Ubuntu 16.04 HVM EBS AMI from Canonical. Note that the data source will fail unless
*exactly* one AMI is returned. In the above example, `most_recent` will cause this to succeed by selecting the newest image.

## Configuration Reference

<!-- Code generated from the comments of the AmiFilterOptions struct in builder/common/ami_filter.go; DO NOT EDIT MANUALLY -->

- `filters` (map[string]string) - Filters used to select an AMI. Any filter described in the docs for
  [DescribeImages](http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeImages.html)
  is valid.

- `owners` ([]string) - Filters the images by their owner. You
  may specify one or more AWS account IDs, "self" (which will use the
  account whose credentials you are using to run Packer), or an AWS owner
  alias: for example, `amazon`, `aws-marketplace`, or `microsoft`. This
  option is required for security reasons.

- `most_recent` (bool) - Selects the newest created image when true.
  This is most useful for selecting a daily distro build.

<!-- End of code generated from the comments of the AmiFilterOptions struct in builder/common/ami_filter.go; -->


## Output Data

<!-- Code generated from the comments of the DatasourceOutput struct in datasource/ami/data.go; DO NOT EDIT MANUALLY -->

- `id` (string) - The ID of the AMI.

- `name` (string) - The name of the AMI.

- `creation_date` (string) - The date of creation of the AMI.

- `owner` (string) - The AWS account ID of the owner.

- `owner_name` (string) - The owner alias.

- `tags` (map[string]string) - The key/value combination of the tags assigned to the AMI.

<!-- End of code generated from the comments of the DatasourceOutput struct in datasource/ami/data.go; -->


## Authentication

The authentication for Amazon Data Sources uses the same configuration options as [Amazon Builders](/packer/integrations/BrandonRomano/amazon). To learn more about all of the available authentication options please see [Amazon Builders authentication](/packer/integrations/BrandonRomano/amazon#authentication).

-> **Note:** The authentication session started by a data source is separate from any authentication sessions started by an Amazon builder. Users are encouraged to use `variables` for defining and sharing configuration values between datasources and builders.

Basic example of an Amazon data source authentication using `assume_role`:

```hcl
data "amazon-secretsmanager" "basic-example" {
  name = "packer_test_secret"
  key  = "packer_test_key"

  assume_role {
      role_arn     = "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
      session_name = "SESSION_NAME"
      external_id  = "EXTERNAL_ID"
  }
}
```
