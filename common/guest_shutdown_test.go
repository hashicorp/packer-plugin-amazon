// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package common

import "testing"

func TestGuestShutdownConfirmed(t *testing.T) {
	const token = "packer-shutdown-unique-token"
	for _, tc := range []struct {
		name, output string
		want         bool
	}{
		{"fresh poweroff", "[ 10.123] " + token + "\r\n[ 12.000] reboot: Power down\r\n", true},
		{"boot after poweroff", "[ 10.123] " + token + "\n[ 12.000] reboot: Power down\n[ 0.000] Linux version foo\n", false},
		{"old poweroff", "[ 5.000] reboot: Power down\n[ 10.123] " + token + "\n", false},
		{"wrong token", "[ 10.123] packer-shutdown-other\n[ 12.000] reboot: Power down\n", false},
		{"only marker", "[ 10.123] " + token + "\n", false},
		{"quoted command", "echo '[ 10.123] " + token + "'\n[ 12.000] reboot: Power down\n", false},
		{"userspace message", "[ 10.123] " + token + "\n[ 12.000] systemd-shutdown[1]: Powering off.\n", false},
		{"reboot", "[ 10.123] " + token + "\n[ 12.000] reboot: Restarting system\n", false},
		{"reboot between marker and poweroff", "[ 10.123] " + token + "\n[ 12.000] reboot: Restarting system\n[ 1.000] Linux version foo\n[ 5.000] reboot: Power down\n", false},
		{"empty token", "[ 12.000] reboot: Power down\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			marker := token
			if tc.name == "empty token" {
				marker = ""
			}
			if got := guestShutdownConfirmed(tc.output, marker); got != tc.want {
				t.Fatalf("got %t, want %t", got, tc.want)
			}
		})
	}
}
