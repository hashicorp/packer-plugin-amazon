// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package ssm

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
)

type fakeSSM struct {
	ssmiface.SSMAPI
	statuses []string
	err      error
	calls    int
}

func (f *fakeSSM) GetConnectionStatusWithContext(_ aws.Context, _ *ssm.GetConnectionStatusInput, _ ...request.Option) (*ssm.GetConnectionStatusOutput, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	status := f.statuses[0]
	f.statuses = f.statuses[1:]
	return &ssm.GetConnectionStatusOutput{Status: aws.String(status)}, nil
}

func checkAll(m *agentConnectionMonitor, n int) []bool {
	got := make([]bool, n)
	for i := range got {
		got[i] = m.agentLost(context.Background())
	}
	return got
}

func TestAgentConnectionMonitor_RestartsOnlyAfterConsecutiveDisconnects(t *testing.T) {
	connected, notConnected := ssm.ConnectionStatusConnected, ssm.ConnectionStatusNotconnected
	client := &fakeSSM{statuses: []string{connected, notConnected, connected, notConnected, notConnected, notConnected}}
	m := &agentConnectionMonitor{client: client, instanceID: "i-123"}

	got := checkAll(m, 6)
	want := []bool{false, false, false, false, true, true}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("check %d: agentLost = %t, want %t (all results: %v)", i, got[i], want[i], got)
		}
	}
}

func TestAgentConnectionMonitor_AccessDeniedDisablesDetection(t *testing.T) {
	client := &fakeSSM{err: awserr.New("AccessDeniedException", "not authorized", nil)}
	m := &agentConnectionMonitor{client: client, instanceID: "i-123"}

	for i, lost := range checkAll(m, 3) {
		if lost {
			t.Fatalf("check %d: agentLost = true, want false when GetConnectionStatus is denied", i)
		}
	}
	if client.calls != 1 {
		t.Fatalf("GetConnectionStatus called %d times, want 1: detection must stop after AccessDenied", client.calls)
	}
}

func TestAgentConnectionMonitor_TransientErrorDoesNotRestartSession(t *testing.T) {
	client := &fakeSSM{err: errors.New("throttled")}
	m := &agentConnectionMonitor{client: client, instanceID: "i-123"}

	for i, lost := range checkAll(m, 3) {
		if lost {
			t.Fatalf("check %d: agentLost = true, want false on API errors", i)
		}
	}
	if client.calls != 3 {
		t.Fatalf("GetConnectionStatus called %d times, want 3: transient errors must not disable detection", client.calls)
	}
}

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
