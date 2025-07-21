// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

type Ec2Client interface {
	ec2.DescribeImagesAPIClient
	DescribeRegions(ctx context.Context, params *ec2.DescribeRegionsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error)
	ImportImage(ctx context.Context, params *ec2.ImportImageInput, optFns ...func(*ec2.Options)) (*ec2.ImportImageOutput, error)
	ec2.DescribeImportImageTasksAPIClient
	CopyImage(ctx context.Context, params *ec2.CopyImageInput, optFns ...func(*ec2.Options)) (*ec2.CopyImageOutput, error)
	DeregisterImage(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error)
	DeleteSnapshot(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error)
	CreateTags(ctx context.Context, params *ec2.CreateTagsInput, optFns ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
	ModifyImageAttribute(ctx context.Context, params *ec2.ModifyImageAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifyImageAttributeOutput, error)
}
