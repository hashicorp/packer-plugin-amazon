# For full specification on the configuration of this file visit:
# https://github.com/hashicorp/integration-template#metadata-configuration
integration {
  name = "Amazon"
  description = "The Amazon multi-component plugin can be used with HashiCorp Packer to create custom images."
  identifier = "packer/BrandonRomano/amazon"
  component {
    type = "data-source"
    name = "Parameter Store"
    slug = "parameterstore"
  }
  component {
    type = "data-source"
    name = "Secrets Manager"
    slug = "secretsmanager"
  }
  component {
    type = "data-source"
    name = "Amazon AMI"
    slug = "ami"
  }
  component {
    type = "builder"
    name = "Amazon chroot"
    slug = "chroot"
  }
  component {
    type = "builder"
    name = "Amazon EBS"
    slug = "ebs"
  }
  component {
    type = "builder"
    name = "Amazon EBS Surrogate"
    slug = "ebssurrogate"
  }
  component {
    type = "builder"
    name = "Amazon instance-store"
    slug = "instance"
  }
  component {
    type = "builder"
    name = "Amazon EBS Volume"
    slug = "ebsvolume"
  }
  component {
    type = "post-processor"
    name = "Amazon Import"
    slug = "import"
  }
}
