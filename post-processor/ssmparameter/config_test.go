// Copyright IBM Corp. 2013, 2026
// SPDX-License-Identifier: MPL-2.0

package ssmparameter

import (
	"context"
	"fmt"
	"strings"
	"testing"

	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/zclconf/go-cty/cty"
)

func TestConfigureParameterName(t *testing.T) {
	for _, test := range []struct {
		name string
		want string
	}{
		{"", "must be set"},
		{"golden-ami", ""},
		{"/golden-ami", ""},
		{"/images/ssm-latest", ""},
		{"/images/a b", "invalid characters"},
		{"awsLatest", "reserved prefix"},
		{"/SsM/ami", "reserved prefix"},
		{"images/latest", "leading slash"},
		{"/", "empty path"},
		{"/images//latest", "empty path"},
		{"/images/latest/", "empty path"},
		{strings.Repeat("/a", 15), ""},
		{strings.Repeat("/a", 16), "15 levels"},
		{strings.Repeat("a", 1012), "length"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := new(PostProcessor).Configure(map[string]any{"parameter_name": test.name})
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Configure error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestConfigureParameterTags(t *testing.T) {
	for _, test := range []struct {
		name string
		tags map[string]string
		want string
	}{
		{"valid", map[string]string{"Owner / 東京_1.2:=+-@": "é Ω ١", "Empty": "", "Type": "aws:ec2:image"}, ""},
		{"unicode length boundaries", map[string]string{strings.Repeat("é", 128): strings.Repeat("界", 256)}, ""},
		{"empty key", map[string]string{"": "value"}, "tag key"},
		{"long key", map[string]string{strings.Repeat("é", 129): ""}, "128"},
		{"long value", map[string]string{"Owner": strings.Repeat("界", 257)}, "256"},
		{"reserved key", map[string]string{"aws:Owner": ""}, "reserved prefix"},
		{"key unsupported punctuation", map[string]string{"bad!": ""}, "tag key"},
		{"value unsupported whitespace", map[string]string{"Owner": "one\ttwo"}, "tag value"},
		{"value unsupported symbol", map[string]string{"Owner": "☃"}, "tag value"},
		{"50 tags", numberedTags(50), ""},
		{"51 tags", numberedTags(51), "50 tags"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := new(PostProcessor).Configure(map[string]any{"parameter_name": "/images/latest", "tags": test.tags})
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Configure error = %v, want %q", err, test.want)
			}
		})
	}
}

func numberedTags(count int) map[string]string {
	tags := make(map[string]string, count)
	for i := 0; i < count; i++ {
		tags[fmt.Sprintf("Tag%d", i)] = ""
	}
	return tags
}

func TestPublicationTierAndDescription(t *testing.T) {
	p := new(PostProcessor)
	configure := func(tier, description string) error {
		return p.Configure(map[string]any{"parameter_name": "/images/latest", "tier": tier, "description": description})
	}
	if err := configure("<unknown>", "<unknown>"); err != nil {
		t.Fatalf("initial prepare rejected a dynamic tier: %v", err)
	}
	if errs := p.config.validatePublication(false); len(errs.Errors) == 0 {
		t.Fatal("unresolved tier accepted for publication")
	}
	if err := configure("Advanced", strings.Repeat("é", 1024)); err != nil {
		t.Fatalf("resolved configuration rejected: %v", err)
	}
	if err := configure("Advanced", strings.Repeat("é", 1025)); err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("expected an error for an oversized SSM description, got %v", err)
	}
}

func TestConfigureDynamicParameterAndTags(t *testing.T) {
	p := new(PostProcessor)
	prepare := func(name, key, value string) error {
		return p.Configure(cty.ObjectVal(map[string]cty.Value{
			"parameter_name": cty.StringVal(name),
			"tags":           cty.MapVal(map[string]cty.Value{key: cty.StringVal(value)}),
		}))
	}
	if err := prepare("/images/<unknown>", "build:<UNKNOWN>", "<unknown>"); err != nil {
		t.Fatalf("initial Configure: %v", err)
	}
	if err := prepare("/images/ami-123", "build:AMI-123", ""); err != nil {
		t.Fatalf("resolved Configure: %v", err)
	}
	if value, ok := p.config.Tags["build:AMI-123"]; p.config.ParameterName != "/images/ami-123" || len(p.config.Tags) != 1 || !ok || value != "" {
		t.Fatalf("resolved config: name=%q, tags=%v", p.config.ParameterName, p.config.Tags)
	}
	if err := prepare("/images/ami-123", "aws:AMI-123", ""); err == nil || !strings.Contains(err.Error(), "reserved prefix") {
		t.Fatalf("Configure accepted invalid resolved tag key: %v", err)
	}
}

func TestPostProcessValidatesBeforeAWSAccessAndRetention(t *testing.T) {
	for _, test := range []struct {
		name string
		tags map[string]string
		want string
	}{
		{"/images/<unknown>", nil, "parameter_name"},
		{"/images/latest", map[string]string{"build:<UNKNOWN>": ""}, "tag key"},
		{"/images/latest", map[string]string{"build": "<unknown>"}, "tag value"},
		// Fits the first region's ARN, but not the second region's ARN.
		{strings.Repeat("a", 965), nil, "us-gov-west-1"},
	} {
		p := new(PostProcessor)
		if err := p.Configure(map[string]any{
			"parameter_name": test.name, "tags": test.tags, "profile": "missing-packer-test-profile",
		}); err != nil {
			t.Fatalf("initial Configure: %v", err)
		}
		input := &configTestArtifact{Artifact: &awscommon.Artifact{Amis: map[string]string{
			"us-east-1": "ami-00000000000000001", "us-gov-west-1": "ami-00000000000000002",
		}}}
		_, keep, force, err := p.PostProcess(context.Background(), new(packersdk.MockUi), input)
		if err == nil || !strings.Contains(err.Error(), test.want) || input.retainCalled || keep || force {
			t.Fatalf("PostProcess = %v, keep=%t force=%t retention called=%t; want early configuration error %q", err, keep, force, input.retainCalled, test.want)
		}
	}
}

type configTestArtifact struct {
	packersdk.Artifact
	retainCalled bool
}

func (a *configTestArtifact) State(key string) any {
	if key == awscommon.RetainAMIsStateKey {
		a.retainCalled = true
	}
	return a.Artifact.State(key)
}
