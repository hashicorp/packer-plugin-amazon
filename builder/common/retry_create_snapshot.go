package common

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/packer-plugin-sdk/retry"
)

func RetryCreateSnapshot(ctx context.Context, ec2conn *ec2.EC2, input *ec2.CreateSnapshotInput) (*ec2.Snapshot, error) {
	retryConfig := retry.Config{
		Tries: 10,
		ShouldRetry: func(err error) bool {
			// TODO make this less brittle
			return strings.Contains(err.Error(), "SnapshotCreationPerVolumeRateExceeded")
		},
		// TODO Hone retry/tries to reasonable limits
		RetryDelay: (&retry.Backoff{
			InitialBackoff: 10 * time.Second,
			MaxBackoff:     15 * time.Second,
			Multiplier:     1.5,
		}).Linear,
	}
	var snapshot *ec2.Snapshot
	err := retryConfig.Run(ctx, func(ctx context.Context) error {
		var err error
		snapshot, err = ec2conn.CreateSnapshot(input)
		return err
	})

	return snapshot, err
}
