// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDescribeImagesClient is a minimal Ec2Client that only overrides DescribeImages.
type mockDescribeImagesClient struct {
	clients.Ec2Client
	images []types.Image
	err    error
}

func (m *mockDescribeImagesClient) DescribeImages(_ context.Context, _ *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &ec2.DescribeImagesOutput{Images: m.images}, nil
}

func TestGetFilteredImages_ReturnsAllImages(t *testing.T) {
	mockClient := &mockDescribeImagesClient{
		images: []types.Image{
			{ImageId: aws.String("ami-1"), Name: aws.String("image-a")},
			{ImageId: aws.String("ami-2"), Name: aws.String("image-b")},
		},
	}
	opts := AmiFilterOptions{
		Owners:  []string{"123456789"},
		Filters: map[string]string{"name": "image-*"},
	}
	images, err := opts.GetFilteredImages(context.Background(), &ec2.DescribeImagesInput{}, mockClient)
	require.NoError(t, err)
	assert.Len(t, images, 2)
}

func TestGetFilteredImages_EmptyResultReturnsError(t *testing.T) {
	mockClient := &mockDescribeImagesClient{images: []types.Image{}}
	opts := AmiFilterOptions{Owners: []string{"123456789"}}
	_, err := opts.GetFilteredImages(context.Background(), &ec2.DescribeImagesInput{}, mockClient)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "No AMI was found")
}

func TestGetFilteredImages_APIErrorPropagated(t *testing.T) {
	mockClient := &mockDescribeImagesClient{err: fmt.Errorf("aws api error")}
	opts := AmiFilterOptions{Owners: []string{"123456789"}}
	_, err := opts.GetFilteredImages(context.Background(), &ec2.DescribeImagesInput{}, mockClient)
	assert.Error(t, err)
}
