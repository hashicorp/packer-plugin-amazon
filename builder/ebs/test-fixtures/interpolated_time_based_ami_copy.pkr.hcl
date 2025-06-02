# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

packer {
  required_plugins {
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = "~> 1"
    }
  }
}

variable "ami_name" {
  type = string
}

source "amazon-ebs" "time_based_copy" {
  ami_name      = var.ami_name
  ami_regions   = ["us-east-1", "us-west-2"]
  instance_type = "m3.medium"
  region        = "us-east-1"
  source_ami    = "ami-76b2a71e"
  ssh_username  = "ubuntu"
  snapshot_copy_duration_minutes = 15
}

build {
  sources = ["source.amazon-ebs.time_based_copy"]

}
