The Secrets Manager data source provides information about a Secrets Manager secret version,
including its secret value.

-> **Note:** Data sources is a feature exclusively available to HCL2 templates.

Basic examples of usage:

```hcl
data "amazon-secretsmanager" "basic-example" {
  name = "packer_test_secret"
  key  = "packer_test_key"
  version_stage = "example"
}

# usage example of the data source output
locals {
  value         = data.amazon-secretsmanager.basic-example.value
  secret_string = data.amazon-secretsmanager.basic-example.secret_string
  version_id    = data.amazon-secretsmanager.basic-example.version_id
  secret_value  = jsondecode(data.amazon-secretsmanager.basic-example.secret_string)["packer_test_key"]
}
```

Reading key-value pairs from JSON back into a native Packer map can be accomplished
with the [jsondecode() function](/packer/docs/templates/hcl_templates/functions/encoding/jsondecode).

## Configuration Reference

### Required

<!-- Code generated from the comments of the Config struct in datasource/secretsmanager/data.go; DO NOT EDIT MANUALLY -->

- `name` (string) - Specifies the secret containing the version that you want to retrieve.
  You can specify either the Amazon Resource Name (ARN) or the friendly name of the secret.

<!-- End of code generated from the comments of the Config struct in datasource/secretsmanager/data.go; -->


### Optional

<!-- Code generated from the comments of the Config struct in datasource/secretsmanager/data.go; DO NOT EDIT MANUALLY -->

- `key` (string) - Optional key for JSON secrets that contain more than one value. When set, the `value` output will
  contain the value for the provided key.

- `version_id` (string) - Specifies the unique identifier of the version of the secret that you want to retrieve.
  Overrides version_stage.

- `version_stage` (string) - Specifies the secret version that you want to retrieve by the staging label attached to the version.
  Defaults to AWSCURRENT.

<!-- End of code generated from the comments of the Config struct in datasource/secretsmanager/data.go; -->


## Output Data

<!-- Code generated from the comments of the DatasourceOutput struct in datasource/secretsmanager/data.go; DO NOT EDIT MANUALLY -->

- `value` (string) - When a [key](#key) is provided, this will be the value for that key. If a key is not provided,
  `value` will contain the first value found in the secret string.

- `secret_string` (string) - The decrypted part of the protected secret information that
  was originally provided as a string.

- `secret_binary` (string) - The decrypted part of the protected secret information that
  was originally provided as a binary. Base64 encoded.

- `version_id` (string) - The unique identifier of this version of the secret.

<!-- End of code generated from the comments of the DatasourceOutput struct in datasource/secretsmanager/data.go; -->


## Authentication

The Amazon Data Sources authentication works just like for the [Amazon Builders](/packer/integrations/hashicorp/amazon). Both
have the same authentication options, and you can refer to the [Amazon Builders authentication](/packer/integrations/hashicorp/amazon#authentication)
to learn the options to authenticate for data sources.

-> **Note:** A data source will start and execute in your own authentication session. The authentication in the data source
doesn't relate with the authentication on Amazon Builders.

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
