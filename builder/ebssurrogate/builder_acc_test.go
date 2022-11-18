package ebssurrogate

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	amazon_acc "github.com/hashicorp/packer-plugin-amazon/builder/ebs/acceptance"
	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

func TestAccBuilder_EbssurrogateBasic(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   fmt.Sprintf("packer-plugin-amazon-ebs-basic-acc-test %d", time.Now().Unix()),
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
	aminame := fmt.Sprintf("packer-plugin-amazon-ebs-basic-acc-test %d", time.Now().Unix())
	ami := amazon_acc.AMIHelper{
		Region: "us-east-1",
		Name:   aminame,
	}
	testCase := &acctest.PluginTestCase{
		Name:     "amazon-ebssurrogate_basic_test_imdsv2",
		Template: fmt.Sprintf(testBuilderAccBasicIMDSv2, aminame),
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}

			ami := amazon_acc.AMIHelper{
				Region: "us-east-1",
				Name:   aminame,
			}
			images, err := ami.GetAmi()
			if err != nil {
				return fmt.Errorf("failed to get AMI %q: %s", aminame, err)
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
