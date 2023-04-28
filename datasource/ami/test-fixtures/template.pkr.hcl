# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "amazon-ami" "test" {
  filters = {
    name                = "ubuntu/images/*ubuntu-xenial-16.04-amd64-server-*"
    root-device-type    = "ebs"
    virtualization-type = "hvm"
  }
  most_recent = true
  owners      = ["099720109477"]
  region      = "us-west-2"
}

source "amazon-ebs" "basic-example" {
  region        = "us-west-2"
  source_ami    = data.amazon-ami.test.id
  instance_type = "t2.micro"
  ami_name      = "%s"
  communicator  = "ssh"
  ssh_username  = "ubuntu"
}

build {
  sources = [
    "source.amazon-ebs.basic-example"
  ]
}
