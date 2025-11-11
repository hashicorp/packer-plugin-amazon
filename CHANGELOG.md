# Latest Release

Please refer to [releases](https://github.com/hashicorp/packer-plugin-amazon/releases) for the latest CHANGELOG information.

---
## 1.7.0 (November 11, 2025)

## ✨ Features
- **Upgrade Builder Ebs Surrogate to AWS SDK v2** – by @kp2099 ([#606](https://github.com/hashicorp/packer-plugin-amazon/pull/606))  
  Modernizes the Ebs Surrogate Builder by migrating to AWS SDK v2.


---
## 1.6.0 (October 21, 2025)

## ✨ Features
- **Upgrade Builder Ebs Volume to AWS SDK v2** – by @kp2099 ([#578](https://github.com/hashicorp/packer-plugin-amazon/pull/578))  
  Modernizes the Ebs Volume Builder by migrating to AWS SDK v2.

---

## 📦 Dependencies
- **Bump `github.com/hashicorp/packer-plugin-sdk`** from **0.6.3 → 0.6.4** – by @dependabot ([#622](https://github.com/hashicorp/packer-plugin-amazon/pull/622))

---
## 1.5.0 (September 22, 2025)

## ✨ Features
- **IPv6 Support** – by @siriusfreak ([#544](https://github.com/hashicorp/packer-plugin-amazon/pull/544))  
  Adds support for IPv6-only configurations, expanding networking compatibility.
- **Upgrade AMI Import Post-Processor to AWS SDK v2** – by @kp2099 ([#573](https://github.com/hashicorp/packer-plugin-amazon/pull/573))  
    Modernizes the AMI import post-processor by migrating to AWS SDK v2.

---

## 🐛 Bug Fixes
- **Remove `KmsKeyId` from device details in AMI creation** – by @drrk ([#589](https://github.com/hashicorp/packer-plugin-amazon/pull/589))  
  Eliminates the unnecessary `KmsKeyId` field in `step_create_ami` to prevent misconfigurations.

- **Fix `UserAgentProducts` handling** – by @kp2099 ([#600](https://github.com/hashicorp/packer-plugin-amazon/pull/600))  
  Corrects construction of `UserAgentProducts` when building AWS requests.

- **Acceptance test fix: `EbsCopyRegionEncryptedBootWithDeprecation`** – by @kp2099 ([#609](https://github.com/hashicorp/packer-plugin-amazon/pull/609))  
  Ensures stable and accurate results in the EBS encrypted boot with deprecation acceptance test.

---

## 🛠 Improvements
- **Acceptance Test Notifications** – by @kp2099 ([#613](https://github.com/hashicorp/packer-plugin-amazon/pull/613))  
  Adds notifications for failed acceptance tests to improve visibility.

---

## 📦 Dependencies
- **Bump `github.com/hashicorp/packer-plugin-sdk`** from **0.6.2 → 0.6.3** – by @dependabot ([#601](https://github.com/hashicorp/packer-plugin-amazon/pull/601))
- **Bump `github.com/ulikunitz/xz`** from **0.5.10 → 0.5.15** – by @kp2099 ([#607](https://github.com/hashicorp/packer-plugin-amazon/pull/607))

## 1.4.0 (August 27, 2025)
### AWS SDK V2 UPGRADES:

* Datasource: Secretsmanager Aws Sdk v2 upgrade by @kp2099 in https://github.com/hashicorp/packer-plugin-amazon/pull/569
* Datasource: Parameterstore Aws Sdk v2 upgrade by @kp2099 in https://github.com/hashicorp/packer-plugin-amazon/pull/562

### OTHER CHANGES:
* [COMPLIANCE] Add Copyright and License Headers by @hashicorp-copywrite[bot] in https://github.com/hashicorp/packer-plugin-amazon/pull/590

## 1.3.10 (August 6, 2025)
### IMPROVEMENTS:

* Updated plugin release process: Plugin binaries are now published on the HashiCorp official [release site](https://releases.hashicorp.com/packer-plugin-amazon), ensuring a secure and standardized delivery pipeline.
* Imds support for Amazon AMI-Import Post Processor [GH-577](https://github.com/hashicorp/packer-plugin-amazon/pull/577)
* Fixing typos in the documentation for ebs volume builder [GH-576](https://github.com/hashicorp/packer-plugin-amazon/pull/576)

### NOTES:
* **Binary Distribution Update**: To streamline our release process and align with other HashiCorp tools, all 
  release binaries will now be published exclusively to the official HashiCorp [release](https://releases.hashicorp.com/packer-plugin-amazon) site. We will no longer attach release assets to GitHub Releases. Any scripts or automation 
  that rely on the old location will need to be updated. For more information, see our post [here](https://discuss.hashicorp.com/t/important-update-official-packer-plugin-distribution-moving-to-releases-hashicorp-com/75972).

## 1.0.5 (December 22, 2021)

### IMPROVEMENTS
* builder/chroot: Add support for i386 and x86_64_mac architectures. [GH-154]
* builder/ebssurrogate: Add support for i386 and x86_64_mac architectures.
    [GH-154]

## 1.0.4 (October 27, 2021)

### BUG FIXES:
* Fix invalid KMS key error for multi-region keys. [GH-147]
* Fix variable interpolation for builder `run_tags`. [GH-151]

## 1.0.3 (October 19, 2021)

### BUG FIXES:
* Fix panic in GetCredentials helper. [GH-145]


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
