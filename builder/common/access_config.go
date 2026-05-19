// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type VaultAWSEngineOptions,AssumeRoleConfig

package common

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	awsbase "github.com/hashicorp/aws-sdk-go-base/v2"
	basediag "github.com/hashicorp/aws-sdk-go-base/v2/diag"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"

	"github.com/hashicorp/packer-plugin-sdk/common"
	vaultapi "github.com/hashicorp/vault/api"
)

// AssumeRoleConfig lets users set configuration options for assuming a special
// role when executing Packer.
//
// Usage example:
//
// HCL config example:
//
// ```HCL
//
//	source "amazon-ebs" "example" {
//		assume_role {
//			role_arn     = "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
//			session_name = "SESSION_NAME"
//			external_id  = "EXTERNAL_ID"
//		}
//	}
//
// ```
//
// JSON config example:
//
// ```json
//
//	builder{
//		"type": "amazon-ebs",
//		"assume_role": {
//			"role_arn"    :  "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME",
//			"session_name":  "SESSION_NAME",
//			"external_id" :  "EXTERNAL_ID"
//		}
//	}
//
// ```
type AssumeRoleConfig struct {
	// Amazon Resource Name (ARN) of the IAM Role to assume.
	AssumeRoleARN string `mapstructure:"role_arn" required:"false"`
	// Number of seconds to restrict the assume role session duration.
	AssumeRoleDurationSeconds int `mapstructure:"duration_seconds" required:"false"`
	// The external ID to use when assuming the role. If omitted, no external
	// ID is passed to the AssumeRole call.
	AssumeRoleExternalID string `mapstructure:"external_id" required:"false"`
	// IAM Policy JSON describing further restricting permissions for the IAM
	// Role being assumed.
	AssumeRolePolicy string `mapstructure:"policy" required:"false"`
	// Set of Amazon Resource Names (ARNs) of IAM Policies describing further
	// restricting permissions for the IAM Role being
	AssumeRolePolicyARNs []string `mapstructure:"policy_arns" required:"false"`
	// Session name to use when assuming the role.
	AssumeRoleSessionName string `mapstructure:"session_name" required:"false"`
	// Map of assume role session tags.
	AssumeRoleTags map[string]string `mapstructure:"tags" required:"false"`
	// Set of assume role session tag keys to pass to any subsequent sessions.
	AssumeRoleTransitiveTagKeys []string `mapstructure:"transitive_tag_keys" required:"false"`
}

type VaultAWSEngineOptions struct {
	Name    string `mapstructure:"name"`
	RoleARN string `mapstructure:"role_arn"`
	// Specifies the TTL for the use of the STS token. This
	// is specified as a string with a duration suffix. Valid only when
	// credential_type is assumed_role or federation_token. When not
	// specified, the default_sts_ttl set for the role will be used. If that
	// is also not set, then the default value of 3600s will be used. AWS
	// places limits on the maximum TTL allowed. See the AWS documentation on
	// the DurationSeconds parameter for AssumeRole (for assumed_role
	// credential types) and GetFederationToken (for federation_token
	// credential types) for more details.
	TTL        string `mapstructure:"ttl" required:"false"`
	EngineName string `mapstructure:"engine_name"`
}

func (v *VaultAWSEngineOptions) Empty() bool {
	return len(v.Name) == 0 && len(v.RoleARN) == 0 &&
		len(v.EngineName) == 0 && len(v.TTL) == 0
}

// AccessConfig is for common configuration related to AWS access
type AccessConfig struct {
	// The access key used to communicate with AWS. [Learn how  to set this](/packer/plugins/builders/amazon#specifying-amazon-credentials).
	// On EBS, this is not required if you are using `use_vault_aws_engine`
	// for authentication instead.
	AccessKey string `mapstructure:"access_key" required:"true"`
	// If provided with a role ARN, Packer will attempt to assume this role
	// using the supplied credentials. See
	// [AssumeRoleConfig](#assume-role-configuration) below for more
	// details on all of the options available, and for a usage example.
	AssumeRole AssumeRoleConfig `mapstructure:"assume_role" required:"false"`
	// This option is useful if you use a cloud
	// provider whose API is compatible with aws EC2. Specify another endpoint
	// like this https://ec2.custom.endpoint.com.
	CustomEndpointEc2 string `mapstructure:"custom_endpoint_ec2" required:"false"`
	// Path to a credentials file to load credentials from
	CredsFilename string `mapstructure:"shared_credentials_file" required:"false"`
	// Enable automatic decoding of any encoded authorization (error) messages
	// using the `sts:DecodeAuthorizationMessage` API. Note: requires that the
	// effective user/role have permissions to `sts:DecodeAuthorizationMessage`
	// on resource `*`. Default `false`.
	DecodeAuthZMessages bool `mapstructure:"decode_authorization_messages" required:"false"`
	// This allows skipping TLS
	// verification of the AWS EC2 endpoint. The default is false.
	InsecureSkipTLSVerify bool `mapstructure:"insecure_skip_tls_verify" required:"false"`
	// This is the maximum number of times an API call is retried, in the case
	// where requests are being throttled or experiencing transient failures.
	// The delay between the subsequent API calls increases exponentially.
	MaxRetries int `mapstructure:"max_retries" required:"false"`
	// The MFA
	// [TOTP](https://en.wikipedia.org/wiki/Time-based_One-time_Password_Algorithm)
	// code. This should probably be a user variable since it changes all the
	// time.
	MFACode string `mapstructure:"mfa_code" required:"false"`
	// The profile to use in the shared credentials file for
	// AWS. See Amazon's documentation on [specifying
	// profiles](https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#specifying-profiles)
	// for more details.
	ProfileName string `mapstructure:"profile" required:"false"`
	// The name of the region, such as `us-east-1`, in which
	// to launch the EC2 instance to create the AMI.
	// When chroot building, this value is guessed from environment.
	RawRegion string `mapstructure:"region" required:"true"`
	// The secret key used to communicate with AWS. [Learn how to set
	// this](/packer/plugins/builders/amazon#specifying-amazon-credentials). This is not required
	// if you are using `use_vault_aws_engine` for authentication instead.
	SecretKey            string `mapstructure:"secret_key" required:"true"`
	SkipMetadataApiCheck bool   `mapstructure:"skip_metadata_api_check"`
	// Set to true if you want to skip validating AWS credentials before runtime.
	SkipCredsValidation bool `mapstructure:"skip_credential_validation"`
	// The access token to use. This is different from the
	// access key and secret key. If you're not sure what this is, then you
	// probably don't need it. This will also be read from the AWS_SESSION_TOKEN
	// environmental variable.
	Token string `mapstructure:"token" required:"false"`
	// Get credentials from HashiCorp Vault's aws secrets engine. You must
	// already have created a role to use. For more information about
	// generating credentials via the Vault engine, see the [Vault
	// docs.](https://www.vaultproject.io/api/secret/aws#generate-credentials)
	// If you set this flag, you must also set the below options:
	// -   `name` (string) - Required. Specifies the name of the role to generate
	//     credentials against. This is part of the request URL.
	// -   `engine_name` (string) - The name of the aws secrets engine. In the
	//     Vault docs, this is normally referred to as "aws", and Packer will
	//     default to "aws" if `engine_name` is not set.
	// -   `role_arn` (string)- The ARN of the role to assume if credential\_type
	//     on the Vault role is assumed\_role. Must match one of the allowed role
	//     ARNs in the Vault role. Optional if the Vault role only allows a single
	//     AWS role ARN; required otherwise.
	// -   `ttl` (string) - Specifies the TTL for the use of the STS token. This
	//     is specified as a string with a duration suffix. Valid only when
	//     credential\_type is assumed\_role or federation\_token. When not
	//     specified, the default\_sts\_ttl set for the role will be used. If that
	//     is also not set, then the default value of 3600s will be used. AWS
	//     places limits on the maximum TTL allowed. See the AWS documentation on
	//     the DurationSeconds parameter for AssumeRole (for assumed\_role
	//     credential types) and GetFederationToken (for federation\_token
	//     credential types) for more details.
	//
	// HCL2 example:
	//
	// ```hcl
	// vault_aws_engine {
	//     name = "myrole"
	//     role_arn = "myarn"
	//     ttl = "3600s"
	// }
	// ```
	//
	// JSON example:
	//
	// ```json
	// {
	//     "vault_aws_engine": {
	//         "name": "myrole",
	//         "role_arn": "myarn",
	//         "ttl": "3600s"
	//     }
	// }
	// ```
	VaultAWSEngine VaultAWSEngineOptions `mapstructure:"vault_aws_engine" required:"false"`
	// [Polling configuration](#polling-configuration) for the AWS waiter. Configures the waiter that checks
	// resource state.
	PollingConfig    *AWSPollingConfig `mapstructure:"aws_polling" required:"false"`
	awsConfig        *aws.Config
	getEC2Connection func() clients.Ec2Client

	// packerConfig is set by Prepare() containing information about Packer,
	// including the CorePackerVersionString
	packerConfig *common.PackerConfig
}

func (c *AccessConfig) GetAWSConfig(ctx context.Context) (*aws.Config, error) {

	// Reload values into the config used by the Packer-Terraform shared SDK
	var assumeRoles []awsbase.AssumeRole
	if c.AssumeRole.AssumeRoleARN != "" {
		awsbaseAssumeRole := awsbase.AssumeRole{
			RoleARN:           c.AssumeRole.AssumeRoleARN,
			Duration:          time.Duration(c.AssumeRole.AssumeRoleDurationSeconds) * time.Second,
			ExternalID:        c.AssumeRole.AssumeRoleExternalID,
			Policy:            c.AssumeRole.AssumeRolePolicy,
			PolicyARNs:        c.AssumeRole.AssumeRolePolicyARNs,
			SessionName:       c.AssumeRole.AssumeRoleSessionName,
			Tags:              c.AssumeRole.AssumeRoleTags,
			TransitiveTagKeys: c.AssumeRole.AssumeRoleTransitiveTagKeys,
		}
		assumeRoles = append(assumeRoles, awsbaseAssumeRole)
	}

	awsbaseConfig := awsbase.Config{
		AccessKey:           c.AccessKey,
		AssumeRole:          assumeRoles,
		Insecure:            c.InsecureSkipTLSVerify,
		MaxRetries:          c.MaxRetries,
		Profile:             c.ProfileName,
		Region:              c.RawRegion,
		SecretKey:           c.SecretKey,
		SkipCredsValidation: c.SkipCredsValidation,
		Token:               c.Token,
	}

	_, awsConfig, awsDiags := awsbase.GetAwsConfig(ctx, &awsbaseConfig)

	for _, d := range awsDiags {
		switch d.Severity() {
		case basediag.SeverityWarning:
			log.Printf("[WARN] Detail: %s, Summary: %s", d.Detail(), d.Summary())
		case basediag.SeverityError:

			return nil, fmt.Errorf("Error: %s, Detail: %s", d.Summary(), d.Detail())

		}
	}

	c.awsConfig = &awsConfig
	return c.awsConfig, nil
}

func (c *AccessConfig) SessionRegion() string {
	if c.awsConfig == nil {
		panic("access config should be set.")
	}
	return c.awsConfig.Region
}

func (c *AccessConfig) IsGovCloud() bool {
	return strings.HasPrefix(c.SessionRegion(), "us-gov-")
}

func (c *AccessConfig) IsChinaCloud() bool {
	return strings.HasPrefix(c.SessionRegion(), "cn-")
}

func (c *AccessConfig) getCredsFromVault(cli *vaultapi.Client) (*vaultapi.Secret, error) {
	if len(c.VaultAWSEngine.RoleARN) > 0 {
		data := map[string]interface{}{
			"role_arn": c.VaultAWSEngine.RoleARN,
		}
		if len(c.VaultAWSEngine.TTL) > 0 {
			data["ttl"] = c.VaultAWSEngine.TTL
		}
		path := fmt.Sprintf("/%s/sts/%s", c.VaultAWSEngine.EngineName,
			c.VaultAWSEngine.Name)
		return cli.Logical().Write(path, data)
	}

	path := fmt.Sprintf("/%s/creds/%s", c.VaultAWSEngine.EngineName,
		c.VaultAWSEngine.Name)
	return cli.Logical().Read(path)
}

func (c *AccessConfig) GetCredsFromVault() error {
	// const EnvVaultAddress = "VAULT_ADDR"
	// const EnvVaultToken = "VAULT_TOKEN"
	vaultConfig := vaultapi.DefaultConfig()
	cli, err := vaultapi.NewClient(vaultConfig)
	if err != nil {
		return fmt.Errorf("Error getting Vault client: %s", err)
	}
	if c.VaultAWSEngine.EngineName == "" {
		c.VaultAWSEngine.EngineName = "aws"
	}

	secret, err := c.getCredsFromVault(cli)

	if err != nil {
		return fmt.Errorf("Error reading vault secret: %s", err)
	}
	if secret == nil {
		return fmt.Errorf("Vault Secret does not exist at the given path.")
	}

	c.AccessKey = secret.Data["access_key"].(string)
	c.SecretKey = secret.Data["secret_key"].(string)
	token := secret.Data["security_token"]
	if token != nil {
		c.Token = token.(string)
	} else {
		c.Token = ""
	}

	return nil
}

func (c *AccessConfig) Prepare(packerConfig *common.PackerConfig) []error {
	var errs []error

	if c.SkipMetadataApiCheck {
		log.Println("(WARN) skip_metadata_api_check ignored.")
	}

	// Make sure it's obvious from the config how we're getting credentials:
	// Vault, Packer config, or environment.
	if !c.VaultAWSEngine.Empty() {
		if len(c.AccessKey) > 0 {
			errs = append(errs,
				fmt.Errorf("If you have set vault_aws_engine, you must not set"+
					" the access_key or secret_key."))
		}
		// Go ahead and grab those credentials from Vault now, so we can set
		// the keys and token now.
		err := c.GetCredsFromVault()
		if err != nil {
			errs = append(errs, err)
		}
	}

	if (len(c.AccessKey) > 0) != (len(c.SecretKey) > 0) {
		errs = append(errs,
			fmt.Errorf("`access_key` and `secret_key` must both be either set or not set."))
	}

	if c.PollingConfig == nil {
		c.PollingConfig = new(AWSPollingConfig)
	}
	c.PollingConfig.LogEnvOverrideWarnings()
	c.PollingConfig.Prepare()

	// Default MaxRetries to 10, to make throttling issues less likely. The
	// Aws sdk defaults this to 3, which regularly gets tripped by users.
	if c.MaxRetries == 0 {
		c.MaxRetries = 10
	}

	c.packerConfig = packerConfig
	if c.packerConfig == nil {
		c.packerConfig = &common.PackerConfig{
			PackerCoreVersion: "unknown",
		}
	}

	return errs
}

func (c *AccessConfig) NewNoValidCredentialSourcesError(err error) error {
	return fmt.Errorf("No valid credential sources found for AWS Builder. "+
		"Please see https://www.packer.io/docs/builders/amazon#authentication "+
		"for more information on providing credentials for the AWS Builder. "+
		"Error: %w", err)
}

func (c *AccessConfig) NewEC2Connection(ctx context.Context) (clients.Ec2Client, error) {
	if c.getEC2Connection != nil {
		return c.getEC2Connection(), nil
	}
	awscfg, err := c.GetAWSConfig(ctx)
	if err != nil {
		return nil, err
	}

	ec2conn := ec2.NewFromConfig(awscfg.Copy(), func(o *ec2.Options) {
		o.Region = c.SessionRegion()
		if c.CustomEndpointEc2 != "" {
			o.BaseEndpoint = aws.String(c.CustomEndpointEc2)
		}
	})
	return ec2conn, nil
}
