// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type DatasourceOutput,Config
package ami

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/hashicorp/hcl/v2/hcldec"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
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
	common.PackerConfig        `mapstructure:",squash"`
	awscommon.AccessConfig     `mapstructure:",squash"`
	awscommon.AmiFilterOptions `mapstructure:",squash"`
}

func (d *Datasource) ConfigSpec() hcldec.ObjectSpec {
	return d.config.FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Configure(raws ...any) error {
	err := config.Decode(&d.config, nil, raws...)
	if err != nil {
		return err
	}

	var errs *packersdk.MultiError
	errs = packersdk.MultiErrorAppend(errs, d.config.AccessConfig.Prepare(&d.config.PackerConfig)...)

	if d.config.Empty() {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("The `filters` must be specified"))
	}
	if d.config.NoOwner() {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("For security reasons, you must declare an owner."))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}
	return nil
}

type DatasourceOutput struct {
	// The ID of the AMI.
	ID string `mapstructure:"id"`
	// The name of the AMI.
	Name string `mapstructure:"name"`
	// The date of creation of the AMI.
	CreationDate string `mapstructure:"creation_date"`
	// The AWS account ID of the owner.
	Owner string `mapstructure:"owner"`
	// The owner alias.
	OwnerName string `mapstructure:"owner_name"`
	// The key/value combination of the tags assigned to the AMI.
	Tags map[string]string `mapstructure:"tags"`
}

func (d *Datasource) OutputSpec() hcldec.ObjectSpec {
	return (&DatasourceOutput{}).FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Execute() (cty.Value, error) {
	ctx := context.TODO()
	client, err := d.config.NewEC2Client(ctx)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), err
	}

	image, err := d.config.AmiFilterOptions.GetFilteredImage(ctx, &ec2.DescribeImagesInput{}, client)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), err
	}

	imageTags := make(map[string]string, len(image.Tags))
	for _, tag := range image.Tags {
		imageTags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	output := DatasourceOutput{
		ID:           aws.ToString(image.ImageId),
		Name:         aws.ToString(image.Name),
		CreationDate: aws.ToString(image.CreationDate),
		Owner:        aws.ToString(image.OwnerId),
		OwnerName:    aws.ToString(image.ImageOwnerAlias),
		Tags:         imageTags,
	}
	return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
}
