// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package ami

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
)

func TestDatasourceConfigure_FilterBlank(t *testing.T) {
	datasource := Datasource{
		config: Config{
			AmiFilterOptions: awscommon.AmiFilterOptions{},
		},
	}
	if err := datasource.Configure(nil); err == nil {
		t.Fatalf("Should error if filters map is empty or not specified")
	}
}

func TestRunConfigPrepare_SourceAmiFilterOwnersBlank(t *testing.T) {
	datasource := Datasource{
		config: Config{
			AmiFilterOptions: awscommon.AmiFilterOptions{
				Filters: map[string]string{"foo": "bar"},
			},
		},
	}
	if err := datasource.Configure(nil); err == nil {
		t.Fatalf("Should error if Owners is not specified)")
	}
}

func TestRunConfigPrepare_SourceAmiFilterGood(t *testing.T) {
	datasource := Datasource{
		config: Config{
			AmiFilterOptions: awscommon.AmiFilterOptions{
				Owners:  []string{"1234567"},
				Filters: map[string]string{"foo": "bar"},
			},
		},
	}
	if err := datasource.Configure(nil); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestDatasourceConfigure_SortByName_Valid(t *testing.T) {
	datasource := Datasource{
		config: Config{
			AmiFilterOptions: awscommon.AmiFilterOptions{
				Owners:  []string{"1234567"},
				Filters: map[string]string{"name": "my-ami-*"},
			},
			SortBy: "name",
		},
	}
	if err := datasource.Configure(nil); err != nil {
		t.Fatalf("Expected no error with sort_by=name, got: %s", err)
	}
}

func TestDatasourceConfigure_SortByInvalid_Error(t *testing.T) {
	datasource := Datasource{
		config: Config{
			AmiFilterOptions: awscommon.AmiFilterOptions{
				Owners:  []string{"1234567"},
				Filters: map[string]string{"name": "my-ami-*"},
			},
			SortBy: "creation_date",
		},
	}
	if err := datasource.Configure(nil); err == nil {
		t.Fatal("Expected error for invalid sort_by value, got nil")
	}
}

func TestDatasourceConfigure_SortByAndMostRecent_NoError(t *testing.T) {
	datasource := Datasource{
		config: Config{
			AmiFilterOptions: awscommon.AmiFilterOptions{
				Owners:     []string{"1234567"},
				Filters:    map[string]string{"name": "my-ami-*"},
				MostRecent: true,
			},
			SortBy: "name",
		},
	}
	if err := datasource.Configure(nil); err != nil {
		t.Fatalf("Expected no error when both sort_by and most_recent are set, got: %s", err)
	}
}

// mockAmiEC2Client returns a fixed set of images for DescribeImages calls.
type mockAmiEC2Client struct {
	clients.Ec2Client
	images []ec2types.Image
}

func (m *mockAmiEC2Client) DescribeImages(_ context.Context, _ *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return &ec2.DescribeImagesOutput{Images: m.images}, nil
}

func TestDatasourceExecute_SortByName_SelectsLexicographicallyLast(t *testing.T) {
	mockClient := &mockAmiEC2Client{
		images: []ec2types.Image{
			{
				ImageId:      aws.String("ami-sp6"),
				Name:         aws.String("suse-sles-15-sp6-v20250210-hvm-ssd-x86_64"),
				CreationDate: aws.String("2025-02-10T00:00:00Z"),
				OwnerId:      aws.String("013907871322"),
			},
			{
				ImageId:      aws.String("ami-sp7"),
				Name:         aws.String("suse-sles-15-sp7-v20241105-hvm-ssd-x86_64"),
				CreationDate: aws.String("2024-11-05T00:00:00Z"),
				OwnerId:      aws.String("013907871322"),
			},
		},
	}

	ds := Datasource{
		config: Config{
			AccessConfig: *awscommon.FakeAccessConfigWithEC2Client(func() clients.Ec2Client {
				return mockClient
			}),
			AmiFilterOptions: awscommon.AmiFilterOptions{
				Owners:  []string{"013907871322"},
				Filters: map[string]string{"name": "suse-sles-15-sp*"},
			},
			SortBy: "name",
		},
	}

	result, err := ds.Execute()
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %s", err)
	}

	idVal := result.GetAttr("id")
	if idVal.AsString() != "ami-sp7" {
		t.Errorf("Expected ami-sp7 (highest name), got %s", idVal.AsString())
	}
}
