// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-amazon/common/clients"
)

func listEC2Regions(ctx context.Context, ec2conn clients.Ec2Client) ([]string, error) {
	var regions []string
	resultRegions, err := ec2conn.DescribeRegions(ctx, nil)
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
	ec2conn, err := c.NewEC2Connection(ctx)
	if err != nil {
		return err
	}

	validRegions, err := listEC2Regions(ctx, ec2conn)
	if err != nil {
		return err
	}

	var invalidRegions []string
	for _, region := range regions {
		if region == "" {
			continue
		}
		found := false
		for _, validRegion := range validRegions {
			if region == validRegion {
				found = true
				break
			}
		}
		if !found {
			invalidRegions = append(invalidRegions, region)
		}
	}

	if len(invalidRegions) > 0 {
		return fmt.Errorf("Invalid region(s): %v, available regions: %v", invalidRegions, validRegions)
	}
	return nil
}
