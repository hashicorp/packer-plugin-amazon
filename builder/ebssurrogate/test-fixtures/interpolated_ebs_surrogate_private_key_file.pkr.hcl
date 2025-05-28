# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

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