//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"sync"
)

var (
	cafMu  sync.Mutex
	cafCmd *exec.Cmd
)

// On a Mac, `caffeinate -i -w <our pid>` stops idle sleep and exits by itself when we do.
func init() {
	setAwake = func(on bool) {
		cafMu.Lock()
		defer cafMu.Unlock()
		if !on {
			if cafCmd != nil {
				_ = cafCmd.Process.Kill()
				_, _ = cafCmd.Process.Wait()
				cafCmd = nil
			}
			return
		}
		if cafCmd != nil {
			return
		}
		cmd := exec.Command("/usr/bin/caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
		if cmd.Start() != nil {
			return // not a Mac (or no caffeinate): nothing to do
		}
		cafCmd = cmd
		go func(c *exec.Cmd) {
			_ = c.Wait()
			cafMu.Lock()
			if cafCmd == c {
				cafCmd = nil
			}
			cafMu.Unlock()
		}(cmd)
	}
}
