//go:build !windows

package main

import (
	"context"
	"encoding/xml"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const launchLabel = "xyz.triangulet.opener"

var hub *Hub

func launchAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchLabel+".plist")
}

// launchPlist is the file macOS reads to start the opener when you log in.
func launchPlist(exe, dataDir string) string {
	esc := func(s string) string {
		var b strings.Builder
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>` + launchLabel + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + esc(exe) + `</string>
		<string>--background</string>
		<string>--data</string>
		<string>` + esc(dataDir) + `</string>
	</array>
	<key>RunAtLoad</key><true/>
	<key>ProcessType</key><string>Background</string>
	<key>StandardErrorPath</key><string>` + esc(filepath.Join(dataDir, "launch.log")) + `</string>
</dict>
</plist>
`
}

func loginItemOn() bool {
	_, err := os.Stat(launchAgentPath())
	return err == nil
}

func setLoginItem(on bool) error {
	p := launchAgentPath()
	if !on {
		if runtime.GOOS == "darwin" {
			_ = exec.Command("launchctl", "unload", p).Run()
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	// Only the file is written: macOS starts the job at the next login. Loading it now would
	// start a second copy.
	return os.WriteFile(p, []byte(launchPlist(exe, dataDir())), 0o644)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	_ = cmd.Start()
	if cmd.Process != nil {
		go cmd.Wait()
	}
}

func appleQuote(s string) string {
	s = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ").Replace(s)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func notify(title, text string) {
	if runtime.GOOS != "darwin" {
		return
	}
	script := fmt.Sprintf(`display notification "%s" with title "%s"`, appleQuote(text), appleQuote(title))
	cmd := exec.Command("osascript", "-e", script)
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

func playSound(kind string) {
	if runtime.GOOS != "darwin" {
		return
	}
	name := "Glass"
	if kind != "rare" {
		name = "Basso"
	}
	cmd := exec.Command("afplay", "/System/Library/Sounds/"+name+".aiff")
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

func main() {
	port := flag.Int("port", 8765, "port for the dashboard on 127.0.0.1")
	noBrowser := flag.Bool("no-browser", false, "don't open the dashboard in the browser")
	background := flag.Bool("background", false, "started at login: don't open the browser")
	data := flag.String("data", "", "folder for settings and logs")
	install := flag.Bool("install-login", false, "start the opener at login")
	uninstall := flag.Bool("uninstall-login", false, "don't start the opener at login")
	version := flag.Bool("version", false, "print the version")
	flag.Parse()
	if *version {
		fmt.Println("Triangulet Opener", appVersion)
		return
	}
	if *data != "" {
		os.Setenv("TRIOPENER_DIR", *data)
	}
	switch {
	case *install:
		if err := setLoginItem(true); err != nil {
			fmt.Println("could not set up start at login:", err)
			os.Exit(1)
		}
		fmt.Println("The opener will start at your next login.")
		return
	case *uninstall:
		if err := setLoginItem(false); err != nil {
			fmt.Println("could not remove start at login:", err)
			os.Exit(1)
		}
		fmt.Println("Start at login removed.")
		return
	}

	ln, already, err := listenDashboard(*port)
	if already {
		// another copy is running: show its dashboard instead of starting a second one
		if !*background && !*noBrowser {
			openBrowser("http://127.0.0.1:" + strconv.Itoa(*port))
		}
		return
	}
	if err != nil {
		fmt.Println("could not open a local port:", err)
		os.Exit(1)
	}
	actual := ln.Addr().(interface{ String() string }).String()
	portNum, _ := strconv.Atoi(actual[strings.LastIndex(actual, ":")+1:])

	hub = NewHub(dataDir())
	imgHub = hub
	sig := make(chan os.Signal, 1)
	quitNow := func() { sig <- syscall.SIGTERM }
	web := &webServer{hub: hub, port: portNum, loginItm: loginItemOn, setLogin: setLoginItem,
		quit: quitNow, applyUpd: applyUpdateOther(quitNow)}
	srv := &http.Server{Handler: web.handler(), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	_ = os.WriteFile(filepath.Join(dataDir(), "web.port"), []byte(strconv.Itoa(portNum)), 0o600)

	alertHook = playSound
	stopDiscord := make(chan struct{})
	go RunDiscord(hub, dialDiscord, 15*time.Second, stopDiscord)
	go func() { // desktop notifications for alerts
		for range time.Tick(time.Second) {
			if t, x, ok := hub.primary.takeBalloon(); ok {
				notify(t, x)
			}
		}
	}()

	// open on launch like the Windows app: start when a login is saved
	if s := hub.Snapshot(); hub.cfg().AutoStart && (s.HasToken || s.HasCreds) && (hub.cfg().Mode == "hunt" || s.Capsule != "") {
		hub.Start()
	}

	url := "http://127.0.0.1:" + strconv.Itoa(portNum)
	fmt.Println("Triangulet Opener " + appVersion + " is running. Dashboard: " + url)
	if !*background && !*noBrowser {
		openBrowser(url)
	}

	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	close(stopDiscord)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = srv.Shutdown(ctx)
	cancel()
	hub.Close()
	hub.primary.SaveStats()
}
