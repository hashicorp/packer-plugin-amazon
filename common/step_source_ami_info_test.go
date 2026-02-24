// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
)

func TestStepSourceAmiInfo_PVImage(t *testing.T) {
	err := new(StepSourceAMIInfo).canEnableEnhancedNetworking(&types.Image{
		VirtualizationType: types.VirtualizationTypeParavirtual,
	})
	assert.Error(t, err)
}

func TestStepSourceAmiInfo_HVMImage(t *testing.T) {
	err := new(StepSourceAMIInfo).canEnableEnhancedNetworking(&types.Image{
		VirtualizationType: types.VirtualizationTypeHvm,
	})
	assert.NoError(t, err)
}

func TestStepSourceAmiInfo_PVImageWithAMIVirtPV(t *testing.T) {
	stepSourceAMIInfo := StepSourceAMIInfo{
		AMIVirtType: "paravirtual",
	}
	err := stepSourceAMIInfo.canEnableEnhancedNetworking(&types.Image{
		VirtualizationType: types.VirtualizationTypeParavirtual,
	})
	assert.Error(t, err)
}

func TestStepSourceAmiInfo_PVImageWithAMIVirtHVM(t *testing.T) {
	stepSourceAMIInfo := StepSourceAMIInfo{
		AMIVirtType: "hvm",
	}
	err := stepSourceAMIInfo.canEnableEnhancedNetworking(&types.Image{
		VirtualizationType: types.VirtualizationTypeParavirtual,
	})
	assert.NoError(t, err)
}

func TestLatestByNameAmi_ReturnsLexicographicallyLast(t *testing.T) {
	images := []types.Image{
		{ImageId: aws.String("ami-sp7"), Name: aws.String("suse-sles-15-sp7-v20241105-hvm-ssd-x86_64")},
		{ImageId: aws.String("ami-sp6"), Name: aws.String("suse-sles-15-sp6-v20250210-hvm-ssd-x86_64")},
		{ImageId: aws.String("ami-sp5"), Name: aws.String("suse-sles-15-sp5-v20240101-hvm-ssd-x86_64")},
	}
	result := LatestByNameAmi(images)
	assert.Equal(t, "ami-sp7", aws.ToString(result.ImageId))
}

func TestLatestByNameAmi_SingleImage(t *testing.T) {
	images := []types.Image{
		{ImageId: aws.String("ami-only"), Name: aws.String("my-ami")},
	}
	result := LatestByNameAmi(images)
	assert.Equal(t, "ami-only", aws.ToString(result.ImageId))
}

func TestLatestByNameAmi_IdenticalNames(t *testing.T) {
	images := []types.Image{
		{ImageId: aws.String("ami-a"), Name: aws.String("same-name")},
		{ImageId: aws.String("ami-b"), Name: aws.String("same-name")},
	}
	result := LatestByNameAmi(images)
	name := aws.ToString(result.Name)
	assert.Equal(t, "same-name", name)
}
