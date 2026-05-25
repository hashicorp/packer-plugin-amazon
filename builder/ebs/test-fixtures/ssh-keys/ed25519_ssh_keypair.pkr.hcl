# Copyright IBM Corp. 2013, 2025
# SPDX-License-Identifier: MPL-2.0

data "amazon-ami" "test" {
  filters = {
    name                = "ubuntu/images/*ubuntu-jammy-22.04-amd64-server-*"
    root-device-type    = "ebs"
    virtualization-type = "hvm"
  }
  most_recent = true
  owners      = ["099720109477"]
  region      = "us-east-1"
}

source "amazon-ebs" "basic-example" {
  region                  = "us-east-1"
  source_ami              = data.amazon-ami.test.id
  instance_type           = "t2.micro"
  ami_name                = "packer_ed25519_ssh_keypair_acctest"
  communicator            = "ssh"
  ssh_username            = "ubuntu"
  temporary_key_pair_type = "ed25519"
  skip_create_ami         = true
}

build {
  sources = [
    "source.amazon-ebs.basic-example"
  ]

  provisioner "shell" {
    inline = ["echo 'Hello from the other side'", "cat ~/.ssh/authorized_keys"]
  }
}
