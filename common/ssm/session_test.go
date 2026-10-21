// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package ssm

import (
	"testing"
)

func TestSession_buildTunnelInput_DocumentName(t *testing.T) {
	tc := []struct {
		name         string
		documentName string
		expected     string
	}{
		{name: "default document", documentName: "", expected: DefaultDocumentName},
		{name: "custom document", documentName: "MyPortForwardingDocument", expected: "MyPortForwardingDocument"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{
				InstanceID:   "i-1234567890abcdef0",
				LocalPort:    8080,
				RemotePort:   22,
				DocumentName: tt.documentName,
			}

			input := s.buildTunnelInput()
			if got := *input.DocumentName; got != tt.expected {
				t.Errorf("expected document name %q, got %q", tt.expected, got)
			}
			if got := *input.Target; got != s.InstanceID {
				t.Errorf("expected target %q, got %q", s.InstanceID, got)
			}
		})
	}
}
