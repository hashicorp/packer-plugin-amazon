// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package parameterstore

import "testing"

func TestDatasourceConfigure_EmptyParameterName(t *testing.T) {
	datasource := Datasource{
		config: Config{},
	}
	if err := datasource.Configure(nil); err == nil {
		t.Fatalf("Should error if parameter name is not specified")
	}
}
func TestDatasourceConfigure(t *testing.T) {
	datasource := Datasource{
		config: Config{
			Name: "parameter name",
		},
	}
	if err := datasource.Configure(nil); err != nil {
		t.Fatalf("err: %s", err)
	}
}
