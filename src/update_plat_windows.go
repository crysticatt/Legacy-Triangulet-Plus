//go:build windows

package main

import "regexp"

// The release file for Windows is named like TrianguletOpener-v36.exe (the installer,
// TrianguletOpener-Setup-v36.exe, is not matched).
var winAsset = regexp.MustCompile(`(?i)^TrianguletOpener-v[0-9.]+\.exe$`)

var assetMatch = func(name string) bool { return winAsset.MatchString(name) }

// looksExecutable checks for the "MZ" header every Windows program starts with.
var looksExecutable = func(b []byte) bool { return len(b) > 1 && b[0] == 'M' && b[1] == 'Z' }
