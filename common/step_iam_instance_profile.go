// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/retry"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/hashicorp/packer-plugin-sdk/uuid"
)

type StepIamInstanceProfile struct {
	PollingConfig                             *AWSPollingConfig
	IamInstanceProfile                        string
	SkipProfileValidation                     bool
	SSMAgentEnabled                           bool
	IsRestricted                              bool
	TemporaryIamInstanceProfilePolicyDocument *PolicyDocument
	createdInstanceProfileName                string
	createdRoleName                           string
	createdPolicyName                         string
	roleIsAttached                            bool
	Tags                                      map[string]string
	Ctx                                       interpolate.Context
}

func (s *StepIamInstanceProfile) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	iamsvc := state.Get("iam").(*iam.Client)
	ui := state.Get("ui").(packersdk.Ui)

	state.Put("iamInstanceProfile", "")

	if len(s.IamInstanceProfile) > 0 {
		if !s.SkipProfileValidation {
			_, err := iamsvc.GetInstanceProfile(ctx,
				&iam.GetInstanceProfileInput{
					InstanceProfileName: aws.String(s.IamInstanceProfile),
				},
			)
			if err != nil {
				err := fmt.Errorf("Couldn't find specified instance profile: %s", err)
				log.Printf("[DEBUG] %s", err.Error())
				state.Put("error", err)
				return multistep.ActionHalt
			}
		}
		log.Printf("Using specified instance profile: %v", s.IamInstanceProfile)
		state.Put("iamInstanceProfile", s.IamInstanceProfile)
		return multistep.ActionContinue
	}

	if s.TemporaryIamInstanceProfilePolicyDocument != nil {
		// Create the profile
		profileName := fmt.Sprintf("packer-%s", uuid.TimeOrderedUUID())

		policy, err := json.Marshal(s.TemporaryIamInstanceProfilePolicyDocument)
		if err != nil {
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		ui.Say(fmt.Sprintf("Creating temporary instance profile for this instance: %s", profileName))

		region := state.Get("region").(string)
		iamProfileTags, err := TagMap(s.Tags).IamTags(s.Ctx, region, state)
		if err != nil {
			err := fmt.Errorf("Error creating IAM tags: %s", err)
			state.Put("error", err)
			return multistep.ActionHalt
		}
		profileResp, err := iamsvc.CreateInstanceProfile(ctx, &iam.CreateInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			Tags:                iamProfileTags,
		})
		if err != nil {
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}
		s.createdInstanceProfileName = aws.ToString(profileResp.InstanceProfile.InstanceProfileName)

		log.Printf("[DEBUG] Waiting for temporary instance profile: %s", s.createdInstanceProfileName)

		err = iam.NewInstanceProfileExistsWaiter(iamsvc).Wait(ctx, &iam.GetInstanceProfileInput{
			InstanceProfileName: aws.String(s.createdInstanceProfileName),
		}, AwsDefaultInstanceProfileExistsWaitTimeDuration)

		if err == nil {
			log.Printf("[DEBUG] Found instance profile %s", s.createdInstanceProfileName)
		} else {
			err := fmt.Errorf("Timed out waiting for instance profile %s: %s", s.createdInstanceProfileName, err)
			log.Printf("[DEBUG] %s", err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		ui.Say(fmt.Sprintf("Creating temporary role for this instance: %s", profileName))

		roleResp, err := iamsvc.CreateRole(ctx, &iam.CreateRoleInput{
			RoleName:                 aws.String(profileName),
			Description:              aws.String("Temporary role for Packer"),
			AssumeRolePolicyDocument: aws.String("{\"Version\": \"2012-10-17\",\"Statement\": [{\"Effect\": \"Allow\",\"Principal\": {\"Service\": \"ec2.amazonaws.com\"},\"Action\": \"sts:AssumeRole\"}]}"),
			Tags:                     iamProfileTags,
		})
		if err != nil {
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		s.createdRoleName = aws.ToString(roleResp.Role.RoleName)

		log.Printf("[DEBUG] Waiting for temporary role: %s", s.createdInstanceProfileName)

		pollingOptions := s.PollingConfig.getWaiterOptions()
		var optFns []func(*iam.RoleExistsWaiterOptions)

		if pollingOptions.MaxWaitTime == nil {
			pollingOptions.MaxWaitTime = aws.Duration(AwsDefaultRoleExistsWaitTimeDuration)
		}
		if pollingOptions.MinDelay != nil {
			optFns = append(optFns, func(o *iam.RoleExistsWaiterOptions) {
				o.MinDelay = *pollingOptions.MinDelay
			})
		}
		err = iam.NewRoleExistsWaiter(iamsvc).Wait(ctx, &iam.GetRoleInput{
			RoleName: aws.String(s.createdRoleName),
		}, *pollingOptions.MaxWaitTime, optFns...)

		if err == nil {
			log.Printf("[DEBUG] Found temporary role %s", s.createdRoleName)
		} else {
			err := fmt.Errorf("Timed out waiting for temporary role %s: %s", s.createdRoleName, err)
			log.Printf("[DEBUG] %s", err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		ui.Say(fmt.Sprintf("Attaching policy to the temporary role: %s", profileName))

		_, err = iamsvc.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
			RoleName:       roleResp.Role.RoleName,
			PolicyName:     aws.String(profileName),
			PolicyDocument: aws.String(string(policy)),
		})
		if err != nil {
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		s.createdPolicyName = aws.ToString(roleResp.Role.RoleName)

		_, err = iamsvc.AddRoleToInstanceProfile(ctx, &iam.AddRoleToInstanceProfileInput{
			RoleName:            roleResp.Role.RoleName,
			InstanceProfileName: profileResp.InstanceProfile.InstanceProfileName,
		})
		if err != nil {
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		// In aws sdk go v2, we noticed if there was no Wait, the spot fleet requests were failing even with retry.
		// Running a dummy instance in DryRun mode to validate that the instance profile is visible to EC2

		ui.Say("Waiting for the change to propagate because of eventual consistency...")

		ec2Client := state.Get("ec2v2").(clients.Ec2Client)
		sourceImageRaw, exists := state.GetOk("source_image")
		if !exists {
			err := fmt.Errorf("source_image not available in state for IAM validation")
			state.Put("error", err)
			return multistep.ActionHalt
		}
		sourceImage := sourceImageRaw.(*ec2types.Image)

		err = retry.Config{
			Tries: 11,
			ShouldRetry: func(err error) bool {
				errStr := err.Error()
				return strings.Contains(errStr, "Invalid IAM Instance Profile")
			},
			RetryDelay: (&retry.Backoff{
				InitialBackoff: 500 * time.Millisecond,
				MaxBackoff:     5 * time.Second,
				Multiplier:     2,
			}).Linear,
		}.Run(ctx, func(ctx context.Context) error {

			_, err := ec2Client.RunInstances(ctx, &ec2.RunInstancesInput{
				ImageId:      sourceImage.ImageId,
				MinCount:     aws.Int32(1),
				MaxCount:     aws.Int32(1),
				InstanceType: ec2types.InstanceTypeT3Nano,
				DryRun:       aws.Bool(true),
				IamInstanceProfile: &ec2types.IamInstanceProfileSpecification{
					Name: aws.String(s.createdInstanceProfileName),
				},
			})

			// For dry run, we expect a DryRunOperation error if the call would succeed
			// Any other error indicates the instance profile isn't visible to EC2 yet
			if err != nil {
				errStr := err.Error()
				if strings.Contains(errStr, "DryRunOperation") {
					log.Printf("[DEBUG] EC2 can see IAM instance profile %s", s.createdInstanceProfileName)
					return nil
				}
				log.Printf("[DEBUG] EC2 dry run failed: %s", errStr)
				return err
			}
			return nil
		})

		if err != nil {
			err := fmt.Errorf("timed out waiting for IAM changes to propagate to EC2: %s", err)
			log.Printf("[DEBUG] %s", err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		s.roleIsAttached = true
		state.Put("iamInstanceProfile", aws.ToString(profileResp.InstanceProfile.InstanceProfileName))
	}

	return multistep.ActionContinue
}

func (s *StepIamInstanceProfile) Cleanup(state multistep.StateBag) {
	ctx := context.TODO()
	iamsvc := state.Get("iam").(*iam.Client)
	ui := state.Get("ui").(packersdk.Ui)
	var err error

	if s.roleIsAttached == true {
		ui.Say("Detaching temporary role from instance profile...")

		_, err := iamsvc.RemoveRoleFromInstanceProfile(ctx, &iam.RemoveRoleFromInstanceProfileInput{
			InstanceProfileName: aws.String(s.createdInstanceProfileName),
			RoleName:            aws.String(s.createdRoleName),
		})
		if err != nil {
			ui.Error(fmt.Sprintf(
				"Error %s. Please delete the role manually: %s", err.Error(), s.createdRoleName))
		}
	}

	if s.createdPolicyName != "" {
		ui.Say("Removing policy from temporary role...")
		_, _ = iamsvc.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
			PolicyName: aws.String(s.createdPolicyName),
			RoleName:   aws.String(s.createdRoleName),
		})
	}
	if s.createdRoleName != "" {
		ui.Say("Deleting temporary role...")

		_, err = iamsvc.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: &s.createdRoleName})
		if err != nil {
			ui.Error(fmt.Sprintf(
				"Error %s. Please delete the role manually: %s", err.Error(), s.createdRoleName))
		}
	}

	if s.createdInstanceProfileName != "" {
		ui.Say("Deleting temporary instance profile...")

		_, err = iamsvc.DeleteInstanceProfile(ctx, &iam.DeleteInstanceProfileInput{
			InstanceProfileName: &s.createdInstanceProfileName})

		if err != nil {
			ui.Error(fmt.Sprintf(
				"Error %s. Please delete the instance profile manually: %s", err.Error(), s.createdInstanceProfileName))
		}
	}
}
