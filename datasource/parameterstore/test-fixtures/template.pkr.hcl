data "amazon-parameterstore" "test" {
  name = "packer_datasource_parameterstore_test_parameter"
  with_decryption = false
}

locals {
  value   = data.amazon-parameterstore.test.value
  version = data.amazon-parameterstore.test.version
  arn     = data.amazon-parameterstore.test.arn
}

source "null" "basic-example" {
  communicator = "none"
}

build {
  sources = [
    "source.null.basic-example"
  ]

  provisioner "shell-local" {
    inline = [
      "echo parameter value: ${local.value}",
      "echo parameter version: ${local.version}",
      "echo parameter arn: ${local.arn}"
    ]
  }
}

