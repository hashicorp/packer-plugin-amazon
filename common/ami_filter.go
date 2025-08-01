// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
package common

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type AmiFilterOptions struct {
	// Filters used to select an AMI. Any filter described in the docs for
	// [DescribeImages](http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeImages.html)
	// is valid.
	Filters map[string]string `mapstructure:"filters"`
	// Filters the images by their owner. You
	// may specify one or more AWS account IDs, "self" (which will use the
	// account whose credentials you are using to run Packer), or an AWS owner
	// alias: for example, `amazon`, `aws-marketplace`, or `microsoft`. This
	// option is required for security reasons.
	Owners []string `mapstructure:"owners"`
	// Selects the newest created image when true.
	// This is most useful for selecting a daily distro build.
	MostRecent bool `mapstructure:"most_recent"`
	// Include deprecated AMIs in the filtered response. Defaults to false.
	// If you are the AMI owner, deprecated AMIs appear in the response
	// regardless of what is specified for `include_deprecated`.
	IncludeDeprecated bool `mapstructure:"include_deprecated"`
}

func (d *AmiFilterOptions) GetOwners() []string {
	res := make([]string, 0, len(d.Owners))
	for _, owner := range d.Owners {
		i := owner
		res = append(res, i)
	}
	return res
}

func (d *AmiFilterOptions) Empty() bool {
	return len(d.Owners) == 0 && len(d.Filters) == 0
}

func (d *AmiFilterOptions) NoOwner() bool {
	return len(d.Owners) == 0
}

func prettyFilters(params *ec2.DescribeImagesInput) string {
	b, err := json.MarshalIndent(params.Filters, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (d *AmiFilterOptions) GetFilteredImage(ctx context.Context, params *ec2.DescribeImagesInput, client Ec2Client) (*types.Image, error) {
	// We have filters to apply
	if len(d.Filters) > 0 {
		amiFilters, err := buildEc2Filters(d.Filters)
		if err != nil {
			err := fmt.Errorf("Couldn't parse ami filters: %s", err)
			return nil, err
		}
		params.Filters = amiFilters
	}
	if len(d.Owners) > 0 {
		params.Owners = d.GetOwners()
	}

	params.IncludeDeprecated = &d.IncludeDeprecated

	log.Printf("Using AMI Filters %s", prettyFilters(params))
	imageResp, err := client.DescribeImages(ctx, params, func(o *ec2.Options) {
		o.Retryer = retry.NewStandard(func(so *retry.StandardOptions) {
			so.MaxAttempts = 11
		})
	})
	if err != nil {
		err := fmt.Errorf("Error querying AMI: %s", err)
		return nil, err
	}

	if len(imageResp.Images) == 0 {
		err := fmt.Errorf("No AMI was found matching filters: %v", params)
		return nil, err
	}

	if len(imageResp.Images) > 1 && !d.MostRecent {
		err := fmt.Errorf("Your query returned more than one result. Please try a more specific search, or set most_recent to true.")
		return nil, err
	}

	var image types.Image
	if d.MostRecent {
		image = mostRecentAmi(imageResp.Images)
	} else {
		image = imageResp.Images[0]
	}
	return &image, nil
}
