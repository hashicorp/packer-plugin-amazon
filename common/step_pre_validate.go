// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"

	"github.com/hashicorp/packer-plugin-amazon/common/awserrors"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/retry"
)

// StepPreValidate provides an opportunity to pre-validate any configuration for
// the build before actually doing any time consuming work
type StepPreValidate struct {
	DestAmiName        string
	ForceDeregister    bool
	AMISkipBuildRegion bool
	AMISkipCreateImage bool
	VpcId              string
	SubnetId           string
	HasSubnetFilter    bool
}

func (s *StepPreValidate) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)

	if accessConfig, ok := state.GetOk("access_config"); ok {
		accessconf := accessConfig.(*AccessConfig)
		if !accessconf.VaultAWSEngine.Empty() {
			// loop over the authentication a few times to give vault-created creds
			// time to become eventually-consistent
			ui.Say("You're using Vault-generated AWS credentials. It may take a " +
				"few moments for them to become available on AWS. Waiting...")
			err := retry.Config{
				Tries: 11,
				ShouldRetry: func(err error) bool {
					if awserrors.Matches(err, "AuthFailure", "") {
						log.Printf("Waiting for Vault-generated AWS credentials" +
							" to pass authentication... trying again.")
						return true
					}
					return false
				},
				RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
			}.Run(ctx, func(ctx context.Context) error {
				ec2Client, err := accessconf.NewEC2Client(ctx)
				if err != nil {
					return err
				}
				_, err = listEC2Regions(ctx, ec2Client)
				return err
			})

			if err != nil {
				state.Put("error", fmt.Errorf("Was unable to Authenticate to AWS using Vault-"+
					"Generated Credentials within the retry timeout."))
				return multistep.ActionHalt
			}
		}

		if amiConfig, ok := state.GetOk("ami_config"); ok {
			amiconf := amiConfig.(*AMIConfig)
			if !amiconf.AMISkipRegionValidation {
				regionsToValidate := append(amiconf.AMIRegions, accessconf.RawRegion)
				err := accessconf.ValidateRegion(ctx, regionsToValidate...)
				if err != nil {
					state.Put("error", fmt.Errorf("error validating regions: %v", err))
					return multistep.ActionHalt
				}
			}
		}
	}

	if s.ForceDeregister {
		ui.Say("Force Deregister flag found, skipping prevalidating AMI Name")
		return multistep.ActionContinue
	}

	if s.AMISkipBuildRegion {
		ui.Say("skip_build_region was set; not prevalidating AMI name")
		return multistep.ActionContinue
	}

	if s.AMISkipCreateImage {
		ui.Say("skip_create_ami was set; not prevalidating AMI name")
		return multistep.ActionContinue
	}

	ec2Client := state.Get("ec2v2").(clients.Ec2Client)

	// Validate VPC settings for non-default VPCs
	ui.Say("Prevalidating any provided VPC information")
	if err := s.checkVpc(ctx, ec2Client); err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	ui.Say(fmt.Sprintf("Prevalidating AMI Name: %s", s.DestAmiName))
	resp, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{"self"},
		Filters: []ec2types.Filter{{
			Name:   aws.String("name"),
			Values: []string{s.DestAmiName},
		}}}, func(o *ec2.Options) {
		o.RetryMaxAttempts = 11
	})

	if err != nil {
		err = fmt.Errorf("Error querying AMI: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	if len(resp.Images) > 0 {
		err := fmt.Errorf("Error: AMI Name: '%s' is used by an existing AMI: %s", *resp.Images[0].Name, *resp.Images[0].ImageId)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	return multistep.ActionContinue
}

func (s *StepPreValidate) checkVpc(ctx context.Context, ec2Client clients.Ec2Client) error {
	if s.VpcId == "" || (s.VpcId != "" && (s.SubnetId != "" || s.HasSubnetFilter)) {
		// Skip validation if:
		// * The user has not provided a VpcId.
		// * Both VpcId and SubnetId are provided; AWS API will error if something is wrong.
		// * Both VpcId and SubnetFilter are provided
		return nil
	}

	res, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{VpcIds: []string{s.VpcId}})
	if awserrors.Matches(err, "InvalidVpcID.NotFound", "") || err != nil {
		return fmt.Errorf("Error retrieving VPC information for vpc_id %s: %s", s.VpcId, err)
	}

	if res != nil && len(res.Vpcs) == 1 && len(res.Vpcs) > 0 {
		if isDefault := aws.ToBool(res.Vpcs[0].IsDefault); !isDefault {
			return fmt.Errorf("Error: subnet_id or subnet_filter must be provided for non-default VPCs (%s)", s.VpcId)
		}
	}
	return nil
}

// Cleanup ...
func (s *StepPreValidate) Cleanup(multistep.StateBag) {}
