/*
deregister the test image with
aws ec2 deregister-image --image-id $(aws ec2 describe-images --output text --filters "Name=name,Values=packer-test-packer-test-dereg" --query 'Images[*].{ID:ImageId}')
*/
//nolint:unparam
package ebs

import (
	_ "embed"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/packer-plugin-amazon/builder/common"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
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

func TestAccBuilder_EbsRegionsCopyWithDeprecation(t *testing.T) {
	amiName := fmt.Sprintf("packer-test-builder-region-copy-deprecate-acc-test-%d", time.Now().Unix())

	amis := []amazon_acc.AMIHelper{
		{
			Region: "us-east-1",
			Name:   amiName,
		},
		{
			Region: "us-west-1",
			Name:   amiName,
		},
	}

	deprecationTime := time.Now().UTC().AddDate(0, 0, 1)
	deprecationTimeStr := deprecationTime.Format(time.RFC3339)
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_region_copy_with_deprecation_test",
		Template: fmt.Sprintf(testBuilderAccRegionCopyDeprecated, deprecationTimeStr, amiName),
		Teardown: func() error {
			err := amis[0].CleanUpAmi()
			if err != nil {
				t.Logf("ami %s cleanup failed: %s", amis[0].Name, err)
			}
			err = amis[1].CleanUpAmi()
			if err != nil {
				t.Logf("ami %s cleanup failed: %s", amis[1].Name, err)
			}
			return nil
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}

			var errors error

			err := checkRegionCopy(
				amiName,
				[]string{"us-east-1", "us-west-1"})
			if err != nil {
				errors = multierror.Append(errors, err)
			}

			for _, ami := range amis {
				err := checkDeprecationEnabled(ami, deprecationTime)
				if err != nil {
					errors = multierror.Append(errors,
						fmt.Errorf(
							"AMI region %s: %s",
							ami.Region,
							err))
				}
			}

			return errors
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
	ec2conn, _ := testEC2Conn("us-east-1")
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
	ec2conn, _ := testEC2Conn("us-east-1")
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

	ec2conn, _ := testEC2Conn("us-east-1")
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

func TestAccBuilder_EbsEncryptedBootWithDeprecation(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-enc-acc-test %d", time.Now().Unix()),
	}

	deprecationTime := time.Now().UTC().AddDate(0, 0, 1)
	deprecationTimeStr := deprecationTime.Format(time.RFC3339)
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_encrypted_boot_with_deprecation_test",
		Template: fmt.Sprintf(testBuilderAccEncryptedDeprecated, deprecationTimeStr, ami.Name),
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			deprecationCheck := checkDeprecationEnabled(ami, deprecationTime)
			if deprecationCheck != nil {
				return deprecationCheck
			}
			return checkBootEncrypted(ami)
		},
	}
	acctest.TestPlugin(t, testCase)
}

func TestAccBuilder_EbsCopyRegionEncryptedBootWithDeprecation(t *testing.T) {
	amiName := fmt.Sprintf(
		"packer-test-builder-region-copy-encrypt-deprecate-acc-test-%d",
		time.Now().Unix())

	amis := []amazon_acc.AMIHelper{
		{
			Region: "us-east-1",
			Name:   amiName,
		},
		{
			Region: "us-west-1",
			Name:   amiName,
		},
	}

	deprecationTime := time.Now().UTC().AddDate(0, 0, 1)
	deprecationTimeStr := deprecationTime.Format(time.RFC3339)
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_region_copy_encrypted_boot_with_deprecation_test",
		Template: fmt.Sprintf(testBuilderAccRegionCopyEncryptedAndDeprecated, deprecationTimeStr, amiName),
		Teardown: func() error {
			err := amis[0].CleanUpAmi()
			if err != nil {
				t.Logf("ami %s cleanup failed: %s", amis[0].Name, err)
			}
			err = amis[1].CleanUpAmi()
			if err != nil {
				t.Logf("ami %s cleanup failed: %s", amis[1].Name, err)
			}
			return nil
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			var result error

			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}

			err := checkRegionCopy(
				amiName,
				[]string{"us-east-1", "us-west-1"})
			if err != nil {
				result = multierror.Append(result, err)
			}

			for _, ami := range amis {
				err := checkDeprecationEnabled(ami, deprecationTime)
				if err != nil {
					result = multierror.Append(result, fmt.Errorf(
						"Deprectiation failed, AMI region %s: %s",
						ami.Region,
						err))
				}

				err = checkBootEncrypted(ami)
				if err != nil {
					result = multierror.Append(result, fmt.Errorf(
						"Encryption check failed, AMI region %s: %s",
						ami.Region,
						err))
				}
			}

			return result
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
	ec2conn, _ := testEC2Conn(ami.Region)
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

			logs, err := os.ReadFile(logfile)
			if err != nil {
				return fmt.Errorf("couldn't read logs from logfile %s: %s", logfile, err)
			}
			if strings.Contains(string(logs), "Uploading SSH public key") {
				return fmt.Errorf("SSH key was uploaded, but shouldn't have been")
			}

			if strings.Contains(string(logs), "Bad exit status") {
				return fmt.Errorf("SSM session did not terminate gracefully and exited with a non-zero exit code")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

func TestAccBuilder_EbsSSMRebootProvisioner(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-ssm-reboot-acc-test %d", time.Now().Unix()),
	}
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_sessionmanager_interface_test_with_reboot",
		Template: fmt.Sprintf(testBuilderAccSSMWithReboot, ami.Name),
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}

			logs, err := os.ReadFile(logfile)
			if err != nil {
				return fmt.Errorf("couldn't read logs from logfile %s: %s", logfile, err)
			}
			if strings.Contains(string(logs), "Uploading SSH public key") {
				return fmt.Errorf("SSH key was uploaded, but shouldn't have been")
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

	ec2conn, err := testEC2Conn(ami.Region)
	if err != nil {
		return fmt.Errorf("Failed to connect to AWS on region %q: %s", ami.Region, err)
	}

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

//go:embed test-fixtures/ssh-keys/rsa_ssh_keypair.pkr.hcl
var testSSHKeyPairRSA string

func TestAccBuilder_EbsKeyPair_rsa(t *testing.T) {
	testcase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_rsa",
		Template: testSSHKeyPairRSA,
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
			}
			logs, err := os.Open(logfile)
			if err != nil {
				return fmt.Errorf("Unable find %s", logfile)
			}
			defer logs.Close()

			logsBytes, err := ioutil.ReadAll(logs)
			if err != nil {
				return fmt.Errorf("Unable to read %s", logfile)
			}
			logsString := string(logsBytes)

			expectedKeyType := "rsa"
			re := regexp.MustCompile(fmt.Sprintf(`(?:amazon-ebs.basic-example:\s+)+(ssh-%s)`, expectedKeyType))
			matched := re.FindStringSubmatch(logsString)

			if len(matched) != 2 {
				return fmt.Errorf("unable to capture key information from  %q", logfile)
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testcase)
}

//go:embed test-fixtures/ssh-keys/ed25519_ssh_keypair.pkr.hcl
var testSSHKeyPairED25519 string

func TestAccBuilder_EbsKeyPair_ed25519(t *testing.T) {
	testcase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_ed25519",
		Template: testSSHKeyPairED25519,
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
			}
			logs, err := os.Open(logfile)
			if err != nil {
				return fmt.Errorf("Unable find %s", logfile)
			}
			defer logs.Close()

			logsBytes, err := ioutil.ReadAll(logs)
			if err != nil {
				return fmt.Errorf("Unable to read %s", logfile)
			}
			logsString := string(logsBytes)

			expectedKeyType := "ed25519"
			re := regexp.MustCompile(fmt.Sprintf(`(?:amazon-ebs.basic-example:\s+)+(ssh-%s)`, expectedKeyType))
			matched := re.FindStringSubmatch(logsString)

			if len(matched) != 2 {
				return fmt.Errorf("unable to capture key information from  %q", logfile)
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testcase)
}

//go:embed test-fixtures/ssh-keys/rsa_sha2_only_server.pkr.hcl
var testRSASHA2OnlyServer string

func TestAccBuilder_EbsKeyPair_rsaSHA2OnlyServer(t *testing.T) {
	testcase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_rsa_sha2_srv_test",
		Template: testRSASHA2OnlyServer,
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
			}
			logs, err := os.Open(logfile)
			if err != nil {
				return fmt.Errorf("Unable find %s", logfile)
			}
			defer logs.Close()

			logsBytes, err := ioutil.ReadAll(logs)
			if err != nil {
				return fmt.Errorf("Unable to read %s", logfile)
			}
			logsString := string(logsBytes)

			re := regexp.MustCompile(`amazon-ebs.basic-example:\s+Successful login`)
			matched := re.FindString(logsString)

			if matched == "" {
				return fmt.Errorf("unable to success string from  %q", logfile)
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testcase)
}

func TestAccBuilder_PrivateKeyFile(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-pkey-file-acc-test-%d", time.Now().Unix()),
	}

	sshFile, err := amazon_acc.GenerateSSHPrivateKeyFile()
	if err != nil {
		t.Fatalf("failed to generate SSH key file: %s", err)
	}

	defer os.Remove(sshFile)

	testcase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_test_private_key_file",
		Template: buildPrivateKeyFileConfig(ami.Name, sshFile),
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
			}
			return nil
		},
	}

	acctest.TestPlugin(t, testcase)
}

func TestAccBuilder_PrivateKeyFileWithReboot(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-pkey-file-reboot-acc-test-%d", time.Now().Unix()),
	}

	sshFile, err := amazon_acc.GenerateSSHPrivateKeyFile()
	if err != nil {
		t.Fatalf("failed to generate SSH key file: %s", err)
	}

	defer os.Remove(sshFile)

	testcase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_test_private_key_file_reboot",
		Template: buildPrivateKeyFileRebootConfig(ami.Name, sshFile),
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
			}

			logs, err := os.ReadFile(logfile)
			if err != nil {
				return fmt.Errorf("couldn't read logs from logfile %s: %s", logfile, err)
			}
			if !strings.Contains(string(logs), "Uploading SSH public key") {
				return fmt.Errorf("SSH key was not uploaded, but should have been")
			}

			return nil
		},
	}

	acctest.TestPlugin(t, testcase)
}

//go:embed test-fixtures/unlimited-credits/burstable_instances.pkr.hcl
var testBurstableInstanceTypes string

func TestAccBuilder_EnableUnlimitedCredits(t *testing.T) {
	testcase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_unlimited_credits_test",
		Template: testBurstableInstanceTypes,
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testcase)
}

//go:embed test-fixtures/unlimited-credits/burstable_spot_instances.pkr.hcl
var testBurstableSpotInstanceTypes string

func TestAccBuilder_EnableUnlimitedCredits_withSpotInstances(t *testing.T) {
	testcase := &acctest.PluginTestCase{
		Name:     "amazon-ebs_unlimited_credits_spot_instance_test",
		Template: testBurstableSpotInstanceTypes,
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testcase)
}

func testEC2Conn(region string) (*ec2.EC2, error) {
	access := &common.AccessConfig{RawRegion: region}
	session, err := access.Session()
	if err != nil {
		return nil, err
	}

	return ec2.New(session), nil
}

func TestAccBuilder_EbsBasicWithIMDSv2(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-ebs-imds-acc-test-%d", time.Now().Unix()),
	}

	testcase := &acctest.PluginTestCase{
		Name:     "amazon-ebs-with-imdsv2",
		Template: fmt.Sprintf(testIMDSv2Support, ami.Name),
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
			}

			amis, err := ami.GetAmi()
			if err != nil {
				return fmt.Errorf("failed to get AMI: %s", err)
			}
			if len(amis) != 1 {
				return fmt.Errorf("got too many AMIs, expected 1, got %d", len(amis))
			}

			ami := amis[0]

			imds := ami.ImdsSupport
			if imds == nil {
				return fmt.Errorf("expected AMI's IMDSSupport to be set, but is null")
			}

			if *imds != "v2.0" {
				return fmt.Errorf("expected AMI's IMDSSupport to be v2.0, got %q", *imds)
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testcase)
}

func TestAccBuilder_EbsCopyRegionKeepTagsInAllAMI(t *testing.T) {
	tests := []struct {
		name     string
		amiName  string
		template string
	}{
		{
			name: "amazon-ebs_region_copy_keep_tags",
			amiName: fmt.Sprintf(
				"packer-test-builder-region-copy-keep-tags-%d",
				time.Now().Unix()),
			template: testAMIRunTagsCopyKeepTags,
		},
		{
			name: "amazon-ebs_region_copy_keep_run_tags",
			amiName: fmt.Sprintf(
				"packer-test-builder-region-copy-keep-run-tags-%d",
				time.Now().Unix()),
			template: testAMIRunTagsCopyKeepRunTags,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amis := []amazon_acc.AMIHelper{
				{
					Region: "us-east-1",
					Name:   tt.amiName,
				},
				{
					Region: "us-west-1",
					Name:   tt.amiName,
				},
			}

			expectedTags := map[string]string{
				"build_name": "build_name",
				"version":    "packer",
				"built_by":   "ebs",
				"simple":     "Simple String",
			}

			testCase := &acctest.PluginTestCase{
				Name:     tt.name,
				Template: fmt.Sprintf(tt.template, tt.amiName),
				Teardown: func() error {
					err := amis[0].CleanUpAmi()
					if err != nil {
						t.Logf("ami %s cleanup failed: %s", amis[0].Name, err)
					}
					err = amis[1].CleanUpAmi()
					if err != nil {
						t.Logf("ami %s cleanup failed: %s", amis[1].Name, err)
					}
					return nil
				},
				Check: func(buildCommand *exec.Cmd, logfile string) error {
					var result error

					if buildCommand.ProcessState != nil {
						if buildCommand.ProcessState.ExitCode() != 0 {
							return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
						}
					}

					err := checkRegionCopy(
						tt.amiName,
						[]string{"us-east-1", "us-west-1"})
					if err != nil {
						result = multierror.Append(result, err)
					}

					for _, ami := range amis {
						err := checkAMITags(ami, expectedTags)
						if err != nil {
							result = multierror.Append(result, err)
						}
					}

					return result
				},
			}

			acctest.TestPlugin(t, testCase)
		})
	}
}

func TestAccBuilder_EbsWindowsFastLaunch(t *testing.T) {
	fastlaunchami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-ebs-windows-fastlaunch-%d", time.Now().Unix()),
	}

	fastlaunchamiwithTemplate := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-ebs-windows-fastlaunch-with-template-%d", time.Now().Unix()),
	}

	tests := []struct {
		name     string
		ami      amazon_acc.AMIHelper
		template string
	}{
		{
			"basic fast-launch enable test",
			fastlaunchami,
			fmt.Sprintf(testWindowsFastBoot, fastlaunchami.Name),
		},
		{
			"basic fast-launch enable test with template",
			fastlaunchamiwithTemplate,
			fmt.Sprintf(testWindowsFastBootWithTemplateID, fastlaunchamiwithTemplate.Name),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testcase := &acctest.PluginTestCase{
				Name:     "amazon-ebs-windows-fastlaunch",
				Template: tt.template,
				Teardown: func() error {
					return tt.ami.CleanUpAmi()
				},
				Check: func(buildCommand *exec.Cmd, logfile string) error {
					if buildCommand.ProcessState.ExitCode() != 0 {
						return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
					}

					amis, err := tt.ami.GetAmi()
					if err != nil {
						return fmt.Errorf("failed to get AMI: %s", err)
					}
					if len(amis) != 1 {
						return fmt.Errorf("got too many AMIs, expected 1, got %d", len(amis))
					}

					accessConfig := &awscommon.AccessConfig{}
					session, err := accessConfig.Session()
					if err != nil {
						return fmt.Errorf("Unable to create aws session %s", err.Error())
					}

					regionconn := ec2.New(session.Copy(&aws.Config{
						Region: aws.String(tt.ami.Region),
					}))

					ami := amis[0]

					fastLaunchImages, err := regionconn.DescribeFastLaunchImages(&ec2.DescribeFastLaunchImagesInput{
						ImageIds: []*string{ami.ImageId},
					})

					if err != nil {
						return fmt.Errorf("failed to get fast-launch images: %s", err)
					}

					if len(fastLaunchImages.FastLaunchImages) != 1 {
						return fmt.Errorf("go too many fast-launch images, expected 1, got %d", len(fastLaunchImages.FastLaunchImages))
					}

					img := fastLaunchImages.FastLaunchImages[0]
					if img.State == nil {
						return fmt.Errorf("unexpected null fast-launch state")
					}

					if *img.State != "enabled" {
						return fmt.Errorf("expected fast-launch state to be enabled, but is %q", *img.State)
					}

					return nil
				},
			}
			acctest.TestPlugin(t, testcase)
		})
	}
}

func checkAMITags(ami amazon_acc.AMIHelper, tagList map[string]string) error {
	images, err := ami.GetAmi()
	if err != nil || len(images) == 0 {
		return fmt.Errorf("failed to find ami %s at region %s", ami.Name, ami.Region)
	}

	amiNameRegion := fmt.Sprintf("%s/%s", ami.Region, ami.Name)

	// describe the image, get block devices with a snapshot
	ec2conn, _ := testEC2Conn(ami.Region)
	imageResp, err := ec2conn.DescribeImages(&ec2.DescribeImagesInput{
		ImageIds: []*string{images[0].ImageId},
	})
	if err != nil {
		return fmt.Errorf("failed to describe AMI %q: %s", amiNameRegion, err)
	}

	var errs error
	image := imageResp.Images[0] // Only requested a single AMI ID
	for tagKey, tagVal := range tagList {
		found := false
		for _, imgTag := range image.Tags {
			if *imgTag.Key != tagKey {
				continue
			}
			found = true
			if *imgTag.Value != tagVal {
				errs = multierror.Append(errs, fmt.Errorf("wrong value for tag %q, expected %q, got %q",
					tagKey, tagVal, *imgTag.Value))
			}
			break
		}
		if !found {
			errs = multierror.Append(errs, fmt.Errorf("tag %q not found in image tags", tagKey))
		}
	}

	return errs
}

func TestAccBuilder_EBSWithSSHPassword_NoTempKeyCreated(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-ebs-ssh-password-auth-test-%d", time.Now().Unix()),
	}

	testcase := &acctest.PluginTestCase{
		Name:     "amazon-ebs-with-ssh-pass-auth",
		Template: fmt.Sprintf(testBuildWithSSHPassword, ami.Name),
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
			}

			logs, err := os.ReadFile(logfile)
			if err != nil {
				return fmt.Errorf("couldn't read logs from logfile %s: %s", logfile, err)
			}
			if strings.Contains(string(logs), "Creating temporary keypair") {
				return fmt.Errorf("ssh password specified, should not create temp keypair.")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testcase)
}

func TestAccBuilder_AssociatePublicIPWithoutSubnet(t *testing.T) {
	nonSpotInstance := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-ebs-explicit-public-ip-%d", time.Now().Unix()),
	}

	spotInstance := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-ebs-spot-explicit-public-ip-%d", time.Now().Unix()),
	}
	tests := []struct {
		name      string
		IPVal     bool
		amiSetup  amazon_acc.AMIHelper
		template  string
		expectErr bool
	}{
		{
			"Spot instance, with public IP explicitely set",
			true,
			spotInstance,
			testSetupPublicIPWithoutVPCOrSubnetOnSpotInstance,
			false,
		},
		{
			"Spot instance, with public IP explicitely unset",
			false,
			spotInstance,
			testSetupPublicIPWithoutVPCOrSubnetOnSpotInstance,
			true, // We expect an error without a public IP since no outbound connections work in this case, so SSM doesn't work with the current config
		},
		{
			"Non-Spot instance, with public IP explicitely set",
			true,
			nonSpotInstance,
			testSetupPublicIPWithoutVPCOrSubnet,
			false,
		},
		{
			"Non-Spot instance, with public IP explicitely unset",
			false,
			nonSpotInstance,
			testSetupPublicIPWithoutVPCOrSubnet,
			true, // We expect an error without a public IP since no outbound connections work in this case, so SSM doesn't work with the current config
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testcase := &acctest.PluginTestCase{
				Name:     tt.name,
				Template: fmt.Sprintf(tt.template, tt.amiSetup.Name, tt.IPVal),
				Check: func(buildCommand *exec.Cmd, logfile string) error {
					if (buildCommand.ProcessState.ExitCode() != 0) != tt.expectErr {
						return fmt.Errorf("Bad exit code, expected %t error, got %d. Logfile: %s",
							tt.expectErr,
							buildCommand.ProcessState.ExitCode(),
							logfile)
					}

					logs, err := os.ReadFile(logfile)
					if err != nil {
						return fmt.Errorf("couldn't read logs from logfile %s: %s", logfile, err)
					}

					expectMsg := fmt.Sprintf("changing public IP address config to %t for instance on subnet", tt.IPVal)

					if !strings.Contains(string(logs), expectMsg) {
						return fmt.Errorf("did not change the public IP setting for the instance")
					}

					if !strings.Contains(string(logs), "No VPC ID provided, Packer will use the default VPC") {
						return fmt.Errorf("did not pick the default VPC when setting subnet")
					}

					return nil
				},
			}
			acctest.TestPlugin(t, testcase)
		})
	}
}

func TestAccBuilder_AssociatePublicIPWithSubnetFilter(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-ebs-with-subnet-filter-%d", time.Now().Unix()),
	}

	testcase := &acctest.PluginTestCase{
		Name:     "ebs-subnet-filter-associate-ip-test",
		Template: fmt.Sprintf(testSubnetFilterWithPublicIP, ami.Name, true),
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code, got %d. Logfile: %s",
					buildCommand.ProcessState.ExitCode(),
					logfile)
			}

			logs, err := os.ReadFile(logfile)
			if err != nil {
				return fmt.Errorf("couldn't read logs from logfile %s: %s", logfile, err)
			}

			if ok := strings.Contains(string(logs), "changing public IP address config to true for instance on subnet"); !ok {
				return fmt.Errorf("did not change the public IP setting for the instance")
			}

			if ok := strings.Contains(string(logs), "Using Subnet Filters"); !ok {
				return fmt.Errorf("did not select subnet with filters")
			}

			if ok := strings.Contains(string(logs), "AvailabilityZone found"); !ok {
				return fmt.Errorf("did not get AZ from subnet")
			}

			if ok := strings.Contains(string(logs), "VpcId found"); !ok {
				return fmt.Errorf("did not get VPC ID from subnet")
			}

			if ok := strings.Contains(string(logs), "Inferring subnet from the selected"); ok {
				return fmt.Errorf("should not have selected a subnet for public IP address config")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testcase)
}

func TestAccBuilder_BasicSubnetFilter(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-ebs-basic-subnet-filter-%d", time.Now().Unix()),
	}

	testcase := &acctest.PluginTestCase{
		Name:     "ebs-subnet-filter-test",
		Template: fmt.Sprintf(testBasicSubnetFilter, ami.Name),
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code, got %d. Logfile: %s",
					buildCommand.ProcessState.ExitCode(),
					logfile)
			}

			logs, err := os.ReadFile(logfile)
			if err != nil {
				return fmt.Errorf("couldn't read logs from logfile %s: %s", logfile, err)
			}

			if ok := strings.Contains(string(logs), "Using Subnet Filters"); !ok {
				return fmt.Errorf("did not select subnet with filters")
			}

			if ok := strings.Contains(string(logs), "AvailabilityZone found"); !ok {
				return fmt.Errorf("did not get AZ from subnet")
			}

			if ok := strings.Contains(string(logs), "VpcId found"); !ok {
				return fmt.Errorf("did not get VPC ID from subnet")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testcase)
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

const testBuilderAccRegionCopyDeprecated = `
{
	"builders": [{
		"type": "amazon-ebs",
		"region": "us-east-1",
		"instance_type": "m3.medium",
		"source_ami":"ami-76b2a71e",
		"ssh_username": "ubuntu",
		"deprecate_at" : "%s",
		"ami_name": "%s",
		"ami_regions": ["us-east-1", "us-west-1"]
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

const testBuilderAccEncryptedDeprecated = `
{
	"builders": [{
		"type": "amazon-ebs",
		"region": "us-east-1",
		"instance_type": "m3.medium",
		"source_ami":"ami-c15bebaa",
		"ssh_username": "ubuntu",
		"deprecate_at" : "%s",
		"ami_name": "%s",
		"encrypt_boot": true
	}]
}
`

const testBuilderAccRegionCopyEncryptedAndDeprecated = `
{
	"builders": [{
		"type": "amazon-ebs",
		"region": "us-east-1",
		"instance_type": "m3.medium",
		"source_ami":"ami-76b2a71e",
		"ssh_username": "ubuntu",
		"deprecate_at" : "%s",
		"ami_name": "%s",
		"encrypt_boot": true,
		"ami_regions": ["us-east-1", "us-west-1"]
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

const testBuilderAccSSMWithReboot = `
source "amazon-ebs" "test" {
	ami_name             = "%s"
	source_ami           = "ami-00874d747dde814fa" # Ubuntu Server 22.04 LTS
	instance_type        = "m3.medium"
	region               = "us-east-1"
	ssh_username         = "ubuntu"
	ssh_interface        = "session_manager"
	iam_instance_profile = "SSMInstanceProfile"
	communicator         = "ssh"
}

build {
	sources = ["amazon-ebs.test"]

	provisioner "shell" {
		expect_disconnect = true
		inline = ["echo 'waiting for 1 minute'; sleep 60; echo 'rebooting VM'; sudo reboot now"]
	}

	provisioner "shell" {
		inline = ["echo 'reboot done!'"]
	}
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

const testPrivateKeyFile = `
source "amazon-ebs" "test" {
	ami_name             = "%s"
	source_ami           = "ami-0b5eea76982371e91" # Amazon Linux 2 AMI - kernel 5.10
	instance_type        = "m3.medium"
	region               = "us-east-1"
	ssh_username         = "ec2-user"
	ssh_interface        = "session_manager"
	iam_instance_profile = "SSMInstanceProfile"
	communicator         = "ssh"
	ssh_private_key_file = "%s"
}

build {
	sources = ["amazon-ebs.test"]
}
`

const testPrivateKeyFileWithReboot = `
source "amazon-ebs" "test" {
	ami_name             = "%s"
	source_ami           = "ami-00874d747dde814fa" # Ubuntu Server 22.04 LTS
	instance_type        = "m3.medium"
	region               = "us-east-1"
	ssh_username         = "ubuntu"
	ssh_interface        = "session_manager"
	iam_instance_profile = "SSMInstanceProfile"
	communicator         = "ssh"
	ssh_private_key_file = "%s"
}

build {
	sources = ["amazon-ebs.test"]

	provisioner "shell" {
		expect_disconnect = true
		inline = ["echo 'waiting for 1 minute'; sleep 60; echo 'rebooting VM'; sudo reboot now"]
	}

	provisioner "shell" {
		inline = ["echo 'reboot done!'"]
	}
}
`

const testIMDSv2Support = `
source "amazon-ebs" "test" {
	ami_name             = "%s"
	source_ami           = "ami-00874d747dde814fa" # Ubuntu Server 22.04 LTS
	instance_type        = "m3.medium"
	region               = "us-east-1"
	ssh_username         = "ubuntu"
	ssh_interface        = "session_manager"
	iam_instance_profile = "SSMInstanceProfile"
	communicator         = "ssh"
	imds_support         = "v2.0"
}

build {
	sources = ["amazon-ebs.test"]
}
`

const testAMIRunTagsCopyKeepRunTags = `
source "amazon-ebs" "test" {
	region        = "us-east-1"
	source_ami    = "ami-00874d747dde814fa" # Ubuntu Server 22.04 LTS
	instance_type = "m3.medium"
	ami_name      = "%s"
	communicator  = "ssh"
	ssh_username  = "ubuntu"
	ami_regions   = ["us-west-1"]

	run_tags = {
		"build_name"  = "build_name"
		"version"     = "packer"
		"built_by"    = "ebs"
		"simple"      = "Simple String"
	}
}

build {
	sources = [
		"source.amazon-ebs.test"
	]
}
`

const testAMIRunTagsCopyKeepTags = `
source "amazon-ebs" "test" {
	region        = "us-east-1"
	source_ami    = "ami-00874d747dde814fa" # Ubuntu Server 22.04 LTS
	instance_type = "m3.medium"
	ami_name      = "%s"
	communicator  = "ssh"
	ssh_username  = "ubuntu"
	ami_regions   = ["us-west-1"]

	tags = {
		"build_name"  = "build_name"
		"version"     = "packer"
		"built_by"    = "ebs"
		"simple"      = "Simple String"
	}
}

build {
	sources = [
		"source.amazon-ebs.test"
	]
}
`

const testBuildWithSSHPassword = `
source "amazon-ebs" "test" {
	region               = "us-east-1"
	source_ami           = "ami-089158c0576f477a7" # Ubuntu Server 22.04 LTS custom with ssh user/password auth setup
	instance_type        = "t3.micro"
	ami_name             = "%s"
	communicator         = "ssh"
	ssh_interface        = "session_manager"
	iam_instance_profile = "SSMInstanceProfile"
	ssh_username         = "user"
	ssh_password         = "password"
}

build {
	sources = ["amazon-ebs.test"]
}
`

const testSetupPublicIPWithoutVPCOrSubnet = `
source "amazon-ebs" "test_build" {
  region                      = "us-east-1"
  ami_name                    = "%s"
  source_ami                  = "ami-06e46074ae430fba6" # Amazon Linux 2023 x86-64
  instance_type               = "t2.micro"
  communicator                = "ssh"
  ssh_username                = "ec2-user"
  ssh_timeout                 = "45s"
  associate_public_ip_address = %t
  skip_create_ami             = true
}

build {
  sources = ["amazon-ebs.test_build"]
}
`

const testSetupPublicIPWithoutVPCOrSubnetOnSpotInstance = `
source "amazon-ebs" "test" {
  region                      = "us-east-1"
  spot_price                  = "auto"
  source_ami                  = "ami-06e46074ae430fba6" # Amazon Linux 2023 x86-64
  instance_type               = "t2.micro"
  ssh_username                = "ec2-user"
  ssh_timeout                 = "45s"
  ami_name                    = "%s"
  skip_create_ami             = true
  associate_public_ip_address = %t
  temporary_iam_instance_profile_policy_document {
    Version = "2012-10-17"
    Statement {
      Effect = "Allow"
      Action = [
        "ec2:GetDefaultCreditSpecification",
        "ec2:DescribeInstanceTypeOfferings",
        "ec2:DescribeInstanceCreditSpecifications"
      ]
      Resource = ["*"]
    }
  }
}

build {
  sources = ["source.amazon-ebs.test"]
}
`

const testWindowsFastBoot = `
source "amazon-ebs" "windows-fastboot" {
	ami_name             = "%s"
	source_ami           = "ami-00b2c40b15619f518" # Windows server 2016 base x86_64
	instance_type        = "m3.medium"
	region               = "us-east-1"
	communicator         = "winrm"
	winrm_username       = "Administrator"
	winrm_password       = "e4sypa55!"
	user_data_file       = "test-fixtures/ps_enable.ps"
	fast_launch {
		target_resource_count = 1
	}
}

build {
	sources = ["amazon-ebs.windows-fastboot"]

	provisioner "powershell" {
		inline = [
			"C:/ProgramData/Amazon/EC2-Windows/Launch/Scripts/InitializeInstance.ps1 -Schedule",
			"C:/ProgramData/Amazon/EC2-Windows/Launch/Scripts/SysprepInstance.ps1 -NoShutdown"
		]
	}
}
`

const testWindowsFastBootWithTemplateID = `
source "amazon-ebs" "windows-fastboot" {
	ami_name             = "%s"
	source_ami           = "ami-00b2c40b15619f518" # Windows server 2016 base x86_64
	instance_type        = "m3.medium"
	region               = "us-east-1"
	communicator         = "winrm"
	winrm_username       = "Administrator"
	winrm_password       = "e4sypa55!"
	user_data_file       = "test-fixtures/ps_enable.ps"
	fast_launch {
		target_resource_count   = 1
		template_id = "lt-0c82d8943c032fc0b"
	}
}

build {
	sources = ["amazon-ebs.windows-fastboot"]

	provisioner "powershell" {
		inline = [
			"C:/ProgramData/Amazon/EC2-Windows/Launch/Scripts/InitializeInstance.ps1 -Schedule",
			"C:/ProgramData/Amazon/EC2-Windows/Launch/Scripts/SysprepInstance.ps1 -NoShutdown"
		]
	}
}
`

const testSubnetFilterWithPublicIP = `
source "amazon-ebs" "test-subnet-filter" {
  subnet_filter {
	filters = {
	availability-zone = "us-east-1a"
}
  }
  region                      = "us-east-1"
  ami_name                    = "%s"
  source_ami                  = "ami-06e46074ae430fba6" # Amazon Linux 2023 x86-64
  instance_type               = "t2.micro"
  communicator                = "ssh"
  ssh_username                = "ec2-user"
  associate_public_ip_address = %t
  skip_create_ami             = true
}

build {
	sources = ["amazon-ebs.test-subnet-filter"]
}
`

const testBasicSubnetFilter = `
source "amazon-ebs" "test-subnet-filter" {
  subnet_filter {
	filters = {
	availability-zone = "us-east-1a"
}
  }
  region                      = "us-east-1"
  ami_name                    = "%s"
  source_ami                  = "ami-06e46074ae430fba6" # Amazon Linux 2023 x86-64
  instance_type               = "t2.micro"
  communicator                = "ssh"
  ssh_username                = "ec2-user"
  skip_create_ami             = true
}

build {
	sources = ["amazon-ebs.test-subnet-filter"]
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

func buildPrivateKeyFileConfig(name, keyPath string) string {
	return fmt.Sprintf(testPrivateKeyFile, name, keyPath)
}

func buildPrivateKeyFileRebootConfig(name, keyPath string) string {
	return fmt.Sprintf(testPrivateKeyFileWithReboot, name, keyPath)
}
