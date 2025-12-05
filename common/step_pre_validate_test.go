// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// DescribeVpcs mocks an ec2.DescribeVpcsOutput for a given input
func (m *mockEC2Conn) DescribeVpcs(ctx context.Context, input *ec2.DescribeVpcsInput,
	optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {

	if input == nil || input.VpcIds[0] == "" {
		return nil, fmt.Errorf("oops looks like we need more input")
	}

	var isDefault bool
	vpcID := input.VpcIds[0]

	//only one default VPC per region
	if strings.Contains("vpc-default-id", vpcID) {
		isDefault = true
	}

	output := &ec2.DescribeVpcsOutput{
		Vpcs: []ec2types.Vpc{
			{
				IsDefault: aws.Bool(isDefault),
				VpcId:     aws.String(vpcID),
			},
		},
	}
	return output, nil
}

func TestStepPreValidate_checkVpc(t *testing.T) {
	tt := []struct {
		name          string
		step          StepPreValidate
		errorExpected bool
	}{
		{"DefaultVpc", StepPreValidate{VpcId: "vpc-default-id"}, false},
		{"NonDefaultVpcNoSubnet", StepPreValidate{VpcId: "vpc-1234567890"}, true},
		{"NonDefaultVpcWithSubnet", StepPreValidate{VpcId: "vpc-1234567890", SubnetId: "subnet-1234567890"}, false},
		{"SubnetWithNoVpc", StepPreValidate{SubnetId: "subnet-1234567890"}, false},
		{"NoVpcInformation", StepPreValidate{}, false},
		{"NonDefaultVpcWithSubnetFilter", StepPreValidate{VpcId: "vpc-1234567890", HasSubnetFilter: true}, false},
	}

	mockConn, err := getMockConn(context.TODO(), nil, "")
	if err != nil {
		t.Fatal("unable to get a mock connection")
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.step.checkVpc(ctx, mockConn)

			if tc.errorExpected && err == nil {
				t.Errorf("expected a validation error for %q but got %q", tc.name, err)
			}

			if !tc.errorExpected && err != nil {
				t.Errorf("expected a validation to pass for %q but got %q", tc.name, err)
			}
		})
	}

}
