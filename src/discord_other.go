//go:build !windows

package main

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

// dialDiscord connects to Discord's local socket (discord-ipc-0 ... 9) in the places the
// Discord app puts it on a Mac or Linux.
func dialDiscord(string) (io.ReadWriteCloser, error) {
	var dirs []string
	for _, e := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if d := os.Getenv(e); d != "" {
			dirs = append(dirs, d)
		}
	}
	dirs = append(dirs, "/tmp")
	for _, d := range dirs {
		for i := 0; i < 10; i++ {
			if c, err := net.Dial("unix", filepath.Join(d, "discord-ipc-"+strconv.Itoa(i))); err == nil {
				return c, nil
			}
		}
	}
	return nil, errors.New("discord is not running")
}
