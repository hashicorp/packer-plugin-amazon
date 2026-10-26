# Copyright IBM Corp. 2013, 2026
# SPDX-License-Identifier: MPL-2.0

variables {
  standardCPUCredit  = "standard"
  unlimitedCPUCredit = "unlimited"
}

variable "region" {
 type    = string
 default = "us-west-2"
}

source "amazon-ebs" "standard" {
  region                  = var.region
  instance_type           = "t3.small"
  ssh_username            = "ubuntu"
  ssh_agent_auth          = false
  temporary_key_pair_type = "ed25519"
  ami_name                = "packer_AWS_{{timestamp}}"
  skip_create_ami         = true
  source_ami_filter {
    filters = {
      name         = "hc-security-base-ubuntu-2204-*"
      architecture = "x86_64"
    }
    owners      = ["888995627335"] # hc-ami_prod
    most_recent = true
  }
  temporary_iam_instance_profile_policy_document {
    Version = "2012-10-17"
    Statement {
      Effect = "Allow"
      Action = [
        "ec2:GetDefaultCreditSpecification",
        "ec2:DescribeInstanceTypeOfferings",
        "ec2:DescribeInstanceCreditSpecifications"
      ]
      Resource = ["*"]
    }
  }
}


source "amazon-ebs" "unlimited" {
  region                   = var.region
  instance_type            = "t2.small"
  ssh_username             = "ubuntu"
  ssh_agent_auth           = false
  enable_unlimited_credits = true
  temporary_key_pair_type  = "ed25519"
  ami_name                 = "packer_AWS_{{timestamp}}"
  skip_create_ami          = true
  source_ami_filter {
    filters = {
      name         = "hc-security-base-ubuntu-2204-*"
      architecture = "x86_64"
    }
    owners      = ["888995627335"] # hc-ami_prod
    most_recent = true
  }
  temporary_iam_instance_profile_policy_document {
    Version = "2012-10-17"
    Statement {
      Effect = "Allow"
      Action = [
        "ec2:GetDefaultCreditSpecification",
        "ec2:DescribeInstanceTypeOfferings",
        "ec2:DescribeInstanceCreditSpecifications"
      ]
      Resource = ["*"]
    }
  }
}

build {
  sources = [
    "source.amazon-ebs.standard",
    "source.amazon-ebs.unlimited"
  ]
  provisioner "shell" {
    inline = [
      "sudo cloud-init status --wait || true",
      "sudo apt-get -o DPkg::Lock::Timeout=300 update -q",
      "sudo DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=300 install -q -y jq awscli"
    ]
  }

  provisioner "shell" {
    only = ["amazon-ebs.standard"]
    inline = [
      "aws configure set region ${var.region} --profile default",
      "CREDITTYPE=$(aws ec2 describe-instance-credit-specifications --instance-ids ${build.ID}| jq --raw-output \".InstanceCreditSpecifications|.[]|.CpuCredits\")",
      "echo CPU Credit Specification is $CREDITTYPE",
      "[ \"$CREDITTYPE\" = \"${var.standardCPUCredit}\" ]"
    ]
  }
  provisioner "shell" {
    only = ["amazon-ebs.unlimited"]
    inline = [
      "aws configure set region ${var.region} --profile default",
      "CREDITTYPE=$(AWS_DEFAULT_REGION=us-west-2 aws ec2 describe-instance-credit-specifications --instance-ids ${build.ID} | jq --raw-output \".InstanceCreditSpecifications|.[]|.CpuCredits\")",
      "echo CPU Credit Specification is $CREDITTYPE",
      "[ \"$CREDITTYPE\" = \"${var.unlimitedCPUCredit}\" ]"
    ]
  }
}
