variables {
  standardCPUCredit  = "standard"
  unlimitedCPUCredit = "unlimited"
}

variable "region" {
 type    = string
 default = "us-west-2"
}

source "amazon-ebs" "standard_spot" {
  region                  = var.region
  spot_price              = "auto"
  source_ami              = "ami-044065b5480679567"
  instance_type           = "t3a.nano"
  ssh_username            = "ec2-user"
  ami_name                = "packer_AWS_{{timestamp}}"
  skip_create_ami         = true
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

source "amazon-ebs" "unlimited_spot" {
  region                   = var.region
  spot_price               = "auto"
  source_ami               = "ami-044065b5480679567"
  instance_type            = "t3a.nano"
  ssh_username             = "ec2-user"
  enable_unlimited_credits = true
  ami_name                 = "packer_AWS_{{timestamp}}"
  skip_create_ami          = true
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
    "source.amazon-ebs.standard_spot",
    "source.amazon-ebs.unlimited_spot"
  ]
  provisioner "shell" {
    inline = ["sudo dnf install -q -y jq"]
  }

  provisioner "shell" {
    only = ["amazon-ebs.standard_spot"]
    inline = [
      "aws configure set region ${var.region} --profile default",
      "CREDITTYPE=$(aws ec2 describe-instance-credit-specifications --instance-ids ${build.ID}| jq --raw-output \".InstanceCreditSpecifications|.[]|.CpuCredits\")",
      "echo CPU Credit Specification is $CREDITTYPE",
      "[[ $CREDITTYPE == ${var.standardCPUCredit} ]]"
    ]
  }
  provisioner "shell" {
    only = ["amazon-ebs.unlimited_spot"]
    inline = [
      "aws configure set region ${var.region} --profile default",
      "CREDITTYPE=$(AWS_DEFAULT_REGION=us-west-2 aws ec2 describe-instance-credit-specifications --instance-ids ${build.ID} | jq --raw-output \".InstanceCreditSpecifications|.[]|.CpuCredits\")",
      "echo CPU Credit Specification is $CREDITTYPE",
      "[[ $CREDITTYPE == ${var.unlimitedCPUCredit} ]]"
    ]
  }
}
