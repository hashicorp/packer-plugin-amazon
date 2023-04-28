// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ebssurrogate

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	amazon_acc "github.com/hashicorp/packer-plugin-amazon/builder/ebs/acceptance"
	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

func TestAccBuilder_EbssurrogateBasic(t *testing.T) {
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
