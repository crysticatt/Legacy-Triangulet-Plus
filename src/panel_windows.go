//go:build windows

package main

import (
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// The main window. scene.go paints everything except text into a 960x540 off-screen
// bitmap; the window stretches that to its size (keeping 16:9) and draws the text on top
// with the system font at the final size, so it stays sharp at any window size. The
// text boxes are real Windows EDIT controls placed over the picture. Closing the window
// hides it to the tray. An animation timer runs only while the window is on screen.

var (
	pShowWindow       = user32.NewProc("ShowWindow")
	pIsWindowVisible  = user32.NewProc("IsWindowVisible")
	pIsIconic         = user32.NewProc("IsIconic")
	pIsDialogMessage  = user32.NewProc("IsDialogMessageW")
	pSendMessage      = user32.NewProc("SendMessageW")
	pGetWindowText    = user32.NewProc("GetWindowTextW")
	pSetWindowText    = user32.NewProc("SetWindowTextW")
	pLoadCursor       = user32.NewProc("LoadCursorW")
	pSetCursor        = user32.NewProc("SetCursor")
	pSetWindowPos     = user32.NewProc("SetWindowPos")
	pGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	pAdjustWindowRect = user32.NewProc("AdjustWindowRectEx")
	pSetFocus         = user32.NewProc("SetFocus")
	pGetWindowRect    = user32.NewProc("GetWindowRect")
	pGetClientRect    = user32.NewProc("GetClientRect")
	pInvalidateRect   = user32.NewProc("InvalidateRect")
	pFindWindow       = user32.NewProc("FindWindowW")
	pBeginPaint       = user32.NewProc("BeginPaint")
	pEndPaint         = user32.NewProc("EndPaint")
	pGetDC            = user32.NewProc("GetDC")
	pReleaseDC        = user32.NewProc("ReleaseDC")
	pDrawText         = user32.NewProc("DrawTextW")
	pTrackMouseEvent  = user32.NewProc("TrackMouseEvent")
	pSetCapture       = user32.NewProc("SetCapture")
	pReleaseCapture   = user32.NewProc("ReleaseCapture")
	pKillTimer        = user32.NewProc("KillTimer")
	pFillRect         = user32.NewProc("FillRect")
	pCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	pCreateFont       = gdi32.NewProc("CreateFontW")
	pSelectObject     = gdi32.NewProc("SelectObject")
	pSetTextColor     = gdi32.NewProc("SetTextColor")
	pSetBkColor       = gdi32.NewProc("SetBkColor")
	pSetBkMode        = gdi32.NewProc("SetBkMode")
	pCreateCompatDC   = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatBmp  = gdi32.NewProc("CreateCompatibleBitmap")
	pDeleteDC         = gdi32.NewProc("DeleteDC")
	pCreateDIBSection = gdi32.NewProc("CreateDIBSection")
	pBitBlt           = gdi32.NewProc("BitBlt")
	pStretchBlt       = gdi32.NewProc("StretchBlt")
	pSetStretchMode   = gdi32.NewProc("SetStretchBltMode")
	pIntersectClip    = gdi32.NewProc("IntersectClipRect")
	pSaveDC           = gdi32.NewProc("SaveDC")
	pRestoreDC        = gdi32.NewProc("RestoreDC")
)

const (
	wmClose        = 0x0010
	wmSize         = 0x0005
	wmSetFont      = 0x0030
	wmGetMinMax    = 0x0024
	wmCommand      = 0x0111
	wmPaint        = 0x000F
	wmEraseBkgnd   = 0x0014
	wmSetCursor    = 0x0020
	wmCtlColorEdit = 0x0133
	wmMouseMove    = 0x0200
	wmLButtonDown  = 0x0201
	wmLButtonUp2   = 0x0202
	wmMouseWheel   = 0x020A
	wmMouseLeave   = 0x02A3
	wmShowReq      = 0x8002 // posted by a second launch: "show the window"
	enSetFocus     = 0x0100
	enKillFocus    = 0x0200
	enChange       = 0x0300
	emSetPwdChar   = 0x00CC

	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsTabStop      = 0x00010000
	wsCaption      = 0x00C00000
	wsSysMenu      = 0x00080000
	wsMinimizeBox  = 0x00020000
	wsMaximizeBox  = 0x00010000
	wsThickFrame   = 0x00040000
	wsClipChildren = 0x02000000
	esPassword     = 0x0020
	esAutoHScrl    = 0x0080

	swHide    = 0
	swShow    = 5
	swRestore = 9

	ctlUser    = 201
	ctlPass    = 202
	ctlDelay   = 207
	ctlSearch  = 208
	ctlAAmount = 209
	ctlABid    = 210
	ctlAPay    = 211
	idOK       = 1

	timerAnim = 7

	ctlNtfy  = 212
	ctlBPass = 213

	panelStyle = wsCaption | wsSysMenu | wsMinimizeBox | wsMaximizeBox | wsThickFrame | wsClipChildren
)

type rect struct{ Left, Top, Right, Bottom int32 }

type paintStruct struct {
	Hdc      uintptr
	FErase   int32
	Rc       rect
	FRestore int32
	FIncUpd  int32
	Reserved [32]byte
}

type trackMouse struct {
	CbSize    uint32
	Flags     uint32
	HwndTrack uintptr
	HoverTime uint32
}

type bitmapInfoHeader struct {
	Size          uint32
	Width, Height int32
	Planes, Bits  uint16
	Compression   uint32
	SizeImage     uint32
	XPels, YPels  int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type minMaxInfo struct {
	Reserved [2]int32
	MaxSize  [2]int32
	MaxPos   [2]int32
	MinTrack [2]int32
	MaxTrack [2]int32
}

// native text boxes take their colours from the current theme (COLORREF = 0x00BBGGRR)
var cFieldRef, cTextRef, cBgRef uintptr

func syncThemeRefs() {
	cFieldRef, cTextRef, cBgRef = colorRef(colField), colorRef(colText), colorRef(colBg)
	if brField != 0 {
		pDeleteObject.Call(brField)
		pDeleteObject.Call(bgBrush)
	}
	brField, bgBrush = makeBrush(cFieldRef), makeBrush(cBgRef)
}

// setThemeNow switches the whole window to a theme and saves the choice.
func setThemeNow(name string) {
	applyTheme(name)
	syncThemeRefs()
	hub.SetTheme(name)
	for _, h := range edits {
		pInvalidateRect.Call(h, 0, 1)
	}
	if panelHwnd != 0 {
		pInvalidateRect.Call(panelHwnd, 0, 1)
	}
}

var editIDs = map[string]uintptr{"ntfy": ctlNtfy, "bpass": ctlBPass, "user": ctlUser, "pass": ctlPass, "delay": ctlDelay, "search": ctlSearch, "aamount": ctlAAmount, "abid": ctlABid, "apay": ctlAPay}

var (
	panelClass = utf16Ptr("TrianguletOpenerPanel")
	hInstance  uintptr
	panelHwnd  uintptr

	edits   = map[string]uintptr{}
	brField uintptr
	bgBrush uintptr
	fonts   = map[[2]int]uintptr{}

	memDC   uintptr // the 960x540 scene
	dibBits []byte
	canvas  = NewCanvas(winW, winH)

	backDC       uintptr // window-sized buffer: scaled scene + text
	backBmp      uintptr
	backW, backH int

	ui        = NewUIState()
	lastFrame Frame
	lastSnap  Snapshot
	snapAt    time.Time
	hoverOn   bool

	viewScale    = 1.0
	viewX, viewY int
	editFontPx   int
	editFont     uintptr
	panelPlaced  bool
	lastTick     = time.Now()
	dirty        = true
	editPlaced   = map[string]rect{}
)

const scrollStep = 76

func makeBrush(c uintptr) uintptr {
	h, _, _ := pCreateSolidBrush.Call(c)
	return h
}

func makeFont(height, weight int, face string) uintptr {
	f := utf16Ptr(face)
	h, _, _ := pCreateFont.Call(uintptr(height), 0, 0, 0, uintptr(weight), 0, 0, 0,
		1 /*DEFAULT_CHARSET*/, 0, 0, 5 /*CLEARTYPE*/, 0, uintptr(unsafe.Pointer(f)))
	runtime.KeepAlive(f)
	return h
}

func fontFor(px int, bold bool) uintptr {
	w := 400
	if bold {
		w = 600
	}
	k := [2]int{px, w}
	if f, ok := fonts[k]; ok {
		return f
	}
	f := makeFont(-px, w, "Segoe UI")
	fonts[k] = f
	return f
}

func setText(h uintptr, s string) {
	p := utf16Ptr(s)
	pSetWindowText.Call(h, uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
}

func getText(h uintptr) string {
	buf := make([]uint16, 512)
	n, _, _ := pGetWindowText.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return string(utf16.Decode(buf[:n]))
}

func newEdit(style, id uintptr) uintptr {
	c := utf16Ptr("EDIT")
	hw, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(c)), 0,
		style|wsChild|wsTabStop|esAutoHScrl, 0, 0, 10, 10, panelHwnd, id, hInstance, 0)
	runtime.KeepAlive(c)
	return hw
}

func createPanel() {
	if panelHwnd != 0 {
		return
	}
	syncThemeRefs()

	cursor, _, _ := pLoadCursor.Call(0, 32512) // IDC_ARROW
	icon := loadAppIcon()
	wc := wndClassEx{
		LpfnWndProc:   syscall.NewCallback(panelWndProc),
		HInstance:     hInstance,
		HCursor:       cursor,
		HIcon:         icon,
		HIconSm:       icon,
		LpszClassName: panelClass,
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	var rc rect
	rc.Right, rc.Bottom = winW, winH
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), panelStyle, 0, 0)

	title := utf16Ptr("Triangulet Opener")
	panelHwnd, _, _ = pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(panelClass)), uintptr(unsafe.Pointer(title)),
		panelStyle, 100, 100, uintptr(rc.Right-rc.Left), uintptr(rc.Bottom-rc.Top), 0, 0, hInstance, 0)
	runtime.KeepAlive(title)
	if panelHwnd == 0 {
		return
	}

	// off-screen bitmap the scene is painted into
	dc, _, _ := pGetDC.Call(panelHwnd)
	memDC, _, _ = pCreateCompatDC.Call(dc)
	bmi := bitmapInfoHeader{Width: winW, Height: -winH, Planes: 1, Bits: 32}
	bmi.Size = uint32(unsafe.Sizeof(bmi))
	var bits uintptr
	bmp, _, _ := pCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bmi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	pReleaseDC.Call(panelHwnd, dc)
	if bmp == 0 || bits == 0 {
		return
	}
	pSelectObject.Call(memDC, bmp)
	dibBits = unsafe.Slice((*byte)(unsafe.Pointer(bits)), winW*winH*4)

	styles := map[string]uintptr{"pass": esPassword, "bpass": esPassword}
	for name, id := range editIDs {
		edits[name] = newEdit(styles[name], id)
	}
	cfg := hub.cfg()
	setText(edits["user"], cfg.Username)
	setText(edits["pass"], cfg.Password)
	setText(edits["delay"], trimFloat(cfg.DelayMs))
	setText(edits["ntfy"], cfg.NtfyTopic)
	setText(edits["aamount"], "1")
	setText(edits["abid"], "100")
	setText(edits["apay"], "0")
	ui.StartupOn = startupEnabled()
	ui.BootOn = bootEnabledCached()
	updateLayout()
	render()
}

func panelVisible() bool {
	if panelHwnd == 0 {
		return false
	}
	r, _, _ := pIsWindowVisible.Call(panelHwnd)
	if r == 0 {
		return false
	}
	ic, _, _ := pIsIconic.Call(panelHwnd)
	return ic == 0
}

func showPanel() {
	if panelHwnd == 0 {
		createPanel()
		if panelHwnd == 0 {
			return
		}
	}
	if !panelPlaced {
		panelPlaced = true
		var rc rect
		pGetWindowRect.Call(panelHwnd, uintptr(unsafe.Pointer(&rc)))
		w, h := rc.Right-rc.Left, rc.Bottom-rc.Top
		sw, _, _ := pGetSystemMetrics.Call(0)
		sh, _, _ := pGetSystemMetrics.Call(1)
		if int32(sh) > 0 && h > int32(sh)-60 { // small or heavily scaled display: fit it
			h = int32(sh) - 60
			w = h * 16 / 9
		}
		x, y := (int32(sw)-w)/2, (int32(sh)-h)/2-10
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		pSetWindowPos.Call(panelHwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x4) // NOZORDER
	}
	if r, _, _ := pIsIconic.Call(panelHwnd); r != 0 {
		pShowWindow.Call(panelHwnd, swRestore)
	} else {
		pShowWindow.Call(panelHwnd, swShow)
	}
	pSetForegroundWindow.Call(panelHwnd)
	ui.StartupOn = startupEnabled()
	startAnimTimer()
	dirty = true
	render()
}

func hidePanel() {
	applyDelay()
	pShowWindow.Call(panelHwnd, swHide)
	pKillTimer.Call(panelHwnd, timerAnim)
}

func startAnimTimer() {
	lastTick = time.Now()
	pSetTimer.Call(panelHwnd, timerAnim, 33, 0)
}

func applyDelay() {
	h := edits["delay"]
	if h == 0 {
		return
	}
	txt := strings.Replace(strings.TrimSpace(getText(h)), ",", ".", 1)
	if n, err := strconv.ParseFloat(txt, 64); err == nil && n >= 0 {
		hub.SetDelay(n)
	}
}

func snapshot() Snapshot {
	if time.Since(snapAt) > 250*time.Millisecond || dirty {
		lastSnap = hub.Snapshot()
		snapAt = time.Now()
	}
	return lastSnap
}

// onSecond runs once a second from the tray timer: refresh data shown in the window.
func onSecond() {
	if !panelVisible() {
		return
	}
	ui.StartupOn = startupEnabled()
	ui.BootOn = bootEnabledCached()
	dirty = true
}

// onAnimTimer runs every 33 ms while the window is visible; it redraws only when
// something is moving or the data changed.
func onAnimTimer() {
	if !panelVisible() {
		return
	}
	now := time.Now()
	dt := now.Sub(lastTick).Seconds()
	lastTick = now
	ui.Step(dt)
	if ui.Animating() || dirty {
		render()
	}
}

// ---------- layout / painting ----------

func clientSize() (int, int) {
	var rc rect
	pGetClientRect.Call(panelHwnd, uintptr(unsafe.Pointer(&rc)))
	return int(rc.Right), int(rc.Bottom)
}

func updateLayout() {
	cw, ch := clientSize()
	if cw <= 0 || ch <= 0 {
		return
	}
	sc := float64(cw) / winW
	if v := float64(ch) / winH; v < sc {
		sc = v
	}
	viewScale = sc
	viewX = (cw - int(winW*sc)) / 2
	viewY = (ch - int(winH*sc)) / 2
	px := int(14*sc + 0.5)
	if px != editFontPx {
		editFontPx = px
		editFont = makeFont(-px, 400, "Segoe UI")
		for _, h := range edits {
			pSendMessage.Call(h, wmSetFont, editFont, 1)
		}
	}
	if cw != backW || ch != backH { // the back buffer follows the window size
		dc, _, _ := pGetDC.Call(panelHwnd)
		if backDC == 0 {
			backDC, _, _ = pCreateCompatDC.Call(dc)
		}
		nb, _, _ := pCreateCompatBmp.Call(dc, uintptr(cw), uintptr(ch))
		pReleaseDC.Call(panelHwnd, dc)
		pSelectObject.Call(backDC, nb)
		if backBmp != 0 {
			pDeleteObject.Call(backBmp)
		}
		backBmp, backW, backH = nb, cw, ch
	}
}

func sx(v int) int { return viewX + int(float64(v)*viewScale+0.5) }
func sy(v int) int { return viewY + int(float64(v)*viewScale+0.5) }
func sl(v int) int { return int(float64(v)*viewScale + 0.5) }

func toScene(x, y int) (int, int) {
	return int(float64(x-viewX) / viewScale), int(float64(y-viewY) / viewScale)
}

func sceneRect(r Rect) rect {
	return rect{int32(sx(r.X)), int32(sy(r.Y)), int32(sx(r.X + r.W)), int32(sy(r.Y + r.H))}
}

func pixelAt(x, y int) Col {
	x, y = maxi(0, mini(winW-1, x)), maxi(0, mini(winH-1, y))
	i := (y*winW + x) * 4
	return Col{canvas.Pix[i], canvas.Pix[i+1], canvas.Pix[i+2], 255}
}

func drawTexts(f *Frame) {
	pSetBkMode.Call(backDC, 1) // TRANSPARENT
	for _, t := range f.Texts {
		if t.S == "" || t.R.W <= 0 || t.Col.A == 0 {
			continue
		}
		pSelectObject.Call(backDC, fontFor(maxi(sl(t.Size), 6), t.Bold))
		col := t.Col
		if col.A < 255 { // fade towards what is behind the text
			col = lerpCol(pixelAt(t.R.X+2, t.R.Y+t.R.H/2), Col{col.R, col.G, col.B, 255}, float64(col.A)/255)
		}
		pSetTextColor.Call(backDC, uintptr(col.R)|uintptr(col.G)<<8|uintptr(col.B)<<16)
		flags := uintptr(0x4 | 0x20 | 0x8000) // VCENTER | SINGLELINE | END_ELLIPSIS
		switch t.Align {
		case 1:
			flags |= 0x1
		case 2:
			flags |= 0x2
		}
		r := sceneRect(t.R)
		clipped := !t.Clip.Empty()
		if clipped {
			pSaveDC.Call(backDC)
			cr := sceneRect(t.Clip)
			pIntersectClip.Call(backDC, uintptr(cr.Left), uintptr(cr.Top), uintptr(cr.Right), uintptr(cr.Bottom))
		}
		p := utf16Ptr(t.S)
		pDrawText.Call(backDC, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags)
		runtime.KeepAlive(p)
		if clipped {
			pRestoreDC.Call(backDC, ^uintptr(0))
		}
	}
}

func render() {
	if memDC == 0 || dibBits == nil || panelHwnd == 0 {
		return
	}
	dirty = false
	s := snapshot()
	ui.ChromaImg, _ = hub.primary.ChromaImage()
	ui.ImgFor = trianImage
	ui.AucMsg = s.AuctionMsg
	f := DrawScene(canvas, s, ui)
	for _, k := range []string{"inv", "auc", "log"} {
		ui.Scroll[k] = maxi(0, mini(ui.Scroll[k], f.MaxScroll[k]))
	}
	src := canvas.Pix // RGBA -> BGRA
	for i := 0; i+3 < len(src) && i+3 < len(dibBits); i += 4 {
		dibBits[i], dibBits[i+1], dibBits[i+2], dibBits[i+3] = src[i+2], src[i+1], src[i], 255
	}
	lastFrame = f
	updateLayout()
	if backDC == 0 {
		return
	}
	full := rect{0, 0, int32(backW), int32(backH)}
	pFillRect.Call(backDC, uintptr(unsafe.Pointer(&full)), bgBrush) // letterbox bars
	pSetStretchMode.Call(backDC, 4)                                 // HALFTONE: smooth scaling
	pStretchBlt.Call(backDC, uintptr(viewX), uintptr(viewY), uintptr(sl(winW)), uintptr(sl(winH)),
		memDC, 0, 0, winW, winH, 0x00CC0020)
	drawTexts(&f)
	placeEdits(&f)
	pInvalidateRect.Call(panelHwnd, 0, 0)
}

func placeEdits(f *Frame) {
	for name, h := range edits {
		r, vis := f.Edits[name]
		if !vis {
			if _, was := editPlaced[name]; was {
				pShowWindow.Call(h, swHide)
				delete(editPlaced, name)
			}
			continue
		}
		pos := sceneRect(r)
		if _, was := editPlaced[name]; !was || editPlaced[name] != pos {
			editPlaced[name] = pos
			// NOZORDER | NOACTIVATE | SHOWWINDOW
			pSetWindowPos.Call(h, 0, uintptr(pos.Left), uintptr(pos.Top), uintptr(pos.Right-pos.Left), uintptr(pos.Bottom-pos.Top), 0x4|0x10|0x40)
		}
	}
}

func hitAt(x, y int) string {
	for i := len(lastFrame.Hits) - 1; i >= 0; i-- { // topmost was added last
		if lastFrame.Hits[i].R.In(x, y) {
			return lastFrame.Hits[i].ID
		}
	}
	return ""
}

func trackLeave() {
	tm := trackMouse{Flags: 2 /*TME_LEAVE*/, HwndTrack: panelHwnd}
	tm.CbSize = uint32(unsafe.Sizeof(tm))
	pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tm)))
}

func setHover(id string) {
	if id != ui.Hover {
		ui.Hover = id
		ui.moving = true
		render()
	}
}

// ---------- actions ----------

func doLogin() {
	applyDelay()
	hub.Login(strings.TrimSpace(getText(edits["user"])), getText(edits["pass"]))
	dirty = true
	render()
}

func doToggle() {
	s := hub.Snapshot()
	if s.Running {
		hub.Stop()
		dirty = true
		render()
		return
	}
	applyDelay()
	if s.Mode != "hunt" && s.Capsule == "" {
		messageBox("Triangulet Opener", "Pick a capsule first (or switch Mode to Hunt).", mbOK|mbIconInformation)
		return
	}
	if !s.HasToken && !s.HasCreds {
		messageBox("Triangulet Opener", "Log in first: enter your username and password and click Save and log in.", mbOK|mbIconInformation)
		return
	}
	hub.Start()
	dirty = true
	render()
}

func toggleShowPassword() {
	ui.ShowPass = !ui.ShowPass
	var ch uintptr = 0x2022 // bullet
	if ui.ShowPass {
		ch = 0
	}
	pSendMessage.Call(edits["pass"], emSetPwdChar, ch, 0)
	pInvalidateRect.Call(edits["pass"], 0, 1)
}

func atoiOr(s string, d int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return d
}

func doAuction() {
	if ui.Sel == "" {
		messageBox("Triangulet Opener", "Pick a trian from the list first.", mbOK|mbIconInformation)
		return
	}
	amount := atoiOr(getText(edits["aamount"]), 0)
	bid := atoiOr(getText(edits["abid"]), 0)
	pay := atoiOr(getText(edits["apay"]), 0)
	hub.StartAuction(ui.Sel, amount, bid, pay)
}

// syncLoginBoxes shows the active saved login in the username and password boxes.
func syncLoginBoxes() {
	c := hub.cfg()
	setText(edits["user"], c.Username)
	setText(edits["pass"], c.Password)
}

func handleClick(id string) {
	c := hub.cfg()
	switch {
	case id == "login":
		doLogin()
	case id == "show":
		toggleShowPassword()
	case id == "refresh":
		hub.Refresh()
	case id == "start":
		doToggle()
	case id == "hide":
		hidePanel()
	case id == "startup":
		setStartup(!startupEnabled())
		ui.StartupOn = startupEnabled()
	case id == "boot":
		if !bootEnabledCached() && !messageBox2("Triangulet Opener", "This makes a Windows startup task that runs the opener with no window before you sign in.\n\nBecause the app is not code-signed, Windows Defender or other antivirus may flag it. That is a false alarm, but if it happens: restore the file in Windows Security > Protection history, then send it to Microsoft as \"I believe this file is clean\".\n\nTurn it on?", mbOKCancel|mbIconInformation) {
			break
		}
		setBoot(!bootEnabledCached())
	case id == "auction":
		doAuction()
	case id == "workers-":
		hub.SetWorkers(c.Workers - 1)
	case id == "workers+":
		hub.SetWorkers(c.Workers + 1)
	case id == "inflight-":
		hub.SetInFlight(c.InFlight - 1)
	case id == "inflight+":
		hub.SetInFlight(c.InFlight + 1)
	case strings.HasPrefix(id, "mode:"):
		hub.SetMode([]string{"fixed", "advance", "hunt"}[atoiOr(id[5:], 1)%3])
	case strings.HasPrefix(id, "spread:"):
		hub.SetSpread(atoiOr(id[7:], 0) + 1)
	case strings.HasPrefix(id, "cap:"):
		hub.SelectCapsule(strings.TrimPrefix(id, "cap:"))
	case strings.HasPrefix(id, "tv:"):
		ui.TriansView = atoiOr(id[3:], 0) & 1
		ui.Scroll["inv"] = 0
	case id == "goal-":
		hub.SetGoal(c.StopAfterMystical - 1)
	case id == "goal+":
		hub.SetGoal(c.StopAfterMystical + 1)
	case id == "update":
		if s := hub.Snapshot(); !s.UpdateBusy && s.UpdateVer != "" {
			if messageBox2("Triangulet Opener", "Install version "+s.UpdateVer+" and restart now?\n\n"+shortNotes(s.UpdateNotes), mbOKCancel|mbIconInformation) {
				hub.InstallUpdate(applyUpdate)
			}
		}
	case id == "speedtest":
		if hub.Snapshot().Running {
			messageBox("Triangulet Opener", "Stop opening first, then run the test. It only reads from the site and spends no coins.", mbOK|mbIconInformation)
		} else {
			hub.MeasureLimits()
		}
	case id == "acct<":
		hub.SwitchAccount(-1)
		syncLoginBoxes()
	case id == "acct>":
		hub.SwitchAccount(1)
		syncLoginBoxes()
	case id == "acct-":
		hub.RemoveAccount()
		syncLoginBoxes()
	case id == "acctnew":
		setText(edits["user"], "")
		setText(edits["pass"], "")
		pSetFocus.Call(edits["user"])
	case strings.HasPrefix(id, "hr:"):
		ui.HistRange = atoiOr(id[3:], 0)
		ui.moving = true
		ui.disp["sg:graph"] = 0
	case id == "chase":
		hub.SetChase(!c.HuntMissing)
	case id == "sound":
		hub.SetSound(!c.AlertSound)
	case id == "openlog":
		openPath(hub.logPath())
	case strings.HasPrefix(id, "tab:"):
		ui.Tab = atoiOr(id[4:], 0)
		ui.Settings = false
	case id == "settings":
		ui.Settings = !ui.Settings
		ui.SetMsg = ""
	case strings.HasPrefix(id, "theme:"):
		setThemeNow(themeNames[atoiOr(id[6:], 0)%len(themeNames)])
	case id == "hourly":
		hub.update(func(c *Config) { c.HourlySummary = !c.HourlySummary })
	case id == "awake":
		hub.update(func(c *Config) { c.KeepAwake = !c.KeepAwake })
	case id == "chkupd":
		hub.CheckUpdateNow(true)
	case id == "updcheck":
		hub.update(func(c *Config) { c.NoUpdateCheck = !c.NoUpdateCheck })
	case id == "autoupd":
		hub.SetAutoUpdate(!c.AutoUpdate)
	case id == "ntfyon":
		applyNtfyTopic()
		ui.SetMsg = hub.SetNtfyOn(!hub.cfg().NtfyOn)
		if ui.SetMsg == "" && !hub.cfg().NtfyOn {
			ui.SetMsg = "Type a topic name first (or press Random)."
		}
	case id == "ntfyrand":
		hub.RandomNtfyTopic()
		setText(edits["ntfy"], hub.cfg().NtfyTopic)
		ui.SetMsg = "New private topic made. Subscribe to it in the ntfy app, then press Test."
	case id == "ntfytest":
		applyNtfyTopic()
		ui.SetMsg = "Sending..."
		go func() { m := hub.NtfyTest(); ui.SetMsg = m; dirty = true }()
	case id == "bexport":
		ui.SetMsg = "Working..."
		pass := getText(edits["bpass"])
		go func() {
			p, err := hub.SaveBackupFile(pass)
			if err != nil {
				ui.SetMsg = "Backup failed: " + err.Error()
			} else {
				ui.SetMsg = "Saved: " + p + "  (keep your passphrase safe, it cannot be recovered)"
			}
			dirty = true
		}()
	case id == "bimport":
		if messageBox2("Triangulet Opener", "Replace your current settings, logins and History with the newest backup file?", mbOKCancel|mbIconInformation) {
			ui.SetMsg = "Working..."
			pass := getText(edits["bpass"])
			go func() {
				p, err := hub.LoadBackupFile(pass)
				if err != nil {
					ui.SetMsg = "Import failed: " + err.Error()
				} else {
					ui.SetMsg = "Restored from " + p
					c := hub.cfg()
					applyTheme(c.Theme)
					syncThemeRefs()
					setText(edits["ntfy"], c.NtfyTopic)
					syncLoginBoxes()
				}
				dirty = true
			}()
		}
	case strings.HasPrefix(id, "pick:"):
		ui.Sel = strings.TrimPrefix(id, "pick:")
	}
	dirty = true
	ui.moving = true
	render()
}

func editName(id uintptr) string {
	for n, v := range editIDs {
		if v == id {
			return n
		}
	}
	return ""
}

func lparamXY(lp uintptr) (int, int) {
	return toScene(int(int16(lp&0xFFFF)), int(int16((lp>>16)&0xFFFF)))
}

func panelWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if backDC != 0 {
			pBitBlt.Call(hdc, 0, 0, uintptr(backW), uintptr(backH), backDC, 0, 0, 0x00CC0020) // SRCCOPY
		}
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmSize:
		if panelHwnd != 0 && memDC != 0 {
			updateLayout()
			dirty = true
			render()
		}
		return 0
	case wmGetMinMax:
		mm := (*minMaxInfo)(unsafe.Pointer(lparam))
		var rc rect
		rc.Right, rc.Bottom = 640, 360
		pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), panelStyle, 0, 0)
		mm.MinTrack = [2]int32{rc.Right - rc.Left, rc.Bottom - rc.Top}
		return 0
	case wmMouseMove:
		x, y := lparamXY(lparam)
		if !hoverOn {
			hoverOn = true
			trackLeave()
		}
		setHover(hitAt(x, y))
		return 0
	case wmMouseLeave:
		hoverOn = false
		setHover("")
		return 0
	case wmSetCursor:
		h := ui.Hover
		if lparam&0xFFFF == 1 && h != "" && !strings.HasPrefix(h, "scroll:") {
			c, _, _ := pLoadCursor.Call(0, 32649) // IDC_HAND
			pSetCursor.Call(c)
			return 1
		}
	case wmLButtonDown:
		x, y := lparamXY(lparam)
		pSetFocus.Call(hwnd) // clicking the background takes focus out of the text boxes
		ui.Down = hitAt(x, y)
		pSetCapture.Call(hwnd)
		ui.moving = true
		render()
		return 0
	case wmLButtonUp2:
		x, y := lparamXY(lparam)
		down := ui.Down
		ui.Down = ""
		pReleaseCapture.Call()
		ui.moving = true
		render()
		if down != "" && hitAt(x, y) == down {
			handleClick(down)
		}
		return 0
	case wmMouseWheel:
		delta := int(int16((wparam >> 16) & 0xFFFF))
		h := ui.Hover
		key := ""
		switch {
		case strings.HasPrefix(h, "inv:"), h == "scroll:inv":
			key = "inv"
		case strings.HasPrefix(h, "pick:"), h == "scroll:auc":
			key = "auc"
		case strings.HasPrefix(h, "log:"), h == "scroll:log":
			key = "log"
		}
		if key != "" {
			ui.Scroll[key] = maxi(0, mini(lastFrame.MaxScroll[key], ui.Scroll[key]-delta/120*scrollStep))
			dirty = true
			render()
		}
		return 0
	case wmCommand:
		id, code := wparam&0xFFFF, (wparam>>16)&0xFFFF
		switch {
		case id == idOK:
			doLogin()
		case code == enSetFocus:
			ui.Focus = editName(id)
			ui.moving = true
			render()
		case code == enKillFocus:
			if ui.Focus == editName(id) {
				ui.Focus = ""
				if id == ctlDelay {
					applyDelay()
				}
				if id == ctlNtfy {
					applyNtfyTopic()
				}
				ui.moving = true
				render()
			}
		case code == enChange && id == ctlSearch:
			ui.Search = strings.ToLower(getText(edits["search"]))
			ui.Scroll["inv"] = 0
			dirty = true
			render()
		}
		return 0
	case wmCtlColorEdit:
		pSetTextColor.Call(wparam, cTextRef)
		pSetBkColor.Call(wparam, cFieldRef)
		return brField
	case wmTimer:
		if wparam == timerAnim {
			onAnimTimer()
			return 0
		}
	case wmShowReq:
		showPanel()
		return 0
	case wmClose:
		hidePanel() // closing just sends it to the tray
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

const mbOKCancel = 0x1

// messageBox2 shows an OK / Cancel box and reports whether OK was chosen.
func messageBox2(title, text string, flags uintptr) bool {
	t, x := utf16Ptr(title), utf16Ptr(text)
	r, _, _ := pMessageBox.Call(0, uintptr(unsafe.Pointer(x)), uintptr(unsafe.Pointer(t)), flags)
	runtime.KeepAlive(t)
	return r == 1
}

func shortNotes(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 400 {
		s = string(r[:400]) + "..."
	}
	return s
}

func applyNtfyTopic() {
	if h := edits["ntfy"]; h != 0 {
		if m := hub.SetNtfy(hub.cfg().NtfyOn, getText(h)); m != "" {
			ui.SetMsg = m
		}
	}
}
