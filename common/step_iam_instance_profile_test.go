// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Guards that the propagation dry-run request carries the configured instance
// type and resolved subnet, rather than a hardcoded type or no networking.
func TestIamValidationRunInput(t *testing.T) {
	sourceImage := &ec2types.Image{ImageId: aws.String("ami-0123456789abcdef0")}

	t.Run("uses configured instance type and subnet", func(t *testing.T) {
		step := &StepIamInstanceProfile{
			InstanceType:               "c7g.large",
			createdInstanceProfileName: "packer-test-profile",
		}

		input := step.iamValidationRunInput(sourceImage, "subnet-abc123")

		if got := string(input.InstanceType); got != "c7g.large" {
			t.Errorf("InstanceType = %q, want %q", got, "c7g.large")
		}
		if input.SubnetId == nil || *input.SubnetId != "subnet-abc123" {
			t.Errorf("SubnetId = %v, want subnet-abc123", input.SubnetId)
		}
		if input.DryRun == nil || !*input.DryRun {
			t.Error("DryRun should be true")
		}
		if input.IamInstanceProfile == nil || aws.ToString(input.IamInstanceProfile.Name) != "packer-test-profile" {
			t.Errorf("IamInstanceProfile.Name = %v, want packer-test-profile", input.IamInstanceProfile)
		}
		if aws.ToString(input.ImageId) != "ami-0123456789abcdef0" {
			t.Errorf("ImageId = %q, want the source AMI", aws.ToString(input.ImageId))
		}
	})

	t.Run("omits subnet when unset", func(t *testing.T) {
		step := &StepIamInstanceProfile{
			InstanceType:               "t3.small",
			createdInstanceProfileName: "packer-test-profile",
		}

		input := step.iamValidationRunInput(sourceImage, "")

		if input.SubnetId != nil {
			t.Errorf("SubnetId = %v, want nil when no subnet is resolved", input.SubnetId)
		}
	})
}
