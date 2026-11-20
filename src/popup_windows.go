//go:build windows

package main

import (
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// The tray icon's right-click pop-up: a small flat window drawn by popup.go.

var (
	popClass  = utf16Ptr("TrianguletOpenerPopup")
	popHwnd   uintptr
	popDC     uintptr
	popDib    []byte
	popCanvas = NewCanvas(popW, popH)
	popState  = &PopupState{}
	popFrame  Frame
	popScale  = 1.0
	popBack   uintptr
	popBackBm uintptr
	popOwner  uintptr

	pDwmSet        = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	pCreateRoundRg = gdi32.NewProc("CreateRoundRectRgn")
	pSetWindowRgn  = user32.NewProc("SetWindowRgn")
	pGetForeground = user32.NewProc("GetForegroundWindow")
)

const (
	wsPopup       = 0x80000000
	wsExToolWin   = 0x00000080
	wsExTopmost   = 0x00000008
	wmActivate    = 0x0006
	popTimerID    = 9
	dwmCornerPref = 33
)

func ensurePopup() {
	if popHwnd != 0 {
		return
	}
	cursor, _, _ := pLoadCursor.Call(0, 32512)
	wc := wndClassEx{LpfnWndProc: syscall.NewCallback(popWndProc), HInstance: hInstance, HCursor: cursor, LpszClassName: popClass}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	popHwnd, _, _ = pCreateWindowEx.Call(wsExToolWin|wsExTopmost, uintptr(unsafe.Pointer(popClass)), 0, wsPopup,
		0, 0, 10, 10, 0, 0, hInstance, 0)
	dc, _, _ := pGetDC.Call(popHwnd)
	popDC, _, _ = pCreateCompatDC.Call(dc)
	bmi := bitmapInfoHeader{Width: popW, Height: -popH, Planes: 1, Bits: 32}
	bmi.Size = uint32(unsafe.Sizeof(bmi))
	var bits uintptr
	bmp, _, _ := pCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bmi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	pReleaseDC.Call(popHwnd, dc)
	if bmp == 0 || bits == 0 {
		return
	}
	pSelectObject.Call(popDC, bmp)
	popDib = unsafe.Slice((*byte)(unsafe.Pointer(bits)), popW*popH*4)
}

func dpiScale() float64 {
	d, _, _ := pGetDpiForSystem.Call()
	if d == 0 {
		return 1
	}
	return float64(d) / 96
}

func showPopup(owner uintptr) {
	ensurePopup()
	if popHwnd == 0 || popDib == nil {
		return
	}
	popOwner = owner
	popScale = dpiScale()
	w, h := int32(float64(popW)*popScale+0.5), int32(float64(popH)*popScale+0.5)

	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	x, y := pt.X-w/2, pt.Y-h-10
	if y < 4 {
		y = pt.Y + 14
	}
	if x+w > int32(sw)-6 {
		x = int32(sw) - w - 6
	}
	if x < 6 {
		x = 6
	}
	if y+h > int32(sh)-6 {
		y = int32(sh) - h - 6
	}
	// rounded corners: Windows 11 does it natively; older Windows gets a region
	pref := int32(2) // DWMWCP_ROUND
	if r, _, _ := pDwmSet.Call(popHwnd, dwmCornerPref, uintptr(unsafe.Pointer(&pref)), 4); r != 0 {
		rg, _, _ := pCreateRoundRg.Call(0, 0, uintptr(w+1), uintptr(h+1), uintptr(int(24*popScale)), uintptr(int(24*popScale)))
		pSetWindowRgn.Call(popHwnd, rg, 1)
	}
	popState = &PopupState{}
	pSetWindowPos.Call(popHwnd, ^uintptr(0) /*HWND_TOPMOST*/, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x40) // SHOWWINDOW
	pSetForegroundWindow.Call(popHwnd)
	pSetTimer.Call(popHwnd, popTimerID, 33, 0)
	renderPopup()
}

func hidePopup() {
	pKillTimer.Call(popHwnd, popTimerID)
	pShowWindow.Call(popHwnd, swHide)
}

func renderPopup() {
	s := hub.Snapshot()
	popFrame = DrawPopup(popCanvas, s, popState)
	src := popCanvas.Pix
	for i := 0; i+3 < len(src) && i+3 < len(popDib); i += 4 {
		popDib[i], popDib[i+1], popDib[i+2], popDib[i+3] = src[i+2], src[i+1], src[i], 255
	}
	pInvalidateRect.Call(popHwnd, 0, 0)
}

func popPaint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	w, h := int(float64(popW)*popScale+0.5), int(float64(popH)*popScale+0.5)
	back, _, _ := pCreateCompatDC.Call(hdc)
	bm, _, _ := pCreateCompatBmp.Call(hdc, uintptr(w), uintptr(h))
	old, _, _ := pSelectObject.Call(back, bm)
	pSetStretchMode.Call(back, 4)
	pStretchBlt.Call(back, 0, 0, uintptr(w), uintptr(h), popDC, 0, 0, popW, popH, 0x00CC0020)
	pSetBkMode.Call(back, 1)
	for _, t := range popFrame.Texts {
		if t.S == "" {
			continue
		}
		pSelectObject.Call(back, fontFor(maxi(int(float64(t.Size)*popScale+0.5), 6), t.Bold))
		pSetTextColor.Call(back, uintptr(t.Col.R)|uintptr(t.Col.G)<<8|uintptr(t.Col.B)<<16)
		flags := uintptr(0x4 | 0x20 | 0x8000)
		switch t.Align {
		case 1:
			flags |= 1
		case 2:
			flags |= 2
		}
		r := rect{int32(float64(t.R.X) * popScale), int32(float64(t.R.Y) * popScale), int32(float64(t.R.X+t.R.W) * popScale), int32(float64(t.R.Y+t.R.H) * popScale)}
		p := utf16Ptr(t.S)
		pDrawText.Call(back, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags)
		runtime.KeepAlive(p)
	}
	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), back, 0, 0, 0x00CC0020)
	pSelectObject.Call(back, old)
	pDeleteObject.Call(bm)
	pDeleteDC.Call(back)
	pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

func popHit(lp uintptr) string {
	x, y := int(float64(int16(lp&0xFFFF))/popScale), int(float64(int16((lp>>16)&0xFFFF))/popScale)
	for _, h := range popFrame.Hits {
		if h.R.In(x, y) {
			return h.ID
		}
	}
	return ""
}

var popLast = time.Now()

func popWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		popPaint(hwnd)
		return 0
	case wmMouseMove:
		popState.Hover = popHit(lparam)
		return 0
	case wmLButtonUp2:
		id := popHit(lparam)
		hidePopup()
		code := map[string]uintptr{"toggle": 1, "show": 2, "quit": 3}[id]
		if code != 0 {
			pPostMessage.Call(popOwner, wmPopupCmd, code, 0)
		}
		return 0
	case wmActivate:
		if wparam&0xFFFF == 0 { // lost focus: clicking elsewhere closes the pop-up
			hidePopup()
		}
		return 0
	case wmSetCursor:
		if popState.Hover != "" && lparam&0xFFFF == 1 {
			c, _, _ := pLoadCursor.Call(0, 32649)
			pSetCursor.Call(c)
			return 1
		}
	case wmTimer:
		now := time.Now()
		popState.Step(now.Sub(popLast).Seconds())
		popLast = now
		renderPopup()
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

var _ = strings.TrimSpace
