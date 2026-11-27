//go:build !windows

package main

import (
	"bytes"
	"regexp"
	"runtime"
)

// The release file for a Mac is a bare program named like TrianguletOpener-mac-arm64-v36
// (or -amd64-) next to the .zip that people download by hand.
var macAsset = regexp.MustCompile(`^Triangulet(Opener|Plus)-mac-` + runtime.GOARCH + `-v[0-9.]+$`)

var assetMatch = func(name string) bool { return macAsset.MatchString(name) }

// looksExecutable checks for a Mach-O (Mac) or ELF (Linux) header.
var looksExecutable = func(b []byte) bool {
	for _, m := range [][]byte{
		{0xcf, 0xfa, 0xed, 0xfe}, // Mach-O 64-bit
		{0xca, 0xfe, 0xba, 0xbe}, // Mach-O universal
		{0x7f, 'E', 'L', 'F'},
	} {
		if bytes.HasPrefix(b, m) {
			return true
		}
	}
	return false
}
