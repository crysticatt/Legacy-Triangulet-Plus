//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

// hub is the opener the window and tray control.
var hub *Hub

// bootEnabled asks Task Scheduler whether the boot task exists.
func bootEnabled() bool {
	cmd := exec.Command("schtasks", "/Query", "/TN", bootTaskName)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run() == nil
}

var (
	bootCached  bool
	bootChecked time.Time
	bootBusy    bool
)

// bootEnabledCached refreshes the answer in the background at most every few seconds.
func bootEnabledCached() bool {
	if !bootBusy && time.Since(bootChecked) > 4*time.Second {
		bootBusy = true
		bootChecked = time.Now()
		go func() {
			v := bootEnabled()
			bootCached, bootBusy = v, false
			dirty = true
		}()
	}
	return bootCached
}

// setBoot creates or removes the task. Creating a task that runs at startup as SYSTEM
// needs administrator rights, so Windows shows its normal approval (UAC) prompt.
func setBoot(on bool) {
	args := bootDeleteArgs()
	if on {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		args = bootCreateArgs(exe, dataDir())
	}
	verb, file, params := utf16Ptr("runas"), utf16Ptr("schtasks.exe"), utf16Ptr(args)
	r, _, _ := pShellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(params)), 0, 0 /*SW_HIDE*/)
	if r <= 32 {
		messageBox("Triangulet Plus", "Windows did not allow the change (the approval prompt was declined).", mbOK|mbIconWarning)
		return
	}
	bootChecked = time.Time{}
	go func() { // the prompt is answered at some point: look again a few times
		for i := 0; i < 20; i++ {
			time.Sleep(1500 * time.Millisecond)
			if v := bootEnabled(); v == on {
				bootCached = v
				dirty = true
				return
			}
		}
	}()
}

var pMessageBeep = user32.NewProc("MessageBeep")

// openPath opens a file with its default program.
func openPath(p string) {
	verb, file := utf16Ptr("open"), utf16Ptr(p)
	pShellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, 1)
}

func init() {
	alertHook = func(kind string) {
		if kind == "rare" {
			pMessageBeep.Call(0x40) // MB_ICONASTERISK
		} else {
			pMessageBeep.Call(0x30) // MB_ICONEXCLAMATION
		}
	}
}
