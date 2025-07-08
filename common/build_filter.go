// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"encoding/csv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Build a slice of EC2 (AMI/Subnet/VPC) filter options from the filters provided.
func buildEc2Filters(input map[string]string) ([]types.Filter, error) {
	var filters []types.Filter

	for k, v := range input {
		name := k
		csvReader := csv.NewReader(strings.NewReader(v))
		csvReader.TrimLeadingSpace = true
		csvReader.LazyQuotes = true

		values, err := csvReader.Read()
		if err != nil {
			return nil, err
		}

		filters = append(filters, types.Filter{
			Name:   &name,
			Values: values,
		})
	}
	return filters, nil
}
