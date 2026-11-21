package main

import (
	"runtime"
	"time"
)

// While opening, the PC must not go to sleep: a sleeping PC opens nothing. The screen is allowed to
// turn off (the opener does not need it), so you can leave a game or this app running and switch the
// monitor off. Windows: SetThreadExecutionState(ES_SYSTEM_REQUIRED). Mac: `caffeinate -i`.
// setAwake is provided per system (keepawake_*.go).

var setAwake = func(on bool) {}

// KeepAwakeLoop holds the PC awake while the hub is opening and the setting is on.
func KeepAwakeLoop(h *Hub, every time.Duration, stop <-chan struct{}) {
	runtime.LockOSThread() // the Windows state belongs to the thread that set it
	defer runtime.UnlockOSThread()
	defer setAwake(false)
	for {
		want := h.cfg().KeepAwake && h.Snapshot().Running
		setAwake(want) // asserting again each round also covers anything that reset it
		select {
		case <-stop:
			return
		case <-time.After(every):
		}
	}
}
