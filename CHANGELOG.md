## 1.0.4 (Upcoming)

### BUG FIXES:
* Fix invalid KMS key error for multi-region keys. [GH-147]

## 1.0.3 (October 19, 2021)

### BUG FIXES:
* Fix panic in GetCredentials helper [GH-145]


## 1.0.2 (October 18, 2021)

### NOTES:
Support for the HCP Packer registry is currently in beta and requires
Packer v1.7.7 [GH-136]

### IMPROVEMENTS
* Add `SourceAMI` to HCP Packer registry image metadata. [GH-136]
* Add tag specification to supported resources to enable security tagging.
    [GH-96]
* Bump packer-plugin-sdk to v0.2.7 [GH-143]

### BUG FIXES:
* builder/ebs: Fix deprecate_at when copying to additional regions. [GH-138]
* Fix `InvalidTagKey.Malformed` tag error for spot instance builds. [GH-92]


## 1.0.1 (September 13, 2021)

### NOTES:
HCP Packer private beta support requires Packer version 1.7.5 or 1.7.6 [GH-129]

### FEATURES:
* Add HCP Packer registry image metadata to builder artifacts. [GH-129]
* Bump Packer plugin SDK to version v0.2.5 [GH-129]

## 1.0.0 (June 14, 2021)

* Bump github.com/hashicorp/packer-plugin-sdk from 0.1.0 to v0.2.3 [GH-89]
* Add packer and packer-plugin-amazon version to the User-Agent header [GH-82]
* Add support for fleet_tags / fleet_tag [GH-83]
* Bump github.com/aws/aws-sdk-go from 1.38.24 to 1.38.25 [GH-72]

## 0.0.1 (March 23, 2021)

* Amazon Plugin break out from Packer core. Changes prior to break out can be found in [Packer's CHANGELOG](https://github.com/hashicorp/packer/blob/master/CHANGELOG.md)
