// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
)

// Returns the current host's public IP
// as returned from https://checkip.amazonaws.com
func CheckPublicIp() (net.IP, error) {
	ip, err := checkPublicIpImpl()
	if err != nil {
		return ip, fmt.Errorf("failed to get current host's public ip: %s", err)
	}
	return ip, nil
}

func checkPublicIpImpl() (net.IP, error) {
	client := cleanhttp.DefaultClient()

	res, err := client.Get("https://checkip.amazonaws.com")
	if err != nil {
		return nil, fmt.Errorf("GET failed: %s", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received status %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err)
	}

	bodyStr := strings.TrimSpace(string(body))

	ip := net.ParseIP(bodyStr)
	if ip == nil {
		return nil, fmt.Errorf("failed to parse response body: %s", bodyStr)
	}

	return ip, nil
}
