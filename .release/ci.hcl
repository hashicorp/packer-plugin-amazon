# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: BUSL-1.1

schema = "1"

project "packer-package-amazon" {
  team = "#proj-packer-releases"
  slack {
    notification_channel = "C032TD9KCMQ"
  }
  github {
    organization = "hashicorp"
    repository = "packer-plugin-amazon"
    release_branches = [
        "main",
        "release/**",
    ]
  }
}


event "merge" {
  // "entrypoint" to use if build is not run automatically
  // i.e. send "merge" complete signal to orchestrator to trigger build
}

event "build" {
  depends = ["merge"]
  action "build" {
    organization = "hashicorp"
    repository = "packer-plugin-amazon"
    workflow = "build"
  }
}

event "prepare" {
  depends = ["build"]

  action "prepare" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "prepare"
    depends      = ["build"]
  }

  notification {
    on = "fail"
  }
}

## These are promotion and post-publish events
## they should be added to the end of the file after the verify event stanza.

event "trigger-staging" {
// This event is dispatched by the bob trigger-promotion command
// and is required - do not delete.
}

event "promote-staging" {
  depends = ["trigger-staging"]
  action "promote-staging" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "promote-staging"
    config = "release-metadata.hcl"
  }

  notification {
    on = "always"
  }
}

event "trigger-production" {
// This event is dispatched by the bob trigger-promotion command
// and is required - do not delete.
}

event "promote-production" {
  depends = ["trigger-production"]
  action "promote-production" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "promote-production"
  }

  notification {
    on = "always"
  }
}

#TODO: Remove this if tags are automatically created
# event "post-publish-website" {
#   depends = ["promote-production"]
#   action "post-publish-website" {
#     organization = "hashicorp"
#     repository = "packer-plugin-amazon"
#     workflow = "notify-integration-release-via-manual.yaml"
#   }
#
#   notification {
#     on = "always"
#   }
# }