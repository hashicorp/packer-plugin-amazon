// Copyright IBM Corp. 2013, 2025
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
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/smithy-go/middleware"
	smithytime "github.com/aws/smithy-go/time"
	smithywaiter "github.com/aws/smithy-go/waiter"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
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
type StateRefreshFunc func() (result interface{}, state string, err error)

// StateChangeConf is the configuration struct used for `WaitForState`.
type StateChangeConf struct {
	Pending   []string
	Refresh   StateRefreshFunc
	StepState multistep.StateBag
	Target    string
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
	MaxAttempts int `mapstructure:"max_attempts" required:"false"`
	// Specifies the delay in seconds between attempts to check the resource state.
	// This value can also be set via the AWS_POLL_DELAY_SECONDS.
	// If both option and environment variable are set, the delay_seconds will be considered over the AWS_POLL_DELAY_SECONDS.
	// If none is set, defaults to AWS waiter default which is 15 seconds.
	DelaySeconds int `mapstructure:"delay_seconds" required:"false"`
	// Specifies the maximum timeout in seconds for the waiter.
	// This value can also be set via the AWS_MAX_TIMEOUT.
	// If both option and environment variable are set, the max_timeout will be considered over the AWS_MAX_TIMEOUT.
	// If none is set, defaults to AWS waiter default which is 600 seconds (10 minutes).
	MaxTimeout int `mapstructure:"max_timeout" required:"false"`
}

func (w *AWSPollingConfig) WaitUntilAMIAvailable(ctx context.Context, conn clients.Ec2Client, imageId string) error {
	imageInput := ec2.DescribeImagesInput{
		ImageIds: []string{imageId},
	}
	log.Printf("Waiting for AMI (%s) to be available...", imageId)
	waiter := ec2.NewImageAvailableWaiter(conn)
	maxTimeout := w.WithMaxAttempts(120).MaxTimeout
	err := waiter.Wait(ctx, &imageInput, time.Duration(maxTimeout)*time.Second)
	if err != nil {
		if strings.Contains(err.Error(), request.WaiterResourceNotReadyErrorCode) {
			err = fmt.Errorf("Failed with ResourceNotReady error, which can "+
				"have a variety of causes. For help troubleshooting, check "+
				"our docs: "+
				"https://www.packer.io/docs/builders/amazon.html#resourcenotready-error\n"+
				"original error: %s", err.Error())
		}
	}

	return err
}

func (w *AWSPollingConfig) WaitUntilInstanceRunning(ctx context.Context, conn clients.Ec2Client, instanceId string) error {

	instanceInput := ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceId},
	}

	waiter := ec2.NewInstanceRunningWaiter(conn)
	err := waiter.Wait(ctx, &instanceInput, time.Duration(w.MaxTimeout)*time.Second)
	return err
}

func (w *AWSPollingConfig) WaitUntilInstanceTerminated(ctx context.Context, conn clients.Ec2Client, instanceId string) error {
	instanceInput := ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceId},
	}

	waiter := ec2.NewInstanceTerminatedWaiter(conn)
	err := waiter.Wait(ctx, &instanceInput, time.Duration(w.MaxTimeout)*time.Second)
	return err
}

// This function works for both requesting and cancelling spot instances.
func (w *AWSPollingConfig) WaitUntilSpotRequestFulfilled(ctx context.Context, conn clients.Ec2Client, spotRequestId string) error {
	spotRequestInput := ec2.DescribeSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []string{spotRequestId},
	}

	waiter := ec2.NewSpotInstanceRequestFulfilledWaiter(conn)
	err := waiter.Wait(ctx, &spotRequestInput, time.Duration(w.MaxTimeout)*time.Second)
	return err
}

func (w *AWSPollingConfig) WaitUntilVolumeAvailable(ctx context.Context, conn clients.Ec2Client, volumeId string) error {
	volumeInput := ec2.DescribeVolumesInput{
		VolumeIds: []string{volumeId},
	}

	waiter := ec2.NewVolumeAvailableWaiter(conn)
	err := waiter.Wait(ctx, &volumeInput, time.Duration(w.MaxTimeout)*time.Second)
	return err
}

func (w *AWSPollingConfig) WaitUntilSnapshotDone(ctx context.Context, conn clients.Ec2Client, snapshotID string) error {
	snapInput := ec2.DescribeSnapshotsInput{
		SnapshotIds: []string{snapshotID},
	}

	maxTimeout := w.WithMaxAttempts(120).MaxTimeout
	waiter := ec2.NewSnapshotCompletedWaiter(conn)
	err := waiter.Wait(ctx, &snapInput, time.Duration(maxTimeout)*time.Second)
	return err
}

// Wrappers for our custom AWS waiters

func (w *AWSPollingConfig) WaitUntilVolumeAttached(ctx context.Context, conn clients.Ec2Client, volumeId string) error {
	volumeInput := ec2.DescribeVolumesInput{
		VolumeIds: []string{volumeId},
	}

	waiter := newVolumeAttachedWaiter(conn)
	err := waiter.Wait(ctx, &volumeInput, time.Duration(w.MaxTimeout)*time.Second)
	return err
}

func (w *AWSPollingConfig) WaitUntilVolumeDetached(ctx context.Context, conn clients.Ec2Client, volumeId string) error {
	volumeInput := ec2.DescribeVolumesInput{
		VolumeIds: []string{volumeId},
	}

	waiter := newVolumeDetachedWaiter(conn)
	err := waiter.Wait(ctx, &volumeInput, time.Duration(w.MaxTimeout)*time.Second)
	return err
}

func (w *AWSPollingConfig) WaitUntilImageImported(ctx context.Context, conn clients.Ec2Client, taskID string) error {
	importInput := ec2.DescribeImportImageTasksInput{
		ImportTaskIds: []string{taskID},
	}

	waiter := newImportImageTaskWaiter(conn)
	err := waiter.Wait(ctx, &importInput, time.Duration(w.MaxTimeout)*time.Second)
	return err
}

func (w *AWSPollingConfig) WaitUntilFastLaunchEnabled(ctx context.Context, conn clients.Ec2Client, imageID string) error {
	fastLaunchDescribeInput := &ec2.DescribeFastLaunchImagesInput{
		ImageIds: []string{imageID},
	}

	waiter := newFastLaunchImageWaiter(conn)
	err := waiter.Wait(ctx, fastLaunchDescribeInput, time.Duration(w.MaxTimeout)*time.Second)
	return err
}

// Custom SDK v2 waiters that we have to implement ourselves because the AWS SDK doesn't have built-in waiters for these actions.
type volumeAttachedDetachedWaiterOptions struct {
	// Set of options to modify how an operation is invoked. These apply to all
	// operations invoked for this client. Use functional options on operation call to
	// modify this list for per operation behavior.
	//
	// Passing options here is functionally equivalent to passing values to this
	// config's ClientOptions field that extend the inner client's APIOptions directly.
	APIOptions []func(*middleware.Stack) error

	// Functional options to be passed to all operations invoked by this client.
	//
	// Function values that modify the inner APIOptions are applied after the waiter
	// config's own APIOptions modifiers.
	ClientOptions []func(*ec2.Options)

	// MinDelay is the minimum amount of time to delay between retries. If unset,
	// VolumeInUseWaiter will use default minimum delay of 15 seconds. Note that
	// MinDelay must resolve to a value lesser than or equal to the MaxDelay.
	MinDelay time.Duration

	// MaxDelay is the maximum amount of time to delay between retries. If unset or
	// set to zero, VolumeInUseWaiter will use default max delay of 120 seconds. Note
	// that MaxDelay must resolve to value greater than or equal to the MinDelay.
	MaxDelay time.Duration

	// LogWaitAttempts is used to enable logging for waiter retry attempts
	LogWaitAttempts bool

	// Retryable is function that can be used to override the service defined
	// waiter-behavior based on operation output, or returned error. This function is
	// used by the waiter to decide if a state is retryable or a terminal state.
	//
	// By default service-modeled logic will populate this option. This option can
	// thus be used to define a custom waiter state with fall-back to service-modeled
	// waiter state mutators.The function returns an error in case of a failure state.
	// In case of retry state, this function returns a bool value of true and nil
	// error, while in case of success it returns a bool value of false and nil error.
	Retryable func(context.Context, *ec2.DescribeVolumesInput, *ec2.DescribeVolumesOutput, error) (bool, error)
}

type volumeAttachedDetachedWaiter struct {
	client ec2.DescribeVolumesAPIClient

	options volumeAttachedDetachedWaiterOptions
}

func (w *volumeAttachedDetachedWaiter) Wait(ctx context.Context, params *ec2.DescribeVolumesInput, maxWaitDur time.Duration, optFns ...func(*volumeAttachedDetachedWaiterOptions)) error {
	if maxWaitDur <= 0 {
		fmt.Errorf("maximum wait time for waiter must be greater than zero")
	}

	options := w.options
	for _, fn := range optFns {
		fn(&options)
	}

	if options.MaxDelay <= 0 {
		options.MaxDelay = 120 * time.Second
	}

	if options.MinDelay > options.MaxDelay {
		return fmt.Errorf("minimum waiter delay %v must be lesser than or equal to maximum waiter delay of %v.", options.MinDelay, options.MaxDelay)
	}

	ctx, cancelFn := context.WithTimeout(ctx, maxWaitDur)
	defer cancelFn()

	logger := smithywaiter.Logger{}
	remainingTime := maxWaitDur

	var attempt int64
	for {

		attempt++
		apiOptions := options.APIOptions
		start := time.Now()

		if options.LogWaitAttempts {
			logger.Attempt = attempt
			apiOptions = append([]func(*middleware.Stack) error{}, options.APIOptions...)
			apiOptions = append(apiOptions, logger.AddLogger)
		}

		out, err := w.client.DescribeVolumes(ctx, params, func(o *ec2.Options) {
			baseOpts := []func(*ec2.Options){
				addIsWaiterUserAgent,
			}
			o.APIOptions = append(o.APIOptions, apiOptions...)
			for _, opt := range baseOpts {
				opt(o)
			}
			for _, opt := range options.ClientOptions {
				opt(o)
			}
		})

		retryable, err := options.Retryable(ctx, params, out, err)
		if err != nil {
			return err
		}
		if !retryable {
			return nil
		}

		remainingTime -= time.Since(start)
		if remainingTime < options.MinDelay || remainingTime <= 0 {
			break
		}

		// compute exponential backoff between waiter retries
		delay, err := smithywaiter.ComputeDelay(
			attempt, options.MinDelay, options.MaxDelay, remainingTime,
		)
		if err != nil {
			return fmt.Errorf("error computing waiter delay, %w", err)
		}

		remainingTime -= delay
		// sleep for the delay amount before invoking a request
		if err := smithytime.SleepWithContext(ctx, delay); err != nil {
			return fmt.Errorf("request cancelled while waiting, %w", err)
		}
	}
	return fmt.Errorf("exceeded max wait time for VolumeAttached waiter")
}

func newVolumeAttachedWaiter(client ec2.DescribeVolumesAPIClient, optFns ...func(*volumeAttachedDetachedWaiterOptions)) *volumeAttachedDetachedWaiter {
	options := volumeAttachedDetachedWaiterOptions{}
	options.MinDelay = 5 * time.Second
	options.MaxDelay = 120 * time.Second
	options.Retryable = volumeAttachedRetryable
	options.ClientOptions = append(options.ClientOptions, func(o *ec2.Options) {
		o.RetryMaxAttempts = 40
	})

	for _, fn := range optFns {
		fn(&options)
	}

	return &volumeAttachedDetachedWaiter{
		client:  client,
		options: options,
	}
}

func volumeAttachedRetryable(ctx context.Context, input *ec2.DescribeVolumesInput, output *ec2.DescribeVolumesOutput, err error) (bool, error) {
	if err != nil {
		return false, err
	}

	for _, volume := range output.Volumes {
		for _, attachment := range volume.Attachments {
			if attachment.State != ec2types.VolumeAttachmentStateAttached {
				return true, nil
			}
		}
	}

	return false, err
}

func newVolumeDetachedWaiter(client ec2.DescribeVolumesAPIClient, optFns ...func(*volumeAttachedDetachedWaiterOptions)) *volumeAttachedDetachedWaiter {
	options := volumeAttachedDetachedWaiterOptions{}
	options.MinDelay = 5 * time.Second
	options.MaxDelay = 120 * time.Second
	options.Retryable = volumeDetachedRetryable
	options.ClientOptions = append(options.ClientOptions, func(o *ec2.Options) {
		o.RetryMaxAttempts = 40
	})

	for _, fn := range optFns {
		fn(&options)
	}

	return &volumeAttachedDetachedWaiter{
		client:  client,
		options: options,
	}
}

func volumeDetachedRetryable(ctx context.Context, input *ec2.DescribeVolumesInput, output *ec2.DescribeVolumesOutput, err error) (bool, error) {
	if err != nil {
		return false, err
	}

	if len(output.Volumes) == 0 {
		return false, nil
	}
	for _, volume := range output.Volumes {
		if len(volume.Attachments) > 0 {
			return true, nil
		}
	}

	return false, err
}

type importImageTaskWaiterOptions struct {
	// Set of options to modify how an operation is invoked. These apply to all
	// operations invoked for this client. Use functional options on operation call to
	// modify this list for per operation behavior.
	//
	// Passing options here is functionally equivalent to passing values to this
	// config's ClientOptions field that extend the inner client's APIOptions directly.
	APIOptions []func(*middleware.Stack) error

	// Functional options to be passed to all operations invoked by this client.
	//
	// Function values that modify the inner APIOptions are applied after the waiter
	// config's own APIOptions modifiers.
	ClientOptions []func(*ec2.Options)

	// MinDelay is the minimum amount of time to delay between retries. If unset,
	// VolumeInUseWaiter will use default minimum delay of 15 seconds. Note that
	// MinDelay must resolve to a value lesser than or equal to the MaxDelay.
	MinDelay time.Duration

	// MaxDelay is the maximum amount of time to delay between retries. If unset or
	// set to zero, VolumeInUseWaiter will use default max delay of 120 seconds. Note
	// that MaxDelay must resolve to value greater than or equal to the MinDelay.
	MaxDelay time.Duration

	// LogWaitAttempts is used to enable logging for waiter retry attempts
	LogWaitAttempts bool

	// Retryable is function that can be used to override the service defined
	// waiter-behavior based on operation output, or returned error. This function is
	// used by the waiter to decide if a state is retryable or a terminal state.
	//
	// By default service-modeled logic will populate this option. This option can
	// thus be used to define a custom waiter state with fall-back to service-modeled
	// waiter state mutators.The function returns an error in case of a failure state.
	// In case of retry state, this function returns a bool value of true and nil
	// error, while in case of success it returns a bool value of false and nil error.
	Retryable func(context.Context, *ec2.DescribeImportImageTasksInput, *ec2.DescribeImportImageTasksOutput, error) (bool, error)
}

type importImageTaskWaiter struct {
	client ec2.DescribeImportImageTasksAPIClient

	options importImageTaskWaiterOptions
}

func (w *importImageTaskWaiter) Wait(ctx context.Context, params *ec2.DescribeImportImageTasksInput, maxWaitDur time.Duration, optFns ...func(*importImageTaskWaiterOptions)) error {
	if maxWaitDur <= 0 {
		fmt.Errorf("maximum wait time for waiter must be greater than zero")
	}

	options := w.options
	for _, fn := range optFns {
		fn(&options)
	}

	if options.MaxDelay <= 0 {
		options.MaxDelay = 120 * time.Second
	}

	if options.MinDelay > options.MaxDelay {
		return fmt.Errorf("minimum waiter delay %v must be lesser than or equal to maximum waiter delay of %v.", options.MinDelay, options.MaxDelay)
	}

	ctx, cancelFn := context.WithTimeout(ctx, maxWaitDur)
	defer cancelFn()

	logger := smithywaiter.Logger{}
	remainingTime := maxWaitDur

	var attempt int64
	for {

		attempt++
		apiOptions := options.APIOptions
		start := time.Now()

		if options.LogWaitAttempts {
			logger.Attempt = attempt
			apiOptions = append([]func(*middleware.Stack) error{}, options.APIOptions...)
			apiOptions = append(apiOptions, logger.AddLogger)
		}

		out, err := w.client.DescribeImportImageTasks(ctx, params, func(o *ec2.Options) {
			baseOpts := []func(*ec2.Options){
				addIsWaiterUserAgent,
			}
			o.APIOptions = append(o.APIOptions, apiOptions...)
			for _, opt := range baseOpts {
				opt(o)
			}
			for _, opt := range options.ClientOptions {
				opt(o)
			}
		})

		retryable, err := options.Retryable(ctx, params, out, err)
		if err != nil {
			return err
		}
		if !retryable {
			return nil
		}

		remainingTime -= time.Since(start)
		if remainingTime < options.MinDelay || remainingTime <= 0 {
			break
		}

		// compute exponential backoff between waiter retries
		delay, err := smithywaiter.ComputeDelay(
			attempt, options.MinDelay, options.MaxDelay, remainingTime,
		)
		if err != nil {
			return fmt.Errorf("error computing waiter delay, %w", err)
		}

		remainingTime -= delay
		// sleep for the delay amount before invoking a request
		if err := smithytime.SleepWithContext(ctx, delay); err != nil {
			return fmt.Errorf("request cancelled while waiting, %w", err)
		}
	}
	return fmt.Errorf("exceeded max wait time for ImportImageTask waiter")
}

func newImportImageTaskWaiter(client ec2.DescribeImportImageTasksAPIClient, optFns ...func(*importImageTaskWaiterOptions)) *importImageTaskWaiter {
	options := importImageTaskWaiterOptions{}
	options.MinDelay = 5 * time.Second
	options.MaxDelay = 120 * time.Second
	options.Retryable = importImageTaskRetryable
	options.ClientOptions = append(options.ClientOptions, func(o *ec2.Options) {
		o.RetryMaxAttempts = 720
	})

	for _, fn := range optFns {
		fn(&options)
	}

	return &importImageTaskWaiter{
		client:  client,
		options: options,
	}
}

func importImageTaskRetryable(ctx context.Context, input *ec2.DescribeImportImageTasksInput, output *ec2.DescribeImportImageTasksOutput, err error) (bool, error) {
	if err != nil {
		return false, err
	}

	match := len(output.ImportImageTasks) > 0
	for _, task := range output.ImportImageTasks {
		if task.Status != nil && aws.ToString(task.Status) != "completed" {
			match = false
			break
		}
	}

	if match {
		return true, nil
	}

	match = false
	for _, task := range output.ImportImageTasks {
		if task.Status != nil && aws.ToString(task.Status) == "deleted" {
			match = true
			break
		}
	}

	if match {
		return false, fmt.Errorf("import image task entered failure state with status 'deleted'")
	}

	return true, nil
}

type fastLaunchImagesWaiterOptions struct {
	// Set of options to modify how an operation is invoked. These apply to all
	// operations invoked for this client. Use functional options on operation call to
	// modify this list for per operation behavior.
	//
	// Passing options here is functionally equivalent to passing values to this
	// config's ClientOptions field that extend the inner client's APIOptions directly.
	APIOptions []func(*middleware.Stack) error

	// Functional options to be passed to all operations invoked by this client.
	//
	// Function values that modify the inner APIOptions are applied after the waiter
	// config's own APIOptions modifiers.
	ClientOptions []func(*ec2.Options)

	// MinDelay is the minimum amount of time to delay between retries. If unset,
	// VolumeInUseWaiter will use default minimum delay of 15 seconds. Note that
	// MinDelay must resolve to a value lesser than or equal to the MaxDelay.
	MinDelay time.Duration

	// MaxDelay is the maximum amount of time to delay between retries. If unset or
	// set to zero, VolumeInUseWaiter will use default max delay of 120 seconds. Note
	// that MaxDelay must resolve to value greater than or equal to the MinDelay.
	MaxDelay time.Duration

	// LogWaitAttempts is used to enable logging for waiter retry attempts
	LogWaitAttempts bool

	// Retryable is function that can be used to override the service defined
	// waiter-behavior based on operation output, or returned error. This function is
	// used by the waiter to decide if a state is retryable or a terminal state.
	//
	// By default service-modeled logic will populate this option. This option can
	// thus be used to define a custom waiter state with fall-back to service-modeled
	// waiter state mutators.The function returns an error in case of a failure state.
	// In case of retry state, this function returns a bool value of true and nil
	// error, while in case of success it returns a bool value of false and nil error.
	Retryable func(context.Context, *ec2.DescribeFastLaunchImagesInput, *ec2.DescribeFastLaunchImagesOutput, error) (bool, error)
}

type fastLaunchImagesWaiter struct {
	client ec2.DescribeFastLaunchImagesAPIClient

	options fastLaunchImagesWaiterOptions
}

func (w *fastLaunchImagesWaiter) Wait(ctx context.Context, params *ec2.DescribeFastLaunchImagesInput, maxWaitDur time.Duration, optFns ...func(*fastLaunchImagesWaiterOptions)) error {
	if maxWaitDur <= 0 {
		fmt.Errorf("maximum wait time for waiter must be greater than zero")
	}

	options := w.options
	for _, fn := range optFns {
		fn(&options)
	}

	if options.MaxDelay <= 0 {
		options.MaxDelay = 120 * time.Second
	}

	if options.MinDelay > options.MaxDelay {
		return fmt.Errorf("minimum waiter delay %v must be lesser than or equal to maximum waiter delay of %v.", options.MinDelay, options.MaxDelay)
	}

	ctx, cancelFn := context.WithTimeout(ctx, maxWaitDur)
	defer cancelFn()

	logger := smithywaiter.Logger{}
	remainingTime := maxWaitDur

	var attempt int64
	for {

		attempt++
		apiOptions := options.APIOptions
		start := time.Now()

		if options.LogWaitAttempts {
			logger.Attempt = attempt
			apiOptions = append([]func(*middleware.Stack) error{}, options.APIOptions...)
			apiOptions = append(apiOptions, logger.AddLogger)
		}

		out, err := w.client.DescribeFastLaunchImages(ctx, params, func(o *ec2.Options) {
			baseOpts := []func(*ec2.Options){
				addIsWaiterUserAgent,
			}
			o.APIOptions = append(o.APIOptions, apiOptions...)
			for _, opt := range baseOpts {
				opt(o)
			}
			for _, opt := range options.ClientOptions {
				opt(o)
			}
		})

		retryable, err := options.Retryable(ctx, params, out, err)
		if err != nil {
			return err
		}
		if !retryable {
			return nil
		}

		remainingTime -= time.Since(start)
		if remainingTime < options.MinDelay || remainingTime <= 0 {
			break
		}

		// compute exponential backoff between waiter retries
		delay, err := smithywaiter.ComputeDelay(
			attempt, options.MinDelay, options.MaxDelay, remainingTime,
		)
		if err != nil {
			return fmt.Errorf("error computing waiter delay, %w", err)
		}

		remainingTime -= delay
		// sleep for the delay amount before invoking a request
		if err := smithytime.SleepWithContext(ctx, delay); err != nil {
			return fmt.Errorf("request cancelled while waiting, %w", err)
		}
	}
	return fmt.Errorf("exceeded max wait time for ImportImageTask waiter")
}

func newFastLaunchImageWaiter(client ec2.DescribeFastLaunchImagesAPIClient, optFns ...func(*fastLaunchImagesWaiterOptions)) *fastLaunchImagesWaiter {
	options := fastLaunchImagesWaiterOptions{}
	options.MinDelay = 5 * time.Second
	options.MaxDelay = 120 * time.Second
	options.Retryable = fastLaunchImageRetryable
	options.ClientOptions = append(options.ClientOptions, func(o *ec2.Options) {
		o.RetryMaxAttempts = 720
	})

	for _, fn := range optFns {
		fn(&options)
	}

	return &fastLaunchImagesWaiter{
		client:  client,
		options: options,
	}
}

func fastLaunchImageRetryable(ctx context.Context, input *ec2.DescribeFastLaunchImagesInput, output *ec2.DescribeFastLaunchImagesOutput, err error) (bool, error) {
	if err != nil {
		return false, err
	}

	match := len(output.FastLaunchImages) > 0
	for _, task := range output.FastLaunchImages {
		if task.State != ec2types.FastLaunchStateCodeEnabled {
			match = false
			break
		}
	}

	if match {
		return true, nil
	}

	match = true
	for _, task := range output.FastLaunchImages {
		match = match && task.State == ec2types.FastLaunchStateCodeEnablingFailed
		if !match {
			break
		}
	}

	if match {
		return false, fmt.Errorf("import image task entered failure state with status 'enabling-failed'")
	}

	match = true
	for _, task := range output.FastLaunchImages {
		match = match && task.State == ec2types.FastLaunchStateCodeEnabledFailed
		if !match {
			break
		}
	}

	if match {
		return false, fmt.Errorf("import image task entered failure state with status 'enabled-failed'")
	}

	return false, nil
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

func (w *AWSPollingConfig) Prepare() *AWSPollingConfig {
	envOverrides := getEnvOverrides()

	if w.MaxAttempts == 0 {
		if envOverrides.awsMaxAttempts.overridden {
			w.MaxAttempts = envOverrides.awsMaxAttempts.Val
		} else {
			w.MaxAttempts = 40
		}
	}
	if w.DelaySeconds == 0 {
		if envOverrides.awsPollDelaySeconds.overridden {
			w.DelaySeconds = envOverrides.awsPollDelaySeconds.Val
		} else {
			w.DelaySeconds = 15
		}
	}
	if w.MaxTimeout == 0 {
		if envOverrides.awsTimeoutSeconds.overridden {
			w.MaxTimeout = envOverrides.awsTimeoutSeconds.Val
		} else {
			w.MaxTimeout = w.MaxAttempts * w.DelaySeconds
			if w.MaxTimeout == 0 {
				w.MaxTimeout = 600
			}
		}
	}
	return w
}

func (w *AWSPollingConfig) WithMaxAttempts(maxAttempts int) *AWSPollingConfig {
	copy := *w
	copy.MaxAttempts = maxAttempts
	copy.MaxTimeout = maxAttempts * copy.DelaySeconds
	return &copy
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

func applyEnvOverrides(envOverrides overridableWaitVars) []request.WaiterOption {
	waitOpts := make([]request.WaiterOption, 0)
	// If user has set poll delay seconds, overwrite it. If user has NOT,
	// default to a poll delay of 2 seconds
	if envOverrides.awsPollDelaySeconds.overridden {
		delaySeconds := request.ConstantWaiterDelay(time.Duration(envOverrides.awsPollDelaySeconds.Val) * time.Second)
		waitOpts = append(waitOpts, request.WithWaiterDelay(delaySeconds))
	}

	// If user has set max attempts, overwrite it. If user hasn't set max
	// attempts, default to whatever the waiter has set as a default.
	if envOverrides.awsMaxAttempts.overridden {
		waitOpts = append(waitOpts, request.WithWaiterMaxAttempts(envOverrides.awsMaxAttempts.Val))
	} else if envOverrides.awsTimeoutSeconds.overridden {
		maxAttempts := envOverrides.awsTimeoutSeconds.Val / envOverrides.awsPollDelaySeconds.Val
		// override the delay so we can get the timeout right
		if !envOverrides.awsPollDelaySeconds.overridden {
			delaySeconds := request.ConstantWaiterDelay(time.Duration(envOverrides.awsPollDelaySeconds.Val) * time.Second)
			waitOpts = append(waitOpts, request.WithWaiterDelay(delaySeconds))
		}
		waitOpts = append(waitOpts, request.WithWaiterMaxAttempts(maxAttempts))
	}

	return waitOpts
}

// helper function copied from AWS SDK v2
func addIsWaiterUserAgent(o *ec2.Options) {
	o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
		ua, err := getOrAddRequestUserAgent(stack)
		if err != nil {
			return err
		}

		ua.AddUserAgentFeature(awsmiddleware.UserAgentFeatureWaiter)
		return nil
	})
}

func getOrAddRequestUserAgent(stack *middleware.Stack) (*awsmiddleware.RequestUserAgent, error) {
	id := (*awsmiddleware.RequestUserAgent)(nil).ID()
	mw, ok := stack.Build.Get(id)
	if !ok {
		mw = awsmiddleware.NewRequestUserAgent()
		if err := stack.Build.Add(mw, middleware.After); err != nil {
			return nil, err
		}
	}

	ua, ok := mw.(*awsmiddleware.RequestUserAgent)
	if !ok {
		return nil, fmt.Errorf("%T for %s middleware did not match expected type", mw, id)
	}

	return ua, nil
}
