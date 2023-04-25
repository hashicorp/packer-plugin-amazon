The Parameter Store data source provides information about a parameter in SSM.

-> **Note:** Data sources is a feature exclusively available to HCL2 templates.

Basic examples of usage:

```hcl
data "amazon-parameterstore" "basic-example" {
  name = "packer_test_parameter"
  with_decryption = false
}

# usage example of the data source output
locals {
  value   = data.amazon-parameterstore.basic-example.value
  version = data.amazon-parameterstore.basic-example.version
  arn     = data.amazon-parameterstore.basic-example.arn
}
```

## Configuration Reference

### Required

<!-- Code generated from the comments of the Config struct in datasource/parameterstore/data.go; DO NOT EDIT MANUALLY -->

- `name` (string) - The name of the parameter you want to query.

<!-- End of code generated from the comments of the Config struct in datasource/parameterstore/data.go; -->


### Optional

<!-- Code generated from the comments of the Config struct in datasource/parameterstore/data.go; DO NOT EDIT MANUALLY -->

- `with_decryption` (bool) - Return decrypted values for secure string parameters.
  This flag is ignored for String and StringList parameter types.

<!-- End of code generated from the comments of the Config struct in datasource/parameterstore/data.go; -->


## Output Data

<!-- Code generated from the comments of the DatasourceOutput struct in datasource/parameterstore/data.go; DO NOT EDIT MANUALLY -->

- `value` (string) - The parameter value.

- `version` (string) - The parameter version.

- `arn` (string) - The Amazon Resource Name (ARN) of the parameter.

<!-- End of code generated from the comments of the DatasourceOutput struct in datasource/parameterstore/data.go; -->


## Authentication

The Amazon Data Sources authentication works just like for the [Amazon Builders](/packer/plugins/builders). Both
have the same authentication options, and you can refer to the [Amazon Builders authentication](/packer/integrations/BrandonRomano/index.mdx#authentication)
to learn the options to authenticate for data sources.

-> **Note:** A data source will start and execute in your own authentication session. The authentication in the data source
doesn't relate with the authentication on Amazon Builders.

Basic example of an Amazon data source authentication using `assume_role`:

```hcl
data "amazon-parameterstore" "basic-example" {
  name = "packer_test_parameter"
  with_decryption = false

  assume_role {
      role_arn     = "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
      session_name = "SESSION_NAME"
      external_id  = "EXTERNAL_ID"
  }
}
```
