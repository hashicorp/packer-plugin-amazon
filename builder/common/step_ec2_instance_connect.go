package common

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2instanceconnect"
	"github.com/hashicorp/packer-plugin-sdk/communicator/sshkey"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"golang.org/x/crypto/ssh/agent"
)

type StepEC2InstanceConnect struct {
	AWSSession    *session.Session
	Region        string
	SSHAgentAuth  bool
	SSHPrivateKey []byte
	SSHUsername   string
}

func (s *StepEC2InstanceConnect) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)

	// Get instance information
	instance, ok := state.Get("instance").(*ec2.Instance)
	if !ok {
		err := fmt.Errorf("error encountered in obtaining target instance id")
		ui.Error(err.Error())
		state.Put("error", err)
		return multistep.ActionHalt
	}

	var authorizedKeys []string
	if s.SSHAgentAuth {
		sshAuthSock, ok := os.LookupEnv("SSH_AUTH_SOCK")
		if !ok {
			err := fmt.Errorf("SSH_AUTH_SOCK is not defined")
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		conn, err := net.Dial("unix", sshAuthSock)
		if err != nil {
			err = fmt.Errorf("failed to open SSH_AUTH_SOCK: %v", err)
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}
		agentClient := agent.NewClient(conn)

		keys, err := agentClient.List()
		if err != nil {
			err = fmt.Errorf("failed to list keys from SSH_AUTH_SOCK: %v", err)
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}

		for _, key := range keys {
			publicKey := key.String()

			authorizedKeys = append(authorizedKeys, publicKey)
		}
	}

	if s.SSHPrivateKey != nil {
		publicKeyBytes, err := sshkey.PublicKeyFromPrivate(s.SSHPrivateKey)
		if err != nil {
			err := fmt.Errorf("error getting public key from private key: %v", err)
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}
		publicKey := string(publicKeyBytes)

		authorizedKeys = append(authorizedKeys, publicKey)
	}

	ui.Say("Waiting 1min before sending SSH Public Keys to EC2 Instance Connect...")
	select {
	case <-time.After(time.Minute):
		break
	case <-ctx.Done():
		return multistep.ActionHalt
	}

	ui.Say(fmt.Sprintf("Sending %d SSH Public Keys to EC2 Instance Connect...", len(authorizedKeys)))
	for i, publicKey := range authorizedKeys {
		err := s.sendSSHPublicKey(instance, publicKey)
		if err != nil {
			err := fmt.Errorf("error sending SSH Public Key %d to EC2 Instance Connect", i)
			ui.Error(err.Error())
			state.Put("error", err)
			return multistep.ActionHalt
		}
	}
	ui.Say(fmt.Sprintf("Sent %d SSH Public Keys to EC2 Instance Connect", len(authorizedKeys)))

	return multistep.ActionContinue
}

func (s *StepEC2InstanceConnect) Cleanup(state multistep.StateBag) {}

func (s *StepEC2InstanceConnect) sendSSHPublicKey(instance *ec2.Instance, publicKey string) error {
	input := &ec2instanceconnect.SendSSHPublicKeyInput{
		AvailabilityZone: aws.String(*instance.Placement.AvailabilityZone),
		InstanceId:       aws.String(*instance.InstanceId),
		InstanceOSUser:   aws.String(s.SSHUsername),
		SSHPublicKey:     aws.String(strings.TrimSpace(publicKey)),
	}

	log.Printf("Send SSH Public Key Input %s", input)

	output, err := ec2instanceconnect.New(s.AWSSession).SendSSHPublicKey(input)
	if err != nil {
		return err
	}

	log.Printf("Send SSH Public Key Output %s", output)

	if !aws.BoolValue(output.Success) {
		return fmt.Errorf("request %s did not succeed", aws.StringValue(output.RequestId))
	}

	return nil
}
