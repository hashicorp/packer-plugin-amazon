// Copyright (c) HashiCorp, Inc.
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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"

	"github.com/hashicorp/packer-plugin-amazon/common/awserrors"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/retry"
	"github.com/hashicorp/packer-plugin-sdk/shell-local/localexec"
)

type Session struct {
	SvcClient             *ssm.Client
	Region                string
	InstanceID            string
	LocalPort, RemotePort int
	Ec2Client             clients.Ec2Client
}

func (s Session) buildTunnelInput() *ssm.StartSessionInput {
	portNumber, localPortNumber := strconv.Itoa(s.RemotePort), strconv.Itoa(s.LocalPort)
	params := map[string][]string{
		"portNumber":      []string{portNumber},
		"localPortNumber": []string{localPortNumber},
	}

	return &ssm.StartSessionInput{
		DocumentName: aws.String("AWS-StartPortForwardingSession"),
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
		session, err = s.SvcClient.StartSession(ctx, input)
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
func (s Session) terminateSession(ctx context.Context, sessionID string, ui packersdk.Ui) {
	log.Printf("ssm: Terminating PortForwarding session %q", sessionID)
	_, err := s.SvcClient.TerminateSession(ctx, &ssm.TerminateSessionInput{SessionId: aws.String(sessionID)})
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
	for ctx.Err() == nil && !exitSession {
		log.Printf("ssm: Starting PortForwarding session to instance %s", s.InstanceID)
		args, sessionID, err := s.getCommand(ctx)
		sessionFinished := make(chan struct{})
		defer close(sessionFinished)
		if sessionID != "" {
			// If the instance is terminated the session must be terminated as well
			// Otherwise the start-session command will exit with status -1
			go func() {
				timer := time.NewTicker(2 * time.Second)
				defer timer.Stop()
				for {
					select {
					// in cases where the session is terminated naturally
					// e.g. the RunAndStream command exits
					// session will be terminated but exitSession is not set to true
					// because the instance is still running and we might want to do a reconnect
					case <-sessionFinished:
						s.terminateSession(ctx, sessionID, ui)
						return
					case <-timer.C: // wait for the session to be created
						instanceState, err := s.Ec2Client.DescribeInstanceStatus(ctx, &ec2.DescribeInstanceStatusInput{
							InstanceIds: []string{s.InstanceID},
						})
						if err != nil {
							log.Printf("ssm: Error describing instance status: %s", err)
						} else if instanceState != nil && len(instanceState.InstanceStatuses) == 0 {
							// if no instance status is returned, the instance is terminated
							exitSession = true
							s.terminateSession(ctx, sessionID, ui)
							return
						}
					case <-ctx.Done():
						s.terminateSession(ctx, sessionID, ui)
						return
					}
				}
			}()
		}
		if err != nil {
			return err
		}

		sessionChan <- struct{}{}
		cmd := exec.CommandContext(ctx, "session-manager-plugin", args...)

		ui.Say(fmt.Sprintf("Starting portForwarding session %q.", sessionID))
		err = localexec.RunAndStream(cmd, ui, nil)
		sessionFinished <- struct{}{}
		if err != nil {
			ui.Error(err.Error())
		}
	}
	ui.Say("ssm: PortForwarding session is finished")
	close(sessionChan)
	return nil
}
