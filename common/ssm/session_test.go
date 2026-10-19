// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package ssm

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type fakeSSM struct {
	mu          sync.Mutex
	statuses    []ssmtypes.ConnectionStatus
	statusFn    func(sessionsStarted int) ssmtypes.ConnectionStatus
	err         error
	statusCalls int
	started     int
	terminated  []string
}

func (f *fakeSSM) GetConnectionStatus(_ context.Context, _ *ssm.GetConnectionStatusInput, _ ...func(*ssm.Options)) (*ssm.GetConnectionStatusOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusCalls++
	if f.err != nil {
		return nil, f.err
	}
	if f.statusFn != nil {
		return &ssm.GetConnectionStatusOutput{Status: f.statusFn(f.started)}, nil
	}
	status := f.statuses[0]
	f.statuses = f.statuses[1:]
	return &ssm.GetConnectionStatusOutput{Status: status}, nil
}

func (f *fakeSSM) StartSession(_ context.Context, _ *ssm.StartSessionInput, _ ...func(*ssm.Options)) (*ssm.StartSessionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started++
	return &ssm.StartSessionOutput{
		SessionId:  aws.String(fmt.Sprintf("session-%d", f.started)),
		StreamUrl:  aws.String("wss://example.invalid"),
		TokenValue: aws.String("token"),
	}, nil
}

func (f *fakeSSM) TerminateSession(_ context.Context, in *ssm.TerminateSessionInput, _ ...func(*ssm.Options)) (*ssm.TerminateSessionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminated = append(f.terminated, aws.ToString(in.SessionId))
	return &ssm.TerminateSessionOutput{}, nil
}

func (f *fakeSSM) snapshot() (started, statusCalls int, terminated []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.started, f.statusCalls, append([]string(nil), f.terminated...)
}

// runningInstanceEC2 reports the instance as running so Start never treats it as terminated.
type runningInstanceEC2 struct {
	clients.Ec2Client
}

func (runningInstanceEC2) DescribeInstanceStatus(_ context.Context, _ *ec2.DescribeInstanceStatusInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstanceStatusOutput, error) {
	return &ec2.DescribeInstanceStatusOutput{InstanceStatuses: []ec2types.InstanceStatus{{}}}, nil
}

func checkAll(m *agentConnectionMonitor, n int) []bool {
	got := make([]bool, n)
	for i := range got {
		got[i] = m.agentLost(context.Background())
	}
	return got
}

func TestAgentConnectionMonitor_RestartsOnlyAfterConsecutiveDisconnects(t *testing.T) {
	connected, notConnected := ssmtypes.ConnectionStatusConnected, ssmtypes.ConnectionStatusNotConnected
	client := &fakeSSM{statuses: []ssmtypes.ConnectionStatus{
		connected, notConnected, connected, notConnected, notConnected, notConnected,
	}}
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
	client := &fakeSSM{err: &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "not authorized"}}
	m := &agentConnectionMonitor{client: client, instanceID: "i-123"}

	for i, lost := range checkAll(m, 3) {
		if lost {
			t.Fatalf("check %d: agentLost = true, want false when GetConnectionStatus is denied", i)
		}
	}
	if client.statusCalls != 1 {
		t.Fatalf("GetConnectionStatus called %d times, want 1: detection must stop after AccessDenied", client.statusCalls)
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
	if client.statusCalls != 3 {
		t.Fatalf("GetConnectionStatus called %d times, want 3: transient errors must not disable detection", client.statusCalls)
	}
}

// startWithStuckPlugin runs Session.Start with a session-manager-plugin stand-in that never
// exits on its own, which is how the real plugin behaves after an instance reboot.
func startWithStuckPlugin(t *testing.T, client *fakeSSM, restart bool) {
	t.Helper()
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep binary not available")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sessionChan := make(chan struct{})
	go func() {
		for range sessionChan {
		}
	}()
	s := Session{
		SvcClient: client, InstanceID: "i-123", Region: "us-east-1", LocalPort: 8000, RemotePort: 22,
		Ec2Client:                runningInstanceEC2{},
		RestartOnAgentDisconnect: restart,
		agentCheckInterval:       20 * time.Millisecond,
		pluginCommand: func(ctx context.Context, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sleep", "600")
		},
	}
	go func() { _ = s.Start(ctx, packersdk.TestUi(t), sessionChan) }()
}

func agentDownDuringFirstSession(started int) ssmtypes.ConnectionStatus {
	if started == 1 {
		return ssmtypes.ConnectionStatusNotConnected
	}
	return ssmtypes.ConnectionStatusConnected
}

func TestSessionStart_RestartsSessionWhenAgentDisconnects(t *testing.T) {
	client := &fakeSSM{statusFn: agentDownDuringFirstSession}
	startWithStuckPlugin(t, client, true)

	deadline := time.Now().Add(10 * time.Second)
	for {
		started, _, terminated := client.snapshot()
		if started >= 2 && len(terminated) > 0 && terminated[0] == "session-1" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session was not restarted after the SSM agent disconnected (sessions started: %d, terminated: %v)", started, terminated)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSessionStart_DisabledByDefault(t *testing.T) {
	client := &fakeSSM{statusFn: agentDownDuringFirstSession}
	startWithStuckPlugin(t, client, false)

	time.Sleep(300 * time.Millisecond) // ~15 agent check intervals
	started, statusCalls, _ := client.snapshot()
	if started != 1 {
		t.Fatalf("started %d sessions with RestartOnAgentDisconnect unset, want 1", started)
	}
	if statusCalls != 0 {
		t.Fatalf("GetConnectionStatus called %d times with RestartOnAgentDisconnect unset, want 0", statusCalls)
	}
}

func TestSessionStart_KeepsSessionWhileAgentConnected(t *testing.T) {
	client := &fakeSSM{statusFn: func(int) ssmtypes.ConnectionStatus { return ssmtypes.ConnectionStatusConnected }}
	startWithStuckPlugin(t, client, true)

	time.Sleep(300 * time.Millisecond) // ~15 agent check intervals
	if started, statusCalls, _ := client.snapshot(); started != 1 || statusCalls == 0 {
		t.Fatalf("started %d sessions after %d status checks while the agent stayed connected, want 1 session and >0 checks", started, statusCalls)
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
