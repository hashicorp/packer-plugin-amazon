// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package amazonimport

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/hashicorp/hcl/v2/hcldec"
	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
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
	S3Bucket           string            `mapstructure:"s3_bucket_name"`
	S3Key              string            `mapstructure:"s3_key_name"`
	S3Encryption       string            `mapstructure:"s3_encryption"`
	S3EncryptionKey    string            `mapstructure:"s3_encryption_key"`
	SkipClean          bool              `mapstructure:"skip_clean"`
	Tags               map[string]string `mapstructure:"tags"`
	Name               string            `mapstructure:"ami_name"`
	Description        string            `mapstructure:"ami_description"`
	Users              []string          `mapstructure:"ami_users"`
	Groups             []string          `mapstructure:"ami_groups"`
	OrgArns            []string          `mapstructure:"ami_org_arns"`
	OuArns             []string          `mapstructure:"ami_ou_arns"`
	Encrypt            bool              `mapstructure:"ami_encrypt"`
	KMSKey             string            `mapstructure:"ami_kms_key"`
	LicenseType        string            `mapstructure:"license_type"`
	RoleName           string            `mapstructure:"role_name"`
	Format             string            `mapstructure:"format"`
	Architecture       string            `mapstructure:"architecture"`
	BootMode           string            `mapstructure:"boot_mode"`
	Platform           string            `mapstructure:"platform"`
	ImportType         string            `mapstructure:"import_type"`
	SnapshotDeviceName string            `mapstructure:"snapshot_device_name"`
	EnaSupport         bool              `mapstructure:"ena_support"`
	VirtualizationType string            `mapstructure:"virtualization_type"`

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
	if p.config.S3Key == "" {
		p.config.S3Key = "packer-import-{{timestamp}}." + p.config.Format
	}

	if p.config.Architecture == "" {
		p.config.Architecture = "x86_64"
	}

	if p.config.ImportType == "" {
		p.config.ImportType = "image"
	}

	if p.config.SnapshotDeviceName == "" {
		p.config.SnapshotDeviceName = "/dev/sda"
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

	switch p.config.ImportType {
	case "image":
		switch p.config.Format {
		case "ova", "raw", "vmdk", "vhd", "vhdx":
		case "":
			p.config.Format = "ova"
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
		if p.config.VirtualizationType != "" {
			errs = packersdk.MultiErrorAppend(errs, errors.New(
				"virtualization_type can only be specified when import_type='snapshot'"))
		}
	case "snapshot":
		// If importing to snapshot, only 3 formats are allowed
		switch p.config.Format {
		case "raw", "vhd", "vmdk":
		case "":
			p.config.Format = "raw"
		default:
			errs = packersdk.MultiErrorAppend(
				errs, fmt.Errorf(
					"invalid format '%s' for snapshot import. Only 'raw', 'vhd', or 'vmdk' are allowed", p.config.Format))
		}
		// Platform is not used as an AWS parameter for snapshot imports, AWS assumes 'linux' platform in this case.
		switch p.config.Platform {
		case "", "linux":
		default:
			errs = packersdk.MultiErrorAppend(
				errs, fmt.Errorf("invalid platform '%s', only 'linux' is allowed when import_type='snapshot'", p.config.Platform))
		}
		switch p.config.VirtualizationType {
		case "paravirtual":
			if p.config.Architecture == "arm64" {
				errs = packersdk.MultiErrorAppend(
					errs, errors.New("only 'hvm' virtualization_type is allowed for 'arm64' architecture"))
			}
		case "hvm":
		case "":
			p.config.VirtualizationType = "hvm"
		default:
			errs = packersdk.MultiErrorAppend(
				errs, fmt.Errorf("invalid virtualization_type '%s', only 'hvm' or 'paravirtual' are allowed", p.config.VirtualizationType))
		}
	default:
		errs = packersdk.MultiErrorAppend(
			errs, fmt.Errorf("invalid import_type '%s'. Only 'image' or 'snapshot' are allowed", p.config.ImportType))
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

	generatedData := artifact.State("generated_data")
	if generatedData == nil {
		// Make sure it's not a nil map so we can assign to it later.
		generatedData = make(map[string]interface{})
	}
	p.config.ctx.Data = generatedData

	session, err := p.config.Session()
	if err != nil {
		return nil, false, false, err
	}
	config := session.Config

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
		ui.Message(fmt.Sprintf("Ignoring s3_encryption_key because s3_encryption is set to '%s'", p.config.S3Encryption))
	}

	// open the source file
	log.Printf("Opening file %s to upload", source)
	file, err := os.Open(source)
	if err != nil {
		return nil, false, false, fmt.Errorf("Failed to open %s: %s", source, err)
	}

	ui.Message(fmt.Sprintf("Uploading %s to s3://%s/%s", source, p.config.S3Bucket, p.config.S3Key))

	// Prepare S3 request
	updata := &s3manager.UploadInput{
		Body:   file,
		Bucket: &p.config.S3Bucket,
		Key:    &p.config.S3Key,
	}

	// Add encryption if specified in the config
	if p.config.S3Encryption != "" {
		updata.ServerSideEncryption = &p.config.S3Encryption
		if p.config.S3Encryption == "aws:kms" && p.config.S3EncryptionKey != "" {
			updata.SSEKMSKeyId = &p.config.S3EncryptionKey
		}
	}

	// Copy the image file into the S3 bucket specified
	uploader := s3manager.NewUploader(session)
	if _, err = uploader.Upload(updata); err != nil {
		return nil, false, false, fmt.Errorf("Failed to upload %s: %s", source, err)
	}

	// May as well stop holding this open now
	file.Close()

	ui.Message(fmt.Sprintf("Completed upload of %s to s3://%s/%s", source, p.config.S3Bucket, p.config.S3Key))

	// Call EC2 image import process
	log.Printf("Calling EC2 to import from s3://%s/%s", p.config.S3Bucket, p.config.S3Key)

	// Split into snapshot or image import
	var createdami string
	ec2conn := ec2.New(session)

	if p.config.ImportType == "snapshot" {
		createdami, err = p.importSnapshot(ui, ctx, ec2conn)
	} else {
		createdami, err = p.importImage(ui, ctx, ec2conn)
	}

	if err != nil {
		return nil, false, false, err
	}

	// Dont rename on snapshot as we set the name on creation
	if p.config.Name != "" && p.config.ImportType != "snapshot" {

		ui.Message(fmt.Sprintf("Starting rename of AMI (%s)", createdami))

		copyInput := &ec2.CopyImageInput{
			Name:          &p.config.Name,
			SourceImageId: &createdami,
			SourceRegion:  config.Region,
		}
		if p.config.Encrypt {
			copyInput.Encrypted = aws.Bool(p.config.Encrypt)
			if p.config.KMSKey != "" {
				copyInput.KmsKeyId = &p.config.KMSKey
			}
		}

		resp, err := ec2conn.CopyImage(copyInput)

		if err != nil {
			return nil, false, false, fmt.Errorf("Error Copying AMI (%s): %s", createdami, err)
		}

		ui.Message("Waiting for AMI rename to complete (may take a while)")

		if err := p.config.PollingConfig.WaitUntilAMIAvailable(aws.BackgroundContext(), ec2conn, *resp.ImageId); err != nil {
			return nil, false, false, fmt.Errorf("Error waiting for AMI (%s): %s", *resp.ImageId, err)
		}

		// Clean up intermediary image now that it has successfully been renamed.
		ui.Message("Destroying intermediary AMI...")
		err = awscommon.DestroyAMIs([]*string{&createdami}, ec2conn)
		if err != nil {
			return nil, false, false, fmt.Errorf("Error deregistering existing AMI: %s", err)
		}

		ui.Message("AMI rename completed")

		createdami = *resp.ImageId
	}

	// If we have tags, then apply them now to both the AMI and snaps
	// created by the import
	if len(p.config.Tags) > 0 {
		var ec2Tags []*ec2.Tag

		log.Printf("Repacking tags into AWS format")

		for key, value := range p.config.Tags {
			ui.Message(fmt.Sprintf("Adding tag \"%s\": \"%s\"", key, value))
			ec2Tags = append(ec2Tags, &ec2.Tag{
				Key:   aws.String(key),
				Value: aws.String(value),
			})
		}

		resourceIds := []*string{&createdami}

		log.Printf("Getting details of %s", createdami)

		imageResp, err := ec2conn.DescribeImages(&ec2.DescribeImagesInput{
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
				ui.Message(fmt.Sprintf("Tagging snapshot %s", *device.Ebs.SnapshotId))
				resourceIds = append(resourceIds, device.Ebs.SnapshotId)
			}
		}

		ui.Message(fmt.Sprintf("Tagging AMI %s", createdami))

		_, err = ec2conn.CreateTags(&ec2.CreateTagsInput{
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
			Description: &ec2.AttributeValue{Value: &p.config.Description},
		}
	}

	if len(p.config.Groups) > 0 {
		groups := make([]*string, len(p.config.Groups))
		adds := make([]*ec2.LaunchPermission, len(p.config.Groups))
		addGroups := &ec2.ModifyImageAttributeInput{
			LaunchPermission: &ec2.LaunchPermissionModifications{},
		}

		for i, g := range p.config.Groups {
			groups[i] = aws.String(g)
			adds[i] = &ec2.LaunchPermission{
				Group: aws.String(g),
			}
		}
		addGroups.UserGroups = groups
		addGroups.LaunchPermission.Add = adds

		options["groups"] = addGroups
	}

	if len(p.config.Users) > 0 {
		users := make([]*string, len(p.config.Users))
		adds := make([]*ec2.LaunchPermission, len(p.config.Users))
		for i, u := range p.config.Users {
			users[i] = aws.String(u)
			adds[i] = &ec2.LaunchPermission{UserId: aws.String(u)}
		}
		options["users"] = &ec2.ModifyImageAttributeInput{
			UserIds: users,
			LaunchPermission: &ec2.LaunchPermissionModifications{
				Add: adds,
			},
		}
	}

	if len(p.config.OrgArns) > 0 {
		orgArns := make([]*string, len(p.config.OrgArns))
		adds := make([]*ec2.LaunchPermission, len(p.config.OrgArns))
		for i, u := range p.config.OrgArns {
			orgArns[i] = aws.String(u)
			adds[i] = &ec2.LaunchPermission{OrganizationArn: aws.String(u)}
		}
		options["ami org arns"] = &ec2.ModifyImageAttributeInput{
			OrganizationArns: orgArns,
			LaunchPermission: &ec2.LaunchPermissionModifications{
				Add: adds,
			},
		}
	}

	if len(p.config.OuArns) > 0 {
		ouArns := make([]*string, len(p.config.OuArns))
		adds := make([]*ec2.LaunchPermission, len(p.config.OuArns))
		for i, u := range p.config.OuArns {
			ouArns[i] = aws.String(u)
			adds[i] = &ec2.LaunchPermission{OrganizationalUnitArn: aws.String(u)}
		}
		options["ami ou arns"] = &ec2.ModifyImageAttributeInput{
			OrganizationalUnitArns: ouArns,
			LaunchPermission: &ec2.LaunchPermissionModifications{
				Add: adds,
			},
		}
	}

	if len(options) > 0 {
		for name, input := range options {
			ui.Message(fmt.Sprintf("Modifying: %s", name))
			input.ImageId = &createdami
			_, err := ec2conn.ModifyImageAttribute(input)
			if err != nil {
				return nil, false, false, fmt.Errorf("Error modifying AMI attributes: %s", err)
			}
		}
	}

	// Add the reported AMI ID to the artifact list
	log.Printf("Adding created AMI ID %s in region %s to output artifacts", createdami, *config.Region)
	artifact = &awscommon.Artifact{
		Amis: map[string]string{
			*config.Region: createdami,
		},
		BuilderIdValue: BuilderId,
		Session:        session,
	}

	if !p.config.SkipClean {
		ui.Message(fmt.Sprintf("Deleting import source s3://%s/%s", p.config.S3Bucket, p.config.S3Key))
		s3conn := s3.New(session)
		_, err = s3conn.DeleteObject(&s3.DeleteObjectInput{
			Bucket: &p.config.S3Bucket,
			Key:    &p.config.S3Key,
		})
		if err != nil {
			return nil, false, false, fmt.Errorf("Failed to delete s3://%s/%s: %s", p.config.S3Bucket, p.config.S3Key, err)
		}
	}

	return artifact, false, false, nil
}

func (p *PostProcessor) importSnapshot(ui packersdk.Ui, ctx context.Context, ec2conn *ec2.EC2) (string, error) {
	var err error
	var importResult *ec2.DescribeImportSnapshotTasksOutput

	params := &ec2.ImportSnapshotInput{
		Encrypted: &p.config.Encrypt,
		DiskContainer: &ec2.SnapshotDiskContainer{
			Format: &p.config.Format,
			UserBucket: &ec2.UserBucket{
				S3Bucket: &p.config.S3Bucket,
				S3Key:    &p.config.S3Key,
			},
		},
	}

	if p.config.Encrypt && p.config.KMSKey != "" {
		params.KmsKeyId = &p.config.KMSKey
	}

	if p.config.RoleName != "" {
		params.SetRoleName(p.config.RoleName)
	}

	var importStart *ec2.ImportSnapshotOutput
	err = retry.Config{
		Tries:      11,
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		importStart, err = ec2conn.ImportSnapshot(params)
		return err
	})

	if err != nil {
		return "", fmt.Errorf("Failed to start snapshot import from s3://%s/%s: %s", p.config.S3Bucket, p.config.S3Key, err)
	}

	importTaskId := importStart.ImportTaskId

	ui.Message(fmt.Sprintf("Started snapshot import of s3://%s/%s, task id %s", p.config.S3Bucket, p.config.S3Key, *importTaskId))

	// Wait for import process to complete, this takes a while
	ui.Message(fmt.Sprintf("Waiting for import snapshot task %s to complete (may take a while)", *importTaskId))

	err = p.config.PollingConfig.WaitUntilSnapshotImported(aws.BackgroundContext(), ec2conn, *importTaskId)
	if err != nil {
		// Retrieve the status message
		if importResult, describeErr := ec2conn.DescribeImportSnapshotTasks(&ec2.DescribeImportSnapshotTasksInput{
			ImportTaskIds: []*string{
				importTaskId,
			},
		}); describeErr != nil {
			return "", fmt.Errorf("Import snapshot task %s failed with status message: %s, error: %s", *importTaskId, *importResult.ImportSnapshotTasks[0].SnapshotTaskDetail.StatusMessage, err)
		}

		return "", fmt.Errorf("Import snapshot task %s failed with status message: Error retrieving status message, error: %s", *importTaskId, err)
	}

	// Retrieve what the outcome was for the import task
	importResult, err = ec2conn.DescribeImportSnapshotTasks(&ec2.DescribeImportSnapshotTasksInput{
		ImportTaskIds: []*string{
			importTaskId,
		},
	})

	if err != nil {
		return "", fmt.Errorf("Failed to find import snapshot task %s: %s", *importTaskId, err)
	}

	snapshotId := importResult.ImportSnapshotTasks[0].SnapshotTaskDetail.SnapshotId

	// Check it was actually completed
	if *importResult.ImportSnapshotTasks[0].SnapshotTaskDetail.Status != "completed" {
		// The most useful error message is from the job itself
		return "", fmt.Errorf("Import snapshot task %s failed: %s", *importTaskId, *importResult.ImportSnapshotTasks[0].SnapshotTaskDetail.StatusMessage)
	}

	ui.Message(fmt.Sprintf("Import snapshot task %s complete", *importTaskId))

	ebsDevice := ec2.EbsBlockDevice{
		SnapshotId: snapshotId,
	}

	blockDevice := ec2.BlockDeviceMapping{
		DeviceName: &p.config.SnapshotDeviceName,
		Ebs:        &ebsDevice,
	}

	var imageName string

	// Unfortunately when importing from snapshot we need to give a name to the AMI,
	// so either get the name from the config or set a name based on the source s3 object.
	if p.config.Name != "" {
		imageName = p.config.Name
	} else {
		imageName = fmt.Sprintf("packer-import-from-%s", p.config.S3Key)
	}

	registerImageOutput, err := ec2conn.RegisterImage(&ec2.RegisterImageInput{
		BlockDeviceMappings: []*ec2.BlockDeviceMapping{
			&blockDevice,
		},
		RootDeviceName:     &p.config.SnapshotDeviceName,
		Name:               &imageName,
		Architecture:       &p.config.Architecture,
		BootMode:           &p.config.BootMode,
		EnaSupport:         &p.config.EnaSupport,
		VirtualizationType: &p.config.VirtualizationType,
	})

	if err != nil {
		return "", fmt.Errorf("Failed to register snapshot %s as AMI: %s", *snapshotId, err)
	}

	// Pull AMI ID out of the completed job
	createdAmi := *registerImageOutput.ImageId
	return createdAmi, err
}

func (p *PostProcessor) importImage(ui packersdk.Ui, ctx context.Context, ec2conn *ec2.EC2) (string, error) {
	var err error
	var value string
	params := &ec2.ImportImageInput{
		Encrypted: &p.config.Encrypt,
		DiskContainers: []*ec2.ImageDiskContainer{
			{
				Format: &p.config.Format,
				UserBucket: &ec2.UserBucket{
					S3Bucket: &p.config.S3Bucket,
					S3Key:    &p.config.S3Key,
				},
			},
		},
		Architecture: &p.config.Architecture,
		BootMode:     &p.config.BootMode,
		Platform:     &p.config.Platform,
	}

	if p.config.Encrypt && p.config.KMSKey != "" {
		params.KmsKeyId = &p.config.KMSKey
	}

	if p.config.RoleName != "" {
		params.SetRoleName(p.config.RoleName)
	}

	if p.config.LicenseType != "" {
		ui.Message(fmt.Sprintf("Setting license type to '%s'", p.config.LicenseType))
		params.LicenseType = &p.config.LicenseType
	}

	var importStart *ec2.ImportImageOutput
	err = retry.Config{
		Tries:      11,
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) error {
		importStart, err = ec2conn.ImportImage(params)
		return err
	})

	if err != nil {
		return value, fmt.Errorf("Failed to start image import from s3://%s/%s: %s", p.config.S3Bucket, p.config.S3Key, err)
	}

	importTaskId := importStart.ImportTaskId

	ui.Message(fmt.Sprintf("Started image import of s3://%s/%s, task id %s", p.config.S3Bucket, p.config.S3Key, *importTaskId))

	// Wait for import process to complete, this takes a while
	ui.Message(fmt.Sprintf("Waiting for import image task %s to complete (may take a while)", *importTaskId))
	err = p.config.PollingConfig.WaitUntilImageImported(aws.BackgroundContext(), ec2conn, *importTaskId)
	if err != nil {
		// Retrieve the status message
		importResult, err2 := ec2conn.DescribeImportImageTasks(&ec2.DescribeImportImageTasksInput{
			ImportTaskIds: []*string{
				importTaskId,
			},
		})

		statusMessage := "Error retrieving status message"

		if err2 == nil {
			statusMessage = *importResult.ImportImageTasks[0].StatusMessage
		}
		return value, fmt.Errorf("Import image task %s failed with status message: %s, error: %s", *importTaskId, statusMessage, err)
	}

	// Retrieve what the outcome was for the import task
	importResult, err := ec2conn.DescribeImportImageTasks(&ec2.DescribeImportImageTasksInput{
		ImportTaskIds: []*string{
			importTaskId,
		},
	})

	if err != nil {
		return value, fmt.Errorf("Failed to find import image task %s: %s", *importTaskId, err)
	}
	// Check it was actually completed
	if *importResult.ImportImageTasks[0].Status != "completed" {
		// The most useful error message is from the job itself
		return value, fmt.Errorf("Import image task %s failed: %s", *importTaskId, *importResult.ImportImageTasks[0].StatusMessage)
	}

	ui.Message(fmt.Sprintf("Import image task %s complete", *importTaskId))

	// Pull AMI ID out of the completed job
	createdAmi := *importResult.ImportImageTasks[0].ImageId
	return createdAmi, err
}
