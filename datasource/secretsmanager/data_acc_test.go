// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package secretsmanager

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-sdk/acctest"
	"github.com/hashicorp/packer-plugin-sdk/retry"
)

//go:embed test-fixtures/template.pkr.hcl
var testDatasourceBasic string

func TestAccAmazonSecretsManager(t *testing.T) {
	t.Parallel()
	secret := &AmazonSecret{
		Name:        "packer_datasource_secretsmanager_test_secret",
		Key:         "packer_test_key",
		Value:       "this_is_the_packer_test_secret_value",
		Description: "this is a secret used in a packer acc test",
	}

	testCase := &acctest.PluginTestCase{
		Name: "amazon_secretsmanager_datasource_basic_test",
		Setup: func() error {
			return secret.Create()
		},
		Teardown: func() error {
			return secret.Delete()
		},
		Template: testDatasourceBasic,
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}

			logs, err := os.Open(logfile)
			if err != nil {
				return fmt.Errorf("Unable find %s", logfile)
			}
			defer logs.Close()

			logsBytes, err := io.ReadAll(logs)
			if err != nil {
				return fmt.Errorf("Unable to read %s", logfile)
			}
			logsString := string(logsBytes)

			valueLog := fmt.Sprintf("null.basic-example: secret value: %s", secret.Value)
			secretStringLog := fmt.Sprintf("null.basic-example: secret secret_string: %s", fmt.Sprintf("{%s:%s}", secret.Key, secret.Value))
			versionIdLog := fmt.Sprintf("null.basic-example: secret version_id: %s", aws.ToString(secret.Info.VersionId))
			secretValueLog := fmt.Sprintf("null.basic-example: secret value: %s", secret.Value)

			if matched, _ := regexp.MatchString(valueLog+".*", logsString); !matched {
				t.Fatalf("logs doesn't contain expected arn %q", logsString)
			}
			if matched, _ := regexp.MatchString(secretStringLog+".*", logsString); !matched {
				t.Fatalf("logs doesn't contain expected secret_string %q", logsString)
			}
			if matched, _ := regexp.MatchString(versionIdLog+".*", logsString); !matched {
				t.Fatalf("logs doesn't contain expected version_id %q", logsString)
			}
			if matched, _ := regexp.MatchString(secretValueLog+".*", logsString); !matched {
				t.Fatalf("logs doesn't contain expected value %q", logsString)
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

type AmazonSecret struct {
	Name        string
	Key         string
	Value       string
	Description string

	Info   *secretsmanager.CreateSecretOutput
	client *secretsmanager.Client
}

func (as *AmazonSecret) Create() error {
	ctx := context.TODO()
	if as.client == nil {
		accessConfig := &awscommon.AccessConfig{}
		cfg, err := accessConfig.Config(ctx)
		if err != nil {
			return fmt.Errorf("Unable to create aws session %s", err.Error())
		}
		as.client = secretsmanager.NewFromConfig(*cfg)
	}

	newSecret := &secretsmanager.CreateSecretInput{
		Description:  aws.String(as.Description),
		Name:         aws.String(as.Name),
		SecretString: aws.String(fmt.Sprintf(`{%q:%q}`, as.Key, as.Value)),
	}

	secret := new(secretsmanager.CreateSecretOutput)
	var err error
	err = retry.Config{
		Tries: 11,
		ShouldRetry: func(err error) bool {
			var resourceExists *types.ResourceExistsException
			var invalidRequestException *types.InvalidRequestException

			if errors.As(err, &resourceExists) {
				_ = as.Delete()
				return true
			}
			if errors.As(err, &invalidRequestException) {
				return true
			}
			return false
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(context.TODO(), func(_ context.Context) error {
		secret, err = as.client.CreateSecret(ctx, newSecret)
		return err
	})
	as.Info = secret
	return err
}

func (as *AmazonSecret) Delete() error {
	ctx := context.TODO()
	if as.client == nil {
		accessConfig := &awscommon.AccessConfig{}
		cfg, err := accessConfig.Config(ctx)
		if err != nil {
			return fmt.Errorf("Unable to create aws session %s", err.Error())
		}
		as.client = secretsmanager.NewFromConfig(*cfg)
	}

	secret := &secretsmanager.DeleteSecretInput{
		ForceDeleteWithoutRecovery: aws.Bool(true),
		SecretId:                   aws.String(as.Name),
	}
	_, err := as.client.DeleteSecret(ctx, secret)
	return err
}
