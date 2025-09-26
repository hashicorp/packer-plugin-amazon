// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go/middleware"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
)

type mockEC2Client struct {
	clients.Ec2Client
}

func (m *mockEC2Client) DescribeRegions(ctx context.Context, params *ec2.DescribeRegionsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	return &ec2.DescribeRegionsOutput{
		Regions: []types.Region{
			{RegionName: aws.String("us-east-1")},
			{RegionName: aws.String("us-east-2")},
			{RegionName: aws.String("us-west-1")},
		},
		ResultMetadata: middleware.Metadata{},
	}, nil
}

func FakeAccessConfig() *AccessConfig {
	accessConfig := AccessConfig{
		getEC2Client: func() clients.Ec2Client {
			return &mockEC2Client{}
		},
		PollingConfig: new(AWSPollingConfig),
	}
	accessConfig.config = mustLoadConfig(config.WithRegion("us-west-1"))
	return &accessConfig
}

func mustLoadConfig(optFns ...func(*config.LoadOptions) error) *aws.Config {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		optFns...,
	)
	if err != nil {
		panic(fmt.Sprintf("failed loading config, %v", err))
	}
	return &cfg
}
