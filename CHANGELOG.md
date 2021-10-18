## 1.2.0 (Upcoming)


### IMPROVEMENTS
* Add `SourceAMI` to HCP Packer registry image metadata. [GH-136]
* Add tag specification to supported resources to enable security tagging.
    [GH-96]
* Bump packer-plugin-sdk to v0.2.7 [GH-143]

### BUG FIXES:
* builder/ebs: Fix deprecate_at when copying to additional regions. [GH-138]
* Fix `InvalidTagKey.Malformed` tag error for spot instance builds. [GH-92]


## 1.0.1 (September 13, 2021)

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
