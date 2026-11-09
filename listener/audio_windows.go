//go:build windows

package main

// Playback through the waveOut API so a specific output device can be chosen.
// (PlaySound only ever uses the Windows default device.)

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	procWaveOutGetNumDevs = winmm.NewProc("waveOutGetNumDevs")
	procWaveOutGetDevCaps = winmm.NewProc("waveOutGetDevCapsW")
	procWaveOutOpen       = winmm.NewProc("waveOutOpen")
	procWaveOutPrepare    = winmm.NewProc("waveOutPrepareHeader")
	procWaveOutWrite      = winmm.NewProc("waveOutWrite")
	procWaveOutReset      = winmm.NewProc("waveOutReset")
	procWaveOutUnprepare  = winmm.NewProc("waveOutUnprepareHeader")
	procWaveOutClose      = winmm.NewProc("waveOutClose")
	waveMapper            = uintptr(0xFFFFFFFF)
	audioMu               sync.Mutex // one clip at a time
	curMu                 sync.Mutex
	curHandle             uintptr // open waveOut handle, for stopping
)

const whdrDone = 0x1

type waveFormatEx struct {
	formatTag      uint16
	channels       uint16
	samplesPerSec  uint32
	avgBytesPerSec uint32
	blockAlign     uint16
	bitsPerSample  uint16
	cbSize         uint16
}

type waveHdr struct {
	data          *byte
	bufferLength  uint32
	bytesRecorded uint32
	user          uintptr
	flags         uint32
	loops         uint32
	next          uintptr
	reserved      uintptr
}

type waveOutCaps struct {
	mid, pid      uint16
	driverVersion uint32
	pname         [32]uint16
	formats       uint32
	channels      uint16
	reserved1     uint16
	support       uint32
}

// outputDevices lists waveOut device names (Windows truncates them to 31 characters).
func outputDevices() []string {
	n, _, _ := procWaveOutGetNumDevs.Call()
	var out []string
	for i := uintptr(0); i < n; i++ {
		var c waveOutCaps
		if r, _, _ := procWaveOutGetDevCaps.Call(i, uintptr(unsafe.Pointer(&c)), unsafe.Sizeof(c)); r == 0 {
			out = append(out, syscall.UTF16ToString(c.pname[:]))
		}
	}
	return out
}

// deviceID resolves a saved device name to its current index (indexes change when devices come and go).
func deviceID(name string) uintptr {
	if name == "" {
		return waveMapper
	}
	for i, d := range outputDevices() {
		if d == name {
			return uintptr(i)
		}
	}
	return waveMapper // device unplugged: fall back to the default
}

// playPCM plays 16-bit mono PCM and blocks until it ends or stopAudio is called.
func playPCM(pcm []byte, rate int, volume int, device string) error {
	audioMu.Lock()
	defer audioMu.Unlock()
	if len(pcm) < 2 {
		return nil
	}
	data := scaleVolume(pcm, volume)
	wf := waveFormatEx{formatTag: 1, channels: 1, samplesPerSec: uint32(rate),
		avgBytesPerSec: uint32(rate * 2), blockAlign: 2, bitsPerSample: 16}
	var h uintptr
	if r, _, _ := procWaveOutOpen.Call(uintptr(unsafe.Pointer(&h)), deviceID(device), uintptr(unsafe.Pointer(&wf)), 0, 0, 0); r != 0 {
		return fmt.Errorf("waveOutOpen failed (%d)", r)
	}
	hdr := &waveHdr{data: &data[0], bufferLength: uint32(len(data))}
	procWaveOutPrepare.Call(h, uintptr(unsafe.Pointer(hdr)), unsafe.Sizeof(*hdr))
	curMu.Lock()
	curHandle = h
	curMu.Unlock()
	procWaveOutWrite.Call(h, uintptr(unsafe.Pointer(hdr)), unsafe.Sizeof(*hdr))

	deadline := time.Now().Add(time.Duration(len(data)/2*1000/rate+2000) * time.Millisecond)
	for (*(*uint32)(unsafe.Pointer(&hdr.flags)))&whdrDone == 0 && time.Now().Before(deadline) {
		time.Sleep(15 * time.Millisecond)
	}
	curMu.Lock()
	curHandle = 0
	curMu.Unlock()
	procWaveOutReset.Call(h)
	procWaveOutUnprepare.Call(h, uintptr(unsafe.Pointer(hdr)), unsafe.Sizeof(*hdr))
	procWaveOutClose.Call(h)
	runtime.KeepAlive(data)
	runtime.KeepAlive(hdr)
	return nil
}

// stopAudio cuts off whatever is playing; playPCM then returns.
func stopAudio() {
	curMu.Lock()
	h := curHandle
	curMu.Unlock()
	if h != 0 {
		procWaveOutReset.Call(h) // marks the buffer done, so playPCM's loop exits
	}
}
