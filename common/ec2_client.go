// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

type Ec2Client interface {
	ec2.DescribeImagesAPIClient
	ec2.DescribeInstancesAPIClient

	CreateLaunchTemplate(ctx context.Context, params *ec2.CreateLaunchTemplateInput, optFns ...func(*ec2.Options)) (*ec2.CreateLaunchTemplateOutput, error)
	CreateFleet(ctx context.Context, params *ec2.CreateFleetInput, optFns ...func(*ec2.Options)) (*ec2.CreateFleetOutput, error)
	CreateTags(ctx context.Context, params *ec2.CreateTagsInput, optFns ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)

	DescribeRegions(ctx context.Context, params *ec2.DescribeRegionsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error)
	DeleteLaunchTemplate(ctx context.Context, params *ec2.DeleteLaunchTemplateInput, optFns ...func(*ec2.Options)) (*ec2.DeleteLaunchTemplateOutput, error)

	TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}
