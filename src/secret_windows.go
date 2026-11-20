//go:build windows

package main

import (
	"encoding/base64"
	"syscall"
	"unsafe"
)

var (
	crypt32        = syscall.NewLazyDLL("crypt32.dll")
	pProtectData   = crypt32.NewProc("CryptProtectData")
	pUnprotectData = crypt32.NewProc("CryptUnprotectData")
	kernel32Sec    = syscall.NewLazyDLL("kernel32.dll")
	pLocalFree     = kernel32Sec.NewProc("LocalFree")
)

type dataBlob struct {
	cb   uint32
	data *byte
}

func blobOf(b []byte) dataBlob {
	if len(b) == 0 {
		return dataBlob{}
	}
	return dataBlob{uint32(len(b)), &b[0]}
}

// Machine scope (CRYPTPROTECT_LOCAL_MACHINE) on purpose: "Before sign-in" mode runs as SYSTEM, which
// could not open a per-user blob. The text still can't be read when the file is copied to another PC.
func dpapi(proc *syscall.LazyProc, in []byte, label string, flags uintptr) ([]byte, bool) {
	inb := blobOf(in)
	var out dataBlob
	ent := []byte("TrianguletOpener:" + label)
	entb := blobOf(ent)
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&inb)), 0, uintptr(unsafe.Pointer(&entb)), 0, 0, flags, uintptr(unsafe.Pointer(&out)))
	if r == 0 || out.data == nil {
		return nil, false
	}
	res := make([]byte, out.cb)
	copy(res, unsafe.Slice(out.data, out.cb))
	pLocalFree.Call(uintptr(unsafe.Pointer(out.data)))
	return res, true
}

func init() {
	sealImpl = func(label, plain string) (string, bool) {
		b, ok := dpapi(pProtectData, []byte(plain), label, 0x1|0x4) // UI_FORBIDDEN | LOCAL_MACHINE
		if !ok {
			return "", false
		}
		return "dpapi:" + base64.StdEncoding.EncodeToString(b), true
	}
	unsealImpl = func(label, stored string) (string, bool) {
		if len(stored) < 6 || stored[:6] != "dpapi:" {
			return "", false
		}
		raw, err := base64.StdEncoding.DecodeString(stored[6:])
		if err != nil {
			return "", false
		}
		b, ok := dpapi(pUnprotectData, raw, label, 0x1)
		return string(b), ok
	}
}
