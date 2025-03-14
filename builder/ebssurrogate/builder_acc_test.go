// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebssurrogate

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/packer-plugin-amazon/builder/common"
	amazon_acc "github.com/hashicorp/packer-plugin-amazon/builder/ebs/acceptance"
	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

func testEC2Conn(region string) (*ec2.EC2, error) {
	access := &common.AccessConfig{RawRegion: region}
	session, err := access.Session()
	if err != nil {
		return nil, err
	}

	return ec2.New(session), nil
}

func TestAccBuilder_EbssurrogateBasic(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "ebs-basic-acc-test",
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

func TestAccBuilder_EbssurrogateBasic_forceIMDSv2(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "ebs-basic-acc-test-imdsv2",
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

			if img.ImdsSupport != nil && *img.ImdsSupport != "v2.0" {
				return fmt.Errorf("expected AMI to have IMDSv2 support, got %q", *img.ImdsSupport)
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

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
	}

	acctest.TestPlugin(t, testcase)
}

func TestAccBuilder_EbssurrogateUseCreateImageTrue(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "ebs-image-method-create-acc-test",
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
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

func TestAccBuilder_EbssurrogateUseCreateImageFalse(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "ebs-image-method-register-acc-test",
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

func TestAccBuilder_EbssurrogateUseCreateImageOptional(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   "ebs-image-method-empty-acc-test",
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

func TestAccBuilder_EbssurrogateWithAMIDeprecate(t *testing.T) {
	t.Parallel()
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("ebssurrogate-deprecate-at-acctest-%d", time.Now().Unix()),
	}
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

				out, err := conn.DescribeImages(&ec2.DescribeImagesInput{
					Filters: []*ec2.Filter{{
						Name:   aws.String("name"),
						Values: []*string{&ami.Name},
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

const testBuilderAccBasic = `
source "amazon-ebssurrogate" "test" {
	ami_name = "%s"
	region = "us-east-1"
	instance_type = "m3.medium"
	source_ami = "ami-76b2a71e"
	ssh_username = "ubuntu"
	launch_block_device_mappings {
		device_name = "/dev/xvda"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
	ami_virtualization_type = "hvm"
	ami_root_device {
		source_device_name = "/dev/xvda"
		device_name = "/dev/sda1"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
}

build {
	sources = ["amazon-ebssurrogate.test"]
}
`

const testBuilderAccBasicIMDSv2 = `
source "amazon-ebssurrogate" "test" {
	ami_name = "%s"
	region = "us-east-1"
	instance_type = "m3.medium"
	source_ami = "ami-76b2a71e"
	ssh_username = "ubuntu"
	launch_block_device_mappings {
		device_name = "/dev/xvda"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
	ami_virtualization_type = "hvm"
	ami_root_device {
		source_device_name = "/dev/xvda"
		device_name = "/dev/sda1"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
	imds_support = "v2.0"
}

build {
	sources = ["amazon-ebssurrogate.test"]
}
`

const testPrivateKeyFile = `
source "amazon-ebssurrogate" "test" {
	ami_name             = "%s"
	source_ami           = "ami-0b5eea76982371e91" # Amazon Linux 2 AMI - kernel 5.10
	instance_type        = "m3.medium"
	region               = "us-east-1"
	ssh_username         = "ec2-user"
	ssh_interface        = "session_manager"
	iam_instance_profile = "SSMInstanceProfile"
	communicator         = "ssh"
	ssh_private_key_file = "%s"
	launch_block_device_mappings {
		device_name = "/dev/xvda"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
	ami_virtualization_type = "hvm"
	ami_root_device {
		source_device_name = "/dev/xvda"
		device_name = "/dev/sda1"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
}

build {
	sources = ["amazon-ebssurrogate.test"]
}
`

const testBuilderAccUseCreateImageTrue = `
source "amazon-ebssurrogate" "test" {
	ami_name = "%s"
	region = "us-east-1"
	instance_type = "m3.medium"
	source_ami = "ami-76b2a71e"
	ssh_username = "ubuntu"
	use_create_image = true
	launch_block_device_mappings {
		device_name = "/dev/xvda"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
	ami_virtualization_type = "hvm"
	ami_root_device {
		source_device_name = "/dev/xvda"
		device_name = "/dev/sda1"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
}

build {
	sources = ["amazon-ebssurrogate.test"]
}
`

const testBuilderAccUseCreateImageFalse = `
source "amazon-ebssurrogate" "test" {
	ami_name = "%s"
	region = "us-east-1"
	instance_type = "m3.medium"
	source_ami = "ami-76b2a71e"
	ssh_username = "ubuntu"
	use_create_image = false
	launch_block_device_mappings {
		device_name = "/dev/xvda"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
	ami_virtualization_type = "hvm"
	ami_root_device {
		source_device_name = "/dev/xvda"
		device_name = "/dev/sda1"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
}

build {
	sources = ["amazon-ebssurrogate.test"]
}
`

const testBuilderAccUseCreateImageOptional = `
source "amazon-ebssurrogate" "test" {
	ami_name = "%s"
	region = "us-east-1"
	instance_type = "m3.medium"
	source_ami = "ami-76b2a71e"
	ssh_username = "ubuntu"
	launch_block_device_mappings {
		device_name = "/dev/xvda"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
	ami_virtualization_type = "hvm"
	ami_root_device {
		source_device_name = "/dev/xvda"
		device_name = "/dev/sda1"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
}

build {
	sources = ["amazon-ebssurrogate.test"]
}
`

const testBuilderAcc_WithDeprecateAt = `
source "amazon-ebssurrogate" "test" {
	ami_name = "%s"
	region = "us-east-1"
	instance_type = "m3.medium"
	source_ami = "ami-76b2a71e"
	ssh_username = "ubuntu"
	launch_block_device_mappings {
		device_name = "/dev/xvda"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
	ami_virtualization_type = "hvm"
	ami_root_device {
		source_device_name = "/dev/xvda"
		device_name = "/dev/sda1"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
	deprecate_at = "%s"
}

build {
	sources = ["amazon-ebssurrogate.test"]
}
`
