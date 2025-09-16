// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	//ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
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

	lock sync.Mutex
}

func (m *mockEC2Conn) CopyImage(ctx context.Context, copyInput *ec2.CopyImageInput,
	optFns ...func(*ec2.Options)) (*ec2.CopyImageOutput, error) {
	if !*copyInput.CopyImageTags {
		return nil, fmt.Errorf("CopyImageTags should always be true, but was %t", *copyInput.CopyImageTags)
	}
	m.lock.Lock()
	m.copyImageCount++
	m.lock.Unlock()
	copiedImage := fmt.Sprintf("%s-copied-%d", *copyInput.SourceImageId, m.copyImageCount)
	output := &ec2.CopyImageOutput{
		ImageId: &copiedImage,
	}
	return output, nil
}

func (m *mockEC2Conn) DeregisterImage(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	m.lock.Lock()
	m.deregisterImageCount++
	m.lock.Unlock()
	output := &ec2.DeregisterImageOutput{}
	return output, nil
}

func (m *mockEC2Conn) DeleteSnapshot(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	m.lock.Lock()
	m.deleteSnapshotCount++
	m.lock.Unlock()
	output := &ec2.DeleteSnapshotOutput{}
	return output, nil
}

func (m *mockEC2Conn) DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	m.lock.Lock()
	m.describeImagesCount++
	defer m.lock.Unlock()

	return &ec2.DescribeImagesOutput{
		Images: []ec2types.Image{
			{
				ImageId: aws.String(params.ImageIds[0]),
				State:   ec2types.ImageStateAvailable,
			},
		},
	}, nil
}

func getMockConn(ctx context.Context, config *AccessConfig, target string) (clients.Ec2Client, error) {
	mockConn := &mockEC2Conn{
		Config: aws.NewConfig(),
	}
	return mockConn, nil
}

// Create statebag for running test
func tState() multistep.StateBag {
	state := new(multistep.BasicStateBag)
	state.Put("ui", &packersdk.BasicUi{
		Reader: new(bytes.Buffer),
		Writer: new(bytes.Buffer),
	})
	state.Put("amis", map[string]string{"us-east-1": "ami-12345"})
	state.Put("snapshots", map[string][]string{"us-east-1": {"snap-0012345"}})
	conn, _ := getMockConn(context.TODO(), &AccessConfig{}, "us-east-2")
	state.Put("ec2v2", conn)
	return state
}

func TestStepAMIRegionCopy_duplicates(t *testing.T) {
	// ------------------------------------------------------------------------
	// Test that if the original region is added to both Regions and Region,
	// the ami is only copied once (with encryption).
	// ------------------------------------------------------------------------

	stepAMIRegionCopy := StepAMIRegionCopy{
		AccessConfig: FakeAccessConfig(),
		Regions:      []string{"us-east-1"},
		AMIKmsKeyId:  "12345",
		// Original region key in regionkeyids is different than in amikmskeyid
		RegionKeyIds:      map[string]string{"us-east-1": "12345"},
		EncryptBootVolume: config.TriTrue,
		Name:              "fake-ami-name",
		OriginalRegion:    "us-east-1",
	}
	// mock out the region connection code
	stepAMIRegionCopy.getRegionConn = getMockConn
	// 👇 Override PollingConfig to use mock waiter
	state := tState()
	state.Put("intermediary_image", true)
	stepAMIRegionCopy.Run(context.Background(), state)

	if len(stepAMIRegionCopy.Regions) != 1 {
		t.Fatalf("Should have added original ami to Regions one time only")
	}

	// ------------------------------------------------------------------------
	// Both Region and Regions set, but no encryption - shouldn't copy anything
	// ------------------------------------------------------------------------

	// the ami is only copied once.
	stepAMIRegionCopy = StepAMIRegionCopy{
		AccessConfig:   FakeAccessConfig(),
		Regions:        []string{"us-east-1"},
		Name:           "fake-ami-name",
		OriginalRegion: "us-east-1",
	}
	// mock out the region connection code
	state.Put("intermediary_image", false)
	stepAMIRegionCopy.getRegionConn = getMockConn
	stepAMIRegionCopy.Run(context.Background(), state)

	if len(stepAMIRegionCopy.Regions) != 0 {
		t.Fatalf("Should not have added original ami to Regions; not encrypting")
	}

	// ------------------------------------------------------------------------
	// Both Region and Regions set, but no encryption - shouldn't copy anything,
	// this tests false as opposed to nil value above.
	// ------------------------------------------------------------------------

	// the ami is only copied once.
	stepAMIRegionCopy = StepAMIRegionCopy{
		AccessConfig:      FakeAccessConfig(),
		Regions:           []string{"us-east-1"},
		EncryptBootVolume: config.TriFalse,
		Name:              "fake-ami-name",
		OriginalRegion:    "us-east-1",
	}
	// mock out the region connection code
	state.Put("intermediary_image", false)
	stepAMIRegionCopy.getRegionConn = getMockConn
	stepAMIRegionCopy.Run(context.Background(), state)

	if len(stepAMIRegionCopy.Regions) != 0 {
		t.Fatalf("Should not have added original ami to Regions once; not" +
			"encrypting")
	}

	// ------------------------------------------------------------------------
	// Multiple regions, many duplicates, and encryption (this shouldn't ever
	// happen because of our template validation, but good to test it.)
	// ------------------------------------------------------------------------

	stepAMIRegionCopy = StepAMIRegionCopy{
		AccessConfig: FakeAccessConfig(),
		// Many duplicates for only 3 actual values
		Regions:     []string{"us-east-1", "us-west-2", "us-west-2", "ap-east-1", "ap-east-1", "ap-east-1"},
		AMIKmsKeyId: "IlikePancakes",
		// Original region key in regionkeyids is different than in amikmskeyid
		RegionKeyIds:      map[string]string{"us-east-1": "12345", "us-west-2": "abcde", "ap-east-1": "xyz"},
		EncryptBootVolume: config.TriTrue,
		Name:              "fake-ami-name",
		OriginalRegion:    "us-east-1",
	}
	// mock out the region connection code
	stepAMIRegionCopy.getRegionConn = getMockConn
	state.Put("intermediary_image", true)
	stepAMIRegionCopy.Run(context.Background(), state)

	if len(stepAMIRegionCopy.Regions) != 3 {
		t.Fatalf("Each AMI should have been added to Regions one time only.")
	}

	// Also verify that we respect RegionKeyIds over AMIKmsKeyIds:
	if stepAMIRegionCopy.RegionKeyIds["us-east-1"] != "12345" {
		t.Fatalf("RegionKeyIds should take precedence over AmiKmsKeyIds")
	}

	// ------------------------------------------------------------------------
	// Multiple regions, many duplicates, NO encryption
	// ------------------------------------------------------------------------

	stepAMIRegionCopy = StepAMIRegionCopy{
		AccessConfig: FakeAccessConfig(),
		// Many duplicates for only 3 actual values
		Regions:        []string{"us-east-1", "us-west-2", "us-west-2", "ap-east-1", "ap-east-1", "ap-east-1"},
		Name:           "fake-ami-name",
		OriginalRegion: "us-east-1",
	}
	// mock out the region connection code
	stepAMIRegionCopy.getRegionConn = getMockConn
	state.Put("intermediary_image", false)
	stepAMIRegionCopy.Run(context.Background(), state)

	if len(stepAMIRegionCopy.Regions) != 2 {
		t.Fatalf("Each AMI should have been added to Regions one time only, " +
			"and original region shouldn't be added at all")
	}
}

func TestStepAmiRegionCopy_nil_encryption(t *testing.T) {
	// create step
	stepAMIRegionCopy := StepAMIRegionCopy{
		AccessConfig:      FakeAccessConfig(),
		Regions:           make([]string, 0),
		AMIKmsKeyId:       "",
		RegionKeyIds:      make(map[string]string),
		EncryptBootVolume: config.TriUnset,
		Name:              "fake-ami-name",
		OriginalRegion:    "us-east-1",
	}
	// mock out the region connection code
	stepAMIRegionCopy.getRegionConn = getMockConn

	state := tState()
	state.Put("intermediary_image", false)
	stepAMIRegionCopy.Run(context.Background(), state)

	if stepAMIRegionCopy.toDelete != "" {
		t.Fatalf("Shouldn't have an intermediary ami if encrypt is nil")
	}
	if len(stepAMIRegionCopy.Regions) != 0 {
		t.Fatalf("Should not have added original ami to original region")
	}
}

func TestStepAmiRegionCopy_true_encryption(t *testing.T) {
	// create step
	stepAMIRegionCopy := StepAMIRegionCopy{
		AccessConfig:      FakeAccessConfig(),
		Regions:           make([]string, 0),
		AMIKmsKeyId:       "",
		RegionKeyIds:      make(map[string]string),
		EncryptBootVolume: config.TriTrue,
		Name:              "fake-ami-name",
		OriginalRegion:    "us-east-1",
	}
	// mock out the region connection code
	stepAMIRegionCopy.getRegionConn = getMockConn

	state := tState()
	state.Put("intermediary_image", true)
	stepAMIRegionCopy.Run(context.Background(), state)

	if stepAMIRegionCopy.toDelete == "" {
		t.Fatalf("Should delete original AMI if encrypted=true")
	}
	if len(stepAMIRegionCopy.Regions) == 0 {
		t.Fatalf("Should have added original ami to Regions")
	}
}

func TestStepAmiRegionCopy_nil_intermediary(t *testing.T) {
	// create step
	stepAMIRegionCopy := StepAMIRegionCopy{
		AccessConfig:      FakeAccessConfig(),
		Regions:           make([]string, 0),
		AMIKmsKeyId:       "",
		RegionKeyIds:      make(map[string]string),
		EncryptBootVolume: config.TriFalse,
		Name:              "fake-ami-name",
		OriginalRegion:    "us-east-1",
	}
	// mock out the region connection code
	stepAMIRegionCopy.getRegionConn = getMockConn

	state := tState()
	stepAMIRegionCopy.Run(context.Background(), state)

	if stepAMIRegionCopy.toDelete != "" {
		t.Fatalf("Should not delete original AMI if no intermediary")
	}
	if len(stepAMIRegionCopy.Regions) != 0 {
		t.Fatalf("Should not have added original ami to Regions")
	}
}

func TestStepAmiRegionCopy_AMISkipBuildRegion(t *testing.T) {
	// ------------------------------------------------------------------------
	// skip build region is true
	// ------------------------------------------------------------------------

	stepAMIRegionCopy := StepAMIRegionCopy{
		AccessConfig:       FakeAccessConfig(),
		Regions:            []string{"us-west-1"},
		AMIKmsKeyId:        "",
		RegionKeyIds:       map[string]string{"us-west-1": "abcde"},
		Name:               "fake-ami-name",
		OriginalRegion:     "us-east-1",
		AMISkipBuildRegion: true,
	}
	// mock out the region connection code
	stepAMIRegionCopy.getRegionConn = getMockConn

	state := tState()
	state.Put("intermediary_image", true)
	stepAMIRegionCopy.Run(context.Background(), state)

	if stepAMIRegionCopy.toDelete == "" {
		t.Fatalf("Should delete original AMI if skip_save_build_region=true")
	}
	if len(stepAMIRegionCopy.Regions) != 1 {
		t.Fatalf("Should not have added original ami to Regions; Regions: %#v", stepAMIRegionCopy.Regions)
	}

	// ------------------------------------------------------------------------
	// skip build region is false.
	// ------------------------------------------------------------------------
	stepAMIRegionCopy = StepAMIRegionCopy{
		AccessConfig:       FakeAccessConfig(),
		Regions:            []string{"us-west-1"},
		AMIKmsKeyId:        "",
		RegionKeyIds:       make(map[string]string),
		Name:               "fake-ami-name",
		OriginalRegion:     "us-east-1",
		AMISkipBuildRegion: false,
	}
	// mock out the region connection code
	stepAMIRegionCopy.getRegionConn = getMockConn

	state.Put("intermediary_image", false) // not encrypted
	stepAMIRegionCopy.Run(context.Background(), state)

	if stepAMIRegionCopy.toDelete != "" {
		t.Fatalf("Shouldn't have an intermediary AMI, so dont delete original ami")
	}
	if len(stepAMIRegionCopy.Regions) != 1 {
		t.Fatalf("Should not have added original ami to Regions; Regions: %#v", stepAMIRegionCopy.Regions)
	}

	// ------------------------------------------------------------------------
	// skip build region is false, but encrypt is true
	// ------------------------------------------------------------------------
	stepAMIRegionCopy = StepAMIRegionCopy{
		AccessConfig:       FakeAccessConfig(),
		Regions:            []string{"us-west-1"},
		AMIKmsKeyId:        "",
		RegionKeyIds:       map[string]string{"us-west-1": "abcde"},
		Name:               "fake-ami-name",
		OriginalRegion:     "us-east-1",
		AMISkipBuildRegion: false,
		EncryptBootVolume:  config.TriTrue,
	}
	// mock out the region connection code
	stepAMIRegionCopy.getRegionConn = getMockConn

	state.Put("intermediary_image", true) //encrypted
	stepAMIRegionCopy.Run(context.Background(), state)

	if stepAMIRegionCopy.toDelete == "" {
		t.Fatalf("Have to delete intermediary AMI")
	}
	if len(stepAMIRegionCopy.Regions) != 2 {
		t.Fatalf("Should have added original ami to Regions; Regions: %#v", stepAMIRegionCopy.Regions)
	}

	// ------------------------------------------------------------------------
	// skip build region is true, and encrypt is true
	// ------------------------------------------------------------------------
	stepAMIRegionCopy = StepAMIRegionCopy{
		AccessConfig:       FakeAccessConfig(),
		Regions:            []string{"us-west-1"},
		AMIKmsKeyId:        "",
		RegionKeyIds:       map[string]string{"us-west-1": "abcde"},
		Name:               "fake-ami-name",
		OriginalRegion:     "us-east-1",
		AMISkipBuildRegion: true,
		EncryptBootVolume:  config.TriTrue,
	}
	// mock out the region connection code
	stepAMIRegionCopy.getRegionConn = getMockConn

	state.Put("intermediary_image", true) //encrypted
	stepAMIRegionCopy.Run(context.Background(), state)

	if stepAMIRegionCopy.toDelete == "" {
		t.Fatalf("Have to delete intermediary AMI")
	}
	if len(stepAMIRegionCopy.Regions) != 1 {
		t.Fatalf("Should not have added original ami to Regions; Regions: %#v", stepAMIRegionCopy.Regions)
	}
}
