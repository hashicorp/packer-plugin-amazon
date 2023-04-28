// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

type TagMap map[string]string
type EC2Tags []*ec2.Tag

func (t EC2Tags) Report(ui packersdk.Ui) {
	for _, tag := range t {
		ui.Message(fmt.Sprintf("Adding tag: \"%s\": \"%s\"",
			aws.StringValue(tag.Key), aws.StringValue(tag.Value)))
	}
}

func (t TagMap) EC2Tags(ictx interpolate.Context, region string, state multistep.StateBag) (EC2Tags, error) {
	var ec2Tags []*ec2.Tag
	generatedData := packerbuilderdata.GeneratedData{State: state}
	ictx.Data = extractBuildInfo(region, state, &generatedData)

	for key, value := range t {
		interpolatedKey, err := interpolate.Render(key, &ictx)
		if err != nil {
			return nil, fmt.Errorf("Error processing tag: %s:%s - %s", key, value, err)
		}
		interpolatedValue, err := interpolate.Render(value, &ictx)
		if err != nil {
			return nil, fmt.Errorf("Error processing tag: %s:%s - %s", key, value, err)
		}
		ec2Tags = append(ec2Tags, &ec2.Tag{
			Key:   aws.String(interpolatedKey),
			Value: aws.String(interpolatedValue),
		})
	}
	return ec2Tags, nil
}

func (t TagMap) IamTags(ictx interpolate.Context, region string, state multistep.StateBag) ([]*iam.Tag, error) {
	var iamTags []*iam.Tag
	generatedData := packerbuilderdata.GeneratedData{State: state}
	ictx.Data = extractBuildInfo(region, state, &generatedData)

	for key, value := range t {
		interpolatedKey, err := interpolate.Render(key, &ictx)
		if err != nil {
			return nil, fmt.Errorf("Error processing tag: %s:%s - %s", key, value, err)
		}
		interpolatedValue, err := interpolate.Render(value, &ictx)
		if err != nil {
			return nil, fmt.Errorf("Error processing tag: %s:%s - %s", key, value, err)
		}
		iamTags = append(iamTags, &iam.Tag{
			Key:   aws.String(interpolatedKey),
			Value: aws.String(interpolatedValue),
		})
	}
	return iamTags, nil
}

func (t EC2Tags) TagSpecifications(resourceType ...string) []*ec2.TagSpecification {
	var tagSpecs []*ec2.TagSpecification
	if len(t) > 0 {
		for _, resource := range resourceType {
			runTags := &ec2.TagSpecification{
				ResourceType: aws.String(resource),
				Tags:         t,
			}
			tagSpecs = append(tagSpecs, runTags)
		}
	}
	return tagSpecs
}
