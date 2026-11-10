//go:build windows

package main

// One voiceloop at a time. A second launch (e.g. from the Start menu) asks the
// running copy to open Settings, then exits.

import (
	"log"
	"os"

	"golang.org/x/sys/windows"
)

const (
	mutexName = `Local\VoiceLoopSingleInstance`
	eventName = `Local\VoiceLoopShowSettings`
)

// claimSingleInstance returns false if another voiceloop is already running
// (after signalling it to show Settings).
func claimSingleInstance() bool {
	mn, _ := windows.UTF16PtrFromString(mutexName)
	en, _ := windows.UTF16PtrFromString(eventName)
	_, err := windows.CreateMutex(nil, false, mn)
	if err == windows.ERROR_ALREADY_EXISTS {
		if ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, en); err == nil {
			windows.SetEvent(ev)
			windows.CloseHandle(ev)
		}
		return false
	}
	ev, err := windows.CreateEvent(nil, 0, 0, en) // auto-reset
	if err != nil {
		log.Printf("single instance event: %v", err)
		return true
	}
	go func() {
		for {
			if r, _ := windows.WaitForSingleObject(ev, windows.INFINITE); r == windows.WAIT_OBJECT_0 {
				openSettings()
			}
		}
	}()
	return true
}

// launchedInBackground is true when the logon task starts us (it passes --background).
func launchedInBackground() bool {
	for _, a := range os.Args[1:] {
		if a == "--background" {
			return true
		}
	}
	return false
}
