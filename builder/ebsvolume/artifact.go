// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebsvolume

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	registryimage "github.com/hashicorp/packer-plugin-sdk/packer/registry/image"
)

// map of region to list of volume IDs
type EbsVolumes map[string][]string

// map of region to list of snapshot IDs
type EbsSnapshots map[string][]string

// Artifact is an artifact implementation that contains built AMIs.
type Artifact struct {
	// A map of regions to EBS Volume IDs.
	Volumes EbsVolumes
	// A map of regions to EBS Snapshot IDs.
	Snapshots EbsSnapshots
	// BuilderId is the unique ID for the builder that created this AMI
	BuilderIdValue string

	// StateData should store data such as GeneratedData
	// to be shared with post-processors
	StateData map[string]interface{}

	// EC2 client for performing API stuff.
	Client clients.Ec2Client
}

func (a *Artifact) BuilderId() string {
	return a.BuilderIdValue
}

func (*Artifact) Files() []string {
	// We have no files
	return nil
}

// returns a sorted list of region:ID pairs
func (a *Artifact) idList() []string {

	parts := make([]string, 0, len(a.Volumes)+len(a.Snapshots))

	for region, volumeIDs := range a.Volumes {
		for _, volumeID := range volumeIDs {
			parts = append(parts, fmt.Sprintf("%s:%s", region, volumeID))
		}
	}

	for region, snapshotIDs := range a.Snapshots {
		for _, snapshotID := range snapshotIDs {
			parts = append(parts, fmt.Sprintf("%s:%s", region, snapshotID))
		}
	}

	sort.Strings(parts)
	return parts
}

func (a *Artifact) Id() string {
	return strings.Join(a.idList(), ",")
}

func (a *Artifact) String() string {
	return fmt.Sprintf("EBS Volumes were created:\n\n%s", strings.Join(a.idList(), "\n"))
}

func (a *Artifact) State(name string) interface{} {
	// To be able to push metadata to HCP Packer Registry, Packer will read the 'par.artifact.metadata'
	// state from artifacts to get a build's metadata.
	if name == registryimage.ArtifactStateURI {
		return a.stateHCPPackerRegistryMetadata()
	}

	if _, ok := a.StateData[name]; ok {
		return a.StateData[name]
	}
	return nil
}

func (a *Artifact) Destroy() error {
	errors := make([]error, 0)

	for region, volumeIDs := range a.Volumes {
		for _, volumeID := range volumeIDs {
			log.Printf("Deregistering Volume ID (%s) from region (%s)", volumeID, region)

			input := &ec2.DeleteVolumeInput{
				VolumeId: &volumeID,
			}
			if _, err := a.Client.DeleteVolume(context.Background(), input); err != nil {
				errors = append(errors, err)
			}
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

// stateHCPPackerRegistryMetadata will write the metadata as an hcpRegistryImage for each of the AMIs
// present in this artifact.
func (a *Artifact) stateHCPPackerRegistryMetadata() interface{} {

	images := make([]*registryimage.Image, 0, len(a.Volumes)+len(a.Snapshots))
	for region, volumeIDs := range a.Volumes {
		for _, volumeID := range volumeIDs {
			volumeID := volumeID
			image := registryimage.Image{
				ImageID:        volumeID,
				ProviderRegion: region,
				ProviderName:   "aws",
			}
			images = append(images, &image)
		}
	}

	for region, snapshotIDs := range a.Snapshots {
		for _, snapshotID := range snapshotIDs {
			snapshotID := snapshotID
			image := registryimage.Image{
				ImageID:        snapshotID,
				ProviderRegion: region,
				ProviderName:   "aws",
			}
			images = append(images, &image)
		}
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
