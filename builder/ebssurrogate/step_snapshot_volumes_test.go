// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package ebssurrogate

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
)

type snapshotRecordingEC2 struct {
	clients.Ec2Client
	mu   sync.Mutex
	tags map[string][]ec2types.Tag
}

func (m *snapshotRecordingEC2) CreateSnapshot(ctx context.Context, params *ec2.CreateSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.CreateSnapshotOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, spec := range params.TagSpecifications {
		m.tags[*params.VolumeId] = append(m.tags[*params.VolumeId], spec.Tags...)
	}
	return &ec2.CreateSnapshotOutput{SnapshotId: aws.String(fmt.Sprintf("snap-of-%s", *params.VolumeId))}, nil
}

func (m *snapshotRecordingEC2) DescribeSnapshots(ctx context.Context, params *ec2.DescribeSnapshotsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSnapshotsOutput, error) {
	return &ec2.DescribeSnapshotsOutput{
		Snapshots: []ec2types.Snapshot{
			{SnapshotId: aws.String(params.SnapshotIds[0]), State: ec2types.SnapshotStateCompleted},
		},
	}, nil
}

func TestStepSnapshotVolumes_multipleLaunchDevices(t *testing.T) {
	// Several runs widen the window for the race detector (and the runtime's own concurrent
	// map write check) to see the volumes being snapshotted in parallel.
	for run := 0; run < 10; run++ {
		const deviceCount = 8
		conn := &snapshotRecordingEC2{tags: map[string][]ec2types.Tag{}}
		state := new(multistep.BasicStateBag)
		state.Put("ui", &packer.BasicUi{Reader: new(bytes.Buffer), Writer: new(bytes.Buffer)})
		state.Put("ec2v2", conn)
		state.Put("aws_config", &aws.Config{Region: "us-east-1"})
		state.Put("generated_data", map[string]interface{}{})
		state.Put("source_image", &ec2types.Image{
			ImageId: aws.String("ami-0123456789abcdef0"),
			Name:    aws.String("source"),
		})

		var launchDevices []ec2types.BlockDeviceMapping
		var instanceDevices []ec2types.InstanceBlockDeviceMapping
		for i := 0; i < deviceCount; i++ {
			name := fmt.Sprintf("/dev/xvd%c", 'f'+i)
			launchDevices = append(launchDevices, ec2types.BlockDeviceMapping{DeviceName: aws.String(name)})
			instanceDevices = append(instanceDevices, ec2types.InstanceBlockDeviceMapping{
				DeviceName: aws.String(name),
				Ebs:        &ec2types.EbsInstanceBlockDevice{VolumeId: aws.String(fmt.Sprintf("vol-%d", i))},
			})
		}
		state.Put("instance", ec2types.Instance{BlockDeviceMappings: instanceDevices})

		step := &StepSnapshotVolumes{
			PollingConfig: new(awscommon.AWSPollingConfig),
			LaunchDevices: launchDevices,
			SnapshotTags:  map[string]string{"Name": "test"},
		}

		if action := step.Run(context.Background(), state); action != multistep.ActionContinue {
			t.Fatalf("run %d: expected ActionContinue, got %v: %v", run, action, state.Get("error"))
		}

		snapshotIds := state.Get("snapshot_ids").(map[string]string)
		if len(snapshotIds) != deviceCount {
			t.Fatalf("run %d: expected %d snapshots, got %#v", run, deviceCount, snapshotIds)
		}
		for i, device := range launchDevices {
			volumeId := fmt.Sprintf("vol-%d", i)
			if got := snapshotIds[*device.DeviceName]; got != "snap-of-"+volumeId {
				t.Fatalf("run %d: %s snapshot is %q, want snap-of-%s", run, *device.DeviceName, got, volumeId)
			}
			tags := conn.tags[volumeId]
			if len(tags) != 1 || *tags[0].Key != "Name" || *tags[0].Value != "test" {
				t.Fatalf("run %d: %s snapshot tags are %#v", run, volumeId, tags)
			}
		}
	}
}
