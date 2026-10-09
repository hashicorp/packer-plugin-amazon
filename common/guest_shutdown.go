// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/random"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

// GuestShutdownConfirmedKey is only set after a fresh Linux power-off message
// and a check that no stopped-only instance attribute changes are required.
const GuestShutdownConfirmedKey = "amazon_guest_shutdown_confirmed"

var kernelConsoleLine = regexp.MustCompile(`^\[\s*[0-9]+\.[0-9]+\]\s+(.*)$`)

func guestShutdownConfirmed(output, token string) bool {
	if token == "" {
		return false
	}
	marked, confirmed := false, false
	for _, line := range strings.Split(output, "\n") {
		match := kernelConsoleLine.FindStringSubmatch(strings.TrimSuffix(line, "\r"))
		if match == nil {
			continue
		}
		message := match[1]
		if message == token {
			marked = true
		}
		if strings.HasPrefix(message, "Linux version ") || strings.HasPrefix(message, "reboot: Restarting system") {
			marked, confirmed = false, false
		}
		if marked && message == "reboot: Power down" {
			confirmed = true
		}
	}
	return confirmed
}

// Write a per-stop nonce before sending StopInstances. Merely finding a poweroff
// line in console output is unsafe: EC2 may return output from an earlier boot.
// A failed probe leaves the normal EC2 stopped-state waiter in place.
func prepareGuestShutdownProbe(ctx context.Context, state multistep.StateBag) string {
	instance := state.Get("instance").(ec2types.Instance)
	if instance.Platform == ec2types.PlatformValuesWindows {
		return ""
	}
	comm, ok := state.Get("communicator").(packersdk.Communicator)
	if !ok {
		return ""
	}
	token := "packer-shutdown-" + random.AlphaNum(32)
	// /dev/kmsg requires root; noninteractive sudo supports the normal Linux
	// image-building user. Priority 0 makes the one-line marker visible even
	// when console_loglevel suppresses informational kernel messages.
	write := fmt.Sprintf("printf \"<0>%s\\n\" > /dev/kmsg", token)
	cmd := &packersdk.RemoteCmd{Command: fmt.Sprintf("if [ \"$(id -u)\" = 0 ]; then %s; else sudo -n sh -c '%s'; fi", write, write)}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ui := state.Get("ui").(packersdk.Ui)
	if err := cmd.RunWithUi(probeCtx, comm, ui); err != nil || cmd.ExitStatus() != 0 {
		ui.Say("Guest shutdown probe unavailable; waiting for EC2 stopped state.")
		return ""
	}
	return token
}

func (s *StepStopEBSBackedInstance) guestShutdownCanFinish(instance ec2types.Instance) bool {
	if s.EnableAMIENASupport != config.TriUnset && (instance.EnaSupport == nil || *instance.EnaSupport != s.EnableAMIENASupport.True()) {
		return false
	}
	return !s.EnableAMISriovNetSupport || aws.ToString(instance.SriovNetSupport) == "simple"
}

func (s *StepStopEBSBackedInstance) waitForGuestShutdown(ctx context.Context, client clients.Ec2Client, id, token string) (bool, error) {
	options := s.PollingConfig.getWaiterOptions()
	maxWait := AwsDefaultMaxWaitTimeDuration
	if options.MaxWaitTime != nil {
		maxWait = *options.MaxWaitTime
	}
	delay := 15 * time.Second
	if options.MinDelay != nil {
		delay = *options.MinDelay
	}
	waitCtx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()
	for {
		result, err := client.DescribeInstances(waitCtx, &ec2.DescribeInstancesInput{InstanceIds: []string{id}})
		if err != nil {
			return false, err
		}
		if result == nil || len(result.Reservations) != 1 || len(result.Reservations[0].Instances) != 1 {
			return false, fmt.Errorf("unexpected instance response while waiting for %s to stop", id)
		}
		instance := result.Reservations[0].Instances[0]
		if instance.State == nil {
			return false, fmt.Errorf("missing instance state for %s", id)
		}
		switch instance.State.Name {
		case ec2types.InstanceStateNameStopped:
			return false, nil
		case ec2types.InstanceStateNameStopping:
			if token != "" && s.EnableAMISriovNetSupport && instance.SriovNetSupport == nil {
				// DescribeInstances can omit SR-IOV even when it is enabled.
				attribute, err := client.DescribeInstanceAttribute(waitCtx, &ec2.DescribeInstanceAttributeInput{InstanceId: aws.String(id), Attribute: ec2types.InstanceAttributeNameSriovNetSupport})
				if err != nil {
					log.Printf("Guest shutdown attribute check unavailable; waiting for EC2 stopped state: %s", err)
					token = ""
				} else if attribute != nil && attribute.SriovNetSupport != nil {
					instance.SriovNetSupport = attribute.SriovNetSupport.Value
				}
			}
			if token != "" && s.guestShutdownCanFinish(instance) {
				console, err := client.GetConsoleOutput(waitCtx, &ec2.GetConsoleOutputInput{InstanceId: aws.String(id), Latest: aws.Bool(true)})
				if err != nil {
					log.Printf("Guest shutdown console unavailable; waiting for EC2 stopped state: %s", err)
					token = ""
				} else if console != nil && console.Output != nil {
					output, err := base64.StdEncoding.DecodeString(*console.Output)
					if err := waitCtx.Err(); err != nil {
						return false, err
					}
					if err == nil && guestShutdownConfirmed(string(output), token) {
						return true, nil
					}
				}
			}
		case ec2types.InstanceStateNameRunning, ec2types.InstanceStateNamePending:
			// StopInstances and DescribeInstances are eventually consistent.
		default:
			return false, fmt.Errorf("instance %s entered %s while waiting for shutdown", id, instance.State.Name)
		}
		timer := time.NewTimer(delay)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return false, waitCtx.Err()
		case <-timer.C:
		}
	}
}
