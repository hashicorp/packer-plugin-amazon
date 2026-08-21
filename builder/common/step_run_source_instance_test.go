// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// fallbackEC2Mock records the instance types RunInstances is called with and
// returns a canned response per instance type.
type fallbackEC2Mock struct {
	ec2iface.EC2API
	responses map[string]fallbackResp
	attempted []string
}

type fallbackResp struct {
	out *ec2.Reservation
	err error
}

func (m *fallbackEC2Mock) RunInstances(in *ec2.RunInstancesInput) (*ec2.Reservation, error) {
	it := aws.StringValue(in.InstanceType)
	m.attempted = append(m.attempted, it)
	r := m.responses[it]
	return r.out, r.err
}

func capacityErr() error {
	return capacityErrMsg("no capacity")
}

func capacityErrMsg(msg string) error {
	return awserr.New("InsufficientInstanceCapacity", msg, nil)
}

func nonCapacityErr() error {
	return awserr.New("AuthFailure", "not authorized", nil)
}

func reservationFor(instanceType string) *ec2.Reservation {
	return &ec2.Reservation{
		Instances: []*ec2.Instance{
			{
				InstanceId:   aws.String("i-" + instanceType),
				InstanceType: aws.String(instanceType),
			},
		},
	}
}

func TestRunInstanceWithFallback_FirstSucceeds(t *testing.T) {
	mock := &fallbackEC2Mock{responses: map[string]fallbackResp{
		"mac2.metal": {out: reservationFor("mac2.metal")},
	}}

	resp, err := runInstanceWithFallback(context.Background(), mock,
		&ec2.RunInstancesInput{}, []string{"mac2.metal", "mac2-m2.metal"}, packersdk.TestUi(t))

	if err != nil {
		t.Fatalf("expected success, got error: %s", err)
	}
	if got := aws.StringValue(resp.Instances[0].InstanceType); got != "mac2.metal" {
		t.Fatalf("expected reservation for mac2.metal, got %s", got)
	}
	if want := []string{"mac2.metal"}; !reflect.DeepEqual(mock.attempted, want) {
		t.Fatalf("expected only the first type attempted %v, got %v", want, mock.attempted)
	}
}

func TestRunInstanceWithFallback_FallsThroughOnCapacity(t *testing.T) {
	mock := &fallbackEC2Mock{responses: map[string]fallbackResp{
		"mac2.metal":    {err: capacityErr()},
		"mac2-m2.metal": {out: reservationFor("mac2-m2.metal")},
	}}

	resp, err := runInstanceWithFallback(context.Background(), mock,
		&ec2.RunInstancesInput{}, []string{"mac2.metal", "mac2-m2.metal", "mac-m4.metal"}, packersdk.TestUi(t))

	if err != nil {
		t.Fatalf("expected success on second type, got error: %s", err)
	}
	if got := aws.StringValue(resp.Instances[0].InstanceType); got != "mac2-m2.metal" {
		t.Fatalf("expected reservation for mac2-m2.metal, got %s", got)
	}
	if want := []string{"mac2.metal", "mac2-m2.metal"}; !reflect.DeepEqual(mock.attempted, want) {
		t.Fatalf("expected first two types attempted %v, got %v", want, mock.attempted)
	}
}

func TestRunInstanceWithFallback_AllExhaustedReturnsLastError(t *testing.T) {
	mock := &fallbackEC2Mock{responses: map[string]fallbackResp{
		"mac2.metal":    {err: capacityErrMsg("first type full")},
		"mac2-m2.metal": {err: capacityErrMsg("last type full")},
	}}

	_, err := runInstanceWithFallback(context.Background(), mock,
		&ec2.RunInstancesInput{}, []string{"mac2.metal", "mac2-m2.metal"}, packersdk.TestUi(t))

	if err == nil {
		t.Fatal("expected error when all types are capacity-exhausted")
	}
	if !strings.Contains(err.Error(), "last type full") {
		t.Fatalf("expected the last error to be returned, got: %s", err)
	}
	if want := []string{"mac2.metal", "mac2-m2.metal"}; !reflect.DeepEqual(mock.attempted, want) {
		t.Fatalf("expected all types attempted %v, got %v", want, mock.attempted)
	}
}

func TestRunInstanceWithFallback_SingleTypeSucceeds(t *testing.T) {
	mock := &fallbackEC2Mock{responses: map[string]fallbackResp{
		"mac2.metal": {out: reservationFor("mac2.metal")},
	}}

	resp, err := runInstanceWithFallback(context.Background(), mock,
		&ec2.RunInstancesInput{}, []string{"mac2.metal"}, packersdk.TestUi(t))

	if err != nil {
		t.Fatalf("expected success, got error: %s", err)
	}
	if got := aws.StringValue(resp.Instances[0].InstanceType); got != "mac2.metal" {
		t.Fatalf("expected reservation for mac2.metal, got %s", got)
	}
	if want := []string{"mac2.metal"}; !reflect.DeepEqual(mock.attempted, want) {
		t.Fatalf("expected the single type attempted %v, got %v", want, mock.attempted)
	}
}

func TestRunInstanceWithFallback_SingleTypeCapacityErrorReturned(t *testing.T) {
	mock := &fallbackEC2Mock{responses: map[string]fallbackResp{
		"mac2.metal": {err: capacityErr()},
	}}

	_, err := runInstanceWithFallback(context.Background(), mock,
		&ec2.RunInstancesInput{}, []string{"mac2.metal"}, packersdk.TestUi(t))

	if err == nil {
		t.Fatal("a capacity error on a lone type must propagate, not be swallowed")
	}
	if want := []string{"mac2.metal"}; !reflect.DeepEqual(mock.attempted, want) {
		t.Fatalf("expected the single type attempted %v, got %v", want, mock.attempted)
	}
}

func TestRunInstanceWithFallback_NonCapacityAbortsImmediately(t *testing.T) {
	mock := &fallbackEC2Mock{responses: map[string]fallbackResp{
		"mac2.metal":    {err: nonCapacityErr()},
		"mac2-m2.metal": {out: reservationFor("mac2-m2.metal")},
	}}

	_, err := runInstanceWithFallback(context.Background(), mock,
		&ec2.RunInstancesInput{}, []string{"mac2.metal", "mac2-m2.metal"}, packersdk.TestUi(t))

	if err == nil {
		t.Fatal("expected the non-capacity error to be returned")
	}
	if want := []string{"mac2.metal"}; !reflect.DeepEqual(mock.attempted, want) {
		t.Fatalf("expected abort after first type %v, got %v", want, mock.attempted)
	}
}
