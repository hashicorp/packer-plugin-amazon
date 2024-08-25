// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/hashicorp/packer-plugin-sdk/uuid"
)

const (
	AmazonSSMManagedInstanceCorePolicyArnPart = "iam::aws:policy/AmazonSSMManagedInstanceCore"
)

type StepIamInstanceProfile struct {
	PollingConfig                             *AWSPollingConfig
	IamInstanceProfile                        string
	SkipProfileValidation                     bool
	TemporaryIamInstanceProfilePolicyDocument *PolicyDocument
	SSMAgentEnabled                           bool
	IsRestricted                              bool
	createdInstanceProfileName                string
	createdRoleName                           string
	createdPolicyName                         string
	roleIsAttached                            bool
	Tags                                      map[string]string
	Ctx                                       interpolate.Context
}

func handleError(state multistep.StateBag, err error, message ...string) multistep.StepAction {
	log.Printf("[DEBUG] %s", err.Error())
	state.Get("ui").(packersdk.Ui).Error(err.Error())
	if len(message) > 0 {
		state.Put("error", fmt.Errorf("%s: %s", message[0], err))
	} else {
		state.Put("error", err)
	}
	return multistep.ActionHalt
}

func (s *StepIamInstanceProfile) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	iamsvc := state.Get("iam").(*iam.IAM)
	ui := state.Get("ui").(packersdk.Ui)

	state.Put("iamInstanceProfile", "")

	if len(s.IamInstanceProfile) > 0 {
		if !s.SkipProfileValidation {
			_, err := iamsvc.GetInstanceProfile(
				&iam.GetInstanceProfileInput{
					InstanceProfileName: aws.String(s.IamInstanceProfile),
				},
			)
			if err != nil {
				return handleError(state, err, "Couldn't find specified instance profile: %s")
			}
		}
		log.Printf("Using specified instance profile: %v", s.IamInstanceProfile)
		state.Put("iamInstanceProfile", s.IamInstanceProfile)
		return multistep.ActionContinue
	}

	if s.SSMAgentEnabled || s.TemporaryIamInstanceProfilePolicyDocument != nil {
		// Create the profile
		profileName := fmt.Sprintf("packer-%s", uuid.TimeOrderedUUID())

		ui.Sayf("Creating temporary instance profile for this instance: %s", profileName)

		region := state.Get("region").(*string)
		iamProfileTags, err := TagMap(s.Tags).IamTags(s.Ctx, *region, state)
		if err != nil {
			return handleError(state, err, "Error creating IAM tags")
		}

		ui.Sayf("Creating temporary role for this instance: %s", profileName)
		service := "ec2.amazonaws.com"
		if s.IsRestricted {
			service = "ec2.amazonaws.com.cn"
		}
		trustPolicy := fmt.Sprintf(`{
			"Version": "2012-10-17",
			"Statement": [
					{
							"Effect": "Allow",
							"Principal": {
									"Service": "%s"
							},
							"Action": "sts:AssumeRole"
					}
			]
	}`, service)
		roleResp, err := iamsvc.CreateRole(&iam.CreateRoleInput{
			RoleName:                 aws.String(profileName),
			Description:              aws.String("Temporary role for Packer"),
			AssumeRolePolicyDocument: aws.String(trustPolicy),
			Tags:                     iamProfileTags,
		})
		if err != nil {
			return handleError(state, err, "Error creating role")
		}
		s.createdRoleName = *roleResp.Role.RoleName

		log.Printf("[DEBUG] Waiting for temporary role: %s", s.createdRoleName)
		err = iamsvc.WaitUntilRoleExistsWithContext(
			aws.BackgroundContext(),
			&iam.GetRoleInput{
				RoleName: aws.String(s.createdRoleName),
			},
			s.PollingConfig.getWaiterOptions()...,
		)
		if err == nil {
			log.Printf("[DEBUG] Found temporary role %s", s.createdRoleName)
		} else {
			return handleError(state, err, fmt.Sprintf("Timed out waiting for temporary role %s", s.createdRoleName))
		}

		ui.Sayf("Attaching policy to the temporary role: %s", profileName)

		if s.TemporaryIamInstanceProfilePolicyDocument != nil {
			inlinePolicyJSON, err := json.Marshal(s.TemporaryIamInstanceProfilePolicyDocument)
			if err != nil {
				return handleError(state, err, "Error parsing policy document")
			}
			_, err = iamsvc.PutRolePolicy(&iam.PutRolePolicyInput{
				RoleName:       aws.String(s.createdRoleName),
				PolicyName:     aws.String(profileName),
				PolicyDocument: aws.String(string(inlinePolicyJSON)),
			})
			if err != nil {
				return handleError(state, err, "Error attaching policy to role")
			}
			s.createdPolicyName = profileName
		}
		if s.SSMAgentEnabled {
			ssmPolicyArn := aws.String(fmt.Sprintf("arn:%s:%s", AwsPartition(s.IsRestricted), AmazonSSMManagedInstanceCorePolicyArnPart))
			_, err = iamsvc.AttachRolePolicy(&iam.AttachRolePolicyInput{
				PolicyArn: ssmPolicyArn,
				RoleName:  aws.String(s.createdRoleName),
			})
			if err != nil {
				return handleError(state, err, "Error attaching AmazonSSMManagedInstanceCore policy to role")
			}
			log.Printf("[DEBUG] Waiting for AmazonSSMManagedInstanceCore attached policy ready")
			err = iamsvc.WaitUntilPolicyExistsWithContext(
				aws.BackgroundContext(),
				&iam.GetPolicyInput{
					PolicyArn: ssmPolicyArn,
				},
				s.PollingConfig.getWaiterOptions()...,
			)
			if err == nil {
				log.Printf("[DEBUG] Found AmazonSSMManagedInstanceCore attached policy in %s", s.createdRoleName)
			} else {
				return handleError(state, err, fmt.Sprintf("Timed out waiting for AmazonSSMManagedInstanceCore attached policy in %s", s.createdRoleName))
			}
		}

		profileResp, err := iamsvc.CreateInstanceProfile(&iam.CreateInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			Tags:                iamProfileTags,
		})
		if err != nil {
			return handleError(state, err, "Error creating instance profile")
		}
		s.createdInstanceProfileName = *profileResp.InstanceProfile.InstanceProfileName

		log.Printf("[DEBUG] Waiting for temporary instance profile: %s", s.createdInstanceProfileName)
		err = iamsvc.WaitUntilInstanceProfileExists(&iam.GetInstanceProfileInput{
			InstanceProfileName: aws.String(s.createdInstanceProfileName),
		})

		if err == nil {
			log.Printf("[DEBUG] Found instance profile %s", s.createdInstanceProfileName)
		} else {
			return handleError(state, err, fmt.Sprintf("Timed out waiting for instance profile %s", s.createdInstanceProfileName))
		}

		_, err = iamsvc.AddRoleToInstanceProfile(&iam.AddRoleToInstanceProfileInput{
			InstanceProfileName: aws.String(s.createdInstanceProfileName),
			RoleName:            aws.String(s.createdRoleName),
		})
		if err != nil {
			return handleError(state, err, "Error attaching role to instance profile")
		}

		s.roleIsAttached = true
		state.Put("iamInstanceProfile", s.createdInstanceProfileName)
	}

	return multistep.ActionContinue
}

func (s *StepIamInstanceProfile) Cleanup(state multistep.StateBag) {
	iamsvc := state.Get("iam").(*iam.IAM)
	ui := state.Get("ui").(packersdk.Ui)
	var err error

	if s.roleIsAttached {
		ui.Say("Detaching temporary role from instance profile...")

		if s.SSMAgentEnabled {
			iamsvc.DetachRolePolicy(&iam.DetachRolePolicyInput{
				PolicyArn: aws.String(fmt.Sprintf("arn:%s:%s", AwsPartition(s.IsRestricted), AmazonSSMManagedInstanceCorePolicyArnPart)),
				RoleName:  aws.String(s.createdRoleName),
			})
		}
		_, err := iamsvc.RemoveRoleFromInstanceProfile(&iam.RemoveRoleFromInstanceProfileInput{
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
		_, _ = iamsvc.DeleteRolePolicy(&iam.DeleteRolePolicyInput{
			PolicyName: aws.String(s.createdPolicyName),
			RoleName:   aws.String(s.createdRoleName),
		})
	}
	if s.createdRoleName != "" {
		ui.Say("Deleting temporary role...")

		_, err = iamsvc.DeleteRole(&iam.DeleteRoleInput{RoleName: &s.createdRoleName})
		if err != nil {
			ui.Error(fmt.Sprintf(
				"Error %s. Please delete the role manually: %s", err.Error(), s.createdRoleName))
		}
	}

	if s.createdInstanceProfileName != "" {
		ui.Say("Deleting temporary instance profile...")

		_, err = iamsvc.DeleteInstanceProfile(&iam.DeleteInstanceProfileInput{
			InstanceProfileName: &s.createdInstanceProfileName})

		if err != nil {
			ui.Error(fmt.Sprintf(
				"Error %s. Please delete the instance profile manually: %s", err.Error(), s.createdInstanceProfileName))
		}
	}
}
