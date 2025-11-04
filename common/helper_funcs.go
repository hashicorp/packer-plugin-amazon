// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/builder/common/awserrors"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/retry"
)

// DestroyAMIs deregisters the AWS machine images in imageids from an active AWS account
func DestroyAMIs(imageIds []string, client clients.Ec2Client) error {
	ctx := context.TODO()
	resp, err := client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds: imageIds,
	})

	if err != nil {
		err := fmt.Errorf("Error describing AMI: %s", err)
		return err
	}

	// Deregister image by name.
	for _, i := range resp.Images {
		err = retry.Config{
			Tries: 11,
			ShouldRetry: func(err error) bool {
				return awserrors.Matches(err, "UnauthorizedOperation", "")
			},
			RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
		}.Run(ctx, func(ctx context.Context) error {
			_, err := client.DeregisterImage(ctx, &ec2.DeregisterImageInput{
				ImageId: i.ImageId,
			})
			return err
		})

		if err != nil {
			err := fmt.Errorf("Error deregistering existing AMI: %s", err)
			return err
		}
		log.Printf("Deregistered AMI id: %s", *i.ImageId)

		// Delete snapshot(s) by image
		for _, b := range i.BlockDeviceMappings {
			if b.Ebs != nil && aws.ToString(b.Ebs.SnapshotId) != "" {

				err = retry.Config{
					Tries: 11,
					ShouldRetry: func(err error) bool {
						return awserrors.Matches(err, "UnauthorizedOperation", "")
					},
					RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
				}.Run(ctx, func(ctx context.Context) error {
					_, err := client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
						SnapshotId: b.Ebs.SnapshotId,
					})
					return err
				})

				if err != nil {
					err := fmt.Errorf("Error deleting existing snapshot: %s", err)
					return err
				}
				log.Printf("Deleted snapshot: %s", *b.Ebs.SnapshotId)
			}
		}
	}
	return nil
}

func prettyFilter(filters []types.Filter) string {
	b, err := json.MarshalIndent(filters, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}
