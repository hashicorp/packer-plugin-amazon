/*
Deregister the test image with
aws ec2 deregister-image --image-id $(aws ec2 describe-images --output text --filters "Name=name,Values=packer-test-packer-test-dereg" --query 'Images[*].{ID:ImageId}')
*/
//nolint:unparam
package ebs

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/packer-plugin-amazon/builder/common"
	amazon_acc "github.com/hashicorp/packer-plugin-amazon/builder/ebs/acceptance"
	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

func TestAccBuilder_EbsBasic(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-plugin-amazon-ebs-basic-acc-test %d", time.Now().Unix()),
	}
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_basic_test",
		Template: fmt.Sprintf(testBuilderAccBasic, ami.Name),
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
	amiName := fmt.Sprintf("packer-test-builder-region-copy-acc-test-%d", time.Now().Unix())
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_region_copy_test",
		Template: fmt.Sprintf(testBuilderAccRegionCopy, amiName),
		Teardown: func() error {
			ami := amazon_acc.AMIHelper{
				Region: "us-east-1",
				Name:   amiName,
			}
			_ = ami.CleanUpAmi()
			ami = amazon_acc.AMIHelper{
				Region: "us-west-2",
				Name:   amiName,
			}
			_ = ami.CleanUpAmi()
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
	amiName := fmt.Sprintf("dereg %d", time.Now().Unix())
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
	amiName := fmt.Sprintf("packer-test-dereg %d", time.Now().Unix())

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
	_ = ec2conn.WaitUntilImageExists(describeInput)
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
		Name:   fmt.Sprintf("packer-sharing-acc-test %d", time.Now().Unix()),
	}

	testCase := &acctest.PluginTestCase{
		Name: "amazon-ebs_ami_sharing_test",
		Setup: func() error {
			missing_v := []string{}
			env_vars := []string{"TESTACC_AWS_ACCOUNT_ID", "TESTACC_AWS_ORG_ARN", "TESTACC_AWS_OU_ARN"}
			for _, var_name := range env_vars {
				v := os.Getenv(var_name)
				if v == "" {
					missing_v = append(missing_v, var_name)
				}
			}
			if len(missing_v) > 0 {
				return fmt.Errorf("%s must be set for acceptance tests", strings.Join(missing_v, ","))
			}
			return nil
		},
		Template: buildSharingConfig(os.Getenv("TESTACC_AWS_ACCOUNT_ID"), os.Getenv("TESTACC_AWS_ORG_ARN"), os.Getenv("TESTACC_AWS_OU_ARN"), ami.Name),
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return checkAMISharing(ami, 4, os.Getenv("TESTACC_AWS_ACCOUNT_ID"), "all")
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
		Name:   fmt.Sprintf("packer-enc-acc-test %d", time.Now().Unix()),
	}

	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_encrypted_boot_test",
		Template: fmt.Sprintf(testBuilderAccEncrypted, ami.Name),
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
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-ssm-acc-test %d", time.Now().Unix()),
	}
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_sessionmanager_interface_test",
		Template: fmt.Sprintf(testBuilderAccSessionManagerInterface, ami.Name),
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

func TestAccBuilder_EbsEnableDeprecation(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-deprecation-acc-test %d", time.Now().Unix()),
	}
	deprecationTime := time.Now().UTC().AddDate(0, 0, 1)
	deprecationTimeStr := deprecationTime.Format(time.RFC3339)
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_enable_deprecation_test",
		Template: buildEnableDeprecationConfig(deprecationTimeStr, ami.Name),
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return checkDeprecationEnabled(ami, deprecationTime)
		},
	}
	acctest.TestPlugin(t, testCase)
}
func checkDeprecationEnabled(ami amazon_acc.AMIHelper, deprecationTime time.Time) error {
	images, err := ami.GetAmi()
	if err != nil || len(images) == 0 {
		return fmt.Errorf("Failed to find ami %s at region %s", ami.Name, ami.Region)
	}

	ec2conn, _ := testEC2Conn()
	imageResp, err := ec2conn.DescribeImages(&ec2.DescribeImagesInput{
		ImageIds: []*string{images[0].ImageId},
	})

	if err != nil {
		return fmt.Errorf("Error Describe Image for AMI (%s): %s", ami.Name, err)
	}

	expectTime := deprecationTime.Round(time.Minute)
	expectTimeStr := expectTime.Format(time.RFC3339)

	image := imageResp.Images[0]
	if image.DeprecationTime == nil {
		return fmt.Errorf("Failed to Enable Deprecation for AMI (%s), expected Deprecation Time (%s), got empty", ami.Name, expectTimeStr)
	}

	actualTimeStr := aws.StringValue(image.DeprecationTime)
	actualTime, _ := time.Parse(time.RFC3339, actualTimeStr)
	if !actualTime.Equal(expectTime) {
		return fmt.Errorf("Wrong Deprecation Time, expected (%s), got (%s)", expectTimeStr, actualTimeStr)
	}

	return nil
}

//go:embed test-fixtures/interpolated_run_tags.pkr.hcl
var testHCLInterpolatedRunTagsSource string

func TestAccBuilder_EbsRunTags(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-west-2",
		Name:   fmt.Sprintf("packer-amazon-run-tags-test %d", time.Now().Unix()),
	}

	testcase := &acctest.PluginTestCase{
		Name: "amazon-ebs_hcl2_run_tags_test",
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Template: fmt.Sprintf(testHCLInterpolatedRunTagsSource, ami.Name),
		Check: func(buildcommand *exec.Cmd, logfile string) error {
			if buildcommand.ProcessState != nil {
				if buildcommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("bad exit code. logfile: %s", logfile)
				}
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testcase)
}

//go:embed test-fixtures/interpolated_run_tags.json
var testJSONInterpolatedRunTagsSource string

func TestAccBuilder_EbsRunTagsJSON(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-west-2",
		Name:   fmt.Sprintf("packer-amazon-run-tags-test %d", time.Now().Unix()),
	}

	testcase := &acctest.PluginTestCase{
		Name: "amazon-ebs_json_run_tags_test",
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Template: testJSONInterpolatedRunTagsSource,
		Check: func(buildcommand *exec.Cmd, logfile string) error {
			if buildcommand.ProcessState != nil {
				if buildcommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("bad exit code. logfile: %s", logfile)
				}
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testcase)
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
		"ami_name": "%s"
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
		"ami_name": "%s",
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
		"ami_users":["%s"],
		"ami_groups":["all"],
		"ami_org_arns": ["%s"],
		"ami_ou_arns": ["%s"],
		"ami_name": "%s"
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
		"ami_name": "%s",
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
		"ami_name": "%s"
	}]
}
`

const testBuilderAccEnableDeprecation = `
{
	"builders": [{
		"type": "amazon-ebs",
		"region": "us-east-1",
		"instance_type": "m3.medium",
		"source_ami": "ami-76b2a71e",
		"ssh_username": "ubuntu",
		"deprecate_at" : "%s",
		"ami_name": "%s"
	}]
}
`

func buildForceDeregisterConfig(val, name string) string {
	return fmt.Sprintf(testBuilderAccForceDeregister, val, name)
}

func buildForceDeleteSnapshotConfig(val, name string) string {
	return fmt.Sprintf(testBuilderAccForceDeleteSnapshot, val, val, name)
}

func buildSharingConfig(val, val2, val3, name string) string {
	return fmt.Sprintf(testBuilderAccSharing, val, val2, val3, name)
}

func buildEnableDeprecationConfig(val, name string) string {
	return fmt.Sprintf(testBuilderAccEnableDeprecation, val, name)
}
