// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type AWSPollingConfig
package common

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

// StateRefreshFunc is a function type used for StateChangeConf that is
// responsible for refreshing the item being watched for a state change.
//
// It returns three results. `result` is any object that will be returned
// as the final object after waiting for state change. This allows you to
// return the final updated object, for example an EC2 instance after refreshing
// it.
//
// `state` is the latest state of that object. And `err` is any error that
// may have happened while refreshing the state.
type StateRefreshFunc func() (result any, state string, err error)

// StateChangeConf is the configuration struct used for `WaitForState`.
type StateChangeConf struct {
	Pending   []string
	Refresh   StateRefreshFunc
	StepState multistep.StateBag
	Target    string
}

type envInfo struct {
	envKey     string
	Val        int
	overridden bool
}

type overridableWaitVars struct {
	awsPollDelaySeconds envInfo
	awsMaxAttempts      envInfo
	awsTimeoutSeconds   envInfo
}

type PollingOptions struct {
	MaxWaitTime time.Duration
	MinDelay    time.Duration
}

// Following are wrapper functions that use Packer's environment-variables to
// determine retry logic, then call the AWS SDK's built-in waiters.

// Polling configuration for the AWS waiter. Configures the waiter for resources creation or actions like attaching
// volumes or importing image.
//
// HCL2 example:
// ```hcl
//
//	aws_polling {
//		 delay_seconds = 30
//		 max_attempts = 50
//	}
//
// ```
//
// JSON example:
// ```json
//
//	"aws_polling" : {
//		 "delay_seconds": 30,
//		 "max_attempts": 50
//	}
//
// ```
type AWSPollingConfig struct {
	// Specifies the maximum number of attempts the waiter will check for resource state.
	// This value can also be set via the AWS_MAX_ATTEMPTS.
	// If both option and environment variable are set, the max_attempts will be considered over the AWS_MAX_ATTEMPTS.
	// If none is set, defaults to AWS waiter default which is 40 max_attempts.
	// In aws sdk go v2, the max attempts is not set directly, but rather set via max wait time and delay seconds.
	// maxWaitTime = maxAttempts * delaySeconds
	MaxAttempts int `mapstructure:"max_attempts" required:"false"`
	// Specifies the delay in seconds between attempts to check the resource state.
	// This value can also be set via the AWS_POLL_DELAY_SECONDS.
	// If both option and environment variable are set, the delay_seconds will be considered over the AWS_POLL_DELAY_SECONDS.
	// If none is set, defaults to AWS waiter default which is 15 seconds.
	DelaySeconds int `mapstructure:"delay_seconds" required:"false"`
}

// This helper function uses the environment variables AWS_TIMEOUT_SECONDS and
// AWS_POLL_DELAY_SECONDS to generate waiter options that can be passed into any
// request.Waiter function. These options will control how many times the waiter
// will retry the request, as well as how long to wait between the retries.

// DEFAULTING BEHAVIOR:
// if AWS_POLL_DELAY_SECONDS is set but the others are not, Packer will set this
// poll delay and use the waiter-specific default

// if AWS_TIMEOUT_SECONDS is set but AWS_MAX_ATTEMPTS is not, Packer will use
// AWS_TIMEOUT_SECONDS and _either_ AWS_POLL_DELAY_SECONDS _or_ 2 if the user has not set AWS_POLL_DELAY_SECONDS, to determine a max number of attempts to make.

// if AWS_TIMEOUT_SECONDS, _and_ AWS_MAX_ATTEMPTS are both set,
// AWS_TIMEOUT_SECONDS will be ignored.

// if AWS_MAX_ATTEMPTS is set but AWS_POLL_DELAY_SECONDS is not, then we will
// use waiter-specific defaults.

func (w *AWSPollingConfig) LogEnvOverrideWarnings() {
	pollDelayEnv := os.Getenv("AWS_POLL_DELAY_SECONDS")
	timeoutSecondsEnv := os.Getenv("AWS_TIMEOUT_SECONDS")
	maxAttemptsEnv := os.Getenv("AWS_MAX_ATTEMPTS")

	maxAttemptsIsSet := maxAttemptsEnv != "" || w.MaxAttempts != 0
	timeoutSecondsIsSet := timeoutSecondsEnv != ""
	pollDelayIsSet := pollDelayEnv != "" || w.DelaySeconds != 0

	if maxAttemptsIsSet && timeoutSecondsIsSet {
		warning := fmt.Sprintf("[WARNING] (aws): AWS_MAX_ATTEMPTS and " +
			"AWS_TIMEOUT_SECONDS are both set. Packer will use " +
			"AWS_MAX_ATTEMPTS and discard AWS_TIMEOUT_SECONDS.")
		if !pollDelayIsSet {
			warning = fmt.Sprintf("%s  Since you have not set the poll delay, "+
				"Packer will default to a 2-second delay.", warning)
		}
		log.Print(warning)
	} else if timeoutSecondsIsSet {
		log.Printf("[WARNING] (aws): env var AWS_TIMEOUT_SECONDS is " +
			"deprecated in favor of AWS_MAX_ATTEMPTS env or aws_polling_max_attempts config option. " +
			"If you have not explicitly set AWS_POLL_DELAY_SECONDS env or aws_polling_delay_seconds config option, " +
			"we are defaulting to a poll delay of 2 seconds, regardless of the AWS waiter's default.")
	}
	if !maxAttemptsIsSet && !timeoutSecondsIsSet && !pollDelayIsSet {
		log.Printf("[INFO] (aws): No AWS timeout and polling overrides have been set. " +
			"Packer will default to waiter-specific delays and timeouts. If you would " +
			"like to customize the length of time between retries and max " +
			"number of retries you may do so by setting the environment " +
			"variables AWS_POLL_DELAY_SECONDS and AWS_MAX_ATTEMPTS or the " +
			"configuration options aws_polling_delay_seconds and aws_polling_max_attempts " +
			"to your desired values.")
	}
}
func applyEnvOverrides(envOverrides overridableWaitVars) *PollingOptions {
	options := PollingOptions{}

	// if any of the env vars are not overridden, we return empty struct to allow the AWS SDK to use its defaults.

	// if poll delay is set, we use that as the minimum delay.
	if envOverrides.awsPollDelaySeconds.overridden {
		options.MinDelay = time.Duration(envOverrides.awsPollDelaySeconds.Val) * time.Second
	}

	// If user has set max attempts, aws sdk go v2 doesn't have a direct way to set max attempts,
	// we calculate the max wait time instead.
	if envOverrides.awsMaxAttempts.overridden {
		maxWaitTime := time.Duration(envOverrides.awsMaxAttempts.Val) * time.Duration(envOverrides.
			awsPollDelaySeconds.Val) * time.Second
		options.MaxWaitTime = maxWaitTime
		// if max attempts is set and poll delay is not set, we default to 2 seconds.
		if !envOverrides.awsPollDelaySeconds.overridden {
			options.MinDelay = time.Duration(envOverrides.awsPollDelaySeconds.Val) * time.Second
		}

	} else if envOverrides.awsTimeoutSeconds.overridden {
		options.MaxWaitTime = time.Duration(envOverrides.awsTimeoutSeconds.Val) * time.Second
		// if timeout is set and poll delay is not set, we default to 2 seconds.
		if !envOverrides.awsPollDelaySeconds.overridden {
			options.MinDelay = time.Duration(envOverrides.awsPollDelaySeconds.Val) * time.Second
		}

	}

	return &options
}

func getOverride(varInfo envInfo) envInfo {
	override := os.Getenv(varInfo.envKey)
	if override != "" {
		n, err := strconv.Atoi(override)
		if err != nil {
			log.Printf("Invalid %s '%s', using default", varInfo.envKey, override)
		} else {
			varInfo.overridden = true
			varInfo.Val = n
		}
	}

	return varInfo
}

func getEnvOverrides() overridableWaitVars {
	// Load env vars from environment.
	envValues := overridableWaitVars{
		envInfo{"AWS_POLL_DELAY_SECONDS", 2, false},
		envInfo{"AWS_MAX_ATTEMPTS", 0, false},
		envInfo{"AWS_TIMEOUT_SECONDS", 0, false},
	}

	envValues.awsMaxAttempts = getOverride(envValues.awsMaxAttempts)
	envValues.awsPollDelaySeconds = getOverride(envValues.awsPollDelaySeconds)
	envValues.awsTimeoutSeconds = getOverride(envValues.awsTimeoutSeconds)

	return envValues
}
func (w *AWSPollingConfig) getWaiterOptions() *PollingOptions {
	envOverrides := getEnvOverrides()

	if w.MaxAttempts != 0 {
		envOverrides.awsMaxAttempts.Val = w.MaxAttempts
		envOverrides.awsMaxAttempts.overridden = true
	}
	if w.DelaySeconds != 0 {
		envOverrides.awsPollDelaySeconds.Val = w.DelaySeconds
		envOverrides.awsPollDelaySeconds.overridden = true
	}

	waitOpts := applyEnvOverrides(envOverrides)
	return waitOpts
}

func (w *AWSPollingConfig) WaitUntilInstanceRunning(ctx context.Context, ec2Client Ec2Client, instanceId string) error {

	instanceInput := ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceId},
	}
	waiter := ec2.NewInstanceRunningWaiter(ec2Client)

	//err := ec2Client.WaitUntilInstanceRunningWithContext(
	//	ctx,
	//	&instanceInput,
	//	w.getWaiterOptions()...)

	//todo fix the wait params.
	err := waiter.Wait(ctx, &instanceInput, 15*time.Minute)
	return err
}

func (w *AWSPollingConfig) WaitUntilInstanceTerminated(ctx context.Context, ec2Client Ec2Client, instanceId string) error {
	instanceInput := ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceId},
	}
	waiter := ec2.NewInstanceTerminatedWaiter(ec2Client)

	//err := conn.WaitUntilInstanceTerminatedWithContext(
	//	ctx,
	//	&instanceInput,
	//	w.getWaiterOptions()...)

	//todo fix the wait params.
	err := waiter.Wait(ctx, &instanceInput, 15*time.Minute)
	return err
}
