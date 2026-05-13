// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
)

type mockEC2Client struct {
	clients.Ec2Client
}

func FakeAccessConfig() *AccessConfig {
	accessConfig := AccessConfig{
		getEC2Connection: func() clients.Ec2Client {
			return &mockEC2Client{}
		},
		PollingConfig: new(AWSPollingConfig),
	}
	accessConfig.awsConfig = &aws.Config{
		Region: "us-east-1",
	}
	return &accessConfig
}
