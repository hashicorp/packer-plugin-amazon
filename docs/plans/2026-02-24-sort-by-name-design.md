# Design: amazon-ami datasource `sort_by` option

**Date:** 2026-02-24
**Issue:** #650
**Status:** Approved

## Problem

The `amazon-ami` datasource selects AMIs by creation date (`most_recent = true`). When multiple AMI
families with semantic versioning coexist in AWS, a patch release for an older version can have a
newer creation date than a later major/minor version, causing the wrong AMI to be selected.

Example: SUSE publishes patches for SP6 after SP7 is released. With a wildcard filter and
`most_recent = true`, Packer may select SP6 (newer creation date) over SP7 (older creation date,
higher version).

## Solution

Add a `sort_by = "name"` option to the `amazon-ami` datasource that selects the AMI with the
lexicographically highest name, correctly reflecting semantic versioning in AMI naming conventions.

## Scope

Datasource only (`datasource/ami/`). The shared `AmiFilterOptions` struct used by builders is not
modified.

## Design Decisions

- `sort_by = "name"` selects the **lexicographically last** (highest) AMI name.
- Only `"name"` is a valid value for `sort_by`.
- If both `sort_by = "name"` and `most_recent = true` are set, `sort_by` wins silently.
- Invalid `sort_by` values produce a validation error at configure time.

## Architecture

### 1. Schema (`datasource/ami/data.go`)

Add `SortBy string` to the datasource `Config` struct (not to the shared `AmiFilterOptions`):

```go
type Config struct {
    common.PackerConfig        `mapstructure:",squash"`
    awscommon.AccessConfig     `mapstructure:",squash"`
    awscommon.AmiFilterOptions `mapstructure:",squash"`
    SortBy string `mapstructure:"sort_by"`
}
```

Validation in `Configure()`: reject any `sort_by` value other than `""` and `"name"`.

The `data.hcl2spec.go` file is auto-generated and must be regenerated after this change.

### 2. New `GetFilteredImages` method (`common/ami_filter.go`)

A new method alongside the existing `GetFilteredImage` (singular) that returns the full
`[]types.Image` slice without selecting one. Shares the same filter/owner/retry setup. Errors if
the result set is empty.

```go
func (d *AmiFilterOptions) GetFilteredImages(ctx context.Context, params *ec2.DescribeImagesInput, client clients.Ec2Client) ([]types.Image, error)
```

`GetFilteredImage` (singular) is unchanged, preserving all existing builder behavior.

### 3. Name-sorting helper (`common/step_source_ami_info.go`)

A new `latestByNameAmi` function alongside the existing `mostRecentAmi`:

```go
func latestByNameAmi(images []types.Image) types.Image {
    sort.Slice(images, func(i, j int) bool {
        return aws.ToString(images[i].Name) < aws.ToString(images[j].Name)
    })
    return images[len(images)-1]
}
```

### 4. Updated `Execute()` (`datasource/ami/data.go`)

```go
if d.config.SortBy == "name" {
    images, err := d.config.AmiFilterOptions.GetFilteredImages(ctx, &ec2.DescribeImagesInput{}, client)
    // handle err
    image = latestByNameAmi(images)
} else {
    image, err = d.config.AmiFilterOptions.GetFilteredImage(ctx, &ec2.DescribeImagesInput{}, client)
    // handle err
}
```

## Testing

### Unit tests — `common/step_source_ami_info_test.go`
- Multiple AMIs with different names → returns lexicographically last
- Single AMI → returns it unchanged
- AMIs with identical names → returns one without panic

### Unit tests — `common/ami_filter.go` (alongside existing filter tests)
- Empty result → error
- Non-empty result → returns full slice

### Unit tests — `datasource/ami/data_test.go`
- `sort_by = "name"` with valid owners/filters → no error
- `sort_by = "invalid"` → validation error
- `sort_by = "name"` + `most_recent = true` → no error
- `Execute()` with mock returning multiple AMIs → returns lexicographically highest name

## Files Changed

| File | Change |
|------|--------|
| `datasource/ami/data.go` | Add `SortBy` field, validation, updated `Execute()` |
| `datasource/ami/data.hcl2spec.go` | Regenerate with `go generate` |
| `common/ami_filter.go` | Add `GetFilteredImages` method |
| `common/step_source_ami_info.go` | Add `latestByNameAmi` helper |
| `datasource/ami/data_test.go` | New unit tests for `sort_by` |
| `common/step_source_ami_info_test.go` | New unit tests for `latestByNameAmi` |
