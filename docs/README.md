# Amazon Components

The Amazon plugin is able to create Amazon AMIs through Packer. To achieve this, the plugin comes with
multiple builders, data sources, and a post-processor to build the AMI depending on the strategy you want to use.

### Builders:
- [amazon-ebs](/docs/builders/ebs.mdx) - Create EBS-backed AMIs by
  launching a source AMI and re-packaging it into a new AMI after
  provisioning. If in doubt, use this builder, which is the easiest to get
  started with.
- [amazon-instance](/docs/builders/instance.mdx) - Create
  instance-store AMIs by launching and provisioning a source instance, then
  rebundling it and uploading it to S3.
- [amazon-chroot](/docs/builders/chroot.mdx) - Create EBS-backed AMIs
  from an existing EC2 instance by mounting the root device and using a
  [Chroot](https://en.wikipedia.org/wiki/Chroot) environment to provision
  that device. This is an **advanced builder and should not be used by
  newcomers**. However, it is also the fastest way to build an EBS-backed AMI
  since no new EC2 instance needs to be launched.
- [amazon-ebssurrogate](/docs/builders/ebssurrogate.mdx) - Create EBS
  -backed AMIs from scratch. Works similarly to the `chroot` builder but does
  not require running in AWS. This is an **advanced builder and should not be
  used by newcomers**.

### Data sources:
- [amazon-ami](/docs/datasources/ami.mdx) - Filter and fetch an Amazon AMI to output all the AMI information.
- [amazon-secretsmanager](/docs/datasources/secretsmanager.mdx) - Retrieve information
  about a Secrets Manager secret version, including its secret value.

### Post-Processors
- [amazon-import](/docs/post-processors/import.mdx) -  The Amazon Import post-processor takes an OVA artifact 
  from various builders and imports it to an AMI available to Amazon Web Services EC2.