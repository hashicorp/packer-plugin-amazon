// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func TestStepStopEBSBackedInstance_SkipCreateAMISkipsStop(t *testing.T) {
	state := new(multistep.BasicStateBag)
	state.Put("ui", packersdk.TestUi(t))

	step := &StepStopEBSBackedInstance{
		AMISkipCreateImage: true,
	}

	action := step.Run(context.Background(), state)

	if action != multistep.ActionContinue {
		t.Fatalf("expected ActionContinue, got %v", action)
	}

	if rawErr, ok := state.GetOk("error"); ok {
		t.Fatalf("expected no error, got %v", rawErr)
	}
}
