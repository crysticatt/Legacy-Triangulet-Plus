//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	winmm    = syscall.NewLazyDLL("winmm.dll")

	pRegisterClassEx       = user32.NewProc("RegisterClassExW")
	pCreateWindowEx        = user32.NewProc("CreateWindowExW")
	pDefWindowProc         = user32.NewProc("DefWindowProcW")
	pGetMessage            = user32.NewProc("GetMessageW")
	pTranslateMessage      = user32.NewProc("TranslateMessage")
	pDispatchMessage       = user32.NewProc("DispatchMessageW")
	pPostQuitMessage       = user32.NewProc("PostQuitMessage")
	pDestroyWindow         = user32.NewProc("DestroyWindow")
	pSetTimer              = user32.NewProc("SetTimer")
	pCreatePopupMenu       = user32.NewProc("CreatePopupMenu")
	pAppendMenu            = user32.NewProc("AppendMenuW")
	pDestroyMenu           = user32.NewProc("DestroyMenu")
	pTrackPopupMenu        = user32.NewProc("TrackPopupMenu")
	pSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	pPostMessage           = user32.NewProc("PostMessageW")
	pGetCursorPos          = user32.NewProc("GetCursorPos")
	pMessageBox            = user32.NewProc("MessageBoxW")
	pRegisterWindowMessage = user32.NewProc("RegisterWindowMessageW")
	pCreateIconIndirect    = user32.NewProc("CreateIconIndirect")
	pOpenClipboard         = user32.NewProc("OpenClipboard")
	pCloseClipboard        = user32.NewProc("CloseClipboard")
	pGetClipboardData      = user32.NewProc("GetClipboardData")
	pShellNotifyIcon       = shell32.NewProc("Shell_NotifyIconW")
	pCreateBitmap          = gdi32.NewProc("CreateBitmap")
	pDeleteObject          = gdi32.NewProc("DeleteObject")
	pGetModuleHandle       = kernel32.NewProc("GetModuleHandleW")
	pCreateMutex           = kernel32.NewProc("CreateMutexW")
	pGlobalLock            = kernel32.NewProc("GlobalLock")
	pGlobalUnlock          = kernel32.NewProc("GlobalUnlock")
	pTimeBeginPeriod       = winmm.NewProc("timeBeginPeriod")
	pShellExecute          = shell32.NewProc("ShellExecuteW")
	pLoadImage             = user32.NewProc("LoadImageW")
	pDestroyIcon           = user32.NewProc("DestroyIcon")
	pSetProcessDPIAware    = user32.NewProc("SetProcessDPIAware")
	pGetDpiForSystem       = user32.NewProc("GetDpiForSystem")
	pRegOpenKeyEx          = advapi32.NewProc("RegOpenKeyExW")
	pRegSetValueEx         = advapi32.NewProc("RegSetValueExW")
	pRegQueryValueEx       = advapi32.NewProc("RegQueryValueExW")
	pRegDeleteValue        = advapi32.NewProc("RegDeleteValueW")
	pRegCloseKey           = advapi32.NewProc("RegCloseKey")
)

const (
	wmNull    = 0x0000
	wmDestroy = 0x0002
	wmTimer   = 0x0113
	wmTray    = 0x8001 // WM_APP + 1

	wmLButtonUp  = 0x0202
	wmLButtonDbl = 0x0203
	wmRButtonUp  = 0x0205

	wmKeyDown = 0x0100
	vkReturn  = 0x0D

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2

	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4
	nifInfo    = 0x10

	niifInfo = 0x1

	mfGrayed    = 0x1
	mfDisabled  = 0x2
	mfSeparator = 0x800

	tpmRightButton = 0x2
	tpmReturnCmd   = 0x100

	mbOK              = 0x0
	mbIconInformation = 0x40
	mbIconWarning     = 0x30

	cfUnicodeText = 13

	hkcu          = 0x80000001
	keyQueryValue = 0x1
	keySetValue   = 0x2
	regSZ         = 1
	runKey        = `Software\Microsoft\Windows\CurrentVersion\Run`

	idShow   = 1
	idToggle = 2
	idToken  = 3
	idReset  = 4
	idFolder = 5
	idQuit   = 6
)

type point struct{ X, Y int32 }

type msgT struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeout         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

var (
	app            *App
	iconOff        uintptr
	iconOn         uintptr
	taskbarCreated uintptr
)

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func messageBox(title, text string, flags uintptr) {
	t, x := utf16Ptr(title), utf16Ptr(text)
	pMessageBox.Call(0, uintptr(unsafe.Pointer(x)), uintptr(unsafe.Pointer(t)), flags)
	runtime.KeepAlive(t)
	runtime.KeepAlive(x)
}

func copyUTF16(dst []uint16, s string) {
	u := utf16.Encode([]rune(s))
	if len(u) > len(dst)-1 {
		u = u[:len(dst)-1]
	}
	copy(dst, u)
	dst[len(u)] = 0
}

func hasArg(want string) bool {
	for _, a := range os.Args[1:] {
		if a == want {
			return true
		}
	}
	return false
}

// ---------- tray icon ----------

// iconFromPixels makes a 32x32 icon from straight-alpha BGRA pixels.
func iconFromPixels(pix []byte) uintptr {
	const size = 32
	mask := make([]byte, size*size/8)
	hColor, _, _ := pCreateBitmap.Call(size, size, 1, 32, uintptr(unsafe.Pointer(&pix[0])))
	hMask, _, _ := pCreateBitmap.Call(size, size, 1, 1, uintptr(unsafe.Pointer(&mask[0])))
	ii := iconInfo{FIcon: 1, HbmMask: hMask, HbmColor: hColor}
	h, _, _ := pCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	pDeleteObject.Call(hColor)
	pDeleteObject.Call(hMask)
	runtime.KeepAlive(pix)
	runtime.KeepAlive(mask)
	return h
}

// loadAppIcon loads the Triangulet icon stored in the exe (falls back to a drawn one).
func loadAppIcon() uintptr {
	h, _, _ := pLoadImage.Call(hInstance, 1, 1 /*IMAGE_ICON*/, 32, 32, 0)
	if h == 0 {
		h = iconFromPixels(iconPixels(appLogo(), false))
	}
	return h
}

func newNID(hwnd uintptr) *notifyIconData {
	n := &notifyIconData{HWnd: hwnd, UID: 1}
	n.CbSize = uint32(unsafe.Sizeof(*n))
	return n
}

func tooltip(s Snapshot) string {
	t := "Triangulet Plus"
	if s.Running {
		t = fmt.Sprintf("Opening - %s opened, %s/min", fmtInt(s.Opened), fmtInt(int(s.PerMin)))
	}
	if r := []rune(t); len(r) > 120 {
		t = string(r[:120])
	}
	return t
}

var (
	trayIcon    uintptr
	trayIconKey string
)

// currentIcon is the rarest pull (Mystical beats Chroma) with a green dot while running.
func currentIcon(s Snapshot) uintptr {
	key := fmt.Sprintf("%d/%v", s.IconVer, s.Running)
	if trayIcon != 0 && key == trayIconKey {
		return trayIcon
	}
	img, _ := hub.primary.ChromaImage()
	if img == nil {
		img = appLogo()
	}
	nh := iconFromPixels(iconPixels(img, s.Running))
	if nh == 0 {
		return trayIcon
	}
	old := trayIcon
	trayIcon, trayIconKey = nh, key
	if old != 0 {
		defer pDestroyIcon.Call(old)
	}
	return nh
}

func addTrayIcon(hwnd uintptr) {
	n := newNID(hwnd)
	n.UFlags = nifMessage | nifIcon | nifTip
	n.UCallbackMessage = wmTray
	n.HIcon = currentIcon(hub.Snapshot())
	copyUTF16(n.SzTip[:], "Triangulet Plus")
	pShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(n)))
}

func refreshTray(hwnd uintptr) {
	s := hub.Snapshot()
	n := newNID(hwnd)
	n.UFlags = nifIcon | nifTip
	n.HIcon = currentIcon(s)
	copyUTF16(n.SzTip[:], tooltip(s))
	if title, text, ok := hub.primary.takeBalloon(); ok {
		n.UFlags |= nifInfo
		n.DwInfoFlags = niifInfo
		copyUTF16(n.SzInfoTitle[:], title)
		copyUTF16(n.SzInfo[:], text)
	}
	pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(n)))
}

func removeTrayIcon(hwnd uintptr) {
	n := newNID(hwnd)
	pShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(n)))
}

// ---------- run at startup (HKCU Run key) ----------

func openRunKey(access uintptr) (uintptr, bool) {
	var h uintptr
	r, _, _ := pRegOpenKeyEx.Call(hkcu, uintptr(unsafe.Pointer(utf16Ptr(runKey))), 0, access, uintptr(unsafe.Pointer(&h)))
	return h, r == 0
}

func startupEnabled() bool {
	h, ok := openRunKey(keyQueryValue)
	if !ok {
		return false
	}
	defer pRegCloseKey.Call(h)
	r, _, _ := pRegQueryValueEx.Call(h, uintptr(unsafe.Pointer(utf16Ptr(appName))), 0, 0, 0, 0)
	return r == 0
}

// setStartup adds/removes the Run entry. At sign-in the app starts hidden in the tray (--tray).
func setStartup(on bool) {
	h, ok := openRunKey(keySetValue)
	if !ok {
		return
	}
	defer pRegCloseKey.Call(h)
	name := utf16Ptr(appName)
	if !on {
		pRegDeleteValue.Call(h, uintptr(unsafe.Pointer(name)))
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	val := utf16.Encode([]rune(`"` + exe + `" --tray`))
	val = append(val, 0)
	pRegSetValueEx.Call(h, uintptr(unsafe.Pointer(name)), 0, regSZ, uintptr(unsafe.Pointer(&val[0])), uintptr(len(val)*2))
	runtime.KeepAlive(val)
}

// ---------- tray window + main ----------

func trayWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	if taskbarCreated != 0 && msg == taskbarCreated {
		trayIcon, trayIconKey = 0, ""
		addTrayIcon(hwnd)
		return 0
	}
	switch msg {
	case wmTray:
		switch uint32(lparam) {
		case wmLButtonUp, wmLButtonDbl:
			showPanel() // clicking the tray icon opens the window
		case wmRButtonUp:
			showPopup(hwnd)
		}
		return 0
	case wmTimer:
		refreshTray(hwnd)
		onSecond()
		return 0
	case wmPopupCmd:
		switch wparam {
		case 1: // start / stop
			if hub.Snapshot().Running {
				hub.Stop()
			} else if s := hub.Snapshot(); s.HasToken || s.HasCreds {
				hub.Start()
			} else {
				showPanel()
			}
		case 2:
			showPanel()
		case 3:
			hub.Close()
			hub.primary.SaveStats()
			pDestroyWindow.Call(hwnd)
		}
		refreshTray(hwnd)
		return 0
	case wmDestroy:
		removeTrayIcon(hwnd)
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

const wmPopupCmd = 0x8003

func main() {
	if dir := argValue("--data"); dir != "" {
		os.Setenv("TRIOPENER_DIR", dir) // the boot task points the program at the person's own folder
	}
	if hasArg("--headless") {
		runHeadless()
		return
	}
	runtime.LockOSThread()
	pSetProcessDPIAware.Call()
	pTimeBeginPeriod.Call(1) // fine-grained sleeps so tiny delays are really tiny

	_, _, err := pCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(utf16Ptr(`Local\TrianguletOpenerTray`))))
	if hasArg("--updated") { // the old copy is closing: wait for it instead of treating it as "already running"
		for i := 0; i < 30; i++ {
			if en, ok := err.(syscall.Errno); !ok || en != 183 {
				break
			}
			time.Sleep(500 * time.Millisecond)
			_, _, err = pCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(utf16Ptr(`Local\TrianguletOpenerTray`))))
		}
	}
	cleanupOldExe()
	if en, ok := err.(syscall.Errno); ok && en == 183 { // ERROR_ALREADY_EXISTS
		// already running: ask that copy to show its window
		if h, _, _ := pFindWindow.Call(uintptr(unsafe.Pointer(panelClass)), 0); h != 0 {
			pPostMessage.Call(h, wmShowReq, 0, 0)
		} else {
			messageBox("Triangulet Plus", "Already running. Look for the Triangulet icon in the system tray (it may be under the ^ arrow).", mbOK|mbIconInformation)
		}
		return
	}

	hub = NewHub(dataDir())
	imgHub = hub
	applyTheme(hub.cfg().Theme)
	hub.applyUpd = applyUpdate
	hub.canAuto = func() bool { return !panelVisible() } // never restart under someone who is looking at the window
	requestTakeover(hub.primary.dir)                     // if the boot-time copy is running, it hands over to this one
	taskbarCreated, _, _ = pRegisterWindowMessage.Call(uintptr(unsafe.Pointer(utf16Ptr("TaskbarCreated"))))

	hInst, _, _ := pGetModuleHandle.Call(0)
	hInstance = hInst
	className := utf16Ptr("TrianguletOpenerWnd")
	wc := wndClassEx{
		LpfnWndProc:   syscall.NewCallback(trayWndProc),
		HInstance:     hInst,
		LpszClassName: className,
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16Ptr("Triangulet Plus tray"))), 0, 0, 0, 0, 0, 0, 0, hInst, 0)
	if hwnd == 0 {
		messageBox("Triangulet Plus", "Could not create the tray window.", mbOK|mbIconWarning)
		return
	}
	trayHwnd = hwnd
	imgDirty = func() { dirty = true }
	addTrayIcon(hwnd)
	pSetTimer.Call(hwnd, 1, 1000, 0)

	// first run: switch "run at startup" on by default
	if !hub.primary.StartupInitDone() {
		setStartup(true)
		hub.primary.MarkStartupInit()
	}
	createPanel()
	if !hasArg("--tray") { // started by Windows at sign-in: stay hidden in the tray
		showPanel()
	}

	go func() { // after an update: show what is new, once
		time.Sleep(2500 * time.Millisecond)
		if s := hub.Snapshot(); s.WhatsNew != "" {
			hub.DismissWhatsNew()
			messageBox("Triangulet Plus updated to v"+appVersion, shortNotes(s.WhatsNew), mbOK|mbIconInformation)
		}
	}()
	go func() {
		for range time.Tick(5 * time.Second) {
			hub.primary.SaveStats()
		}
	}()

	// Discord presence runs on its own, so it keeps working with the window hidden
	stopDiscord := make(chan struct{})
	go RunDiscord(hub, dialDiscord, 15*time.Second, stopDiscord)
	go KeepAwakeLoop(hub, 20*time.Second, make(chan struct{}))

	// start opening on launch if there is a saved login
	go func() {
		c := hub.cfg()
		if c.Username == "" && c.Token == "" {
			return
		}
		if err := hub.primary.EnsureToken(); err == nil && hub.primary.hasToken() {
			_ = hub.primary.LoadCapsules()
			_ = hub.primary.SyncInventory()
		}
		hub.sync()
		if hasArg("--tray") || c.AutoStart {
			if c.Mode == "hunt" || c.Capsule != "" {
				hub.Start()
			}
		}
	}()

	var m msgT
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if panelHwnd != 0 {
			// Enter in the username/password boxes logs in
			if m.Message == wmKeyDown && m.WParam == vkReturn && (m.HWnd == edits["user"] || m.HWnd == edits["pass"]) {
				doLogin()
				continue
			}
			if handled, _, _ := pIsDialogMessage.Call(panelHwnd, uintptr(unsafe.Pointer(&m))); handled != 0 {
				continue
			}
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
	close(stopDiscord)
	hub.Close()
	hub.primary.SaveStats()
}

var trayHwnd uintptr

func argValue(name string) string {
	for i, a := range os.Args[1:] {
		if a == name && i+2 < len(os.Args) {
			return os.Args[i+2]
		}
	}
	return ""
}

// runHeadless is the boot-time mode: no window, no tray icon.
func runHeadless() {
	hub = NewHub(dataDir())
	imgHub = hub
	pTimeBeginPeriod.Call(1)
	go KeepAwakeLoop(hub, 20*time.Second, make(chan struct{}))
	RunHeadless(hub, 2*time.Second, make(chan struct{}))
}
