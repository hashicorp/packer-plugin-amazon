// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package chroot

import (
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/chroot"
)

func TestMountDeviceCleanupFunc_ImplementsCleanupFunc(t *testing.T) {
	var raw interface{}
	raw = new(StepMountDevice)
	if _, ok := raw.(chroot.Cleanup); !ok {
		t.Fatalf("cleanup func should be a CleanupFunc")
	}
}
