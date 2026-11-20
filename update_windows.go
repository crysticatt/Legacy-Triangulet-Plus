//go:build windows

package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// cleanupOldExe removes the previous version left behind by an update.
func cleanupOldExe() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	go func() {
		for i := 0; i < 10; i++ {
			if err := os.Remove(exe + ".old"); err == nil || os.IsNotExist(err) {
				return
			}
			time.Sleep(time.Second)
		}
	}()
}

func copyFileTo(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// applyUpdate swaps the running program for the downloaded one and starts it. Windows lets a running
// exe be renamed but not overwritten, so the old file is moved aside first.
func applyUpdate(newExe string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return errors.New("could not replace the program file (is it in a protected folder?): " + err.Error())
	}
	if err := copyFileTo(newExe, exe); err != nil {
		_ = os.Remove(exe)
		_ = os.Rename(old, exe)
		return err
	}
	_ = os.Remove(newExe)
	args := []string{}
	for _, a := range os.Args[1:] {
		if a != "--updated" {
			args = append(args, a)
		}
	}
	cmd := exec.Command(exe, append(args, "--updated")...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008} // DETACHED_PROCESS
	if err := cmd.Start(); err != nil {
		return errors.New("updated, but the new copy did not start. Start it yourself: " + err.Error())
	}
	pPostMessage.Call(trayHwnd, wmPopupCmd, 3, 0) // quit this copy; the new one is waiting for it
	return nil
}
