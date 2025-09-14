// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
)

// Define a mock struct to be used in unit tests for common aws steps.
type mockEC2Conn struct {
	clients.Ec2Client
	Config *aws.Config

	// Counters to figure out what code path was taken
	copyImageCount       int
	describeImagesCount  int
	deregisterImageCount int
	deleteSnapshotCount  int
	waitCount            int

	lock sync.Mutex
}

func getMockConn(config *AccessConfig, target string) (clients.Ec2Client, error) {
	mockConn := &mockEC2Conn{
		Config: aws.NewConfig(),
	}

	return mockConn, nil
}
