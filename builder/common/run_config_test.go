// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"io/ioutil"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/communicator"
)

func init() {
	// Clear out the AWS access key env vars so they don't
	// affect our tests.
	os.Setenv("AWS_ACCESS_KEY_ID", "")
	os.Setenv("AWS_ACCESS_KEY", "")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "")
	os.Setenv("AWS_SECRET_KEY", "")
}

func testConfig() *RunConfig {
	return &RunConfig{
		SourceAmi:    "abcd",
		InstanceType: "m1.small",

		Comm: communicator.Config{
			SSH: communicator.SSH{
				SSHUsername: "foo",
			},
		},
	}
}

func testConfigFilter() *RunConfig {
	config := testConfig()
	config.SourceAmi = ""
	config.SourceAmiFilter = AmiFilterOptions{}
	return config
}

func TestRunConfigPrepare(t *testing.T) {
	c := testConfig()
	err := c.Prepare(nil)
	if len(err) > 0 {
		t.Fatalf("err: %s", err)
	}
}

func TestRunConfigPrepare_InstanceType(t *testing.T) {
	c := testConfig()
	c.InstanceType = ""
	if err := c.Prepare(nil); len(err) != 1 {
		t.Fatalf("Should error if an instance_type is not specified")
	}
}

func TestRunConfigPrepare_SourceAmi(t *testing.T) {
	c := testConfig()
	c.SourceAmi = ""
	if err := c.Prepare(nil); len(err) != 2 {
		t.Fatalf("Should error if a source_ami (or source_ami_filter) is not specified")
	}
}

func TestRunConfigPrepare_SourceAmiFilterBlank(t *testing.T) {
	c := testConfigFilter()
	if err := c.Prepare(nil); len(err) != 2 {
		t.Fatalf("Should error if source_ami_filter is empty or not specified (and source_ami is not specified)")
	}
}

func TestRunConfigPrepare_SourceAmiFilterOwnersBlank(t *testing.T) {
	c := testConfigFilter()
	filter_key := "name"
	filter_value := "foo"
	c.SourceAmiFilter.Filters = map[string]string{filter_key: filter_value}
	if err := c.Prepare(nil); len(err) != 1 {
		t.Fatalf("Should error if Owners is not specified)")
	}
}

func TestRunConfigPrepare_SourceAmiFilterGood(t *testing.T) {
	c := testConfigFilter()
	owner := "123"
	filter_key := "name"
	filter_value := "foo"
	goodFilter := AmiFilterOptions{
		Owners:  []string{owner},
		Filters: map[string]string{filter_key: filter_value},
	}
	c.SourceAmiFilter = goodFilter
	if err := c.Prepare(nil); len(err) != 0 {
		t.Fatalf("err: %s", err)
	}
}

func TestRunConfigPrepare_EnableT2UnlimitedGood(t *testing.T) {
	c := testConfig()
	// Must have a T2 instance type if T2 Unlimited is enabled
	c.InstanceType = "t2.micro"
	c.EnableT2Unlimited = true
	err := c.Prepare(nil)
	if len(err) > 0 {
		t.Fatalf("err: %s", err)
	}
}

func TestRunConfigPrepare_EnableT2UnlimitedBadInstanceType(t *testing.T) {
	c := testConfig()
	// T2 Unlimited cannot be used with instance types other than T2
	c.InstanceType = "m5.large"
	c.EnableT2Unlimited = true
	err := c.Prepare(nil)
	if len(err) != 1 {
		t.Fatalf("Should error if T2 Unlimited is enabled with non-T2 instance_type")
	}
}

func TestRunConfigPrepare_EnableT2UnlimitedBadWithSpotInstanceRequest(t *testing.T) {
	c := testConfig()
	// T2 Unlimited cannot be used with Spot Instances
	c.InstanceType = "t2.micro"
	c.EnableT2Unlimited = true
	c.SpotPrice = "auto"
	err := c.Prepare(nil)
	if len(err) != 1 {
		t.Fatalf("Should error if T2 Unlimited has been used in conjuntion with a Spot Price request")
	}
}

func TestRunConfigPrepare_EnableT2UnlimitedDeprecation(t *testing.T) {
	c := testConfig()
	// Must have a T2 instance type if T2 Unlimited is enabled
	c.InstanceType = "t2.micro"
	c.EnableT2Unlimited = true
	err := c.Prepare(nil)

	if c.EnableUnlimitedCredits != true {
		t.Errorf("expected EnableUnlimitedCredits to be true when the deprecated EnableT2Unlimited is true, but got %T", c.EnableUnlimitedCredits)
	}

	if len(err) > 0 {
		t.Fatalf("err: %s", err)
	}
}

func TestRunConfigPrepare_EnableUnlimitedCredits(t *testing.T) {

	tc := []struct {
		name          string
		instanceType  string
		enableCredits bool
		errorCount    int
	}{
		{"T2 instance", "t2.micro", true, 0},
		{"T3 instance", "t3.micro", true, 0},
		{"T3a instance", "t3a.xlarge", true, 0},
		{"T4g instance", "t4g.micro", true, 0},
		{"M5 instance", "m5.micro", true, 1},
		{"bogus t4 instance", "t4.micro", true, 1},
		{"bogus t23 instance", "t23.micro", true, 1},
	}

	for _, tt := range tc {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c := testConfig()
			c.InstanceType = tt.instanceType
			c.EnableUnlimitedCredits = tt.enableCredits
			err := c.Prepare(nil)
			if len(err) != tt.errorCount {
				t.Errorf("err: %s", err)
			}

		})
	}
}

func TestRunConfigPrepare_SpotAuto(t *testing.T) {
	c := testConfig()
	c.SpotPrice = "auto"
	if err := c.Prepare(nil); len(err) != 0 {
		t.Fatalf("err: %s", err)
	}

	// Shouldn't error (YET) even though SpotPriceAutoProduct is deprecated
	c.SpotPriceAutoProduct = "Linux/Unix"
	if err := c.Prepare(nil); len(err) != 0 {
		t.Fatalf("err: %s", err)
	}
}

func TestRunConfigPrepare_SSHPort(t *testing.T) {
	c := testConfig()
	c.Comm.SSHPort = 0
	if err := c.Prepare(nil); len(err) != 0 {
		t.Fatalf("err: %s", err)
	}

	if c.Comm.SSHPort != 22 {
		t.Fatalf("invalid value: %d", c.Comm.SSHPort)
	}

	c.Comm.SSHPort = 44
	if err := c.Prepare(nil); len(err) != 0 {
		t.Fatalf("err: %s", err)
	}

	if c.Comm.SSHPort != 44 {
		t.Fatalf("invalid value: %d", c.Comm.SSHPort)
	}
}

func TestRunConfigPrepare_UserData(t *testing.T) {
	c := testConfig()
	tf, err := ioutil.TempFile("", "packer")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer os.Remove(tf.Name())
	defer tf.Close()

	c.UserData = "foo"
	c.UserDataFile = tf.Name()
	if err := c.Prepare(nil); len(err) != 1 {
		t.Fatalf("Should error if user_data string and user_data_file have both been specified")
	}
}

func TestRunConfigPrepare_UserDataFile(t *testing.T) {
	c := testConfig()
	if err := c.Prepare(nil); len(err) != 0 {
		t.Fatalf("err: %s", err)
	}

	c.UserDataFile = "idontexistidontthink"
	if err := c.Prepare(nil); len(err) != 1 {
		t.Fatalf("Should error if the file specified by user_data_file does not exist")
	}

	tf, err := ioutil.TempFile("", "packer")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer os.Remove(tf.Name())
	defer tf.Close()

	c.UserDataFile = tf.Name()
	if err := c.Prepare(nil); len(err) != 0 {
		t.Fatalf("err: %s", err)
	}
}

func TestRunConfigPrepare_TemporaryKeyPairName(t *testing.T) {
	c := testConfig()
	c.Comm.SSHTemporaryKeyPairName = ""
	if err := c.Prepare(nil); len(err) != 0 {
		t.Fatalf("err: %s", err)
	}

	if c.Comm.SSHTemporaryKeyPairName == "" {
		t.Fatal("keypair name is empty")
	}

	// Match prefix and UUID, e.g. "packer_5790d491-a0b8-c84c-c9d2-2aea55086550".
	r := regexp.MustCompile(`\Apacker_(?:(?i)[a-f\d]{8}(?:-[a-f\d]{4}){3}-[a-f\d]{12}?)\z`)
	if !r.MatchString(c.Comm.SSHTemporaryKeyPairName) {
		t.Fatal("keypair name is not valid")
	}

	c.Comm.SSHTemporaryKeyPairName = "ssh-key-123"
	if err := c.Prepare(nil); len(err) != 0 {
		t.Fatalf("err: %s", err)
	}

	if c.Comm.SSHTemporaryKeyPairName != "ssh-key-123" {
		t.Fatal("keypair name does not match")
	}
}

func TestRunConfigPrepare_TemporaryKeyPairTypeDefault(t *testing.T) {
	c := testConfig()
	tc := []struct {
		desc                     string
		keyPairName, keyPairType string
		expectedKeyType          string
	}{
		{desc: "no temporary_key_pair_* config settings should use defaults", expectedKeyType: "rsa"},
		{desc: "setting a temporary_key_pair_name should set default key pair type", keyPairName: "local.name", expectedKeyType: "rsa"},
	}
	for _, tt := range tc {
		tt := tt
		t.Run(tt.desc, func(t *testing.T) {
			c.Comm.SSHTemporaryKeyPairName = tt.keyPairName
			c.Comm.SSHTemporaryKeyPairType = tt.keyPairType
			if err := c.Prepare(nil); len(err) != 0 {
				t.Fatalf("err: %s", err)
			}

			if c.Comm.SSHTemporaryKeyPairType != tt.expectedKeyType {
				t.Fatal("keypair type should have defaulted to rsa")
			}
		})
	}
}

func TestRunConfigPrepare_TemporaryKeyPairTypeRSA(t *testing.T) {
	c := testConfig()
	c.Comm.SSHTemporaryKeyPairType = "rsa"
	if err := c.Prepare(nil); len(err) != 0 {
		t.Fatalf("err: %s", err)
	}

	if c.Comm.SSHTemporaryKeyPairType != "rsa" {
		t.Fatal("keypair type should have been rsa")
	}
}

func TestRunConfigPrepare_TemporaryKeyPairTypeED25519(t *testing.T) {
	c := testConfig()
	c.Comm.SSHTemporaryKeyPairType = "ed25519"
	if err := c.Prepare(nil); len(err) != 0 {
		t.Fatalf("err: %s", err)
	}

	if c.Comm.SSHTemporaryKeyPairType != "ed25519" {
		t.Fatal("keypair type should have been ed25519")
	}
}

func TestRunConfigPrepare_TemporaryKeyPairTypeBad(t *testing.T) {
	c := testConfig()
	c.Comm.SSHTemporaryKeyPairType = "invalid"
	if err := c.Prepare(nil); len(err) == 0 {
		t.Fatalf("should error if temporary_key_pair_type is set to an invalid type")
	}
}

func TestRunConfigPrepare_TenancyBad(t *testing.T) {
	c := testConfig()
	c.Tenancy = "not_real"

	if err := c.Prepare(nil); len(err) != 1 {
		t.Fatal("Should error if tenancy is set to an invalid type")
	}
}

func TestRunConfigPrepare_TenancyGood(t *testing.T) {
	validTenancy := []string{"", "default", "dedicated", "host"}
	for _, vt := range validTenancy {
		c := testConfig()
		c.Tenancy = vt
		if err := c.Prepare(nil); len(err) != 0 {
			t.Fatalf("Should not error if tenancy is set to %s", vt)
		}
	}
}

func TestRunConfigPrepare_EnableNitroEnclaveBadWithSpotInstanceRequest(t *testing.T) {
	c := testConfig()
	// Nitro Enclaves cannot be used with Spot Instances
	c.InstanceType = "c5.xlarge"
	c.EnableNitroEnclave = true
	c.SpotPrice = "auto"
	err := c.Prepare(nil)
	if len(err) != 1 {
		t.Fatalf("Should error if Nitro Enclaves has been used in conjuntion with a Spot Price request")
	}
}

func TestRunConfigPrepare_EnableNitroEnclaveBadWithBurstableInstanceType(t *testing.T) {
	c := testConfig()
	// Nitro Enclaves cannot be used with burstable instances
	c.InstanceType = "t2.micro"
	c.EnableNitroEnclave = true
	err := c.Prepare(nil)
	if len(err) != 1 {
		t.Fatalf("Should error if Nitro Enclaves has been used in conjuntion with a burstable instance type")
	}
}

func TestRunConfigPrepare_EnableNitroEnclaveGood(t *testing.T) {
	c := testConfig()
	// Nitro Enclaves cannot be used with burstable instances
	c.InstanceType = "c5.xlarge"
	c.EnableNitroEnclave = true
	err := c.Prepare(nil)
	if len(err) != 0 {
		t.Fatalf("Should not error with valid Nitro Enclave config")
	}
}

func TestRunConfigPrepare_FailIfBothHostIDAndGroupSpecified(t *testing.T) {
	c := testConfig()
	c.Placement.HostId = "host"
	c.Placement.HostResourceGroupArn = "group"
	err := c.Prepare(nil)
	if len(err) != 1 {
		t.Fatalf("Should error if both host_id and host_resource_group_arn are set")
	}
}

func TestRunConfigPrepare_InvalidTenantForHost(t *testing.T) {
	tests := []struct {
		name         string
		setHost      string
		setGroup     string
		setTenant    string
		expectErrors int
	}{
		{
			"no host_id, no host_resource_group_arn, with valid tenant",
			"",
			"",
			"",
			0,
		},
		{
			"host_id set, tenant host",
			"host",
			"",
			"host",
			0,
		},
		{
			"no host_id, host_resource_group_arn set, with tenant host",
			"",
			"group",
			"host",
			0,
		},
		{
			"host_id set, invalid tenant",
			"host",
			"",
			"dedicated",
			1,
		},
		{
			"no host_id, host_resource_group_arn set, invalid tenant",
			"",
			"group",
			"dedicated",
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testConfig()
			c.Placement.HostId = tt.setHost
			c.Placement.HostResourceGroupArn = tt.setGroup
			c.Placement.Tenancy = tt.setTenant
			errs := c.Prepare(nil)
			if len(errs) != tt.expectErrors {
				t.Errorf("expected %d errors, got %d", tt.expectErrors, len(errs))
			}
		})
	}
}
