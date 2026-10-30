// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

func TestGP3PerformanceValidation(t *testing.T) {
	tests := []struct {
		name       string
		iops       *int32
		throughput *int32
		wantErr    string
	}{
		{name: "performance omitted"},
		{name: "minimum IOPS", iops: aws.Int32(3000)},
		{name: "previous maximum IOPS", iops: aws.Int32(16000)},
		{name: "above previous maximum IOPS", iops: aws.Int32(16001)},
		{name: "maximum IOPS", iops: aws.Int32(80000)},
		{
			name:    "below minimum IOPS",
			iops:    aws.Int32(2999),
			wantErr: "IOPS must be between 3000 and 80000 for device /dev/sdb",
		},
		{
			name:    "above maximum IOPS",
			iops:    aws.Int32(80001),
			wantErr: "IOPS must be between 3000 and 80000 for device /dev/sdb",
		},
		{name: "minimum throughput", throughput: aws.Int32(125)},
		{name: "previous maximum throughput", iops: aws.Int32(8000), throughput: aws.Int32(1000)},
		{name: "above previous maximum throughput", iops: aws.Int32(8000), throughput: aws.Int32(1001)},
		{name: "maximum throughput", iops: aws.Int32(8000), throughput: aws.Int32(2000)},
		{name: "maximum performance", iops: aws.Int32(80000), throughput: aws.Int32(2000)},
		{
			name:       "below minimum throughput",
			throughput: aws.Int32(124),
			wantErr:    "Throughput must be between 125 and 2000 for device /dev/sdb",
		},
		{
			name:       "above maximum throughput",
			throughput: aws.Int32(2001),
			wantErr:    "Throughput must be between 125 and 2000 for device /dev/sdb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: "gp3",
				VolumeSize: 160,
				IOPS:       tt.iops,
				Throughput: tt.throughput,
			}
			err := device.Prepare(&interpolate.Context{})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("expected error %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestProvisionedIOPSValidation(t *testing.T) {
	tests := []struct {
		name       string
		volumeType string
		volumeSize int32
		iops       int32
		wantErr    string
	}{
		{name: "io1 minimum", volumeType: "io1", volumeSize: 4, iops: 100},
		{name: "io1 maximum", volumeType: "io1", volumeSize: 1280, iops: 64000},
		{
			name: "io1 above maximum", volumeType: "io1", volumeSize: 2000, iops: 64001,
			wantErr: "IOPS must be between 100 and 64000 for device /dev/sdb",
		},
		{
			name: "io1 below minimum", volumeType: "io1", volumeSize: 4, iops: 99,
			wantErr: "IOPS must be between 100 and 64000 for device /dev/sdb",
		},
		{
			name: "io1 above ratio", volumeType: "io1", volumeSize: 10, iops: 1000,
			wantErr: "/dev/sdb: the maximum ratio of provisioned IOPS to requested volume size (in GiB) is 50:1 for io1 volumes",
		},
		{name: "io2 minimum", volumeType: "io2", volumeSize: 4, iops: 100},
		{name: "io2 above previous maximum", volumeType: "io2", volumeSize: 256, iops: 64001},
		{name: "io2 maximum and ratio", volumeType: "io2", volumeSize: 256, iops: 256000},
		{
			name: "io2 above maximum", volumeType: "io2", volumeSize: 500, iops: 256001,
			wantErr: "IOPS must be between 100 and 256000 for device /dev/sdb",
		},
		{
			name: "io2 below minimum", volumeType: "io2", volumeSize: 4, iops: 99,
			wantErr: "IOPS must be between 100 and 256000 for device /dev/sdb",
		},
		{
			name: "io2 above ratio", volumeType: "io2", volumeSize: 50, iops: 51000,
			wantErr: "/dev/sdb: the maximum ratio of provisioned IOPS to requested volume size (in GiB) is 1000:1 for io2 volumes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := BlockDevice{
				DeviceName: "/dev/sdb",
				VolumeType: tt.volumeType,
				VolumeSize: tt.volumeSize,
				IOPS:       aws.Int32(tt.iops),
			}
			err := device.Prepare(&interpolate.Context{})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("expected error %q, got %v", tt.wantErr, err)
			}
		})
	}
}
