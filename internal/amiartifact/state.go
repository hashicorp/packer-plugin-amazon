// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

// Package amiartifact implements the AMI state protocol shared by Amazon builders.
package amiartifact

import (
	"log"
	"sync/atomic"
)

// AMIIDsStateKey identifies the region-to-AMI map exposed to post-processors.
const AMIIDsStateKey = "amazon.ami_ids"

// RetainAMIsStateKey is an Amazon plugin RPC request, not a read-only state key.
// It acknowledges that the AMIs won't be deregistered by Packer artifact cleanup.
// The SSM publisher uses it before creating aliases because Packer ignores its
// keep flags on errors, and a regional write can succeed before another fails.
const RetainAMIsStateKey = "amazon.ssm-parameter.retain-amis"

// State holds the AMI retention decision for an artifact. Its zero value allows cleanup.
type State struct {
	retainAMIs atomic.Bool
}

// Lookup handles reserved AMI state keys before an artifact consults its StateData.
func (s *State) Lookup(name string, amis map[string]string) (value interface{}, handled bool) {
	switch name {
	case AMIIDsStateKey:
		return amis, true
	case RetainAMIsStateKey:
		s.retainAMIs.Store(true)
		return true, true
	default:
		return nil, false
	}
}

// SkipDestroy reports whether the AMIs have been retained and must not be deregistered.
func (s *State) SkipDestroy() bool {
	if s.retainAMIs.Load() {
		log.Println("Keeping AMIs referenced by the SSM parameter post-processor")
		return true
	}
	return false
}
