//go:build windows

// Installer for Triangulet Opener (Windows). One file: it contains the app and puts it in
// %LOCALAPPDATA%\Programs\Triangulet Opener, makes Desktop and Start Menu shortcuts and an
// entry in Settings > Apps so it can be uninstalled. No admin rights needed.
//
//	Setup.exe            install (asks first)
//	Setup.exe /S         install silently
//	Setup.exe /uninstall remove it
package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

//go:embed TrianguletOpener.exe
var appExe []byte

var version = "dev" // set with -ldflags "-X main.version=36"

const (
	appTitle  = "Triangulet Opener"
	exeName   = "TrianguletOpener.exe"
	uninstKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\TrianguletOpener`
	runKey    = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
)

var (
	user32  = syscall.NewLazyDLL("user32.dll")
	pMsgBox = user32.NewProc("MessageBoxW")
)

func msgBox(text string, flags uintptr) int {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(appTitle + " " + version)
	r, _, _ := pMsgBox.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
	return int(r)
}

const (
	mbOK, mbYesNo, mbIconInfo, mbIconWarn, mbIconQuestion = 0x0, 0x4, 0x40, 0x30, 0x20
	idYes                                                 = 6
)

func hidden(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return c
}

func installDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}
	return filepath.Join(base, "Programs", appTitle)
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// shortcut makes a .lnk with PowerShell; folder is "Desktop" or "Programs" (Start Menu).
func shortcut(folder, target, workDir string) error {
	script := `$ErrorActionPreference='Stop';` +
		`$sh=New-Object -ComObject WScript.Shell;` +
		`$dir=` + map[string]string{
		"Desktop":  `[Environment]::GetFolderPath('Desktop')`,
		"Programs": `[Environment]::GetFolderPath('Programs')`,
	}[folder] + `;` +
		`$l=$sh.CreateShortcut((Join-Path $dir ` + psQuote(appTitle+".lnk") + `));` +
		`$l.TargetPath=` + psQuote(target) + `;$l.WorkingDirectory=` + psQuote(workDir) + `;` +
		`$l.IconLocation=` + psQuote(target+",0") + `;$l.Description='Open Triangulet capsules from the tray';$l.Save()`
	out, err := hidden("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func removeShortcuts() {
	script := `foreach($f in 'Desktop','Programs'){$p=Join-Path ([Environment]::GetFolderPath($f)) ` + psQuote(appTitle+".lnk") + `;Remove-Item -LiteralPath $p -Force -ErrorAction SilentlyContinue}`
	_ = hidden("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script).Run()
}

func killApp() {
	_ = hidden("taskkill.exe", "/F", "/IM", exeName).Run()
	// give Windows a moment to release the file
	for i := 0; i < 20; i++ {
		f, err := os.OpenFile(filepath.Join(installDir(), exeName), os.O_WRONLY, 0)
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			sleepMs(150)
			continue
		}
		f.Close()
		return
	}
}

func sleepMs(n int) { time.Sleep(time.Duration(n) * time.Millisecond) }

func reg(args ...string) error {
	return hidden("reg.exe", args...).Run()
}

func install(silent bool) error {
	dir := installDir()
	killApp()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	exe := filepath.Join(dir, exeName)
	// an old copy that is still locked: move it aside instead of failing
	if _, err := os.Stat(exe); err == nil {
		_ = os.Remove(exe + ".old")
		_ = os.Rename(exe, exe+".old")
	}
	if err := os.WriteFile(exe, appExe, 0o755); err != nil {
		return fmt.Errorf("could not write %s: %v", exe, err)
	}
	_ = os.Remove(exe + ".old")

	// keep a copy of this installer as the uninstaller
	self, err := os.Executable()
	if err != nil {
		return err
	}
	un := filepath.Join(dir, "Uninstall.exe")
	if b, err := os.ReadFile(self); err == nil && !strings.EqualFold(self, un) {
		_ = os.WriteFile(un, b, 0o755)
	}

	var warn []string
	if err := shortcut("Desktop", exe, dir); err != nil {
		warn = append(warn, "Desktop shortcut: "+err.Error())
	}
	if err := shortcut("Programs", exe, dir); err != nil {
		warn = append(warn, "Start Menu shortcut: "+err.Error())
	}
	set := func(name, typ, val string) { _ = reg("add", uninstKey, "/v", name, "/t", typ, "/d", val, "/f") }
	set("DisplayName", "REG_SZ", appTitle)
	set("DisplayVersion", "REG_SZ", version)
	set("Publisher", "REG_SZ", "crysticatt")
	set("InstallLocation", "REG_SZ", dir)
	set("DisplayIcon", "REG_SZ", exe)
	set("UninstallString", "REG_SZ", `"`+un+`" /uninstall`)
	set("NoModify", "REG_DWORD", "1")
	set("NoRepair", "REG_DWORD", "1")
	// if "run at startup" was already on (e.g. from a loose copy), point it at the installed program
	if reg("query", runKey, "/v", "TrianguletOpener") == nil {
		_ = reg("add", runKey, "/v", "TrianguletOpener", "/t", "REG_SZ", "/d", `"`+exe+`" --tray`, "/f")
	}
	if len(warn) > 0 {
		msgBox("Installed, but:\n\n"+strings.Join(warn, "\n"), mbOK|mbIconWarn)
	}
	return nil
}

func uninstall(silent bool) {
	if !silent && msgBox("Remove Triangulet Opener from this PC?", mbYesNo|mbIconQuestion) != idYes {
		return
	}
	killApp()
	removeShortcuts()
	_ = reg("delete", runKey, "/v", "TrianguletOpener", "/f")
	_ = hidden("schtasks.exe", "/Delete", "/TN", "TrianguletOpenerBoot", "/F").Run() // only works if run as admin
	_ = reg("delete", uninstKey, "/f")
	keep := true
	if !silent {
		keep = msgBox("Keep your settings, pull log and history?\n\nYes = keep them (handy if you install again)\nNo = delete them too", mbYesNo|mbIconQuestion) == idYes
	}
	if !keep {
		if cfg, err := os.UserConfigDir(); err == nil {
			_ = os.RemoveAll(filepath.Join(cfg, "TrianguletOpener"))
		}
	}
	dir := installDir()
	_ = os.Remove(filepath.Join(dir, exeName))
	// this program is running from inside the folder: let a helper delete it after we exit
	_ = hidden("cmd.exe", "/C", "ping 127.0.0.1 -n 3 >nul & rmdir /S /Q \""+dir+"\"").Start()
	if !silent {
		msgBox("Triangulet Opener was removed.\n\nIf you turned on \"Before sign-in\", open Task Scheduler and delete the task named TrianguletOpenerBoot (it needs admin).", mbOK|mbIconInfo)
	}
}

func main() {
	silent, noLaunch, un := false, false, false
	for _, a := range os.Args[1:] {
		switch strings.ToLower(a) {
		case "/s", "-s", "--silent", "/silent":
			silent = true
		case "/uninstall", "/u", "--uninstall":
			un = true
		case "/nolaunch", "--no-launch":
			noLaunch = true
		}
	}
	if un {
		uninstall(silent)
		return
	}
	if !silent && msgBox("Install Triangulet Opener "+version+"?\n\nIt will be installed for your account only, with a shortcut on your Desktop and in the Start Menu. No admin rights needed.", mbYesNo|mbIconQuestion) != idYes {
		return
	}
	if err := install(silent); err != nil {
		msgBox("Install failed:\n\n"+err.Error(), mbOK|mbIconWarn)
		os.Exit(1)
	}
	launch := !noLaunch
	if !silent {
		launch = msgBox("Installed.\n\nOpen Triangulet Opener now?", mbYesNo|mbIconInfo) == idYes
	}
	if launch {
		_ = exec.Command(filepath.Join(installDir(), exeName)).Start()
	}
}
