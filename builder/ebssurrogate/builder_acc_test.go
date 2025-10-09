// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebssurrogate

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/packer-plugin-amazon/common"
	amazon_acc "github.com/hashicorp/packer-plugin-amazon/common/acceptance"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

func testEC2Conn(region string) (clients.Ec2Client, error) {
	ctx := context.TODO()
	access := &common.AccessConfig{RawRegion: region}
	awsConfig, err := access.Config(ctx)
	if err != nil {
		return nil, err
	}
	ec2Client := ec2.NewFromConfig(*awsConfig)
	return ec2Client, nil
}

func checkAMITags(ami amazon_acc.AMIHelper, tagList map[string]string) error {
	images, err := ami.GetAmi()
	if err != nil || len(images) == 0 {
		return fmt.Errorf("failed to find ami %s at region %s", ami.Name, ami.Region)
	}
	ctx := context.TODO()
	amiNameRegion := fmt.Sprintf("%s/%s", ami.Region, ami.Name)

	// describe the image, get block devices with a snapshot
	ec2Client, _ := testEC2Conn(ami.Region)
	imageResp, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds: []string{*images[0].ImageId},
	})
	if err != nil {
		return fmt.Errorf("failed to describe AMI %q: %s", amiNameRegion, err)
	}

	var errs error
	image := imageResp.Images[0] // Only requested a single AMI ID
	if len(tagList) == 0 {
		if len(image.Tags) != 0 {
			return fmt.Errorf("expected no tags for AMI %q, got %d", amiNameRegion, len(image.Tags))
		}
	}
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

//go:embed test-fixtures/interpolated_ebs_surrogate_basic.pkr.hcl
var testBuilderAccBasic string

func TestAccBuilder_EbssurrogateBasic(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "ebssurrogate-basic-acc-test",
	}
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebssurrogate_basic_test",
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

//go:embed test-fixtures/interpolated_skip_run_tags.pkr.hcl
var testBuilderAccBasicSkipAmiRunTags string

func TestAccBuilder_EbssurrogateBasicSkipAmiRunTags(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "ebssurrogate-basic-skip-ami-run-tags-acc-test",
	}
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebssurrogate_basic_skip_ami_run_tags_test",
		Template: fmt.Sprintf(testBuilderAccBasicSkipAmiRunTags, ami.Name),
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			var result error
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}

			expectedTags := map[string]string{}
			err := checkAMITags(ami, expectedTags)
			if err != nil {
				result = multierror.Append(result, err)
			}
			return result
		},
	}
	acctest.TestPlugin(t, testCase)
}

//go:embed test-fixtures/interpolated_ebs_surrogate_basic_imdsv2.pkr.hcl
var testBuilderAccBasicIMDSv2 string

func TestAccBuilder_EbssurrogateBasic_forceIMDSv2(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "ebssurrogate-basic-acc-test-imdsv2",
	}
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebssurrogate_basic_test_imdsv2",
		Template: fmt.Sprintf(testBuilderAccBasicIMDSv2, ami.Name),
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}

			images, err := ami.GetAmi()
			if err != nil {
				return fmt.Errorf("failed to get AMI %q: %s", ami.Name, err)
			}

			if len(images) != 1 {
				return fmt.Errorf("expected 1 image, got %d", len(images))
			}

			img := images[0]

			if img.ImdsSupport != ec2types.ImdsSupportValuesV20 {
				return fmt.Errorf("expected AMI to have IMDSv2 support, got %s", img.ImdsSupport)
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

//go:embed test-fixtures/interpolated_ebs_surrogate_private_key_file.pkr.hcl
var testPrivateKeyFile string

func TestAccBuilder_Ebssurrogate_SSHPrivateKeyFile_SSM(t *testing.T) {
	t.Parallel()
	if os.Getenv(acctest.TestEnvVar) == "" {
		t.Skipf("Acceptance tests skipped unless env '%s' set",
			acctest.TestEnvVar)
		return
	}

	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-ebssurrogate-pkey-file-acc-test-%d", time.Now().Unix()),
	}

	sshFile, err := amazon_acc.GenerateSSHPrivateKeyFile()
	if err != nil {
		t.Fatalf("failed to generate SSH key file: %s", err)
	}

	defer os.Remove(sshFile)

	testcase := &acctest.PluginTestCase{
		Name:     "amazon-ebssurrogate_test_private_key_file",
		Template: fmt.Sprintf(testPrivateKeyFile, ami.Name, sshFile),
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
			}
			return nil
		},
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
	}

	acctest.TestPlugin(t, testcase)
}

//go:embed test-fixtures/interpolated_ebs_surrogate_create_image.pkr.hcl
var testBuilderAccUseCreateImageTrue string

func TestAccBuilder_EbssurrogateUseCreateImageTrue(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "ebssurrogate-image-method-create-acc-test",
	}
	volumeHelper := amazon_acc.VolumeHelper{
		Region: "us-east-1",
		Tags: []map[string]string{
			{"volume_tag": "block_device_tag"},
		},
	}
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebssurrogate_image_method_create_test",
		Template: fmt.Sprintf(testBuilderAccUseCreateImageTrue, ami.Name),
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}

			// Volume tags were applied to the EBS volumes during creation.
			// This check verifies that all such volumes have been deleted,
			// as the 'DeleteOnTermination' flag is set to true in the test template.

			volumes, err := volumeHelper.GetVolumes()
			if err != nil {
				return fmt.Errorf("failed to get volumes: %s", err)
			}
			if len(volumes) != 0 {
				return fmt.Errorf("expected 0 volume, got %d", len(volumes))
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

//go:embed test-fixtures/interpolated_ebs_surrogate_create_image_false.pkr.hcl
var testBuilderAccUseCreateImageFalse string

func TestAccBuilder_EbssurrogateUseCreateImageFalse(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "ebssurrogate-image-method-register-acc-test",
	}
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebssurrogate_image_method_register_test",
		Template: fmt.Sprintf(testBuilderAccUseCreateImageFalse, ami.Name),
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

//go:embed test-fixtures/interpolated_ebs_surrogate_create_image_optional.pkr.hcl
var testBuilderAccUseCreateImageOptional string

func TestAccBuilder_EbssurrogateUseCreateImageOptional(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "ebssurrogate-image-method-empty-acc-test",
	}
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebssurrogate_image_method_empty_test",
		Template: fmt.Sprintf(testBuilderAccUseCreateImageOptional, ami.Name),
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

//go:embed test-fixtures/interpolated_ebs_surrogate_with_deprecate_at.pkr.hcl
var testBuilderAcc_WithDeprecateAt string

func TestAccBuilder_EbssurrogateWithAMIDeprecate(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("ebssurrogate-deprecate-at-acctest-%d", time.Now().Unix()),
	}
	ctx := context.TODO()
	testCase := &acctest.PluginTestCase{
		Name:     "ebssurrogate - deprecate at set",
		Template: fmt.Sprintf(testBuilderAcc_WithDeprecateAt, ami.Name, time.Now().Add(time.Hour).UTC().Format("2006-01-02T15:04:05Z")),
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}

				conn, err := testEC2Conn("us-east-1")
				if err != nil {
					return fmt.Errorf("failed to get connection to us-east-1: %s", err)
				}

				out, err := conn.DescribeImages(ctx, &ec2.DescribeImagesInput{
					Filters: []ec2types.Filter{{
						Name:   aws.String("name"),
						Values: []string{ami.Name},
					}},
				})
				if err != nil {
					return fmt.Errorf("unable to describe images: %s", err)
				}

				if len(out.Images) != 1 {
					return fmt.Errorf("got %d images, should have been one", len(out.Images))
				}

				img := out.Images[0]
				if img.DeprecationTime == nil {
					return fmt.Errorf("no depreciation time set for image %s", ami.Name)
				}

				if *img.DeprecationTime == "" {
					return fmt.Errorf("no depreciation time set for image %s", ami.Name)
				}

				return nil
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}
