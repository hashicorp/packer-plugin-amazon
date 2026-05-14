// Copyright IBM Corp. 2013, 2025
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
	waitCount            int

	lock sync.Mutex
}

func (m *mockEC2Conn) CopyImage(ctx context.Context, copyInput *ec2.CopyImageInput, opts ...func(*ec2.Options)) (*ec2.CopyImageOutput, error) {
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

// functions we have to create mock responses for in order for test to run
func (m *mockEC2Conn) DescribeImages(ctx context.Context, input *ec2.DescribeImagesInput, opts ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	m.lock.Lock()
	m.describeImagesCount++
	m.lock.Unlock()
	output := &ec2.DescribeImagesOutput{
		Images: []ec2types.Image{ec2types.Image{
			ImageId:        aws.String("ami-12345"),
			State:          ec2types.ImageStateAvailable,
			RootDeviceName: aws.String("/dev/sda1"),
		}},
	}
	return output, nil
}

func (m *mockEC2Conn) DeregisterImage(ctx context.Context, input *ec2.DeregisterImageInput, opts ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	m.lock.Lock()
	m.deregisterImageCount++
	m.lock.Unlock()
	output := &ec2.DeregisterImageOutput{}
	return output, nil
}

func (m *mockEC2Conn) DeleteSnapshot(ctx context.Context, input *ec2.DeleteSnapshotInput, opts ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	m.lock.Lock()
	m.deleteSnapshotCount++
	m.lock.Unlock()
	output := &ec2.DeleteSnapshotOutput{}
	return output, nil
}

// we don't need to mock out the waiter
/*
func (m *mockEC2Conn) WaitUntilImageAvailableWithContext(aws.Context, *ec2.DescribeImagesInput, ...request.WaiterOption) error {
	m.lock.Lock()
	m.waitCount++
	m.lock.Unlock()
	return nil
}
*/

func getMockConn(ctx context.Context, config *AccessConfig, target string) (clients.Ec2Client, error) {
	awscfg, err := config.GetAWSConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("Error getting region connection for copy: %s", err)
	}
	awscfg.Region = target
	mockConn := &mockEC2Conn{
		Config: awscfg,
	}

	return mockConn, nil
}

// Create statebag for running test
func tState(ctx context.Context) multistep.StateBag {
	state := new(multistep.BasicStateBag)
	state.Put("ui", &packersdk.BasicUi{
		Reader: new(bytes.Buffer),
		Writer: new(bytes.Buffer),
	})
	state.Put("amis", map[string]string{"us-east-1": "ami-12345"})
	state.Put("snapshots", map[string][]string{"us-east-1": {"snap-0012345"}})
	accessConfig := FakeAccessConfig()
	conn, _ := getMockConn(ctx, accessConfig, "us-east-2")
	state.Put("ec2", conn)
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

	ctx := t.Context()
	state := tState(ctx)
	state.Put("intermediary_image", true)
	stepAMIRegionCopy.Run(ctx, state)

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
	stepAMIRegionCopy.Run(ctx, state)

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
	stepAMIRegionCopy.Run(ctx, state)

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
	stepAMIRegionCopy.Run(ctx, state)

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
	stepAMIRegionCopy.Run(ctx, state)

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

	ctx := t.Context()
	state := tState(ctx)
	state.Put("intermediary_image", false)
	stepAMIRegionCopy.Run(ctx, state)

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

	ctx := t.Context()
	state := tState(ctx)
	state.Put("intermediary_image", true)
	stepAMIRegionCopy.Run(ctx, state)

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

	ctx := t.Context()
	state := tState(ctx)
	stepAMIRegionCopy.Run(ctx, state)

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

	ctx := t.Context()
	state := tState(ctx)
	state.Put("intermediary_image", true)
	stepAMIRegionCopy.Run(ctx, state)

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
	stepAMIRegionCopy.Run(ctx, state)

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
	stepAMIRegionCopy.Run(ctx, state)

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
	stepAMIRegionCopy.Run(ctx, state)

	if stepAMIRegionCopy.toDelete == "" {
		t.Fatalf("Have to delete intermediary AMI")
	}
	if len(stepAMIRegionCopy.Regions) != 1 {
		t.Fatalf("Should not have added original ami to Regions; Regions: %#v", stepAMIRegionCopy.Regions)
	}
}
