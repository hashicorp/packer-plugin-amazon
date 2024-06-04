// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"strings"
	"testing"
)

func TestStepSourceAmiInfo_BuildFilter_SingleValue(t *testing.T) {
	filter_key := "name"
	filter_value := "foo"
	filter_key2 := "name2"
	filter_value2 := " foo2"
	filter_key3 := "name3"
	filter_value3 := "foo3 "

	inputFilter := map[string]string{
		filter_key:  filter_value,
		filter_key2: filter_value2,
		filter_key3: filter_value3,
	}

	outputFilter, err := buildEc2Filters(inputFilter)

	if err != nil {
		t.Fatalf("Fail: should not have failed to parse filter:")
	}
	testFilter := map[string]string{
		filter_key:  filter_value,
		filter_key2: strings.TrimSpace(filter_value2),
		filter_key3: filter_value3,
	}

	// deconstruct filter back into things we can test
	foundMap := map[string]bool{filter_key: false, filter_key2: false}
	for _, filter := range outputFilter {
		for key, value := range testFilter {
			if *filter.Name == key && *filter.Values[0] == value {
				foundMap[key] = true
			}
		}
	}

	for k, v := range foundMap {
		if !v {
			t.Fatalf("Fail: should have found value for key: %s", k)
		}
	}
}

func TestStepSourceAmiInfo_BuildFilter_ListValue(t *testing.T) {
	filter_key := "name-no-space-between-comma"
	filter_value := "foo1-1,foo1-2,foo1-3"
	filter_key2 := "name-space-between-comma"
	filter_value2 := "foo2-1, foo2-2, foo2-3"
	filter_key3 := "name-embedded-comma-and-leading-space"
	filter_value3 := "\"foo3-1, with comma\",foo3-2 without comma, \" foo3-3\""

	inputFilter := map[string]string{
		filter_key:  filter_value,
		filter_key2: filter_value2,
		filter_key3: filter_value3,
	}

	outputFilter, err := buildEc2Filters(inputFilter)

	if err != nil {
		t.Fatalf("Fail: should not have failed to parse filter:")
	}
	testFilter := map[string][]string{
		filter_key:  {"foo1", "foo1-2", "foo1-3"},
		filter_key2: {"foo2-1", "foo2-2", "foo2-3"},
		filter_key3: {"foo3-1, with comma", "foo3-2 without comma", " foo3-3"},
	}

	// deconstruct filter back into things we can test
	foundMap := map[string]bool{filter_key: false, filter_key2: false}
	for _, filter := range outputFilter {
		for key, value := range testFilter {
			if *filter.Name == key {
				for idx, filter_value := range value {
					if *filter.Values[idx] == filter_value {
						foundMap[key] = true
					} else {
						foundMap[key] = false
					}
				}
			}
		}
	}

	for k, v := range foundMap {
		if !v {
			t.Fatalf("Fail: should have found value for key: %s", k)
		}
	}
}

func TestStepSourceAmiInfo_BuildFilter_ValueWithQuote(t *testing.T) {
	filter_key := "tag:test"
	filter_value := "{\"purpose\":\"testing\"}"
	filter_key2 := "tag:test"
	filter_value2 := " {\"purpose\":\"testing\"}"

	inputFilter := map[string]string{
		filter_key:  filter_value,
		filter_key2: filter_value2,
	}

	outputFilter, err := buildEc2Filters(inputFilter)

	if err != nil {
		t.Fatalf("Fail: should not have failed to parse filter: %v", err)
	}
	testFilter := map[string]string{
		filter_key:  filter_value,
		filter_key2: strings.TrimSpace(filter_value2),
	}

	// deconstruct filter back into things we can test
	foundMap := map[string]bool{filter_key: false, filter_key2: false}
	for _, filter := range outputFilter {
		for key, value := range testFilter {
			if *filter.Name == key && *filter.Values[0] == value {
				foundMap[key] = true
			}
		}
	}

	for k, v := range foundMap {
		if !v {
			t.Fatalf("Fail: should have found value for key: %s", k)
		}
	}
}
