// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package parameterstore

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"

	_ "embed"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-sdk/acctest"
	"github.com/hashicorp/packer-plugin-sdk/retry"
)

//go:embed test-fixtures/template.pkr.hcl
var testDatasourceBasic string

func TestAccAmazonParameterStore(t *testing.T) {
	t.Parallel()
	param := &AmazonParameter{
		Name:        "packer_datasource_parameterstore_test_parameter",
		Type:        "String",
		Value:       "this_is_the_packer_test_parameter_value",
		Description: "this is a parameter used in a packer acc test",
	}

	testcase := &acctest.PluginTestCase{
		Name: "amazon_parameterstore_datasource_basic_test",
		Setup: func() error {
			return param.Create()
		},
		Teardown: func() error {
			return param.Delete()
		},
		Template: testDatasourceBasic,
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
			}
			logs, err := os.Open(logfile)
			if err != nil {
				return fmt.Errorf("Unable find %s", logfile)
			}
			defer logs.Close()

			logsBytes, err := ioutil.ReadAll(logs)
			if err != nil {
				return fmt.Errorf("Unable to read %s", logfile)
			}
			logsString := string(logsBytes)

			valueLog := fmt.Sprintf("null.basic-example: parameter value: %s", param.Value)
			versionLog := fmt.Sprintf("null.basic-example: parameter version: %s", fmt.Sprintf("%d", param.Info.Version))

			if matched, _ := regexp.MatchString(valueLog+".*", logsString); !matched {
				t.Fatalf("logs doesn't contain expected value %q", logsString)
			}
			if matched, _ := regexp.MatchString(versionLog+".*", logsString); !matched {
				t.Fatalf("logs doesn't contain expected version %q", logsString)
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testcase)
}

type AmazonParameter struct {
	Name        string
	Value       string
	Type        string
	Description string

	Info *ssm.PutParameterOutput
}

func (ap *AmazonParameter) Create() error {
	accessConfig := &awscommon.AccessConfig{}
	ctx := context.TODO()
	cfg, err := accessConfig.Config(ctx)
	if err != nil {
		return fmt.Errorf("Unable to load Config %s", err)
	}
	ssmsvc := ssm.NewFromConfig(*cfg)
	if err != nil {
		return fmt.Errorf("Unable to create aws session %s", err.Error())
	}
	newParam := &ssm.PutParameterInput{
		Name:        aws.String(ap.Name),
		Value:       aws.String(ap.Value),
		Type:        types.ParameterType(ap.Type),
		Description: aws.String(ap.Description),
	}
	param := new(ssm.PutParameterOutput)
	err = retry.Config{
		Tries: 11,
		ShouldRetry: func(err error) bool {
			var parameterExists *types.ParameterAlreadyExists

			if errors.As(err, &parameterExists) {
				_ = ap.Delete()
				return true
			}

			return false
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(context.TODO(), func(_ context.Context) error {
		param, err = ssmsvc.PutParameter(ctx, newParam)
		return err
	})
	ap.Info = param
	return err
}
func (ap *AmazonParameter) Delete() error {
	accessConfig := &awscommon.AccessConfig{}
	ctx := context.TODO()
	cfg, err := accessConfig.Config(ctx)
	if err != nil {
		return fmt.Errorf("Unable to load Config %s", err)
	}
	ssmsvc := ssm.NewFromConfig(*cfg)
	param := &ssm.DeleteParameterInput{
		Name: aws.String(ap.Name),
	}
	_, err = ssmsvc.DeleteParameter(ctx, param)
	return err
}
