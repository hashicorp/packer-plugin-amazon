data "amazon-ami" "test" {
  filters = {
    virtualization-type = "hvm"
    name                = "Windows_Server-2016-English-Full-Base-*"
    root-device-type    = "ebs"
  }
  most_recent = true
  owners = ["801119661308"]
}

source "amazon-ebs" "basic-example" {
  user_data_file = "./test-fixtures/configure-source-ssh.ps1"
  region = "us-west-2"
  source_ami = data.amazon-ami.test.id
  instance_type =  "t2.small"
  ssh_agent_auth = false
  ami_name =  "packer-amazon-ami-test"
  communicator = "ssh"
  ssh_timeout = "10m"
  ssh_username = "Administrator"
}

build {
  sources = [
    "source.amazon-ebs.basic-example"
  ]
}
