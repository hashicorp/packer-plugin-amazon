/*
Deregister the test image with
aws ec2 deregister-image --image-id $(aws ec2 describe-images --output text --filters "Name=name,Values=packer-test-packer-test-dereg" --query 'Images[*].{ID:ImageId}')
*/
//nolint:unparam
package ebs

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/packer-plugin-amazon/builder/common"
	amazon_acc "github.com/hashicorp/packer-plugin-amazon/builder/ebs/acceptance"
	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

func TestAccBuilder_EbsBasic(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "packer-plugin-amazon-ebs-basic-acc-test",
	}
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_basic_test",
		Template: testBuilderAccBasic,
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

func TestAccBuilder_EbsRegionCopy(t *testing.T) {
	amiName := "packer-test-builder-region-copy-acc-test"
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_region_copy_test",
		Template: testBuilderAccRegionCopy,
		Teardown: func() error {
			ami := amazon_acc.AMIHelper{
				Region: "us-east-1",
				Name:   amiName,
			}
			ami.CleanUpAmi()
			ami = amazon_acc.AMIHelper{
				Region: "us-west-2",
				Name:   amiName,
			}
			ami.CleanUpAmi()
			return nil
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return checkRegionCopy(amiName, []string{"us-east-1", "us-west-2"})
		},
	}
	acctest.TestPlugin(t, testCase)
}

func checkRegionCopy(amiName string, regions []string) error {
	regionSet := make(map[string]struct{})

	for _, r := range regions {
		regionSet[r] = struct{}{}
		ami := amazon_acc.AMIHelper{
			Region: r,
			Name:   amiName,
		}
		images, err := ami.GetAmi()
		if err != nil || len(images) != 1 {
			continue
		}
		delete(regionSet, r)
	}

	if len(regionSet) > 0 {
		return fmt.Errorf("didn't copy to: %#v", regionSet)
	}
	return nil
}

func TestAccBuilder_EbsForceDeregister(t *testing.T) {
	amiName := "dereg"
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_force_deregister_part1_test",
		Template: buildForceDeregisterConfig("false", amiName),
		Teardown: func() error {
			// skip
			return nil
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)

	testCase = &acctest.PluginTestCase{
		Name:     "amazon-ebs_force_deregister_part2_test",
		Template: buildForceDeregisterConfig("true", amiName),
		Teardown: func() error {
			ami := amazon_acc.AMIHelper{
				Region: "us-east-1",
				Name:   amiName,
			}
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

func TestAccBuilder_EbsForceDeleteSnapshot(t *testing.T) {
	amiName := "packer-test-dereg"

	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_force_delete_snapshot_part1_test",
		Template: buildForceDeleteSnapshotConfig("false", amiName),
		Teardown: func() error {
			// skip
			return nil
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)

	// Get image data by AMI name
	ec2conn, _ := testEC2Conn()
	describeInput := &ec2.DescribeImagesInput{Filters: []*ec2.Filter{
		{
			Name:   aws.String("name"),
			Values: []*string{aws.String(amiName)},
		},
	}}
	ec2conn.WaitUntilImageExists(describeInput)
	imageResp, _ := ec2conn.DescribeImages(describeInput)
	image := imageResp.Images[0]

	// Get snapshot ids for image
	snapshotIds := []*string{}
	for _, device := range image.BlockDeviceMappings {
		if device.Ebs != nil && device.Ebs.SnapshotId != nil {
			snapshotIds = append(snapshotIds, device.Ebs.SnapshotId)
		}
	}

	testCase = &acctest.PluginTestCase{
		Name:     "amazon-ebs_force_delete_snapshot_part2_test",
		Template: buildForceDeleteSnapshotConfig("true", amiName),
		Teardown: func() error {
			ami := amazon_acc.AMIHelper{
				Region: "us-east-1",
				Name:   amiName,
			}
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return checkSnapshotsDeleted(snapshotIds)
		},
	}
	acctest.TestPlugin(t, testCase)
}

func checkSnapshotsDeleted(snapshotIds []*string) error {
	// Verify the snapshots are gone
	ec2conn, _ := testEC2Conn()
	snapshotResp, _ := ec2conn.DescribeSnapshots(
		&ec2.DescribeSnapshotsInput{SnapshotIds: snapshotIds},
	)

	if len(snapshotResp.Snapshots) > 0 {
		return fmt.Errorf("Snapshots weren't successfully deleted by `force_delete_snapshot`")
	}
	return nil
}

func TestAccBuilder_EbsAmiSharing(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "packer-sharing-acc-test",
	}

	testCase := &acctest.PluginTestCase{
		Name: "amazon-ebs_ami_sharing_test",
		Setup: func() error {
			if v := os.Getenv("TESTACC_AWS_ACCOUNT_ID"); v == "" {
				return fmt.Errorf("TESTACC_AWS_ACCOUNT_ID must be set for acceptance tests")
			}
			return nil
		},
		Template: buildSharingConfig(os.Getenv("TESTACC_AWS_ACCOUNT_ID")),
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return checkAMISharing(ami, 2, os.Getenv("TESTACC_AWS_ACCOUNT_ID"), "all")
		},
	}
	acctest.TestPlugin(t, testCase)
}

func checkAMISharing(ami amazon_acc.AMIHelper, count int, uid, group string) error {
	images, err := ami.GetAmi()
	if err != nil || len(images) == 0 {
		return fmt.Errorf("failed to find ami %s at region %s", ami.Name, ami.Region)
	}

	ec2conn, _ := testEC2Conn()
	imageResp, err := ec2conn.DescribeImageAttribute(&ec2.DescribeImageAttributeInput{
		Attribute: aws.String("launchPermission"),
		ImageId:   images[0].ImageId,
	})

	if err != nil {
		return fmt.Errorf("Error retrieving Image Attributes for AMI %s in AMI Sharing Test: %s", ami.Name, err)
	}

	// Launch Permissions are in addition to the userid that created it, so if
	// you add 3 additional ami_users, you expect 2 Launch Permissions here
	if len(imageResp.LaunchPermissions) != count {
		return fmt.Errorf("Error in Image Attributes, expected (%d) Launch Permissions, got (%d)", count, len(imageResp.LaunchPermissions))
	}

	userFound := false
	for _, lp := range imageResp.LaunchPermissions {
		if lp.UserId != nil && uid == *lp.UserId {
			userFound = true
		}
	}

	if !userFound {
		return fmt.Errorf("Error in Image Attributes, expected User ID (%s) to have Launch Permissions, but was not found", uid)
	}

	groupFound := false
	for _, lp := range imageResp.LaunchPermissions {
		if lp.Group != nil && group == *lp.Group {
			groupFound = true
		}
	}

	if !groupFound {
		return fmt.Errorf("Error in Image Attributes, expected Group ID (%s) to have Launch Permissions, but was not found", group)
	}

	return nil
}

func TestAccBuilder_EbsEncryptedBoot(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "packer-enc-acc-test",
	}

	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_encrypted_boot_test",
		Template: testBuilderAccEncrypted,
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return checkBootEncrypted(ami)
		},
	}
	acctest.TestPlugin(t, testCase)
}

func checkBootEncrypted(ami amazon_acc.AMIHelper) error {
	images, err := ami.GetAmi()
	if err != nil || len(images) == 0 {
		return fmt.Errorf("failed to find ami %s at region %s", ami.Name, ami.Region)
	}

	// describe the image, get block devices with a snapshot
	ec2conn, _ := testEC2Conn()
	imageResp, err := ec2conn.DescribeImages(&ec2.DescribeImagesInput{
		ImageIds: []*string{images[0].ImageId},
	})

	if err != nil {
		return fmt.Errorf("Error retrieving Image Attributes for AMI (%s) in AMI Encrypted Boot Test: %s", ami.Name, err)
	}

	image := imageResp.Images[0] // Only requested a single AMI ID

	rootDeviceName := image.RootDeviceName

	for _, bd := range image.BlockDeviceMappings {
		if *bd.DeviceName == *rootDeviceName {
			if *bd.Ebs.Encrypted != true {
				return fmt.Errorf("volume not encrypted: %s", *bd.Ebs.SnapshotId)
			}
		}
	}

	return nil
}

func TestAccBuilder_EbsSessionManagerInterface(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_sessionmanager_interface_test",
		Template: testBuilderAccSessionManagerInterface,
		Teardown: func() error {
			helper := amazon_acc.AMIHelper{
				Region: "us-east-1",
				Name:   "packer-ssm-acc-test",
			}
			return helper.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

func testEC2Conn() (*ec2.EC2, error) {
	access := &common.AccessConfig{RawRegion: "us-east-1"}
	session, err := access.Session()
	if err != nil {
		return nil, err
	}

	return ec2.New(session), nil
}

const testBuilderAccBasic = `
{
	"builders": [{
		"type": "amazon-ebs",
		"region": "us-east-1",
		"instance_type": "m3.medium",
		"source_ami": "ami-76b2a71e",
		"ssh_username": "ubuntu",
		"ami_name": "packer-plugin-amazon-ebs-basic-acc-test"
	}]
}
`

const testBuilderAccRegionCopy = `
{
	"builders": [{
		"type": "amazon-ebs",
		"region": "us-east-1",
		"instance_type": "m3.medium",
		"source_ami": "ami-76b2a71e",
		"ssh_username": "ubuntu",
		"ami_name": "packer-test-builder-region-copy-acc-test",
		"ami_regions": ["us-east-1", "us-west-2"]
	}]
}
`

const testBuilderAccForceDeregister = `
{
	"builders": [{
		"type": "amazon-ebs",
		"region": "us-east-1",
		"instance_type": "m3.medium",
		"source_ami": "ami-76b2a71e",
		"ssh_username": "ubuntu",
		"force_deregister": "%s",
		"ami_name": "%s"
	}]
}
`

const testBuilderAccForceDeleteSnapshot = `
{
	"builders": [{
		"type": "amazon-ebs",
		"region": "us-east-1",
		"instance_type": "m3.medium",
		"source_ami": "ami-76b2a71e",
		"ssh_username": "ubuntu",
		"force_deregister": "%s",
		"force_delete_snapshot": "%s",
		"ami_name": "%s"
	}]
}
`

const testBuilderAccSharing = `
{
	"builders": [{
		"type": "amazon-ebs",
		"region": "us-east-1",
		"instance_type": "m3.medium",
		"source_ami": "ami-76b2a71e",
		"ssh_username": "ubuntu",
		"ami_name": "packer-sharing-acc-test",
		"ami_users":["%s"],
		"ami_groups":["all"]
	}]
}
`

const testBuilderAccEncrypted = `
{
	"builders": [{
		"type": "amazon-ebs",
		"region": "us-east-1",
		"instance_type": "m3.medium",
		"source_ami":"ami-c15bebaa",
		"ssh_username": "ubuntu",
		"ami_name": "packer-enc-acc-test",
		"encrypt_boot": true
	}]
}
`

const testBuilderAccSessionManagerInterface = `
{
	"builders": [{
		"type": "amazon-ebs",
		"region": "us-east-1",
		"instance_type": "m3.medium",
		"source_ami_filter": {
				"filters": {
						"virtualization-type": "hvm",
						"name": "ubuntu/images/*ubuntu-xenial-16.04-amd64-server-*",
						"root-device-type": "ebs"
				},
				"owners": [
						"099720109477"
				],
				"most_recent": true
		},
		"ssh_username": "ubuntu",
		"ssh_interface": "session_manager",
		"iam_instance_profile": "SSMInstanceProfile",
		"ami_name": "packer-ssm-acc-test"
	}]
}
`

func buildForceDeregisterConfig(val, name string) string {
	return fmt.Sprintf(testBuilderAccForceDeregister, val, name)
}

func buildForceDeleteSnapshotConfig(val, name string) string {
	return fmt.Sprintf(testBuilderAccForceDeleteSnapshot, val, val, name)
}

func buildSharingConfig(val string) string {
	return fmt.Sprintf(testBuilderAccSharing, val)
}
