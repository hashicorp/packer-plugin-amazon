# Copyright IBM Corp. 2013, 2025
# SPDX-License-Identifier: MPL-2.0

data "amazon-secretsmanager" "test" {
  name = "packer_datasource_secretsmanager_test_secret"
  key  = "packer_test_key"
}

locals {
  value         = data.amazon-secretsmanager.test.value
  secret_string = data.amazon-secretsmanager.test.secret_string
  version_id    = data.amazon-secretsmanager.test.version_id
  secret_value  = jsondecode(data.amazon-secretsmanager.test.secret_string)["packer_test_key"]
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
      "echo secret value: ${local.value}",
      "echo secret secret_string: ${local.secret_string}",
      "echo secret version_id: ${local.version_id}",
      "echo secret value: ${local.secret_value}"
    ]
  }
}
