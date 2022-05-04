data "amazon-ami" "test" {
  filters = {
    name                = "ubuntu/images/*ubuntu-xenial-16.04-amd64-server-*"
    root-device-type    = "ebs"
    virtualization-type = "hvm"
  }
  most_recent = true
  owners      = ["099720109477"]
  region      = "us-east-1"
}

source "amazon-ebs" "basic-example" {
  region           = "us-east-1"
  source_ami       = data.amazon-ami.test.id
  instance_type    = "t2.micro"
  ami_name         = "packer_rsa_ssh_keypair_acctest"
  communicator     = "ssh"
  ssh_username     = "ubuntu"
  skip_create_ami  = true
}

build {
  sources = [
    "source.amazon-ebs.basic-example"
  ]

  provisioner "shell" {
    inline = ["echo 'Hello from the other side'", "cat ~/.ssh/authorized_keys"]
  }
}
