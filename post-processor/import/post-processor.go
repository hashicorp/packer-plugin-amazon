// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package amazonimport

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/hashicorp/hcl/v2/hcldec"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-sdk/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/retry"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

const BuilderId = "packer.post-processor.amazon-import"

// Configuration of this post processor
type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`

	// Variables specific to this post processor
	S3Bucket        string            `mapstructure:"s3_bucket_name"`
	S3Key           string            `mapstructure:"s3_key_name"`
	S3Encryption    string            `mapstructure:"s3_encryption"`
	S3EncryptionKey string            `mapstructure:"s3_encryption_key"`
	SkipClean       bool              `mapstructure:"skip_clean"`
	Tags            map[string]string `mapstructure:"tags"`
	Name            string            `mapstructure:"ami_name"`
	Description     string            `mapstructure:"ami_description"`
	Users           []string          `mapstructure:"ami_users"`
	Groups          []string          `mapstructure:"ami_groups"`
	OrgArns         []string          `mapstructure:"ami_org_arns"`
	OuArns          []string          `mapstructure:"ami_ou_arns"`
	Encrypt         bool              `mapstructure:"ami_encrypt"`
	KMSKey          string            `mapstructure:"ami_kms_key"`
	// Enforce version of the Instance Metadata Service on the built AMI.
	// Valid options are unset (legacy) and `v2.0`. See the documentation on
	// [IMDS](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html)
	// for more information. Defaults to legacy.
	AMIIMDSSupport string `mapstructure:"imds_support" required:"false"`
	LicenseType    string `mapstructure:"license_type"`
	RoleName       string `mapstructure:"role_name"`
	Format         string `mapstructure:"format"`
	Architecture   string `mapstructure:"architecture"`
	BootMode       string `mapstructure:"boot_mode"`
	Platform       string `mapstructure:"platform"`

	ctx interpolate.Context
}

type PostProcessor struct {
	config Config
}

func (p *PostProcessor) ConfigSpec() hcldec.ObjectSpec { return p.config.FlatMapstructure().HCL2Spec() }

func (p *PostProcessor) Configure(raws ...interface{}) error {
	p.config.ctx.Funcs = awscommon.TemplateFuncs
	err := config.Decode(&p.config, &config.DecodeOpts{
		PluginType:         BuilderId,
		Interpolate:        true,
		InterpolateContext: &p.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{
				"s3_key_name",
			},
		},
	}, raws...)
	if err != nil {
		return err
	}

	// Set defaults
	if p.config.Format == "" {
		p.config.Format = "ova"
	}

	if p.config.S3Key == "" {
		p.config.S3Key = "packer-import-{{timestamp}}." + p.config.Format
	}

	if p.config.Architecture == "" {
		p.config.Architecture = "x86_64"
	}

	errs := new(packersdk.MultiError)

	if p.config.BootMode == "" {
		// Graviton instance types run uefi by default
		if p.config.Architecture == "arm64" {
			p.config.BootMode = "uefi"
		} else {
			p.config.BootMode = "legacy-bios"
		}
	} else {
		err := awscommon.IsValidBootMode(p.config.BootMode)
		if err != nil {
			errs = packersdk.MultiErrorAppend(errs, err)
		}
	}

	// Check and render s3_key_name
	if err = interpolate.Validate(p.config.S3Key, &p.config.ctx); err != nil {
		errs = packersdk.MultiErrorAppend(
			errs, fmt.Errorf("Error parsing s3_key_name template: %s", err))
	}

	// Check we have AWS access variables defined somewhere
	errs = packersdk.MultiErrorAppend(errs, p.config.AccessConfig.Prepare(&p.config.PackerConfig)...)

	// define all our required parameters
	templates := map[string]*string{
		"s3_bucket_name": &p.config.S3Bucket,
	}
	// Check out required params are defined
	for key, ptr := range templates {
		if *ptr == "" {
			errs = packersdk.MultiErrorAppend(
				errs, fmt.Errorf("%s must be set", key))
		}
	}

	switch p.config.Format {
	case "ova", "raw", "vmdk", "vhd", "vhdx":
	default:
		errs = packersdk.MultiErrorAppend(
			errs, fmt.Errorf("invalid format '%s'. Only 'ova', 'raw', 'vhd', 'vhdx', or 'vmdk' are allowed", p.config.Format))
	}

	switch p.config.Platform {
	case "windows", "linux":
	case "":
		if p.config.BootMode == "uefi" {
			errs = packersdk.MultiErrorAppend(
				errs, fmt.Errorf("invalid platform '%s', 'platform' must be set for 'uefi' image imports", p.config.Platform))
		}
	default:
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(
			"invalid platform '%s'. Only 'linux' and 'windows' are allowed", p.config.Platform))
	}
	if p.config.S3Encryption != "" && p.config.S3Encryption != "AES256" && p.config.S3Encryption != "aws:kms" {
		errs = packersdk.MultiErrorAppend(
			errs, fmt.Errorf("invalid s3 encryption format '%s'. Only 'AES256' and 'aws:kms' are allowed", p.config.S3Encryption))
	}

	if p.config.BootMode != "legacy-bios" && p.config.BootMode != "uefi" {
		errs = packersdk.MultiErrorAppend(
			errs, fmt.Errorf("invalid boot mode '%s'. Only 'uefi' and 'legacy-bios' are allowed", p.config.BootMode))
	}

	if p.config.Architecture == "arm64" && p.config.BootMode != "uefi" {
		errs = packersdk.MultiErrorAppend(
			errs, fmt.Errorf("invalid boot mode '%s' for 'arm64' architecture", p.config.BootMode))
	}

	if p.config.AMIIMDSSupport != "" && p.config.AMIIMDSSupport != string(ec2types.ImdsSupportValuesV20) {
		errs = packersdk.MultiErrorAppend(errs,
			fmt.Errorf(`The only valid imds_support values are %q or the empty string`,
				string(ec2types.ImdsSupportValuesV20)),
		)
	}

	// Anything which flagged return back up the stack
	if len(errs.Errors) > 0 {
		return errs
	}
	if p.config.PollingConfig == nil {
		p.config.PollingConfig = new(awscommon.AWSPollingConfig)
	}
	p.config.PollingConfig.LogEnvOverrideWarnings()

	packersdk.LogSecretFilter.Set(p.config.AccessKey, p.config.SecretKey, p.config.Token)
	log.Println(p.config)
	return nil
}

func (p *PostProcessor) PostProcess(ctx context.Context, ui packersdk.Ui, artifact packersdk.Artifact) (packersdk.Artifact, bool, bool, error) {
	var err error
	config, err := p.config.Config(ctx)

	if err != nil {
		return nil, false, false, err
	}

	generatedData := artifact.State("generated_data")
	if generatedData == nil {
		// Make sure it's not a nil map so we can assign to it later.
		generatedData = make(map[string]interface{})
	}
	p.config.ctx.Data = generatedData

	s3Client := s3.NewFromConfig(*config)

	// Render this key since we didn't in the configure phase
	p.config.S3Key, err = interpolate.Render(p.config.S3Key, &p.config.ctx)
	if err != nil {
		return nil, false, false, fmt.Errorf("Error rendering s3_key_name template: %s", err)
	}
	log.Printf("Rendered s3_key_name as %s", p.config.S3Key)

	log.Println("Looking for image in artifact")
	// Locate the files output from the builder
	source := ""
	for _, path := range artifact.Files() {
		if strings.HasSuffix(path, "."+p.config.Format) {
			source = path
			break
		}
	}

	// Hope we found something useful
	if source == "" {
		return nil, false, false, fmt.Errorf("No %s image file found in artifact from builder", p.config.Format)
	}

	if p.config.S3Encryption == "AES256" && p.config.S3EncryptionKey != "" {
		ui.Say(fmt.Sprintf("Ignoring s3_encryption_key because s3_encryption is set to '%s'", p.config.S3Encryption))
	}

	// open the source file
	log.Printf("Opening file %s to upload", source)
	file, err := os.Open(source)
	if err != nil {
		return nil, false, false, fmt.Errorf("Failed to open %s: %s", source, err)
	}

	ui.Say(fmt.Sprintf("Uploading %s to s3://%s/%s", source, p.config.S3Bucket, p.config.S3Key))

	// Prepare S3 request
	updata := &s3.PutObjectInput{
		Body:   file,
		Bucket: &p.config.S3Bucket,
		Key:    &p.config.S3Key,
	}

	// Add encryption if specified in the config
	if p.config.S3Encryption != "" {
		updata.ServerSideEncryption = s3types.ServerSideEncryption(p.config.S3Encryption)
		if p.config.S3Encryption == string(s3types.ServerSideEncryptionAwsKms) && p.config.S3EncryptionKey != "" {
			updata.SSEKMSKeyId = aws.String(p.config.S3EncryptionKey)
		}
	}

	// Copy the image file into the S3 bucket specified
	uploader := manager.NewUploader(s3Client)
	if _, err = uploader.Upload(ctx, updata); err != nil {
		return nil, false, false, fmt.Errorf("Failed to upload %s: %s", source, err)
	}

	// May as well stop holding this open now
	file.Close()

	ui.Say(fmt.Sprintf("Completed upload of %s to s3://%s/%s", source, p.config.S3Bucket, p.config.S3Key))

	// Call EC2 image import process
	log.Printf("Calling EC2 to import from s3://%s/%s", p.config.S3Bucket, p.config.S3Key)

	ec2Client, err := p.config.NewEC2Client(ctx)
	if err != nil {
		return nil, false, false, fmt.Errorf("failed to create EC2 client: %s", err)
	}

	params := &ec2.ImportImageInput{
		Encrypted: &p.config.Encrypt,
		DiskContainers: []ec2types.ImageDiskContainer{
			{
				Format: &p.config.Format,
				UserBucket: &ec2types.UserBucket{
					S3Bucket: &p.config.S3Bucket,
					S3Key:    &p.config.S3Key,
				},
			},
		},
		Architecture: &p.config.Architecture,
		BootMode:     ec2types.BootModeValues(p.config.BootMode),
		Platform:     &p.config.Platform,
	}

	if p.config.Encrypt && p.config.KMSKey != "" {
		params.KmsKeyId = &p.config.KMSKey
	}

	if p.config.RoleName != "" {
		params.RoleName = &p.config.RoleName
	}

	if p.config.LicenseType != "" {
		ui.Say(fmt.Sprintf("Setting license type to '%s'", p.config.LicenseType))
		params.LicenseType = &p.config.LicenseType
	}

	var importStart *ec2.ImportImageOutput
	err = retry.Config{
		Tries:      11,
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		importStart, err = ec2Client.ImportImage(ctx, params)
		return err
	})

	if err != nil {
		return nil, false, false, fmt.Errorf("Failed to start import from s3://%s/%s: %s", p.config.S3Bucket, p.config.S3Key, err)
	}

	ui.Say(fmt.Sprintf("Started import of s3://%s/%s, task id %s", p.config.S3Bucket, p.config.S3Key,
		*importStart.ImportTaskId))

	// Wait for import process to complete, this takes a while
	ui.Say(fmt.Sprintf("Waiting for task %s to complete (may take a while)", *importStart.ImportTaskId))

	err = p.config.PollingConfig.WaitUntilImageImported(ctx, ec2Client, *importStart.ImportTaskId)
	if err != nil {

		// Retrieve the status message
		importResult, err2 := ec2Client.DescribeImportImageTasks(ctx, &ec2.DescribeImportImageTasksInput{
			ImportTaskIds: []string{
				*importStart.ImportTaskId,
			},
		})

		statusMessage := "Error retrieving status message"

		if err2 == nil {
			statusMessage = *importResult.ImportImageTasks[0].StatusMessage
		}
		return nil, false, false, fmt.Errorf("Import task %s failed with status message: %s, error: %s", *importStart.ImportTaskId, statusMessage, err)
	}

	// Retrieve what the outcome was for the import task
	importResult, err := ec2Client.DescribeImportImageTasks(ctx, &ec2.DescribeImportImageTasksInput{
		ImportTaskIds: []string{
			*importStart.ImportTaskId,
		},
	})

	if err != nil {
		return nil, false, false, fmt.Errorf("Failed to find import task %s: %s", *importStart.ImportTaskId, err)
	}
	// Check it was actually completed
	if *importResult.ImportImageTasks[0].Status != "completed" {
		// The most useful error message is from the job itself
		return nil, false, false, fmt.Errorf("Import task %s failed: %s", *importStart.ImportTaskId, *importResult.ImportImageTasks[0].StatusMessage)
	}

	ui.Say(fmt.Sprintf("Import task %s complete", *importStart.ImportTaskId))

	// Pull AMI ID out of the completed job
	createdami := *importResult.ImportImageTasks[0].ImageId

	if p.config.Name != "" {

		ui.Say(fmt.Sprintf("Starting rename of AMI (%s)", createdami))

		copyInput := &ec2.CopyImageInput{
			Name:          &p.config.Name,
			SourceImageId: &createdami,
			SourceRegion:  aws.String(config.Region),
		}
		if p.config.Encrypt {
			copyInput.Encrypted = aws.Bool(p.config.Encrypt)
			if p.config.KMSKey != "" {
				copyInput.KmsKeyId = &p.config.KMSKey
			}
		}

		resp, err := ec2Client.CopyImage(ctx, copyInput)

		if err != nil {
			return nil, false, false, fmt.Errorf("Error Copying AMI (%s): %s", createdami, err)
		}

		ui.Say("Waiting for AMI rename to complete (may take a while)")

		if err := p.config.PollingConfig.WaitUntilAMIAvailable(ctx, ec2Client, *resp.ImageId); err != nil {
			return nil, false, false, fmt.Errorf("Error waiting for AMI (%s): %s", *resp.ImageId, err)
		}

		// Clean up intermediary image now that it has successfully been renamed.
		ui.Say("Destroying intermediary AMI...")
		err = awscommon.DestroyAMIs([]string{createdami}, ec2Client)
		if err != nil {
			return nil, false, false, fmt.Errorf("Error deregistering existing AMI: %s", err)
		}

		ui.Say("AMI rename completed")

		createdami = *resp.ImageId
	}

	// If we have tags, then apply them now to both the AMI and snaps
	// created by the import
	if len(p.config.Tags) > 0 {
		var ec2Tags []ec2types.Tag

		log.Printf("Repacking tags into AWS format")

		for key, value := range p.config.Tags {
			ui.Say(fmt.Sprintf("Adding tag \"%s\": \"%s\"", key, value))
			ec2Tags = append(ec2Tags, ec2types.Tag{
				Key:   aws.String(key),
				Value: aws.String(value),
			})
		}

		resourceIds := []string{createdami}

		log.Printf("Getting details of %s", createdami)

		imageResp, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
			ImageIds: resourceIds,
		})

		if err != nil {
			return nil, false, false, fmt.Errorf("Failed to retrieve details for AMI %s: %s", createdami, err)
		}

		if len(imageResp.Images) == 0 {
			return nil, false, false, fmt.Errorf("AMI %s has no images", createdami)
		}

		image := imageResp.Images[0]

		log.Printf("Walking block device mappings for %s to find snapshots", createdami)

		for _, device := range image.BlockDeviceMappings {
			if device.Ebs != nil && device.Ebs.SnapshotId != nil {
				ui.Say(fmt.Sprintf("Tagging snapshot %s", *device.Ebs.SnapshotId))
				resourceIds = append(resourceIds, *device.Ebs.SnapshotId)
			}
		}

		ui.Say(fmt.Sprintf("Tagging AMI %s", createdami))

		_, err = ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: resourceIds,
			Tags:      ec2Tags,
		})

		if err != nil {
			return nil, false, false, fmt.Errorf("Failed to add tags to resources %#v: %s", resourceIds, err)
		}

	}

	// Apply attributes for AMI specified in config
	// (duped from builder/amazon/common/step_modify_ami_attributes.go)
	options := make(map[string]*ec2.ModifyImageAttributeInput)
	if p.config.Description != "" {
		options["description"] = &ec2.ModifyImageAttributeInput{
			Description: &ec2types.AttributeValue{Value: &p.config.Description},
		}
	}

	if len(p.config.Groups) > 0 {
		groups := make([]string, len(p.config.Groups))
		adds := make([]ec2types.LaunchPermission, len(p.config.Groups))
		addGroups := &ec2.ModifyImageAttributeInput{
			LaunchPermission: &ec2types.LaunchPermissionModifications{},
		}

		for i, g := range p.config.Groups {
			groups[i] = g
			adds[i] = ec2types.LaunchPermission{
				Group: ec2types.PermissionGroup(g),
			}
		}
		addGroups.UserGroups = groups
		addGroups.LaunchPermission.Add = adds

		options["groups"] = addGroups
	}

	if len(p.config.Users) > 0 {
		users := make([]string, len(p.config.Users))
		adds := make([]ec2types.LaunchPermission, len(p.config.Users))
		for i, u := range p.config.Users {
			users[i] = u
			adds[i] = ec2types.LaunchPermission{UserId: aws.String(u)}
		}
		options["users"] = &ec2.ModifyImageAttributeInput{
			UserIds: users,
			LaunchPermission: &ec2types.LaunchPermissionModifications{
				Add: adds,
			},
		}
	}

	if len(p.config.OrgArns) > 0 {
		orgArns := make([]string, len(p.config.OrgArns))
		adds := make([]ec2types.LaunchPermission, len(p.config.OrgArns))
		for i, u := range p.config.OrgArns {
			orgArns[i] = u
			adds[i] = ec2types.LaunchPermission{OrganizationArn: aws.String(u)}
		}
		options["ami org arns"] = &ec2.ModifyImageAttributeInput{
			OrganizationArns: orgArns,
			LaunchPermission: &ec2types.LaunchPermissionModifications{
				Add: adds,
			},
		}
	}

	if len(p.config.OuArns) > 0 {
		ouArns := make([]string, len(p.config.OuArns))
		adds := make([]ec2types.LaunchPermission, len(p.config.OuArns))
		for i, u := range p.config.OuArns {
			ouArns[i] = u
			adds[i] = ec2types.LaunchPermission{OrganizationalUnitArn: aws.String(u)}
		}
		options["ami ou arns"] = &ec2.ModifyImageAttributeInput{
			OrganizationalUnitArns: ouArns,
			LaunchPermission: &ec2types.LaunchPermissionModifications{
				Add: adds,
			},
		}
	}

	if p.config.AMIIMDSSupport != "" {
		options["ami imds support"] = &ec2.ModifyImageAttributeInput{
			ImdsSupport: &ec2types.AttributeValue{Value: &p.config.AMIIMDSSupport},
		}

	}

	if len(options) > 0 {
		for name, input := range options {
			ui.Say(fmt.Sprintf("Modifying: %s", name))
			input.ImageId = &createdami
			_, err := ec2Client.ModifyImageAttribute(ctx, input)
			if err != nil {
				return nil, false, false, fmt.Errorf("Error modifying AMI attributes: %s", err)
			}
		}
	}

	// Add the reported AMI ID to the artifact list
	log.Printf("Adding created AMI ID %s in region %s to output artifacts", createdami, config.Region)
	artifact = &awscommon.Artifact{
		Amis: map[string]string{
			config.Region: createdami,
		},
		BuilderIdValue: BuilderId,
		Config:         config,
	}

	if !p.config.SkipClean {
		ui.Say(fmt.Sprintf("Deleting import source s3://%s/%s", p.config.S3Bucket, p.config.S3Key))

		_, err = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: &p.config.S3Bucket,
			Key:    &p.config.S3Key,
		})
		if err != nil {
			return nil, false, false, fmt.Errorf("Failed to delete s3://%s/%s: %s", p.config.S3Bucket, p.config.S3Key, err)
		}
	}

	return artifact, false, false, nil
}
