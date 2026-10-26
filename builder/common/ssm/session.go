// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package ssm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/hashicorp/packer-plugin-amazon/builder/common/awserrors"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/retry"
	"github.com/hashicorp/packer-plugin-sdk/shell-local/localexec"
)

// DefaultDocumentName is the AWS managed document used to start a port
// forwarding session when no custom document is specified.
const DefaultDocumentName = "AWS-StartPortForwardingSession"

type Session struct {
	SvcClient             ssmiface.SSMAPI
	Region                string
	InstanceID            string
	LocalPort, RemotePort int
	Ec2Conn               *ec2.EC2
	DocumentName          string
	// RestartOnAgentDisconnect restarts the port forwarding session when the SSM agent on
	// the instance disconnects from Session Manager, e.g. during a reboot.
	RestartOnAgentDisconnect bool
}

const (
	// How often the SSM agent connection is checked while a session is open.
	agentConnectionCheckInterval = 5 * time.Second
	// Consecutive "notconnected" reports required before a session is considered dead,
	// so a single stale status does not restart a healthy session.
	agentDisconnectThreshold = 2
)

// agentConnectionMonitor detects when the SSM agent on the instance disconnects from
// Session Manager, e.g. while the instance reboots. AWS does not support resuming a port
// forwarding session after a reboot, and session-manager-plugin does not reliably exit in
// that case, which leaves a local tunnel that accepts connections but forwards nothing.
type agentConnectionMonitor struct {
	client     ssmiface.SSMAPI
	instanceID string
	misses     int
	disabled   bool
}

// agentLost reports whether the agent has been seen disconnected enough times in a row
// to consider the current session dead.
func (m *agentConnectionMonitor) agentLost(ctx context.Context) bool {
	if m.disabled {
		return false
	}
	out, err := m.client.GetConnectionStatusWithContext(ctx, &ssm.GetConnectionStatusInput{Target: aws.String(m.instanceID)})
	if err != nil {
		if awserrors.Matches(err, "AccessDeniedException", "") {
			log.Printf("ssm: not allowed to call GetConnectionStatus, the session will not be restarted when the SSM agent disconnects: %s", err)
			m.disabled = true
		} else if ctx.Err() == nil {
			log.Printf("ssm: error getting connection status for instance %s: %s", m.instanceID, err)
		}
		return false
	}
	if aws.StringValue(out.Status) != ssm.ConnectionStatusNotconnected {
		m.misses = 0
		return false
	}
	m.misses++
	return m.misses >= agentDisconnectThreshold
}

func (s Session) buildTunnelInput() *ssm.StartSessionInput {
	portNumber, localPortNumber := strconv.Itoa(s.RemotePort), strconv.Itoa(s.LocalPort)
	params := map[string][]*string{
		"portNumber":      []*string{aws.String(portNumber)},
		"localPortNumber": []*string{aws.String(localPortNumber)},
	}

	documentName := s.DocumentName
	if documentName == "" {
		documentName = DefaultDocumentName
	}

	return &ssm.StartSessionInput{
		DocumentName: aws.String(documentName),
		Parameters:   params,
		Target:       aws.String(s.InstanceID),
	}
}

// getCommand return a valid ordered set of arguments to pass to the driver command.
func (s Session) getCommand(ctx context.Context) ([]string, string, error) {
	input := s.buildTunnelInput()

	var session *ssm.StartSessionOutput
	err := retry.Config{
		ShouldRetry: func(err error) bool { return awserrors.Matches(err, "TargetNotConnected", "") },
		RetryDelay:  (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 60 * time.Second, Multiplier: 2}).Linear,
	}.Run(ctx, func(ctx context.Context) (err error) {
		session, err = s.SvcClient.StartSessionWithContext(ctx, input)
		return err
	})

	if err != nil {
		return nil, "", err
	}

	if session == nil {
		return nil, "", fmt.Errorf("an active Amazon SSM Session is required before trying to open a session tunnel")
	}

	// AWS session-manager-plugin requires a valid session be passed in JSON.
	sessionDetails, err := json.Marshal(session)
	if err != nil {
		return nil, *session.SessionId, fmt.Errorf("error encountered in reading session details %s", err)
	}

	// AWS session-manager-plugin requires the parameters used in the session to be passed in JSON as well.
	sessionParameters, err := json.Marshal(input)
	if err != nil {
		return nil, "", fmt.Errorf("error encountered in reading session parameter details %s", err)
	}

	// Args must be in this order
	args := []string{
		string(sessionDetails),
		s.Region,
		"StartSession",
		"", // ProfileName
		string(sessionParameters),
		*session.StreamUrl,
	}
	return args, *session.SessionId, nil
}

// terminate an interactive Systems Manager session with a remote instance via the
// AWS session-manager-plugin. Session cannot be resumed after termination.
func (s Session) terminateSession(sessionID string, ui packersdk.Ui) {
	log.Printf("ssm: Terminating PortForwarding session %q", sessionID)
	_, err := s.SvcClient.TerminateSession(&ssm.TerminateSessionInput{SessionId: aws.String(sessionID)})
	if err != nil {
		ui.Error(fmt.Sprintf("Error terminating SSM Session %q, this does not affect the built AMI. Please terminate the session manually: %s", sessionID, err))
	}
}

// Start an interactive Systems Manager session with a remote instance via the
// AWS session-manager-plugin. To terminate the session you must cancel the
// context. If you do not wish to terminate the session manually: calling
// StopSession on a instance of this driver will terminate the active session
// created from calling StartSession.
// To stop the session you must cancel the context.
func (s Session) Start(ctx context.Context, ui packersdk.Ui, sessionChan chan struct{}) error {
	exitSession := false
	var monitor *agentConnectionMonitor
	if s.RestartOnAgentDisconnect {
		monitor = &agentConnectionMonitor{client: s.SvcClient, instanceID: s.InstanceID}
	}
	for ctx.Err() == nil && !exitSession {
		log.Printf("ssm: Starting PortForwarding session to instance %s", s.InstanceID)
		args, sessionID, err := s.getCommand(ctx)
		// Cancelling sessionCtx stops only the current session-manager-plugin process,
		// which lets the loop start a new session.
		sessionCtx, cancelSession := context.WithCancel(ctx)
		sessionFinished := make(chan struct{})
		defer close(sessionFinished)
		if sessionID != "" {
			// If the instance is terminated the session must be terminated as well
			// Otherwise the start-session command will exit with status -1
			go func() {
				timer := time.NewTicker(2 * time.Second)
				defer timer.Stop()
				// Stays nil, and never fires, unless RestartOnAgentDisconnect is set.
				var agentCheck <-chan time.Time
				if monitor != nil {
					ticker := time.NewTicker(agentConnectionCheckInterval)
					defer ticker.Stop()
					agentCheck = ticker.C
					monitor.misses = 0
				}
				for {
					select {
					// in cases where the session is terminated naturally
					// e.g. the RunAndStream command exits
					// session will be terminated but exitSession is not set to true
					// because the instance is still running and we might want to do a reconnect
					case <-sessionFinished:
						s.terminateSession(sessionID, ui)
						return
					case <-agentCheck:
						if sessionCtx.Err() == nil && monitor.agentLost(ctx) {
							ui.Say(fmt.Sprintf("ssm: SSM agent on instance %s disconnected (e.g. reboot), restarting PortForwarding session", s.InstanceID))
							cancelSession()
						}
					case <-timer.C: // wait for the session to be created
						instanceState, err := s.Ec2Conn.DescribeInstanceStatus(&ec2.DescribeInstanceStatusInput{
							InstanceIds: []*string{aws.String(s.InstanceID)},
						})
						if err != nil {
							log.Printf("ssm: Error describing instance status: %s", err)
						} else if instanceState != nil && len(instanceState.InstanceStatuses) == 0 {
							// if no instance status is returned, the instance is terminated
							exitSession = true
							s.terminateSession(sessionID, ui)
							return
						}
					case <-ctx.Done():
						s.terminateSession(sessionID, ui)
						return
					}
				}
			}()
		}
		if err != nil {
			cancelSession()
			return err
		}

		sessionChan <- struct{}{}
		cmd := exec.CommandContext(sessionCtx, "session-manager-plugin", args...)

		ui.Message(fmt.Sprintf("Starting portForwarding session %q.", sessionID))
		err = localexec.RunAndStream(cmd, ui, nil)
		sessionFinished <- struct{}{}
		restarted := sessionCtx.Err() != nil && ctx.Err() == nil
		cancelSession()
		if err != nil && !restarted {
			ui.Error(err.Error())
		}
	}
	ui.Say("ssm: PortForwarding session is finished")
	close(sessionChan)
	return nil
}
