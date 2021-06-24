//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type DatasourceOutput,Config

package parameterstore

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/hashicorp/hcl/v2/hcldec"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
	"github.com/hashicorp/packer-plugin-amazon/builder/common/awserrors"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/hcl2helper"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/zclconf/go-cty/cty"
)

type Datasource struct {
	config Config
}

type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`

	// The name of the parameter you want to query.
	Name string `mapstructure:"name" required:"true"`
	// Return decrypted values for secure string parameters.
	// This flag is ignored for String and StringList parameter types.
	WithDecryption bool `mapstructure:"with_decryption"`
}

func (d *Datasource) ConfigSpec() hcldec.ObjectSpec {
	return d.config.FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Configure(raws ...interface{}) error {
	err := config.Decode(&d.config, nil, raws...)
	if err != nil {
		return err
	}
	var errs *packersdk.MultiError
	errs = packersdk.MultiErrorAppend(errs, d.config.AccessConfig.Prepare(&d.config.PackerConfig)...)

	if d.config.Name == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("a 'name' must be provided"))
	}
	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}
	return nil
}

type DatasourceOutput struct {
	// The parameter value.
	Value string `mapstructure:"value"`
	// The parameter version.
	Version string `mapstructure:"version"`
	// The Amazon Resource Name (ARN) of the parameter.
	ARN string `mapstructure:"arn"`
}

func (d *Datasource) OutputSpec() hcldec.ObjectSpec {
	return (&DatasourceOutput{}).FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Execute() (cty.Value, error) {
	session, err := d.config.Session()
	if err != nil {
		return cty.NullVal(cty.EmptyObject), err
	}
	ssmsvc := ssm.New(session, aws.NewConfig().WithRegion(*session.Config.Region))
	param, err := ssmsvc.GetParameter(&ssm.GetParameterInput{Name: &d.config.Name, WithDecryption: &d.config.WithDecryption})

	if err != nil {
		if awserrors.Matches(err, ssm.ErrCodeParameterNotFound, "") {
			return cty.NullVal(cty.EmptyObject), fmt.Errorf("The parameter name %q not found", d.config.Name)
		}
		if awserrors.Matches(err, ssm.ErrCodeParameterVersionNotFound, "") {
			return cty.NullVal(cty.EmptyObject), fmt.Errorf("The parameter version %q not found", d.config.Name)
		}
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("error to get parameter value: %q", err.Error())
	}
	output := DatasourceOutput{
		Value:   aws.StringValue(param.Parameter.Value),
		Version: fmt.Sprintf("%d", aws.Int64Value(param.Parameter.Version)),
		ARN:     aws.StringValue(param.Parameter.ARN),
	}
	return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
}
