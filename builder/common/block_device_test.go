// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

func TestBlockDevice(t *testing.T) {
	cases := []struct {
		Config *BlockDevice
		Result ec2types.BlockDeviceMapping
	}{
		{
			Config: &BlockDevice{
				DeviceName:          "/dev/sdb",
				SnapshotId:          "snap-1234",
				VolumeType:          string(ec2types.VolumeTypeStandard),
				VolumeSize:          8,
				DeleteOnTermination: true,
			},

			Result: ec2types.BlockDeviceMapping{
				DeviceName: aws.String("/dev/sdb"),
				Ebs: &ec2types.EbsBlockDevice{
					SnapshotId:          aws.String("snap-1234"),
					VolumeType:          ec2types.VolumeTypeStandard,
					VolumeSize:          aws.Int32(8),
					DeleteOnTermination: aws.Bool(true),
				},
			},
		},
		{
			Config: &BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeSize: 8,
			},

			Result: ec2types.BlockDeviceMapping{
				DeviceName: aws.String("/dev/sdb"),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeSize:          aws.Int32(8),
					DeleteOnTermination: aws.Bool(false),
				},
			},
		},
		{
			Config: &BlockDevice{
				DeviceName:          "/dev/sdb",
				VolumeType:          string(ec2types.VolumeTypeIo1),
				VolumeSize:          8,
				DeleteOnTermination: true,
				IOPS:                aws.Int32(1000),
			},

			Result: ec2types.BlockDeviceMapping{
				DeviceName: aws.String("/dev/sdb"),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeType:          ec2types.VolumeTypeIo1,
					VolumeSize:          aws.Int32(8),
					DeleteOnTermination: aws.Bool(true),
					Iops:                aws.Int32(1000),
				},
			},
		},
		{
			Config: &BlockDevice{
				DeviceName:          "/dev/sdb",
				VolumeType:          string(ec2types.VolumeTypeIo2),
				VolumeSize:          8,
				DeleteOnTermination: true,
				IOPS:                aws.Int32(1000),
			},

			Result: ec2types.BlockDeviceMapping{
				DeviceName: aws.String("/dev/sdb"),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeType:          ec2types.VolumeTypeIo2,
					VolumeSize:          aws.Int32(8),
					DeleteOnTermination: aws.Bool(true),
					Iops:                aws.Int32(1000),
				},
			},
		},
		{
			Config: &BlockDevice{
				DeviceName:          "/dev/sdb",
				VolumeType:          string(ec2types.VolumeTypeGp2),
				VolumeSize:          8,
				DeleteOnTermination: true,
				Encrypted:           config.TriTrue,
			},

			Result: ec2types.BlockDeviceMapping{
				DeviceName: aws.String("/dev/sdb"),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeType:          ec2types.VolumeTypeGp2,
					VolumeSize:          aws.Int32(8),
					DeleteOnTermination: aws.Bool(true),
					Encrypted:           aws.Bool(true),
				},
			},
		},
		{
			Config: &BlockDevice{
				DeviceName:          "/dev/sdb",
				VolumeType:          string(ec2types.VolumeTypeGp2),
				VolumeSize:          8,
				DeleteOnTermination: true,
				Encrypted:           config.TriTrue,
				KmsKeyId:            "2Fa48a521f-3aff-4b34-a159-376ac5d37812",
			},

			Result: ec2types.BlockDeviceMapping{
				DeviceName: aws.String("/dev/sdb"),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeType:          ec2types.VolumeTypeGp2,
					VolumeSize:          aws.Int32(8),
					DeleteOnTermination: aws.Bool(true),
					Encrypted:           aws.Bool(true),
					KmsKeyId:            aws.String("2Fa48a521f-3aff-4b34-a159-376ac5d37812"),
				},
			},
		},
		{
			Config: &BlockDevice{
				DeviceName:          "/dev/sdb",
				VolumeType:          string(ec2types.VolumeTypeStandard),
				DeleteOnTermination: true,
			},

			Result: ec2types.BlockDeviceMapping{
				DeviceName: aws.String("/dev/sdb"),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeType:          ec2types.VolumeTypeStandard,
					DeleteOnTermination: aws.Bool(true),
				},
			},
		},
		{
			Config: &BlockDevice{
				DeviceName:  "/dev/sdb",
				VirtualName: "ephemeral0",
			},

			Result: ec2types.BlockDeviceMapping{
				DeviceName:  aws.String("/dev/sdb"),
				VirtualName: aws.String("ephemeral0"),
			},
		},
		{
			Config: &BlockDevice{
				DeviceName: "/dev/sdb",
				NoDevice:   true,
			},

			Result: ec2types.BlockDeviceMapping{
				DeviceName: aws.String("/dev/sdb"),
				NoDevice:   aws.String(""),
			},
		},
		{
			Config: &BlockDevice{
				DeviceName:          "/dev/sdb",
				VolumeType:          string(ec2types.VolumeTypeGp3),
				VolumeSize:          8,
				Throughput:          aws.Int32(125),
				IOPS:                aws.Int32(3000),
				DeleteOnTermination: true,
				Encrypted:           config.TriTrue,
			},

			Result: ec2types.BlockDeviceMapping{
				DeviceName: aws.String("/dev/sdb"),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeType:          ec2types.VolumeTypeGp3,
					VolumeSize:          aws.Int32(8),
					Throughput:          aws.Int32(125),
					Iops:                aws.Int32(3000),
					DeleteOnTermination: aws.Bool(true),
					Encrypted:           aws.Bool(true),
				},
			},
		},
	}

	for _, tc := range cases {
		var amiBlockDevices BlockDevices = []BlockDevice{*tc.Config}

		var launchBlockDevices BlockDevices = []BlockDevice{*tc.Config}

		expected := []ec2types.BlockDeviceMapping{tc.Result}

		amiResults := amiBlockDevices.BuildEC2BlockDeviceMappings()
		if diff := cmp.Diff(expected, amiResults, cmpopts.IgnoreUnexported(ec2types.BlockDeviceMapping{}, ec2types.EbsBlockDevice{})); diff != "" {
			t.Fatalf("Bad block device: %s", diff)
		}

		launchResults := launchBlockDevices.BuildEC2BlockDeviceMappings()
		if diff := cmp.Diff(expected, launchResults, cmpopts.IgnoreUnexported(ec2types.BlockDeviceMapping{}, ec2types.EbsBlockDevice{})); diff != "" {
			t.Fatalf("Bad block device: %s", diff)
		}
	}
}

func pointerSlice[T any](s []T) []*T {
	result := make([]*T, len(s))
	for i := range s {
		result[i] = &s[i]
	}
	return result
}

func TestIOPSValidation(t *testing.T) {

	cases := []struct {
		device BlockDevice
		ok     bool
		msg    string
	}{
		// volume size unknown
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeIo1),
				IOPS:       aws.Int32(1000),
			},
			ok: true,
		},
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeIo2),
				IOPS:       aws.Int32(1000),
			},
			ok: true,
		},
		// ratio requirement satisfied
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeIo1),
				VolumeSize: 50,
				IOPS:       aws.Int32(1000),
			},
			ok: true,
		},
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeIo2),
				VolumeSize: 100,
				IOPS:       aws.Int32(1000),
			},
			ok: true,
		},
		// ratio requirement not satisfied
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeIo1),
				VolumeSize: 10,
				IOPS:       aws.Int32(2000),
			},
			ok:  false,
			msg: "/dev/sdb: the maximum ratio of provisioned IOPS to requested volume size (in GiB) is 50:1 for io1 volumes",
		},
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeIo2),
				VolumeSize: 50,
				IOPS:       aws.Int32(30000),
			},
			ok:  false,
			msg: "/dev/sdb: the maximum ratio of provisioned IOPS to requested volume size (in GiB) is 500:1 for io2 volumes",
		},
		// exceed max iops
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeIo2),
				VolumeSize: 500,
				IOPS:       aws.Int32(99999),
			},
			ok:  false,
			msg: "IOPS must be between 100 and 64000 for device /dev/sdb",
		},
		// lower than min iops
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeIo2),
				VolumeSize: 50,
				IOPS:       aws.Int32(10),
			},
			ok:  false,
			msg: "IOPS must be between 100 and 64000 for device /dev/sdb",
		},
		// exceed max iops
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeGp3),
				VolumeSize: 50,
				Throughput: aws.Int32(125),
				IOPS:       aws.Int32(99999),
			},
			ok:  false,
			msg: "IOPS must be between 3000 and 16000 for device /dev/sdb",
		},
		// lower than min iops
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeGp3),
				VolumeSize: 50,
				Throughput: aws.Int32(125),
				IOPS:       aws.Int32(10),
			},
			ok:  false,
			msg: "IOPS must be between 3000 and 16000 for device /dev/sdb",
		},
	}

	ctx := interpolate.Context{}
	for _, testCase := range cases {
		err := testCase.device.Prepare(&ctx)
		if testCase.ok && err != nil {
			t.Fatalf("should not error, but: %v", err)
		}
		if !testCase.ok {
			if err == nil {
				t.Fatalf("should error")
			} else if err.Error() != testCase.msg {
				t.Fatalf("wrong error: expected %s, found: %v", testCase.msg, err)
			}
		}
	}
}

func TestThroughputValidation(t *testing.T) {

	cases := []struct {
		device BlockDevice
		ok     bool
		msg    string
	}{
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeGp3),
				Throughput: aws.Int32(125),
				IOPS:       aws.Int32(3000),
			},
			ok: true,
		},
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeGp3),
				Throughput: aws.Int32(1000),
				IOPS:       aws.Int32(3000),
			},
			ok: true,
		},
		// exceed max Throughput
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeGp3),
				Throughput: aws.Int32(1001),
				IOPS:       aws.Int32(3000),
			},
			ok:  false,
			msg: "Throughput must be between 125 and 1000 for device /dev/sdb",
		},
		// lower than min Throughput
		{
			device: BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: string(ec2types.VolumeTypeGp3),
				Throughput: aws.Int32(124),
				IOPS:       aws.Int32(3000),
			},
			ok:  false,
			msg: "Throughput must be between 125 and 1000 for device /dev/sdb",
		},
	}

	ctx := interpolate.Context{}
	for _, testCase := range cases {
		err := testCase.device.Prepare(&ctx)
		if testCase.ok && err != nil {
			t.Fatalf("should not error, but: %v", err)
		}
		if !testCase.ok {
			if err == nil {
				t.Fatalf("should error")
			} else if err.Error() != testCase.msg {
				t.Fatalf("wrong error: expected %s, found: %v", testCase.msg, err)
			}
		}
	}
}
