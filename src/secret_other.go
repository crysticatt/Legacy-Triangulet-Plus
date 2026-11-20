//go:build !windows

package main

import (
	"os/exec"
	"runtime"
	"strings"
)

// On a Mac the secret goes into the login Keychain through the built-in `security` tool.
// (The value is visible to other programs of the same user for a split second while it is passed on
// the command line; the Keychain entry itself is protected by the login.)
const keychainService = "TrianguletOpener"

func init() {
	if runtime.GOOS != "darwin" {
		return // other systems keep plain text (0600 file)
	}
	sealImpl = func(label, plain string) (string, bool) {
		err := exec.Command("/usr/bin/security", "add-generic-password", "-U", "-s", keychainService, "-a", label, "-w", plain).Run()
		if err != nil {
			return "", false
		}
		return "keychain:" + label, true
	}
	unsealImpl = func(label, stored string) (string, bool) {
		if !strings.HasPrefix(stored, "keychain:") {
			return "", false
		}
		out, err := exec.Command("/usr/bin/security", "find-generic-password", "-s", keychainService, "-a", strings.TrimPrefix(stored, "keychain:"), "-w").Output()
		if err != nil {
			return "", false
		}
		return strings.TrimRight(string(out), "\n"), true
	}
}
