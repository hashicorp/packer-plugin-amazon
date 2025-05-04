// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type mockEC2KeyPairConn struct {
	ec2iface.EC2API

	CreateKeyPairCount int
	CreateKeyPairArgs  []ec2.CreateKeyPairInput

	lock sync.Mutex
}

func (m *mockEC2KeyPairConn) CreateKeyPair(keyPairInput *ec2.CreateKeyPairInput) (*ec2.CreateKeyPairOutput, error) {
	m.lock.Lock()
	m.CreateKeyPairCount++
	m.CreateKeyPairArgs = append(m.CreateKeyPairArgs, *keyPairInput)
	m.lock.Unlock()
	keyMaterial := "I'm a cool key"
	output := &ec2.CreateKeyPairOutput{
		KeyMaterial: &keyMaterial,
	}
	return output, nil
}

func getKeyPairMockConn() ec2iface.EC2API {
	return &mockEC2KeyPairConn{}
}

func keyPairState() multistep.StateBag {
	state := new(multistep.BasicStateBag)
	state.Put("ui", &packersdk.BasicUi{
		Reader: new(bytes.Buffer),
		Writer: new(bytes.Buffer),
	})
	conn := getKeyPairMockConn()
	state.Put("ec2", conn)
	state.Put("region", aws.String("us-east-1"))
	return state
}

func TestStepKeyPair_withDefault(t *testing.T) {
	testSSHTemporaryKeyPair := communicator.SSHTemporaryKeyPair{SSHTemporaryKeyPairType: "rsa"}
	testSSH := communicator.SSH{
		SSHTemporaryKeyPairName: "temp-key-name",
		SSHTemporaryKeyPair:     testSSHTemporaryKeyPair,
	}
	comm := communicator.Config{
		SSH: testSSH,
	}
	stepKeyPair := StepKeyPair{
		Debug: false,
		Comm:  &comm,
	}

	state := keyPairState()
	stepKeyPair.Run(context.Background(), state)
	createKeyPairCallCount := state.Get("ec2").(*mockEC2KeyPairConn).CreateKeyPairCount
	createKeyPairArgs := state.Get("ec2").(*mockEC2KeyPairConn).CreateKeyPairArgs
	if createKeyPairCallCount != 1 {
		t.Fatalf("Expected CreateKeyPair to be called %d times, was called %d times", 1, createKeyPairCallCount)
	}
	if *createKeyPairArgs[0].KeyName != "temp-key-name" {
		t.Fatalf("Unexpected Key Type expected %s, got %s", "temp-key-name", *createKeyPairArgs[0].KeyName)
	}

	if *createKeyPairArgs[0].KeyType != "rsa" {
		t.Fatalf("Expeccted KeyType %s got %s", "rsa", *createKeyPairArgs[0].KeyType)
	}
}
