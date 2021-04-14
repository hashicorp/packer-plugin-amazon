package ebs

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"

	"github.com/aws/aws-sdk-go/service/ec2"
	amazon_acc "github.com/hashicorp/packer-plugin-amazon/builder/ebs/acceptance"
	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

type TFBuilder struct {
	Type         string            `json:"type"`
	Region       string            `json:"region"`
	SourceAmi    string            `json:"source_ami"`
	InstanceType string            `json:"instance_type"`
	SshUsername  string            `json:"ssh_username"`
	AmiName      string            `json:"ami_name"`
	Tags         map[string]string `json:"tags"`
	SnapshotTags map[string]string `json:"snapshot_tags"`
}

type TFConfig struct {
	Builders []TFBuilder `json:"builders"`
}

func TestAccBuilder_EbsTagsBasic(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "packer-tags-acc-testing",
	}
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_tags_test",
		Template: testBuilderTagsAccBasic,
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return checkTags(ami)
		},
	}
	acctest.TestPlugin(t, testCase)
}

func checkTags(ami amazon_acc.AMIHelper) error {
	images, err := ami.GetAmi()
	if err != nil || len(images) == 0 {
		return fmt.Errorf("failed to find ami %s at region %s", ami.Name, ami.Region)
	}

	config := TFConfig{}
	json.Unmarshal([]byte(testBuilderTagsAccBasic), &config)
	tags := config.Builders[0].Tags
	snapshotTags := config.Builders[0].SnapshotTags

	// Describe the image, get block devices with a snapshot
	ec2conn, _ := testEC2Conn()
	imageResp, err := ec2conn.DescribeImages(&ec2.DescribeImagesInput{
		ImageIds: []*string{images[0].ImageId},
	})

	if err != nil {
		return fmt.Errorf("Error retrieving details for AMI Artifact (%s) in Tags Test: %s", ami.Name, err)
	}

	if len(imageResp.Images) == 0 {
		return fmt.Errorf("No images found for AMI Artifact (%s) in Tags Test: %s", ami.Name, err)
	}

	image := imageResp.Images[0]

	// Check only those with a Snapshot ID, i.e. not Ephemeral
	var snapshots []*string
	for _, device := range image.BlockDeviceMappings {
		if device.Ebs != nil && device.Ebs.SnapshotId != nil {
			snapshots = append(snapshots, device.Ebs.SnapshotId)
		}
	}

	// Grab matching snapshot info
	resp, err := ec2conn.DescribeSnapshots(&ec2.DescribeSnapshotsInput{
		SnapshotIds: snapshots,
	})

	if err != nil {
		return fmt.Errorf("Error retrieving Snapshots for AMI Artifact (%s) in Tags Test: %s", ami.Name, err)
	}

	if len(resp.Snapshots) == 0 {
		return fmt.Errorf("No Snapshots found for AMI Artifact (%s) in Tags Test", ami.Name)
	}

	// Grab the snapshots, check the tags
	for _, s := range resp.Snapshots {
		expected := len(tags)
		for _, t := range s.Tags {
			for key, value := range tags {
				if val, ok := snapshotTags[key]; ok && val == *t.Value {
					expected--
				} else if key == *t.Key && value == *t.Value {
					expected--
				}
			}
		}

		if expected > 0 {
			return fmt.Errorf("Not all tags found")
		}
	}

	return nil
}

const testBuilderTagsAccBasic = `
{
  "builders": [
    {
      "type": "amazon-ebs",
      "region": "us-east-1",
      "source_ami": "ami-9eaa1cf6",
      "instance_type": "t2.micro",
      "ssh_username": "ubuntu",
      "ami_name": "packer-tags-acc-testing",
      "tags": {
        "OS_Version": "Ubuntu",
        "Release": "Latest",
        "Name": "Bleep"
      },
      "snapshot_tags": {
        "Name": "Foobar"
      }
    }
  ]
}
`
