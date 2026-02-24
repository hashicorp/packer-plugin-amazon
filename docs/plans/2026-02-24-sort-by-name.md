# sort_by=name for amazon-ami datasource — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `sort_by = "name"` option to the `amazon-ami` datasource that selects the AMI with the lexicographically highest name.

**Architecture:** A new `latestByNameAmi` helper (alongside `mostRecentAmi`) sorts images by name. A new `GetFilteredImages` method (alongside `GetFilteredImage`) returns the raw image slice without selecting one. The datasource `Execute()` branches on `SortBy` to call the right selection path. `AmiFilterOptions` and all builders are untouched.

**Tech Stack:** Go, AWS SDK v2 (`github.com/aws/aws-sdk-go-v2/service/ec2/types`), packer-plugin-sdk

---

### Task 1: Add `latestByNameAmi` helper with tests

**Files:**
- Modify: `common/step_source_ami_info.go` (after line 50, after `mostRecentAmi`)
- Modify: `common/step_source_ami_info_test.go` (append)

**Step 1: Write the failing tests**

Append to `common/step_source_ami_info_test.go`:

```go
func TestLatestByNameAmi_ReturnsLexicographicallyLast(t *testing.T) {
	images := []types.Image{
		{ImageId: aws.String("ami-sp7"), Name: aws.String("suse-sles-15-sp7-v20241105-hvm-ssd-x86_64")},
		{ImageId: aws.String("ami-sp6"), Name: aws.String("suse-sles-15-sp6-v20250210-hvm-ssd-x86_64")},
		{ImageId: aws.String("ami-sp5"), Name: aws.String("suse-sles-15-sp5-v20240101-hvm-ssd-x86_64")},
	}
	result := latestByNameAmi(images)
	assert.Equal(t, "ami-sp7", aws.ToString(result.ImageId))
}

func TestLatestByNameAmi_SingleImage(t *testing.T) {
	images := []types.Image{
		{ImageId: aws.String("ami-only"), Name: aws.String("my-ami")},
	}
	result := latestByNameAmi(images)
	assert.Equal(t, "ami-only", aws.ToString(result.ImageId))
}

func TestLatestByNameAmi_IdenticalNames(t *testing.T) {
	images := []types.Image{
		{ImageId: aws.String("ami-a"), Name: aws.String("same-name")},
		{ImageId: aws.String("ami-b"), Name: aws.String("same-name")},
	}
	// Should not panic; returns one of the two
	result := latestByNameAmi(images)
	name := aws.ToString(result.Name)
	assert.Equal(t, "same-name", name)
}
```

The new import needed: `"github.com/aws/aws-sdk-go-v2/aws"` (add to existing imports block).

**Step 2: Run tests to confirm they fail**

```bash
go test ./common/ -run TestLatestByNameAmi -v
```

Expected: `FAIL — undefined: latestByNameAmi`

**Step 3: Implement `latestByNameAmi`**

In `common/step_source_ami_info.go`, add after `mostRecentAmi` (after line 50):

```go
// latestByNameAmi returns the AMI with the lexicographically highest name.
func latestByNameAmi(images []types.Image) types.Image {
	sortedImages := images
	sort.Slice(sortedImages, func(i, j int) bool {
		return aws.ToString(sortedImages[i].Name) < aws.ToString(sortedImages[j].Name)
	})
	return sortedImages[len(sortedImages)-1]
}
```

Add `"github.com/aws/aws-sdk-go-v2/aws"` to the imports block in `step_source_ami_info.go` if not already present. (Check: it's not currently imported there — add it.)

**Step 4: Run tests to confirm they pass**

```bash
go test ./common/ -run TestLatestByNameAmi -v
```

Expected: `PASS`

**Step 5: Commit**

```bash
git add common/step_source_ami_info.go common/step_source_ami_info_test.go
git commit -m "feat: add latestByNameAmi helper for lexicographic AMI selection"
```

---

### Task 2: Add `GetFilteredImages` method with tests

**Files:**
- Modify: `common/ami_filter.go` (append after `GetFilteredImage`)
- Create: `common/ami_filter_test.go`

**Step 1: Write the failing tests**

Create `common/ami_filter_test.go`:

```go
// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDescribeImagesClient is a minimal Ec2Client that only overrides DescribeImages.
type mockDescribeImagesClient struct {
	clients.Ec2Client
	images []types.Image
	err    error
}

func (m *mockDescribeImagesClient) DescribeImages(_ context.Context, _ *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &ec2.DescribeImagesOutput{Images: m.images}, nil
}

func TestGetFilteredImages_ReturnsAllImages(t *testing.T) {
	mockClient := &mockDescribeImagesClient{
		images: []types.Image{
			{ImageId: aws.String("ami-1"), Name: aws.String("image-a")},
			{ImageId: aws.String("ami-2"), Name: aws.String("image-b")},
		},
	}
	opts := AmiFilterOptions{
		Owners:  []string{"123456789"},
		Filters: map[string]string{"name": "image-*"},
	}
	images, err := opts.GetFilteredImages(context.Background(), &ec2.DescribeImagesInput{}, mockClient)
	require.NoError(t, err)
	assert.Len(t, images, 2)
}

func TestGetFilteredImages_EmptyResultReturnsError(t *testing.T) {
	mockClient := &mockDescribeImagesClient{images: []types.Image{}}
	opts := AmiFilterOptions{Owners: []string{"123456789"}}
	_, err := opts.GetFilteredImages(context.Background(), &ec2.DescribeImagesInput{}, mockClient)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "No AMI was found")
}

func TestGetFilteredImages_APIErrorPropagated(t *testing.T) {
	mockClient := &mockDescribeImagesClient{err: fmt.Errorf("aws api error")}
	opts := AmiFilterOptions{Owners: []string{"123456789"}}
	_, err := opts.GetFilteredImages(context.Background(), &ec2.DescribeImagesInput{}, mockClient)
	assert.Error(t, err)
}
```

**Step 2: Run tests to confirm they fail**

```bash
go test ./common/ -run TestGetFilteredImages -v
```

Expected: `FAIL — undefined: GetFilteredImages`

**Step 3: Implement `GetFilteredImages`**

In `common/ami_filter.go`, append after `GetFilteredImage` (after line 108):

```go
// GetFilteredImages returns all images matching the filters without selecting one.
// Returns an error if no images match. Unlike GetFilteredImage, it does not enforce
// a single-result constraint — the caller is responsible for selection.
func (d *AmiFilterOptions) GetFilteredImages(ctx context.Context, params *ec2.DescribeImagesInput, client clients.Ec2Client) ([]types.Image, error) {
	if len(d.Filters) > 0 {
		amiFilters, err := buildEc2Filters(d.Filters)
		if err != nil {
			return nil, fmt.Errorf("Couldn't parse ami filters: %s", err)
		}
		params.Filters = amiFilters
	}
	if len(d.Owners) > 0 {
		params.Owners = d.GetOwners()
	}

	params.IncludeDeprecated = &d.IncludeDeprecated

	log.Printf("Using AMI Filters %s", prettyFilters(params))
	imageResp, err := client.DescribeImages(ctx, params, func(o *ec2.Options) {
		o.Retryer = retry.NewStandard(func(so *retry.StandardOptions) {
			so.MaxAttempts = 11
		})
	})
	if err != nil {
		return nil, fmt.Errorf("Error querying AMI: %s", err)
	}

	if len(imageResp.Images) == 0 {
		return nil, fmt.Errorf("No AMI was found matching filters: %v", params)
	}

	return imageResp.Images, nil
}
```

**Step 4: Run tests to confirm they pass**

```bash
go test ./common/ -run TestGetFilteredImages -v
```

Expected: `PASS`

**Step 5: Commit**

```bash
git add common/ami_filter.go common/ami_filter_test.go
git commit -m "feat: add GetFilteredImages method returning raw image slice"
```

---

### Task 3: Add `FakeAccessConfigWithEC2Client` test helper

**Files:**
- Modify: `common/test_helper_funcs.go` (append)

This helper is needed by the datasource package tests to inject a mock EC2 client.

**Step 1: Append to `common/test_helper_funcs.go`**

```go
// FakeAccessConfigWithEC2Client returns a fake AccessConfig that uses the provided
// getClient function to create EC2 clients. Use this in tests that need to inject
// a mock client with specific DescribeImages behavior.
func FakeAccessConfigWithEC2Client(getClient func() clients.Ec2Client) *AccessConfig {
	accessConfig := AccessConfig{
		getEC2Client: getClient,
		PollingConfig: new(AWSPollingConfig),
	}
	accessConfig.config = mustLoadConfig(config.WithRegion("us-west-1"))
	return &accessConfig
}
```

**Step 2: Run existing common tests to confirm nothing broke**

```bash
go test ./common/ -v -count=1 2>&1 | tail -20
```

Expected: all existing tests still `PASS`

**Step 3: Commit**

```bash
git add common/test_helper_funcs.go
git commit -m "test: add FakeAccessConfigWithEC2Client helper for datasource tests"
```

---

### Task 4: Add `SortBy` field + validation to the datasource Config

**Files:**
- Modify: `datasource/ami/data.go`
- Modify: `datasource/ami/data_test.go` (append)

**Step 1: Write the failing validation tests**

Append to `datasource/ami/data_test.go`:

```go
func TestDatasourceConfigure_SortByName_Valid(t *testing.T) {
	datasource := Datasource{
		config: Config{
			AmiFilterOptions: awscommon.AmiFilterOptions{
				Owners:  []string{"1234567"},
				Filters: map[string]string{"name": "my-ami-*"},
			},
			SortBy: "name",
		},
	}
	if err := datasource.Configure(nil); err != nil {
		t.Fatalf("Expected no error with sort_by=name, got: %s", err)
	}
}

func TestDatasourceConfigure_SortByInvalid_Error(t *testing.T) {
	datasource := Datasource{
		config: Config{
			AmiFilterOptions: awscommon.AmiFilterOptions{
				Owners:  []string{"1234567"},
				Filters: map[string]string{"name": "my-ami-*"},
			},
			SortBy: "creation_date",
		},
	}
	if err := datasource.Configure(nil); err == nil {
		t.Fatal("Expected error for invalid sort_by value, got nil")
	}
}

func TestDatasourceConfigure_SortByAndMostRecent_NoError(t *testing.T) {
	datasource := Datasource{
		config: Config{
			AmiFilterOptions: awscommon.AmiFilterOptions{
				Owners:     []string{"1234567"},
				Filters:    map[string]string{"name": "my-ami-*"},
				MostRecent: true,
			},
			SortBy: "name",
		},
	}
	// sort_by wins silently — no validation error
	if err := datasource.Configure(nil); err != nil {
		t.Fatalf("Expected no error when both sort_by and most_recent are set, got: %s", err)
	}
}
```

**Step 2: Run tests to confirm they fail**

```bash
go test ./datasource/ami/ -run "TestDatasourceConfigure_SortBy" -v
```

Expected: `FAIL — undefined field SortBy` (or similar compilation error)

**Step 3: Add `SortBy` field and validation to `datasource/ami/data.go`**

Change the `Config` struct (lines 27–31):

```go
type Config struct {
	common.PackerConfig        `mapstructure:",squash"`
	awscommon.AccessConfig     `mapstructure:",squash"`
	awscommon.AmiFilterOptions `mapstructure:",squash"`
	// Selects the AMI with the lexicographically highest name when set to "name".
	// Cannot be used together with most_recent; sort_by takes precedence.
	// Currently only "name" is supported.
	SortBy string `mapstructure:"sort_by"`
}
```

Add validation in `Configure()`, after the `NoOwner()` check (after line 51):

```go
if d.config.SortBy != "" && d.config.SortBy != "name" {
	errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(
		"sort_by must be \"name\" or unset; got %q", d.config.SortBy))
}
```

**Step 4: Run tests to confirm they pass**

```bash
go test ./datasource/ami/ -run "TestDatasourceConfigure" -v
```

Expected: all `PASS`

**Step 5: Commit**

```bash
git add datasource/ami/data.go datasource/ami/data_test.go
git commit -m "feat: add sort_by field and validation to amazon-ami datasource config"
```

---

### Task 5: Update `Execute()` to use `sort_by`, with mock test

**Files:**
- Modify: `datasource/ami/data.go` (update `Execute`)
- Modify: `datasource/ami/data_test.go` (append)

**Step 1: Write the failing Execute test**

Append to `datasource/ami/data_test.go`. First, add new imports at the top (update the import block):

```go
import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
)
```

Then append the test and mock struct:

```go
// mockAmiEC2Client returns a fixed set of images for DescribeImages calls.
type mockAmiEC2Client struct {
	clients.Ec2Client
	images []ec2types.Image
}

func (m *mockAmiEC2Client) DescribeImages(_ context.Context, _ *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return &ec2.DescribeImagesOutput{Images: m.images}, nil
}

func TestDatasourceExecute_SortByName_SelectsLexicographicallyLast(t *testing.T) {
	mockClient := &mockAmiEC2Client{
		images: []ec2types.Image{
			{
				ImageId:      aws.String("ami-sp6"),
				Name:         aws.String("suse-sles-15-sp6-v20250210-hvm-ssd-x86_64"),
				CreationDate: aws.String("2025-02-10T00:00:00Z"),
				OwnerId:      aws.String("013907871322"),
			},
			{
				ImageId:      aws.String("ami-sp7"),
				Name:         aws.String("suse-sles-15-sp7-v20241105-hvm-ssd-x86_64"),
				CreationDate: aws.String("2024-11-05T00:00:00Z"),
				OwnerId:      aws.String("013907871322"),
			},
		},
	}

	ds := Datasource{
		config: Config{
			AccessConfig: *awscommon.FakeAccessConfigWithEC2Client(func() clients.Ec2Client {
				return mockClient
			}),
			AmiFilterOptions: awscommon.AmiFilterOptions{
				Owners:  []string{"013907871322"},
				Filters: map[string]string{"name": "suse-sles-15-sp*"},
			},
			SortBy: "name",
		},
	}

	result, err := ds.Execute()
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %s", err)
	}

	// The result is a cty.Value — extract the "id" attribute
	idVal := result.GetAttr("id")
	if idVal.AsString() != "ami-sp7" {
		t.Errorf("Expected ami-sp7 (highest name), got %s", idVal.AsString())
	}
}
```

**Step 2: Run test to confirm it fails**

```bash
go test ./datasource/ami/ -run TestDatasourceExecute_SortByName -v
```

Expected: `FAIL` — `Execute()` ignores `SortBy` and calls `GetFilteredImage`, which errors because multiple results exist without `most_recent = true`.

**Step 3: Update `Execute()` in `datasource/ami/data.go`**

Replace the current `Execute()` body (lines 78–104) with:

```go
func (d *Datasource) Execute() (cty.Value, error) {
	ctx := context.TODO()
	client, err := d.config.NewEC2Client(ctx)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), err
	}

	var image *ec2.types.Image
	if d.config.SortBy == "name" {
		images, err := d.config.AmiFilterOptions.GetFilteredImages(ctx, &ec2.DescribeImagesInput{}, client)
		if err != nil {
			return cty.NullVal(cty.EmptyObject), err
		}
		selected := awscommon.LatestByNameAmi(images)
		image = &selected
	} else {
		image, err = d.config.AmiFilterOptions.GetFilteredImage(ctx, &ec2.DescribeImagesInput{}, client)
		if err != nil {
			return cty.NullVal(cty.EmptyObject), err
		}
	}

	imageTags := make(map[string]string, len(image.Tags))
	for _, tag := range image.Tags {
		imageTags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	output := DatasourceOutput{
		ID:           aws.ToString(image.ImageId),
		Name:         aws.ToString(image.Name),
		CreationDate: aws.ToString(image.CreationDate),
		Owner:        aws.ToString(image.OwnerId),
		OwnerName:    aws.ToString(image.ImageOwnerAlias),
		Tags:         imageTags,
	}
	return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
}
```

**Important note:** `latestByNameAmi` is unexported in `common`. Since `Execute()` is in the `ami` package (different package), it can't call `awscommon.latestByNameAmi`. You must **export** the function by renaming it to `LatestByNameAmi` in `common/step_source_ami_info.go`.

Update `common/step_source_ami_info.go`:
- Rename `latestByNameAmi` → `LatestByNameAmi`
- Update the three test calls in `common/step_source_ami_info_test.go` to use `LatestByNameAmi`

Also update the import in `datasource/ami/data.go` — add the `ec2/types` import alias:

```go
import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	awscommon "github.com/hashicorp/packer-plugin-amazon/common"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/hcl2helper"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/zclconf/go-cty/cty"
)
```

The `image` variable type in the `else` branch is `*types.Image` (returned by `GetFilteredImage`). Use a temporary variable to avoid shadowing the outer `image`:

```go
var image *ec2types.Image  // use ec2types alias for types.Image
```

Actually, looking at the existing code more carefully — `GetFilteredImage` returns `*types.Image` where `types` is `github.com/aws/aws-sdk-go-v2/service/ec2/types`. The cleanest approach is to keep the existing pattern with a local variable name. Here is the corrected `Execute()`:

```go
func (d *Datasource) Execute() (cty.Value, error) {
	ctx := context.TODO()
	client, err := d.config.NewEC2Client(ctx)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), err
	}

	var image *ec2types.Image
	if d.config.SortBy == "name" {
		images, imagesErr := d.config.AmiFilterOptions.GetFilteredImages(ctx, &ec2.DescribeImagesInput{}, client)
		if imagesErr != nil {
			return cty.NullVal(cty.EmptyObject), imagesErr
		}
		selected := awscommon.LatestByNameAmi(images)
		image = &selected
	} else {
		image, err = d.config.AmiFilterOptions.GetFilteredImage(ctx, &ec2.DescribeImagesInput{}, client)
		if err != nil {
			return cty.NullVal(cty.EmptyObject), err
		}
	}

	imageTags := make(map[string]string, len(image.Tags))
	for _, tag := range image.Tags {
		imageTags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	output := DatasourceOutput{
		ID:           aws.ToString(image.ImageId),
		Name:         aws.ToString(image.Name),
		CreationDate: aws.ToString(image.CreationDate),
		Owner:        aws.ToString(image.OwnerId),
		OwnerName:    aws.ToString(image.ImageOwnerAlias),
		Tags:         imageTags,
	}
	return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
}
```

Add `ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"` to imports.

**Step 4: Run test to confirm it passes**

```bash
go test ./datasource/ami/ -run TestDatasourceExecute_SortByName -v
```

Expected: `PASS`

**Step 5: Run all datasource tests**

```bash
go test ./datasource/ami/ -v -count=1
```

Expected: all `PASS`

**Step 6: Run all common tests**

```bash
go test ./common/ -v -count=1 2>&1 | tail -30
```

Expected: all `PASS`

**Step 7: Commit**

```bash
git add common/step_source_ami_info.go common/step_source_ami_info_test.go \
        datasource/ami/data.go datasource/ami/data_test.go
git commit -m "feat: implement sort_by=name in Execute() using LatestByNameAmi"
```

---

### Task 6: Update the auto-generated HCL2 spec

**Files:**
- Modify: `datasource/ami/data.hcl2spec.go`

The `data.hcl2spec.go` file is auto-generated by `packer-sdc mapstructure-to-hcl2`. You need to add the `sort_by` attribute manually (or regenerate if `packer-sdc` is available).

**Step 1: Try auto-regeneration**

```bash
which packer-sdc
```

- If found: `go generate ./datasource/ami/` — skip to Step 3.
- If not found: do Step 2.

**Step 2: Manually add `sort_by` to `data.hcl2spec.go`**

In `FlatConfig` struct (after `IncludeDeprecated` on line 41), add:

```go
SortBy *string `mapstructure:"sort_by" cty:"sort_by" hcl:"sort_by"`
```

In `HCL2Spec()` map (after the `"include_deprecated"` entry on line 83), add:

```go
"sort_by": &hcldec.AttrSpec{Name: "sort_by", Type: cty.String, Required: false},
```

**Step 3: Verify the build still compiles**

```bash
go build ./datasource/ami/
```

Expected: no errors

**Step 4: Commit**

```bash
git add datasource/ami/data.hcl2spec.go
git commit -m "feat: add sort_by to generated HCL2 spec for amazon-ami datasource"
```

---

### Task 7: Final verification — run all tests

**Step 1: Run the full non-acceptance test suite**

```bash
go test $(go list ./... | grep -v acc) -count=1 2>&1 | tail -30
```

Expected: all `PASS`, no compilation errors

**Step 2: Run vet and build**

```bash
go vet ./... && go build ./...
```

Expected: no errors or warnings

**Step 3: Commit if anything was fixed**

Only commit if Step 1 or 2 revealed issues requiring fixes. Otherwise, the work is complete.
