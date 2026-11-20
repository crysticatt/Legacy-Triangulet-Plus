//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// replaceExe swaps the program file for the downloaded one. On a Mac/Linux a running program can be
// replaced by renaming a new file over it.
func replaceExe(newExe string) (exe string, err error) {
	exe, err = os.Executable()
	if err != nil {
		return "", err
	}
	if p, e := filepath.EvalSymlinks(exe); e == nil {
		exe = p
	}
	tmp := exe + ".new"
	in, err := os.ReadFile(newExe)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(tmp, in, 0o755); err != nil {
		return "", errors.New("could not write next to the program (is it in a protected folder?): " + err.Error())
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	_ = os.Remove(newExe)
	return exe, nil
}

// relaunchCommand builds the command that starts the updated copy after this one has quit.
func relaunchCommand(exe string) *exec.Cmd {
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i > 0 {
		app := exe[:i+4]
		return exec.Command("/bin/sh", "-c", "sleep 2; open -n "+shQuote(app)+" --args --no-browser")
	}
	return exec.Command("/bin/sh", "-c", "sleep 2; exec "+shQuote(exe)+" --no-browser")
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// applyUpdateOther installs the new file, then starts the new copy and asks this one to quit.
func applyUpdateOther(quit func()) func(string) error {
	return func(newExe string) error {
		exe, err := replaceExe(newExe)
		if err != nil {
			return err
		}
		cmd := relaunchCommand(exe)
		if err := cmd.Start(); err != nil {
			return errors.New("updated, but could not restart. Open the app again: " + err.Error())
		}
		go cmd.Wait()
		if quit != nil {
			quit()
		}
		return nil
	}
}
