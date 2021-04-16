package amazon_acc

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
	"github.com/hashicorp/packer-plugin-sdk/retry"
)

type AMIHelper struct {
	Region string
	Name   string
}

func (a *AMIHelper) CleanUpAmi() error {
	accessConfig := &awscommon.AccessConfig{}
	session, err := accessConfig.Session()
	if err != nil {
		return fmt.Errorf("AWSAMICleanUp: Unable to create aws session %s", err.Error())
	}

	regionconn := ec2.New(session.Copy(&aws.Config{
		Region: aws.String(a.Region),
	}))

	var resp *ec2.DescribeImagesOutput

	ctx := context.TODO()
	err = retry.Config{
		Tries: 11,
		ShouldRetry: func(err error) bool {
			return true // TODO make retry more specific to eventual consitencey
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		resp, err = regionconn.DescribeImages(&ec2.DescribeImagesInput{
			Owners: aws.StringSlice([]string{"self"}),
			Filters: []*ec2.Filter{{
				Name:   aws.String("name"),
				Values: aws.StringSlice([]string{a.Name}),
			}}})
		return err
	})

	if err != nil {
		return fmt.Errorf("AWSAMICleanUp: Unable to find Image %s: %s", a.Name, err.Error())
	}

	if resp != nil && len(resp.Images) > 0 {
		ctx = context.TODO()
		err = retry.Config{
			Tries: 11,
			ShouldRetry: func(err error) bool {
				return true // TODO make retry more specific to eventual consitencey
			},
			RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
		}.Run(ctx, func(ctx context.Context) error {
			_, err = regionconn.DeregisterImage(&ec2.DeregisterImageInput{
				ImageId: resp.Images[0].ImageId,
			})
			return err
		})

		if err != nil {
			return fmt.Errorf("AWSAMICleanUp: Unable to Deregister Image %s", err.Error())
		}
	}

	return nil
}

func (a *AMIHelper) GetAmi() ([]*ec2.Image, error) {
	accessConfig := &awscommon.AccessConfig{}
	session, err := accessConfig.Session()
	if err != nil {
		return nil, fmt.Errorf("Unable to create aws session %s", err.Error())
	}

	regionconn := ec2.New(session.Copy(&aws.Config{
		Region: aws.String(a.Region),
	}))

	var resp *ec2.DescribeImagesOutput
	ctx := context.TODO()
	err = retry.Config{
		Tries: 11,
		ShouldRetry: func(err error) bool {
			return true // TODO make retry more specific to eventual consitencey
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		resp, err = regionconn.DescribeImages(&ec2.DescribeImagesInput{
			Owners: aws.StringSlice([]string{"self"}),
			Filters: []*ec2.Filter{{
				Name:   aws.String("name"),
				Values: aws.StringSlice([]string{a.Name}),
			}}})
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("Unable to find Image %s: %s", a.Name, err.Error())
	}
	return resp.Images, nil
}
