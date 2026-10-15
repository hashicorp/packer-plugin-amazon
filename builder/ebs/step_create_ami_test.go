// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package ebs

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type createImageClient struct {
	clients.Ec2Client
	input *ec2.CreateImageInput
}

func (m *createImageClient) CreateImage(_ context.Context, input *ec2.CreateImageInput, _ ...func(*ec2.Options)) (*ec2.CreateImageOutput, error) {
	m.input = input
	return &ec2.CreateImageOutput{ImageId: aws.String("ami-test")}, nil
}
func (m *createImageClient) DescribeImages(context.Context, *ec2.DescribeImagesInput, ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return &ec2.DescribeImagesOutput{Images: []types.Image{{ImageId: aws.String("ami-test"), State: types.ImageStateAvailable, BlockDeviceMappings: []types.BlockDeviceMapping{{Ebs: &types.EbsBlockDevice{SnapshotId: aws.String("snap-test")}}}}}}, nil
}
func TestCreateAMI_NoRebootRequiresConfirmedShutdown(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		name := "default"
		if confirmed {
			name = "guest confirmed"
		}
		t.Run(name, func(t *testing.T) {
			client := new(createImageClient)
			state := new(multistep.BasicStateBag)
			state.Put("ec2v2", client)
			state.Put("aws_config", &aws.Config{Region: "us-east-1"})
			c := new(Config)
			c.AMIName = "test"
			state.Put("config", c)
			state.Put("instance", types.Instance{InstanceId: aws.String("i-test")})
			state.Put("ui", new(packersdk.MockUi))
			if confirmed {
				state.Put(awscommon.GuestShutdownConfirmedKey, true)
			}
			step := &stepCreateAMI{PollingConfig: &awscommon.AWSPollingConfig{}, IsRestricted: true}
			if step.Run(context.Background(), state) != multistep.ActionContinue {
				t.Fatalf("CreateAMI failed: %v", state.Get("error"))
			}
			if confirmed && !aws.ToBool(client.input.NoReboot) {
				t.Fatal("confirmed shutdown must set NoReboot")
			}
			if !confirmed && client.input.NoReboot != nil {
				t.Fatal("default CreateImage request changed")
			}
			if state.Get("amis").(map[string]string)["us-east-1"] != "ami-test" {
				t.Fatal("AMI artifact missing")
			}
			if state.Get("snapshots").(map[string][]string)["us-east-1"][0] != "snap-test" {
				t.Fatal("snapshot artifact missing")
			}
		})
	}
}
