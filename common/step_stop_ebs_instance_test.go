// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

type shutdownClient struct {
	clients.Ec2Client
	attributeErr               error
	instance                   types.Instance
	comm                       *packersdk.MockCommunicator
	stopErr, consoleErr        error
	stops, describes, consoles int
	stale                      bool
	stopAfter, modifies        int
	cancel                     context.CancelFunc
}

func (m *shutdownClient) StopInstances(context.Context, *ec2.StopInstancesInput, ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error) {
	m.stops++
	return &ec2.StopInstancesOutput{}, m.stopErr
}
func (m *shutdownClient) DescribeInstances(ctx context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	m.describes++
	if m.stopAfter > 0 && m.describes >= m.stopAfter {
		m.instance.State.Name = types.InstanceStateNameStopped
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return &ec2.DescribeInstancesOutput{Reservations: []types.Reservation{{Instances: []types.Instance{m.instance}}}}, nil
}
func (m *shutdownClient) GetConsoleOutput(_ context.Context, input *ec2.GetConsoleOutputInput, _ ...func(*ec2.Options)) (*ec2.GetConsoleOutputOutput, error) {
	m.consoles++
	if !aws.ToBool(input.Latest) {
		panic("must request latest console")
	}
	if m.cancel != nil {
		m.cancel()
	}
	if m.consoleErr != nil {
		return nil, m.consoleErr
	}
	token := "old-token"
	if !m.stale {
		token = regexp.MustCompile(`packer-shutdown-[A-Za-z0-9]+`).FindString(m.comm.StartCmd.Command)
	}
	output := "[ 10.000] " + token + "\n[ 12.000] reboot: Power down\n"
	return &ec2.GetConsoleOutputOutput{Output: aws.String(base64.StdEncoding.EncodeToString([]byte(output)))}, nil
}
func shutdownState(client *shutdownClient) *multistep.BasicStateBag {
	state := new(multistep.BasicStateBag)
	state.Put("ec2v2", client)
	state.Put("instance", client.instance)
	state.Put("communicator", client.comm)
	state.Put("ui", new(packersdk.MockUi))
	return state
}
func TestStepStopEBSBackedInstance(t *testing.T) {
	for _, tc := range []struct {
		name                                                                                   string
		enabled, stopping, probeFail, stopFail, stale, denyConsole, attributeMismatch, windows bool
		wantAction                                                                             multistep.StepAction
		wantEarly                                                                              bool
	}{
		{name: "default", wantAction: multistep.ActionContinue},
		{name: "confirmed", enabled: true, stopping: true, wantAction: multistep.ActionContinue, wantEarly: true},
		{name: "already stopped", enabled: true, wantAction: multistep.ActionContinue},
		{name: "probe fails", enabled: true, probeFail: true, wantAction: multistep.ActionContinue},
		{name: "stop fails", enabled: true, stopFail: true, wantAction: multistep.ActionHalt},
		{name: "stale console", enabled: true, stopping: true, stale: true, wantAction: multistep.ActionHalt},
		{name: "console denied", enabled: true, stopping: true, denyConsole: true, wantAction: multistep.ActionHalt},
		{name: "attribute change waits", enabled: true, stopping: true, attributeMismatch: true, wantAction: multistep.ActionHalt},
		{name: "windows falls back", enabled: true, windows: true, wantAction: multistep.ActionContinue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instance := types.Instance{InstanceId: aws.String("i-test"), State: &types.InstanceState{Name: types.InstanceStateNameStopped}, EnaSupport: aws.Bool(true), SriovNetSupport: aws.String("simple")}
			if tc.stopping {
				instance.State.Name = types.InstanceStateNameStopping
			}
			if tc.windows {
				instance.Platform = types.PlatformValuesWindows
			}
			comm := new(packersdk.MockCommunicator)
			if tc.probeFail {
				comm.StartExitStatus = 1
			}
			client := &shutdownClient{instance: instance, comm: comm, stale: tc.stale}
			if tc.stopFail {
				client.stopErr = errors.New("stop denied")
			}
			if tc.denyConsole {
				client.consoleErr = errors.New("console denied")
			}
			step := &StepStopEBSBackedInstance{PollingConfig: &AWSPollingConfig{}, WaitForGuestShutdown: tc.enabled, EnableAMIENASupport: config.TriTrue, EnableAMISriovNetSupport: true}
			if tc.attributeMismatch {
				step.EnableAMIENASupport = config.TriFalse
			}
			state := shutdownState(client)
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			if got := step.Run(ctx, state); got != tc.wantAction {
				t.Fatalf("action=%v want %v; error=%v", got, tc.wantAction, state.Get("error"))
			}
			early, _ := state.Get(GuestShutdownConfirmedKey).(bool)
			if early != tc.wantEarly {
				t.Fatalf("early=%t want %t", early, tc.wantEarly)
			}
			if client.stops != 1 {
				t.Fatalf("StopInstances calls=%d", client.stops)
			}
			if !tc.enabled && (comm.StartCalled || client.consoles != 0) {
				t.Fatal("default behavior must not probe or read console")
			}
			if tc.attributeMismatch && client.consoles != 0 {
				t.Fatal("cannot overlap when attributes need changing")
			}
		})
	}
}
func TestGuestShutdownCancellation(t *testing.T) {
	comm := new(packersdk.MockCommunicator)
	instance := types.Instance{InstanceId: aws.String("i-test"), State: &types.InstanceState{Name: types.InstanceStateNameStopping}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &shutdownClient{instance: instance, comm: comm, cancel: cancel}
	state := shutdownState(client)
	step := &StepStopEBSBackedInstance{PollingConfig: &AWSPollingConfig{}, WaitForGuestShutdown: true}
	if step.Run(ctx, state) != multistep.ActionHalt {
		t.Fatal("cancelled wait continued")
	}
	if state.Get(GuestShutdownConfirmedKey) != nil {
		t.Fatal("cancelled wait confirmed shutdown")
	}
}
func TestModifyInstanceAfterConfirmedShutdown(t *testing.T) {
	// No attribute mutation is allowed while the instance is stopping.
	client := &shutdownClient{instance: types.Instance{InstanceId: aws.String("i-test")}}
	state := shutdownState(client)
	state.Put(GuestShutdownConfirmedKey, true)
	step := &StepModifyEBSBackedInstance{EnableAMIENASupport: config.TriTrue, EnableAMISriovNetSupport: true}
	if step.Run(context.Background(), state) != multistep.ActionContinue || client.modifies != 0 {
		t.Fatal("did not skip unchanged instance attributes")
	}
}

func (m *shutdownClient) ModifyInstanceAttribute(context.Context, *ec2.ModifyInstanceAttributeInput, ...func(*ec2.Options)) (*ec2.ModifyInstanceAttributeOutput, error) {
	m.modifies++
	return &ec2.ModifyInstanceAttributeOutput{}, nil
}

func TestGuestShutdownFallbackCompletes(t *testing.T) {
	for _, reason := range []string{"stale console", "console denied", "attribute mismatch", "attribute query denied"} {
		t.Run(reason, func(t *testing.T) {
			instance := types.Instance{InstanceId: aws.String("i-test"), State: &types.InstanceState{Name: types.InstanceStateNameStopping}, EnaSupport: aws.Bool(true)}
			client := &shutdownClient{instance: instance, comm: new(packersdk.MockCommunicator), stopAfter: 2, stale: reason == "stale console"}
			if reason == "console denied" {
				client.consoleErr = errors.New("denied")
			}
			ena := config.TriTrue
			if reason == "attribute mismatch" {
				ena = config.TriFalse
			}
			step := &StepStopEBSBackedInstance{PollingConfig: &AWSPollingConfig{DelaySeconds: 1}, WaitForGuestShutdown: true, EnableAMIENASupport: ena}
			if reason == "attribute query denied" {
				step.EnableAMISriovNetSupport = true
				client.attributeErr = errors.New("attribute denied")
			}
			state := shutdownState(client)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if step.Run(ctx, state) != multistep.ActionContinue {
				t.Fatalf("fallback failed: %v", state.Get("error"))
			}
			if state.Get(GuestShutdownConfirmedKey) != nil {
				t.Fatal("fallback marked early shutdown")
			}
			modify := &StepModifyEBSBackedInstance{EnableAMIENASupport: ena, EnableAMISriovNetSupport: step.EnableAMISriovNetSupport}
			wantModifies := 1
			if step.EnableAMISriovNetSupport {
				wantModifies++
			}
			if modify.Run(ctx, state) != multistep.ActionContinue || client.modifies != wantModifies {
				t.Fatal("fallback skipped normal attribute update")
			}
		})
	}
}

func TestGuestShutdownAttributeRequirements(t *testing.T) {
	for _, tc := range []struct {
		name        string
		ena         config.Trilean
		actualENA   *bool
		sriov       bool
		actualSriov *string
		want        bool
	}{
		{name: "unset ENA", ena: config.TriUnset, want: true},
		{name: "disabled ENA matches", ena: config.TriFalse, actualENA: aws.Bool(false), want: true},
		{name: "missing ENA", ena: config.TriFalse, want: false},
		{name: "ENA mismatch", ena: config.TriFalse, actualENA: aws.Bool(true), want: false},
		{name: "SRIOV matches", sriov: true, actualSriov: aws.String("simple"), want: true},
		{name: "missing SRIOV", sriov: true, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			step := &StepStopEBSBackedInstance{EnableAMIENASupport: tc.ena, EnableAMISriovNetSupport: tc.sriov}
			if got := step.guestShutdownCanFinish(types.Instance{EnaSupport: tc.actualENA, SriovNetSupport: tc.actualSriov}); got != tc.want {
				t.Fatalf("got %t want %t", got, tc.want)
			}
		})
	}
}

func (m *shutdownClient) DescribeInstanceAttribute(_ context.Context, input *ec2.DescribeInstanceAttributeInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstanceAttributeOutput, error) {
	if aws.ToString(input.InstanceId) != aws.ToString(m.instance.InstanceId) || input.Attribute != types.InstanceAttributeNameSriovNetSupport {
		panic("unexpected attribute lookup")
	}
	if m.attributeErr != nil {
		return nil, m.attributeErr
	}
	return &ec2.DescribeInstanceAttributeOutput{SriovNetSupport: &types.AttributeValue{Value: aws.String("simple")}}, nil
}

func TestGuestShutdownQueriesMissingSRIOV(t *testing.T) {
	client := &shutdownClient{instance: types.Instance{InstanceId: aws.String("i-test"), State: &types.InstanceState{Name: types.InstanceStateNameStopping}}, comm: new(packersdk.MockCommunicator)}
	state := shutdownState(client)
	step := &StepStopEBSBackedInstance{PollingConfig: &AWSPollingConfig{}, WaitForGuestShutdown: true, EnableAMISriovNetSupport: true}
	if step.Run(context.Background(), state) != multistep.ActionContinue || state.Get(GuestShutdownConfirmedKey) != true {
		t.Fatalf("did not confirm actual SRIOV attribute: %v", state.Get("error"))
	}
}
