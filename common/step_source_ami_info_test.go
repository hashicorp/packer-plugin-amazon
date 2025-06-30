// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"testing"

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
