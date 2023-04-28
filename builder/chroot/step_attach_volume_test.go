// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package chroot

import (
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/chroot"
)

func TestAttachVolumeCleanupFunc_ImplementsCleanupFunc(t *testing.T) {
	var raw interface{}
	raw = new(StepAttachVolume)
	if _, ok := raw.(chroot.Cleanup); !ok {
		t.Fatalf("cleanup func should be a CleanupFunc")
	}
}
