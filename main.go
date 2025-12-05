// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"os"

	"github.com/hashicorp/packer-plugin-amazon/builder/chroot"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebs"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebssurrogate"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebsvolume"
	"github.com/hashicorp/packer-plugin-amazon/builder/instance"
	"github.com/hashicorp/packer-plugin-amazon/datasource/ami"
	"github.com/hashicorp/packer-plugin-amazon/datasource/parameterstore"
	"github.com/hashicorp/packer-plugin-amazon/datasource/secretsmanager"
	amazonimport "github.com/hashicorp/packer-plugin-amazon/post-processor/import"
	"github.com/hashicorp/packer-plugin-amazon/version"
	"github.com/hashicorp/packer-plugin-sdk/plugin"
)

func main() {
	pps := plugin.NewSet()
	pps.RegisterBuilder("chroot", new(chroot.Builder))
	pps.RegisterBuilder("ebs", new(ebs.Builder))
	pps.RegisterBuilder("ebssurrogate", new(ebssurrogate.Builder))
	pps.RegisterBuilder("ebsvolume", new(ebsvolume.Builder))
	pps.RegisterBuilder("instance", new(instance.Builder))
	pps.RegisterDatasource("ami", new(ami.Datasource))
	pps.RegisterDatasource("secretsmanager", new(secretsmanager.Datasource))
	pps.RegisterDatasource("parameterstore", new(parameterstore.Datasource))
	pps.RegisterPostProcessor("import", new(amazonimport.PostProcessor))
	pps.SetVersion(version.PluginVersion)
	err := pps.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
