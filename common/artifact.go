// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	registryimage "github.com/hashicorp/packer-plugin-sdk/packer/registry/image"
)

// Artifact is an artifact implementation that contains built AMIs.
type Artifact struct {
	// A map of regions to AMI IDs.
	Amis map[string]string

	// BuilderId is the unique ID for the builder that created this AMI
	BuilderIdValue string

	// StateData should store data such as GeneratedData
	// to be shared with post-processors
	StateData map[string]interface{}

	// EC2 config for performing API stuff.
	Config *aws.Config
}

func (a *Artifact) BuilderId() string {
	return a.BuilderIdValue
}

func (*Artifact) Files() []string {
	// We have no files
	return nil
}

func (a *Artifact) Id() string {
	parts := make([]string, 0, len(a.Amis))
	for region, amiId := range a.Amis {
		parts = append(parts, fmt.Sprintf("%s:%s", region, amiId))
	}

	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (a *Artifact) String() string {
	amiStrings := make([]string, 0, len(a.Amis))
	for region, id := range a.Amis {
		single := fmt.Sprintf("%s: %s", region, id)
		amiStrings = append(amiStrings, single)
	}

	sort.Strings(amiStrings)
	return fmt.Sprintf("AMIs were created:\n%s\n", strings.Join(amiStrings, "\n"))
}

func (a *Artifact) State(name string) interface{} {
	if _, ok := a.StateData[name]; ok {
		return a.StateData[name]
	}

	switch name {
	case "atlas.artifact.metadata":
		return a.stateAtlasMetadata()
		// To be able to push metadata to HCP Packer Registry, Packer will read the 'par.artifact.metadata'
		// state from artifacts to get a build's metadata.
	case registryimage.ArtifactStateURI:
		return a.stateHCPPackerRegistryMetadata()
	default:
		return nil
	}
}

func (a *Artifact) Destroy() error {
	errors := make([]error, 0)
	ctx := context.TODO()
	for region, imageId := range a.Amis {
		log.Printf("Deregistering image ID (%s) from region (%s)", imageId, region)
		opt := []func(o *ec2.Options){
			func(o *ec2.Options) {
				o.Region = region
			},
		}
		ec2Client := ec2.NewFromConfig(*a.Config, opt...)

		// Get image metadata
		imageResp, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
			ImageIds: []string{imageId},
		})
		if err != nil {
			errors = append(errors, err)
		}
		if len(imageResp.Images) == 0 {
			err := fmt.Errorf("Error retrieving details for AMI (%s), no images found", imageId)
			errors = append(errors, err)
		}

		err = DestroyAMIs([]string{imageId}, ec2Client)
		if err != nil {
			errors = append(errors, err)
		}

	}

	if len(errors) > 0 {
		if len(errors) == 1 {
			return errors[0]
		} else {
			return &packersdk.MultiError{Errors: errors}
		}
	}

	return nil
}

func (a *Artifact) stateAtlasMetadata() interface{} {
	metadata := make(map[string]string)
	for region, imageId := range a.Amis {
		k := fmt.Sprintf("region.%s", region)
		metadata[k] = imageId
	}

	return metadata
}

// stateHCPPackerRegistryMetadata will write the metadata as an hcpRegistryImage for each of the AMIs
// present in this artifact.
func (a *Artifact) stateHCPPackerRegistryMetadata() interface{} {

	f := func(k, v interface{}) (*registryimage.Image, error) {

		region, ok := k.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected type of key in Amis map")
		}
		imageId, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected type for value in Amis map")
		}
		image := registryimage.Image{
			ImageID:        imageId,
			ProviderRegion: region,
			ProviderName:   "aws",
		}

		return &image, nil

	}

	images, err := registryimage.FromMappedData(a.Amis, f)
	if err != nil {
		log.Printf("[TRACE] error encountered when creating HCP Packer registry image for artifact.Amis: %s", err)
		return nil
	}

	if a.StateData == nil {
		return images
	}

	data, ok := a.StateData["generated_data"].(map[string]interface{})
	if !ok {
		return images
	}

	for _, image := range images {
		image.SourceImageID = data["SourceAMI"].(string)
	}

	return images
}
