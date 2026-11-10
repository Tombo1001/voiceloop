//go:build windows

package main

// Session lock: with "Mute while the PC is locked" on, locking Windows stops
// whatever is playing, and nothing plays (not even chimes or /resend) until unlock.

import (
	"log"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wtsapi32          = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSRegister   = wtsapi32.NewProc("WTSRegisterSessionNotification")
	procWTSQueryInfo  = wtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSFreeMemory = wtsapi32.NewProc("WTSFreeMemory")

	sessionLocked atomic.Bool
)

const (
	wmWTSSessionChange = 0x02B1
	wtsSessionLock     = 0x7
	wtsSessionUnlock   = 0x8
)

// lockWatch asks Windows to send lock and unlock messages to hwnd, and reads
// the current state in case voiceloop starts (or is redeployed) while locked.
func lockWatch(hwnd uintptr) {
	if r, _, err := procWTSRegister.Call(hwnd, 0); r == 0 { // NOTIFY_FOR_THIS_SESSION
		log.Printf("lock watch unavailable: %v", err)
		return
	}
	if locked, ok := queryLocked(); ok && locked {
		sessionLocked.Store(true)
		log.Printf("started while the PC is locked")
	}
}

// queryLocked reads WTSINFOEXW.Data.WTSInfoExLevel1.SessionFlags for this session.
func queryLocked() (locked, ok bool) {
	var buf uintptr
	var n uint32
	const wtsSessionInfoEx = 25
	r, _, _ := procWTSQueryInfo.Call(0, 0xFFFFFFFF, wtsSessionInfoEx, uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&n)))
	if r == 0 || buf == 0 || n < 20 {
		return false, false
	}
	defer procWTSFreeMemory.Call(buf)
	// Level (4) + padding (4, the union holds LARGE_INTEGERs), SessionId (4), SessionState (4), SessionFlags (4).
	flags := *(*int32)(unsafe.Pointer(buf + 16))
	switch flags {
	case 0: // WTS_SESSIONSTATE_LOCK
		return true, true
	case 1: // WTS_SESSIONSTATE_UNLOCK
		return false, true
	}
	return false, false
}

func onSessionChange(event uintptr) {
	switch event {
	case wtsSessionLock:
		sessionLocked.Store(true)
		if store.get().MuteWhenLocked {
			drain()
			stopPlayback()
			log.Printf("PC locked: muted")
		} else {
			log.Printf("PC locked")
		}
	case wtsSessionUnlock:
		sessionLocked.Store(false)
		log.Printf("PC unlocked")
	}
}

// lockedMute reports whether playback is held back because the PC is locked.
func lockedMute() bool { return sessionLocked.Load() && store.get().MuteWhenLocked }
