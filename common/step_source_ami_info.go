// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

// StepSourceAMIInfo extracts critical information from the source AMI
// that is used throughout the AMI creation process.
//
// Produces:
//
//	source_image *ec2.Image - the source AMI info
type StepSourceAMIInfo struct {
	SourceAmi                string
	EnableAMISriovNetSupport bool
	EnableAMIENASupport      config.Trilean
	AMIVirtType              string
	AmiFilters               AmiFilterOptions
	IncludeDeprecated        bool
}

type imageSort []types.Image

func (a imageSort) Len() int      { return len(a) }
func (a imageSort) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a imageSort) Less(i, j int) bool {
	itime, _ := time.Parse(time.RFC3339, *a[i].CreationDate)
	jtime, _ := time.Parse(time.RFC3339, *a[j].CreationDate)
	return itime.Unix() < jtime.Unix()
}

// Returns the most recent AMI out of a slice of images.
func mostRecentAmi(images []types.Image) types.Image {
	sortedImages := images
	sort.Sort(imageSort(sortedImages))
	return sortedImages[len(sortedImages)-1]
}

func (s *StepSourceAMIInfo) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	client := state.Get("ec2v2").(clients.Ec2Client)
	ui := state.Get("ui").(packersdk.Ui)

	params := &ec2.DescribeImagesInput{
		IncludeDeprecated: &s.IncludeDeprecated,
	}

	if s.SourceAmi != "" {
		params.ImageIds = []string{s.SourceAmi}
	}

	image, err := s.AmiFilters.GetFilteredImage(ctx, params, client)
	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	ui.Say(fmt.Sprintf("Found Image ID: %s", *image.ImageId))

	// Enhanced Networking can only be enabled on HVM AMIs.
	// See http://goo.gl/icuXh5
	if s.EnableAMIENASupport.True() || s.EnableAMISriovNetSupport {
		err = s.canEnableEnhancedNetworking(image)
		if err != nil {
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	state.Put("source_image", image)
	return multistep.ActionContinue
}

func (s *StepSourceAMIInfo) Cleanup(multistep.StateBag) {}

func (s *StepSourceAMIInfo) canEnableEnhancedNetworking(image *types.Image) error {
	if s.AMIVirtType == "hvm" {
		return nil
	}
	if s.AMIVirtType != "" {
		return fmt.Errorf("Cannot enable enhanced networking, AMIVirtType '%s' is not HVM", s.AMIVirtType)
	}
	if image.VirtualizationType != types.VirtualizationTypeHvm {
		return fmt.Errorf("Cannot enable enhanced networking, source AMI '%s' is not HVM", s.SourceAmi)
	}
	return nil
}
