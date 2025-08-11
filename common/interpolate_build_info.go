// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
)

type BuildInfoTemplate struct {
	BuildRegion           string
	SourceAMI             string
	SourceAMICreationDate string
	SourceAMIName         string
	SourceAMIOwner        string
	SourceAMIOwnerName    string
	SourceAMITags         map[string]string
}

func extractBuildInfo(region string, state multistep.StateBag, generatedData *packerbuilderdata.GeneratedData) *BuildInfoTemplate {
	rawSourceAMI, hasSourceAMI := state.GetOk("source_image")
	if !hasSourceAMI {
		return &BuildInfoTemplate{
			BuildRegion: region,
		}
	}

	sourceAMI := rawSourceAMI.(*ec2types.Image)
	sourceAMITags := make(map[string]string, len(sourceAMI.Tags))
	for _, tag := range sourceAMI.Tags {
		sourceAMITags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	buildInfoTemplate := &BuildInfoTemplate{
		BuildRegion:           region,
		SourceAMI:             aws.ToString(sourceAMI.ImageId),
		SourceAMICreationDate: aws.ToString(sourceAMI.CreationDate),
		SourceAMIName:         aws.ToString(sourceAMI.Name),
		SourceAMIOwner:        aws.ToString(sourceAMI.OwnerId),
		SourceAMIOwnerName:    aws.ToString(sourceAMI.ImageOwnerAlias),
		SourceAMITags:         sourceAMITags,
	}

	generatedData.Put("BuildRegion", buildInfoTemplate.BuildRegion)
	generatedData.Put("SourceAMI", buildInfoTemplate.SourceAMI)
	generatedData.Put("SourceAMICreationDate", buildInfoTemplate.SourceAMICreationDate)
	generatedData.Put("SourceAMIName", buildInfoTemplate.SourceAMIName)
	generatedData.Put("SourceAMIOwner", buildInfoTemplate.SourceAMIOwner)
	generatedData.Put("SourceAMIOwnerName", buildInfoTemplate.SourceAMIOwnerName)

	return buildInfoTemplate
}

func GetGeneratedDataList() []string {
	return []string{
		"SourceAMIName",
		"BuildRegion",
		"SourceAMI",
		"SourceAMICreationDate",
		"SourceAMIOwner",
		"SourceAMIOwnerName",
	}
}
