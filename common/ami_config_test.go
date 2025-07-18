// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

func testAMIConfig() *AMIConfig {
	return &AMIConfig{
		AMIName: "foo",
	}
}

func getFakeAccessConfig(region string) *AccessConfig {
	c := FakeAccessConfig()
	c.RawRegion = region
	return c
}

func TestAMIConfigPrepare_name(t *testing.T) {
	c := testAMIConfig()
	accessConf := FakeAccessConfig()
	if err := c.Prepare(accessConf, nil); err != nil {
		t.Fatalf("shouldn't have err: %s", err)
	}

	c.AMIName = ""
	if err := c.Prepare(accessConf, nil); err == nil {
		t.Fatal("should have error")
	}
}

func TestAMIConfigPrepare_regions(t *testing.T) {
	c := testAMIConfig()
	c.AMIRegions = nil

	var errs []error
	var err error
	accessConf := FakeAccessConfig()
	mockClient := &mockEC2Client{}
	if errs = c.prepareRegions(accessConf); len(errs) > 0 {
		t.Fatalf("shouldn't have err: %#v", errs)
	}

	c.AMIRegions, err = listEC2Regions(context.TODO(), mockClient)
	if err != nil {
		t.Fatalf("shouldn't have err: %s", err.Error())
	}
	if errs = c.prepareRegions(accessConf); len(errs) > 0 {
		t.Fatalf("shouldn't have err: %#v", errs)
	}

	c.AMIRegions = []string{"us-east-1", "us-west-1", "us-east-1"}
	if errs = c.prepareRegions(accessConf); len(errs) > 0 {
		t.Fatalf("bad: %s", errs[0])
	}

	expected := []string{"us-east-1", "us-west-1"}
	if !reflect.DeepEqual(c.AMIRegions, expected) {
		t.Fatalf("bad: %#v", c.AMIRegions)
	}

	c.AMIRegions = []string{"custom"}
	if errs = c.prepareRegions(accessConf); len(errs) > 0 {
		t.Fatal("shouldn't have error")
	}

	c.AMIRegions = []string{"us-east-1", "us-east-2", "us-west-1"}
	c.AMIRegionKMSKeyIDs = map[string]string{
		"us-east-1": "123-456-7890",
		"us-west-1": "789-012-3456",
		"us-east-2": "456-789-0123",
	}
	if errs = c.prepareRegions(accessConf); len(errs) > 0 {
		t.Fatalf("shouldn't have error: %s", errs[0])
	}

	c.AMIRegions = []string{"us-east-1", "us-east-2", "us-west-1"}
	c.AMIRegionKMSKeyIDs = map[string]string{
		"us-east-1": "123-456-7890",
		"us-west-1": "789-012-3456",
		"us-east-2": "",
	}
	if errs = c.prepareRegions(accessConf); len(errs) > 0 {
		t.Fatal("should have passed; we are able to use default KMS key if not sharing")
	}

	c.SnapshotUsers = []string{"user-foo", "user-bar"}
	c.AMIRegions = []string{"us-east-1", "us-east-2", "us-west-1"}
	c.AMIRegionKMSKeyIDs = map[string]string{
		"us-east-1": "123-456-7890",
		"us-west-1": "789-012-3456",
		"us-east-2": "",
	}
	if errs = c.prepareRegions(accessConf); len(errs) > 0 {
		t.Fatal("should have an error b/c can't use default KMS key if sharing")
	}

	c.AMIRegions = []string{"us-east-1", "us-west-1"}
	c.AMIRegionKMSKeyIDs = map[string]string{
		"us-east-1": "123-456-7890",
		"us-west-1": "789-012-3456",
		"us-east-2": "456-789-0123",
	}
	if errs = c.prepareRegions(accessConf); len(errs) > 0 {
		t.Fatal("should have error b/c theres a region in the key map that isn't in ami_regions")
	}

	c.AMIRegions = []string{"us-east-1", "us-west-1", "us-east-2"}
	c.AMIRegionKMSKeyIDs = map[string]string{
		"us-east-1": "123-456-7890",
		"us-west-1": "789-012-3456",
	}

	if err := c.Prepare(accessConf, nil); err == nil {
		t.Fatal("should have error b/c theres a region in in ami_regions that isn't in the key map")
	}

	c.SnapshotUsers = []string{"foo", "bar"}
	c.AMIKmsKeyId = "123-abc-456"
	c.AMIEncryptBootVolume = config.TriTrue
	c.AMIRegions = []string{"us-east-1", "us-west-1"}
	c.AMIRegionKMSKeyIDs = map[string]string{
		"us-east-1": "123-456-7890",
		"us-west-1": "",
	}
	if errs = c.prepareRegions(accessConf); len(errs) > 0 {
		t.Fatal("should have error b/c theres a region in in ami_regions that isn't in the key map")
	}

	// allow rawregion to exist in ami_regions list.
	accessConf = getFakeAccessConfig("us-east-1")
	c.AMIRegions = []string{"us-east-1", "us-west-1", "us-east-2"}
	c.AMIRegionKMSKeyIDs = nil
	if errs = c.prepareRegions(accessConf); len(errs) > 0 {
		t.Fatal("should allow user to have the raw region in ami_regions")
	}

}

func TestAMIConfigPrepare_Share_EncryptedBoot(t *testing.T) {
	c := testAMIConfig()
	c.AMIUsers = []string{"testAccountID"}
	c.AMIEncryptBootVolume = config.TriTrue

	accessConf := FakeAccessConfig()

	c.AMIKmsKeyId = ""
	if err := c.Prepare(accessConf, nil); err == nil {
		t.Fatal("shouldn't be able to share ami with encrypted boot volume")
	}
	c.AMIKmsKeyId = "89c3fb9a-de87-4f2a-aedc-fddc5138193c"
	if err := c.Prepare(accessConf, nil); err != nil {
		t.Fatal("should be able to share ami with encrypted boot volume")
	}

	c = testAMIConfig()
	c.AMIOrgArns = []string{"arn:aws:organizations::111122223333:organization/o-123example"}
	c.AMIEncryptBootVolume = config.TriTrue

	accessConf = FakeAccessConfig()

	c.AMIKmsKeyId = ""
	if err := c.Prepare(accessConf, nil); err == nil {
		t.Fatal("shouldn't be able to share ami with encrypted boot volume")
	}
	c.AMIKmsKeyId = "89c3fb9a-de87-4f2a-aedc-fddc5138193c"
	if err := c.Prepare(accessConf, nil); err != nil {
		t.Fatal("should be able to share ami with encrypted boot volume")
	}

	c = testAMIConfig()
	c.AMIOuArns = []string{"arn:aws:organizations::111122223333:ou/o-123example/ou-1234-5example"}
	c.AMIEncryptBootVolume = config.TriTrue

	accessConf = FakeAccessConfig()

	c.AMIKmsKeyId = ""
	if err := c.Prepare(accessConf, nil); err == nil {
		t.Fatal("shouldn't be able to share ami with encrypted boot volume")
	}
	c.AMIKmsKeyId = "89c3fb9a-de87-4f2a-aedc-fddc5138193c"
	if err := c.Prepare(accessConf, nil); err != nil {
		t.Fatal("should be able to share ami with encrypted boot volume")
	}
}

func TestAMIConfigPrepare_ValidateKmsKey(t *testing.T) {
	c := testAMIConfig()
	c.AMIEncryptBootVolume = config.TriTrue

	accessConf := FakeAccessConfig()

	validCases := []string{
		"abcd1234-e567-890f-a12b-a123b4cd56ef",
		"mrk-f4224f9362ac4ed2b32a6bc77cf43510",
		"alias/foo/bar",
		"arn:aws:kms:us-east-1:012345678910:key/abcd1234-a123-456a-a12b-a123b4cd56ef",
		"arn:aws:kms:us-east-1:012345678910:alias/foo/bar",
		"arn:aws:kms:us-east-1:012345678910:key/mrk-12345678-1234-abcd-0000-123456789012",
		"arn:aws:kms:us-east-1:012345678910:key/mrk-f4224f9362ac4ed2b32a6bc77cf43510",
		"arn:aws-us-gov:kms:us-gov-east-1:123456789012:key/12345678-1234-abcd-0000-123456789012",
		"arn:aws-cn:kms:cn-north-1:012345678910:alias/my-alias",
	}
	for _, validCase := range validCases {
		c.AMIKmsKeyId = validCase
		if err := c.Prepare(accessConf, nil); err != nil {
			t.Fatalf("%s should not have failed KMS key validation", validCase)
		}
	}

	invalidCases := []string{
		"ABCD1234-e567-890f-a12b-a123b4cd56ef",
		"ghij1234-e567-890f-a12b-a123b4cd56ef",
		"ghij1234+e567_890f-a12b-a123b4cd56ef",
		"mrk-ghij1234+e567a12ba123b4cd56ef",
		"mrk--f4224f9362ac4ed2b32a6bc77cf43510",
		"foo/bar",
		"arn:aws:kms:us-east-1:012345678910:foo/bar",
		"arn:aws:kms:us-east-1:012345678910:key/zab-12345678-1234-abcd-0000-123456789012",
		"arn:aws:kms:us-east-1:012345678910:key/mkr-12345678-1234-abcd-0000-123456789012",
		"arn:foo:kms:us-east-1:012345678910:key/abcd1234-a123-456a-a12b-a123b4cd56ef",
		"arn:aws-gov:kms:cn-north-1:012345678910:alias/my-alias",
	}
	for _, invalidCase := range invalidCases {
		c.AMIKmsKeyId = invalidCase
		if err := c.Prepare(accessConf, nil); err == nil {
			t.Fatalf("%s should have failed KMS key validation", invalidCase)
		}
	}

}

func TestAMINameValidation(t *testing.T) {
	c := testAMIConfig()

	accessConf := FakeAccessConfig()

	c.AMIName = "aa"
	if err := c.Prepare(accessConf, nil); err == nil {
		t.Fatal("shouldn't be able to have an ami name with less than 3 characters")
	}

	var longAmiName string
	for range 129 {
		longAmiName += "a"
	}
	c.AMIName = longAmiName
	if err := c.Prepare(accessConf, nil); err == nil {
		t.Fatal("shouldn't be able to have an ami name with great than 128 characters")
	}

	c.AMIName = "+aaa"
	if err := c.Prepare(accessConf, nil); err == nil {
		t.Fatal("shouldn't be able to have an ami name with invalid characters")
	}

	c.AMIName = "fooBAR1()[] ./-'@_"
	if err := c.Prepare(accessConf, nil); err != nil {
		t.Fatal("should be able to use all of the allowed AMI characters")
	}

	c.AMIName = `xyz-base-2017-04-05-1934`
	if err := c.Prepare(accessConf, nil); err != nil {
		t.Fatalf("expected `xyz-base-2017-04-05-1934` to pass validation.")
	}

}

func TestEnableDeregistrationProtection(t *testing.T) {
	c := testAMIConfig()

	accessConf := FakeAccessConfig()

	c.DeregistrationProtection = DeregistrationProtectionOptions{
		Enabled: true,
	}
	if err := c.Prepare(accessConf, nil); err != nil {
		t.Fatal("expected simple enabled case to pass validation")
	}

	c.DeregistrationProtection = DeregistrationProtectionOptions{
		Enabled:      true,
		WithCooldown: true,
	}
	if err := c.Prepare(accessConf, nil); err != nil {
		t.Fatal("expected with cooldown case to pass validation")
	}

	c.DeregistrationProtection = DeregistrationProtectionOptions{
		WithCooldown: true,
	}
	if err := c.Prepare(accessConf, nil); err != nil {
		t.Fatal("expected forgot enabled but have cooldown case to pass validation")
	}
	if !c.DeregistrationProtection.Enabled {
		t.Fatal("expected setting cooldown must also enabled")
	}
}
