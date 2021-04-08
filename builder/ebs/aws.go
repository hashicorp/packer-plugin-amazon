package ebs

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
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

	resp, err := regionconn.DescribeImages(&ec2.DescribeImagesInput{
		Owners: aws.StringSlice([]string{"self"}),
		Filters: []*ec2.Filter{{
			Name:   aws.String("name"),
			Values: aws.StringSlice([]string{a.Name}),
		}}})
	if err != nil {
		return fmt.Errorf("AWSAMICleanUp: Unable to find Image %s: %s", a.Name, err.Error())
	}

	if resp != nil && len(resp.Images) > 0 {
		_, err = regionconn.DeregisterImage(&ec2.DeregisterImageInput{
			ImageId: resp.Images[0].ImageId,
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

	resp, err := regionconn.DescribeImages(&ec2.DescribeImagesInput{
		Owners: aws.StringSlice([]string{"self"}),
		Filters: []*ec2.Filter{{
			Name:   aws.String("name"),
			Values: aws.StringSlice([]string{a.Name}),
		}}})
	if err != nil {
		return nil, fmt.Errorf("Unable to find Image %s: %s", a.Name, err.Error())
	}
	return resp.Images, nil
}
