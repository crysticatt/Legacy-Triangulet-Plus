//go:build windows

package main

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	pEmptyClipboard   = user32.NewProc("EmptyClipboard")
	pSetClipboardData = user32.NewProc("SetClipboardData")
	pGlobalAllocClip  = kernel32.NewProc("GlobalAlloc")
	pGlobalLockClip   = kernel32.NewProc("GlobalLock")
	pGlobalUnlockClip = kernel32.NewProc("GlobalUnlock")
)

// setClipboardText puts text on the Windows clipboard.
func setClipboardText(s string) bool {
	u := utf16.Encode([]rune(s))
	u = append(u, 0)
	size := uintptr(len(u) * 2)
	const gmemMoveable, cfUnicodeText = 0x2, 13
	if r, _, _ := pOpenClipboard.Call(0); r == 0 {
		return false
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	h, _, _ := pGlobalAllocClip.Call(gmemMoveable, size)
	if h == 0 {
		return false
	}
	p, _, _ := pGlobalLockClip.Call(h)
	if p == 0 {
		return false
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(u)), u)
	pGlobalUnlockClip.Call(h)
	r, _, _ := pSetClipboardData.Call(cfUnicodeText, h)
	_ = syscall.Errno(0)
	return r != 0
}
