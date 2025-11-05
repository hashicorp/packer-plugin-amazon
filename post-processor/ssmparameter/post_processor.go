// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package ssmparameter

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/hashicorp/hcl/v2/hcldec"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	sdkcommon "github.com/hashicorp/packer-plugin-sdk/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

const BuilderId = "packer.post-processor.amazon-ssm-parameter"

const (
	publishTimeout      = 2 * time.Minute
	publishPollInterval = 2 * time.Second
	maxParameterARNSize = 1011
	maxParameterTags    = 50
)

// Config defines the parameter and AWS credentials used to publish the AMIs.
type Config struct {
	sdkcommon.PackerConfig `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`

	// Packer reserves "name" for the post-processor block's own name.
	ParameterName string            `mapstructure:"parameter_name"`
	Description   string            `mapstructure:"description"`
	Overwrite     bool              `mapstructure:"overwrite"`
	Tier          string            `mapstructure:"tier"`
	Tags          map[string]string `mapstructure:"tags"`
}

type PostProcessor struct{ config Config }

func (p *PostProcessor) ConfigSpec() hcldec.ObjectSpec {
	return p.config.FlatMapstructure().HCL2Spec()
}

var (
	ssmNameAllowed = regexp.MustCompile(`^[A-Za-z0-9_.\-/]+$`)
	// SSM uses the same character pattern for both tag keys and values.
	// https://docs.aws.amazon.com/systems-manager/latest/APIReference/API_Tag.html
	ssmTagAllowed = regexp.MustCompile(`^[\p{L}\p{Z}\p{N}_.:/=+\-@]*$`)
)

func (p *PostProcessor) Configure(raws ...interface{}) error {
	p.config = Config{}
	interp := interpolate.Context{Funcs: awscommon.TemplateFuncs}
	if err := config.Decode(&p.config, &config.DecodeOpts{
		PluginType:         BuilderId,
		Interpolate:        true,
		InterpolateContext: &interp,
	}, raws...); err != nil {
		return err
	}
	errs := p.config.validatePublication(true)
	if len(errs.Errors) > 0 {
		return errs
	}
	// Preparing Vault-backed access requests credentials. Defer that until
	// after the final publication settings and all artifact regions are known.
	if p.config.AccessConfig.VaultAWSEngine.Empty() {
		errs = packersdk.MultiErrorAppend(errs, p.config.AccessConfig.Prepare(&p.config.PackerConfig)...)
		if len(errs.Errors) > 0 {
			return errs
		}
	}
	return nil
}

func (c *Config) validatePublication(allowUnknown bool) *packersdk.MultiError {
	errs := new(packersdk.MultiError)
	// Packer passes <unknown> for HCL build values at initial Configure, then
	// reconfigures the post-processor with artifact data before PostProcess.
	unknown := func(value string) bool {
		return allowUnknown && strings.Contains(strings.ToLower(value), "<unknown>")
	}
	if !unknown(c.ParameterName) {
		if err := validateParameterName(c.ParameterName); err != nil {
			errs = packersdk.MultiErrorAppend(errs, err)
		}
	}
	if !unknown(c.Tier) && c.Tier != "" && c.Tier != string(types.ParameterTierStandard) && c.Tier != string(types.ParameterTierAdvanced) {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("tier must be Standard or Advanced if specified"))
	}
	// Blank descriptions are replaced with a short default before publication.
	if !unknown(c.Description) && strings.TrimSpace(c.Description) != "" && utf8.RuneCountInString(c.Description) > 1024 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("description exceeds 1024 characters"))
	}
	// The per-resource limit is lower than the capacity of an API request.
	// https://docs.aws.amazon.com/systems-manager/latest/APIReference/API_AddTagsToResource.html
	if len(c.Tags) > maxParameterTags {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("a parameter can have at most %d tags", maxParameterTags))
	}
	for _, key := range slices.Sorted(maps.Keys(c.Tags)) {
		if !unknown(key) {
			switch {
			case key == "" || utf8.RuneCountInString(key) > 128:
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("tag key %q must be between 1 and 128 characters", key))
			case strings.HasPrefix(key, "aws:"):
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("tag key %q uses the reserved prefix aws:", key))
			case !ssmTagAllowed.MatchString(key):
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("tag key %q contains invalid characters", key))
			}
		}
		if value := c.Tags[key]; !unknown(value) {
			switch {
			case utf8.RuneCountInString(value) > 256:
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("tag value for %q exceeds 256 characters", key))
			case !ssmTagAllowed.MatchString(value):
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("tag value for %q contains invalid characters", key))
			}
		}
	}
	return errs
}

// https://docs.aws.amazon.com/systems-manager/latest/APIReference/API_PutParameter.html#systemsmanager-PutParameter-request-Name
func validateParameterName(name string) error {
	path := strings.TrimPrefix(name, "/")
	switch {
	case strings.TrimSpace(name) == "":
		return fmt.Errorf("parameter_name must be set")
	case len(path) > maxParameterARNSize:
		return fmt.Errorf("parameter_name length %d exceeds limit %d", len(path), maxParameterARNSize)
	case !ssmNameAllowed.MatchString(name):
		return fmt.Errorf("parameter_name %q contains invalid characters", name)
	case path == "" || strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") || strings.Contains(path, "//"):
		return fmt.Errorf("parameter_name %q contains an empty path component", name)
	case strings.Contains(path, "/") && path == name:
		return fmt.Errorf("parameter_name %q requires a leading slash for a hierarchy", name)
	case strings.Count(path, "/") >= 15:
		return fmt.Errorf("parameter_name %q exceeds 15 levels", name)
	}
	if lower := strings.ToLower(path); strings.HasPrefix(lower, "aws") || strings.HasPrefix(lower, "ssm") {
		return fmt.Errorf("parameter_name %q begins with a reserved prefix (aws or ssm)", name)
	}
	return nil
}

// PostProcess publishes AMI IDs as SSM parameters across regions.
func (p *PostProcessor) PostProcess(ctx context.Context, ui packersdk.Ui, artifact packersdk.Artifact) (packersdk.Artifact, bool, bool, error) {
	amis, err := artifactAMIs(artifact)
	if err != nil {
		return nil, false, false, err
	}

	regions := slices.Sorted(maps.Keys(amis))

	if errs := p.config.validatePublication(false); len(errs.Errors) > 0 {
		return nil, false, false, errs
	}
	if err := validateRegionalNameLength(p.config.ParameterName, regions); err != nil {
		return nil, false, false, err
	}

	access := p.config.AccessConfig
	if !access.VaultAWSEngine.Empty() {
		if errs := access.Prepare(&p.config.PackerConfig); len(errs) > 0 {
			return nil, false, false, packersdk.MultiErrorAppend(nil, errs...)
		}
	}
	if access.RawRegion == "" {
		access.RawRegion = regions[0]
	}
	cfg, err := access.Config(ctx)
	if err != nil {
		return nil, false, false, fmt.Errorf("configure AWS access: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, false, false, err
	}
	// Retain the original artifact before making a write whose outcome may be
	// unknown on failure or cancellation. The keep flags only cover success.
	if retained, _ := artifact.State(awscommon.RetainAMIsStateKey).(bool); !retained {
		return nil, false, false, fmt.Errorf("artifact from %q does not support retaining AMIs for SSM publication", artifact.BuilderId())
	}
	ui.Message(fmt.Sprintf("Publishing SSM parameter %q to %d regions", p.config.ParameterName, len(regions)))

	var tags []types.Tag
	for _, key := range slices.Sorted(maps.Keys(p.config.Tags)) {
		tags = append(tags, types.Tag{Key: aws.String(key), Value: aws.String(p.config.Tags[key])})
	}
	var errs *packersdk.MultiError
	for _, region := range regions {
		if err := ctx.Err(); err != nil {
			errs = packersdk.MultiErrorAppend(errs, err)
			break
		}
		amiID := amis[region]
		client := ssm.NewFromConfig(*cfg, func(o *ssm.Options) { o.Region = region })
		parameter, err := p.config.publishAMI(ctx, client, amiID, tags)
		if err != nil {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("publish SSM parameter %q for AMI %s in %s: %w", p.config.ParameterName, amiID, region, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		ui.Message(fmt.Sprintf("Published %s (version %d) for AMI %s in %s", aws.ToString(parameter.ARN), parameter.Version, amiID, region))
	}

	if errs != nil {
		return nil, false, false, errs
	}
	// SSM aliases point to the input AMIs, so do not allow this step to delete them.
	return artifact, true, true, nil
}

// The maximum name length includes the ARN prefix. Check every destination
// before publishing so a longer partition or region cannot cause a partial write.
func validateRegionalNameLength(name string, regions []string) error {
	resolver := ssm.NewDefaultEndpointResolver()
	nameLength := len(strings.TrimPrefix(name, "/"))
	for _, region := range regions {
		endpoint, err := resolver.ResolveEndpoint(region, ssm.EndpointResolverOptions{})
		if err != nil {
			return fmt.Errorf("resolve SSM partition for %s: %w", region, err)
		}
		prefix := fmt.Sprintf("arn:%s:ssm:%s:000000000000:parameter/", endpoint.PartitionID, region)
		if limit := maxParameterARNSize - len(prefix); nameLength > limit {
			return fmt.Errorf("parameter_name length %d exceeds limit %d in %s", nameLength, limit, region)
		}
	}
	return nil
}

func (c *Config) publishAMI(ctx context.Context, client *ssm.Client, amiID string, tags []types.Tag) (*types.Parameter, error) {
	description := c.Description
	if strings.TrimSpace(description) == "" {
		description = fmt.Sprintf("Packer published AMI alias for %s", amiID)
	}
	put := &ssm.PutParameterInput{
		Name:        aws.String(c.ParameterName),
		Description: aws.String(description),
		Type:        types.ParameterTypeString,
		DataType:    aws.String("aws:ec2:image"),
		Tier:        types.ParameterTier(c.Tier),
		Overwrite:   aws.Bool(c.Overwrite),
		Value:       aws.String(amiID),
	}
	if !c.Overwrite {
		put.Tags = tags
	}
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	parameter, err := publishParameter(ctx, client, put, publishPollInterval)
	if err != nil {
		return nil, err
	}
	// SSM accepts tags in PutParameter only for create-only requests. For an
	// upsert, tag after the asynchronous write has been confirmed.
	if c.Overwrite && len(tags) > 0 {
		_, err := client.AddTagsToResource(ctx, &ssm.AddTagsToResourceInput{
			ResourceType: types.ResourceTypeForTaggingParameter,
			ResourceId:   put.Name,
			Tags:         tags,
		})
		if err != nil {
			return nil, fmt.Errorf("version %d was published but tagging failed: %w", parameter.Version, err)
		}
	}
	return parameter, nil
}

func artifactAMIs(artifact packersdk.Artifact) (map[string]string, error) {
	amis := make(map[string]string)
	state := artifact.State(awscommon.AMIIDsStateKey)
	switch values := state.(type) {
	case map[string]string:
		amis = values
	case map[interface{}]interface{}: // Packer RPC decodes maps into interface keys and values.
		for key, value := range values {
			region, regionOK := key.(string)
			id, idOK := value.(string)
			if !regionOK || !idOK {
				return nil, fmt.Errorf("invalid region or AMI ID in artifact state %q", awscommon.AMIIDsStateKey)
			}
			amis[region] = id
		}
	case nil:
	default:
		return nil, fmt.Errorf("unexpected AMI artifact state type %T", state)
	}
	if len(amis) == 0 {
		return nil, fmt.Errorf("no AMIs found in artifact from %q", artifact.BuilderId())
	}
	return amis, nil
}
