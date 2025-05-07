# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "amazon-ami" "test" {
  filters = {
    name                = "ubuntu/images/*ubuntu-jammy-22.04-amd64-server-*"
    root-device-type    = "ebs"
    virtualization-type = "hvm"
  }
  most_recent = true
  owners      = ["099720109477"]
  region      = "us-west-2"
}

variable "builder" {
  type    = string
  default = "Packer"
}

source "amazon-ebs" "basic-example" {
  region        = "us-west-2"
  source_ami    = data.amazon-ami.test.id
  instance_type = "t2.micro"
  ami_name      = "%s"
  communicator  = "ssh"
  ssh_username  = "ubuntu"
  skip_ami_run_tags = true

  run_tags = {
    "build_name" = "{{build_name}}"
    "source_name" = source.name
    "version"    = packer.version
    "built_by"   = var.builder
    "simple"     = "Simple String"
  }
}

build {
  sources = [
    "source.amazon-ebs.basic-example"
  ]
}
