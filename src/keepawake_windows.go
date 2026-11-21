//go:build windows

package main

import "syscall"

var pSetExecState = syscall.NewLazyDLL("kernel32.dll").NewProc("SetThreadExecutionState")

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

func init() {
	setAwake = func(on bool) {
		flags := uintptr(esContinuous)
		if on {
			flags |= esSystemRequired // keeps the PC awake; the display may still turn off
		}
		pSetExecState.Call(flags)
	}
}
