# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "builder" {
  type    = string
  default = "Packer"
}

source "amazon-ebs" "basic-example" {
  region            = "us-east-1"
  source_ami        = "ami-76b2a71e"
  instance_type     = "t2.micro"
  ami_name          = "%s"
  communicator      = "ssh"
  ssh_username      = "ubuntu"
  skip_ami_run_tags = true

  run_tags = {
    "build_name"  = "{{build_name}}"
    "source_name" = source.name
    "version"     = packer.version
    "built_by"    = var.builder
    "simple"      = "Simple String"
  }
  tags = {
    "ami_tag" = "yes"
  }
}

build {
  sources = [
    "source.amazon-ebs.basic-example"
  ]
}
