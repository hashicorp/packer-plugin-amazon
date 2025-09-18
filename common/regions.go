// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/packer-plugin-amazon/common/clients"
)

func listEC2Regions(ctx context.Context, client clients.Ec2Client) ([]string, error) {
	var regions []string
	resultRegions, err := client.DescribeRegions(ctx, nil)
	if err != nil {
		return []string{}, err
	}
	for _, region := range resultRegions.Regions {
		regions = append(regions, *region.RegionName)
	}

	return regions, nil
}

// ValidateRegion returns an nil if the regions are valid
// and exists; otherwise an error.
// ValidateRegion calls ec2conn.DescribeRegions to get the list of
// regions available to this account.
func (c *AccessConfig) ValidateRegion(ctx context.Context, regions ...string) error {
	client, err := c.NewEC2Client(ctx)
	if err != nil {
		return err
	}
	validRegions, err := listEC2Regions(ctx, client)
	if err != nil {
		return err
	}

	var invalidRegions []string
	for _, region := range regions {
		if region == "" {
			continue
		}
		found := slices.Contains(validRegions, region)
		if !found {
			invalidRegions = append(invalidRegions, region)
		}
	}

	if len(invalidRegions) > 0 {
		return fmt.Errorf("Invalid region(s): %v, available regions: %v", invalidRegions, validRegions)
	}
	return nil
}
