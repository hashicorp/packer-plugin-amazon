// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package amazon_acc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-sdk/retry"
)

type AMIHelper struct {
	Region string
	Name   string
}

func (a *AMIHelper) CleanUpAmi() error {
	ctx := context.TODO()
	accessConfig := &awscommon.AccessConfig{}
	config, err := accessConfig.Config(ctx)
	if err != nil {
		return fmt.Errorf("AWSAMICleanUp: Unable to create aws config %s", err.Error())
	}

	regionEc2Client := ec2.NewFromConfig(*config, func(o *ec2.Options) {
		o.Region = a.Region
	})

	var resp *ec2.DescribeImagesOutput

	err = retry.Config{
		Tries: 11,
		ShouldRetry: func(err error) bool {
			return true // TODO make retry more specific to eventual consitencey
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		resp, err = regionEc2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
			Owners: []string{"self"},
			Filters: []ec2types.Filter{{
				Name:   aws.String("name"),
				Values: []string{a.Name},
			}}})
		return err
	})

	if err != nil {
		return fmt.Errorf("AWSAMICleanUp: Unable to find Image %s: %s", a.Name, err.Error())
	}
	if resp == nil {
		return errors.New("AWSAMICleanUp: Response from describe images should not be nil")
	}
	if len(resp.Images) == 0 {
		return errors.New("AWSAMICleanUp: No image was found by describes images")
	}

	image := resp.Images[0]
	ctx = context.TODO()
	err = retry.Config{
		Tries: 11,
		ShouldRetry: func(err error) bool {
			return true // TODO make retry more specific to eventual consitencey
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		_, err = regionEc2Client.DeregisterImage(ctx, &ec2.DeregisterImageInput{
			ImageId: image.ImageId,
		})
		if err != nil {
			return err
		}
		if len(image.BlockDeviceMappings) == 0 {
			return fmt.Errorf("AWSAMICleanUp: Image should contain at least 1 BlockDeviceMapping, got %d", len(image.BlockDeviceMappings))
		}
		for _, bdm := range image.BlockDeviceMappings {
			if bdm.Ebs != nil && bdm.Ebs.SnapshotId != nil {
				_, err = regionEc2Client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
					SnapshotId: bdm.Ebs.SnapshotId,
				})
				return err

			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("AWSAMICleanUp: Unable to Deregister Image %s", err.Error())
	}

	return nil
}

func (a *AMIHelper) GetAmi() ([]ec2types.Image, error) {
	ctx := context.TODO()
	accessConfig := &awscommon.AccessConfig{}
	config, err := accessConfig.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("AWSAMICleanUp: Unable to create aws config %s", err.Error())
	}

	regionEc2Client := ec2.NewFromConfig(*config, func(o *ec2.Options) {
		o.Region = a.Region
	})

	var resp *ec2.DescribeImagesOutput
	err = retry.Config{
		Tries: 11,
		ShouldRetry: func(err error) bool {
			return true // TODO make retry more specific to eventual consitencey
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		resp, err = regionEc2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
			Owners: []string{"self"},
			Filters: []ec2types.Filter{{
				Name:   aws.String("name"),
				Values: []string{a.Name},
			}}})
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("Unable to find Image %s: %s", a.Name, err.Error())
	}
	return resp.Images, nil
}

type VolumeHelper struct {
	Region string
	Tags   []map[string]string
}

// GetVolumes retrieves all EBS volumes in the specified region that match the provided tags and are not in a deleting or deleted state.
func (v *VolumeHelper) GetVolumes() ([]ec2types.Volume, error) {
	ctx := context.TODO()
	accessConfig := &awscommon.AccessConfig{}
	config, err := accessConfig.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("AWSAMICleanUp: Unable to create aws config %s", err.Error())
	}

	regionEc2Client := ec2.NewFromConfig(*config, func(o *ec2.Options) {
		o.Region = v.Region
	})

	var resp *ec2.DescribeVolumesOutput
	var filters []ec2types.Filter
	var activeVolumes []ec2types.Volume

	for _, tag := range v.Tags {
		for key, value := range tag {
			filters = append(filters, ec2types.Filter{
				Name:   aws.String(fmt.Sprintf("tag:%s", key)),
				Values: []string{value},
			})
		}
	}

	err = retry.Config{
		Tries: 11,
		ShouldRetry: func(err error) bool {
			return true // TODO make retry more specific to eventual consistency
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		resp, err = regionEc2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
			Filters: filters,
		})
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("Unable to find Volumes with specified tags %s: %s", v.Tags, err.Error())
	}

	for _, volume := range resp.Volumes {
		if volume.State != ec2types.VolumeStateDeleting && volume.State != ec2types.VolumeStateDeleted {
			activeVolumes = append(activeVolumes, volume)
		}
	}
	return activeVolumes, nil
}
