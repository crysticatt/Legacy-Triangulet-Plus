//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// dialDiscord opens the first Discord IPC pipe that answers (discord-ipc-0 ... 9).
func dialDiscord(string) (io.ReadWriteCloser, error) {
	for i := 0; i < 10; i++ {
		f, err := os.OpenFile(fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i), os.O_RDWR, 0)
		if err == nil {
			return f, nil
		}
	}
	return nil, errors.New("discord is not running")
}
