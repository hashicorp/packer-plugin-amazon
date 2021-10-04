package ebsvolume

import (
	"reflect"
	"testing"

	registryimage "github.com/hashicorp/packer-plugin-sdk/packer/registry/image"
	"github.com/mitchellh/mapstructure"
)

func TestArtifactState(t *testing.T) {
	expectedData := "this is the data"
	artifact := &Artifact{
		StateData: map[string]interface{}{"state_data": expectedData},
	}

	// Valid state
	result := artifact.State("state_data")
	if result != expectedData {
		t.Fatalf("Bad: State data was %s instead of %s", result, expectedData)
	}

	// Invalid state
	result = artifact.State("invalid_key")
	if result != nil {
		t.Fatalf("Bad: State should be nil for invalid state data name")
	}

	// Nil StateData should not fail and should return nil
	artifact = &Artifact{}
	result = artifact.State("key")
	if result != nil {
		t.Fatalf("Bad: State should be nil for nil StateData")
	}
}

func TestArtifactState_hcpPackerRegistryMetadata(t *testing.T) {

	volumes := make(EbsVolumes)
	volumes["west"] = []string{"vol-4567", "vol-0987"}
	snapshots := make(EbsSnapshots)
	snapshots["west"] = []string{"snap-4567", "snap-0987"}

	artifact := &Artifact{
		Volumes:   volumes,
		Snapshots: snapshots,
		StateData: map[string]interface{}{"generated_data": map[string]interface{}{"SourceAMI": "ami-12345"}},
	}

	result := artifact.State(registryimage.ArtifactStateURI)
	if result == nil {
		t.Fatalf("Bad: HCP Packer registry image data was nil")
	}

	var images []registryimage.Image
	err := mapstructure.Decode(result, &images)
	if err != nil {
		t.Errorf("Bad: unexpected error when trying to decode state into registryimage.Image %v", err)
	}

	if len(images) != 4 {
		t.Errorf("Bad: we should have four images for this test Artifact but we got %d", len(images))
	}

	expected := []registryimage.Image{
		{
			ImageID:        "vol-4567",
			ProviderName:   "aws",
			ProviderRegion: "west",
			SourceImageID:  "ami-12345",
		},
		{
			ImageID:        "vol-0987",
			ProviderName:   "aws",
			ProviderRegion: "west",
			SourceImageID:  "ami-12345",
		},
		{
			ImageID:        "snap-4567",
			ProviderName:   "aws",
			ProviderRegion: "west",
			SourceImageID:  "ami-12345",
		},
		{
			ImageID:        "snap-0987",
			ProviderName:   "aws",
			ProviderRegion: "west",
			SourceImageID:  "ami-12345",
		},
	}
	if !reflect.DeepEqual(images, expected) {
		t.Fatalf("bad: %#v", images)
	}
}
