//go:build windows

package main

// The on-screen pill shown while VoiceLoop is preparing or playing audio.
// A layered, topmost, no-activate window: it never takes focus from what you're typing in.

import (
	"image"
	"log"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")

	procRegisterClassEx   = user32.NewProc("RegisterClassExW")
	procCreateWindowEx    = user32.NewProc("CreateWindowExW")
	procDefWindowProc     = user32.NewProc("DefWindowProcW")
	procShowWindow        = user32.NewProc("ShowWindow")
	procSetWindowPos      = user32.NewProc("SetWindowPos")
	procUpdateLayered     = user32.NewProc("UpdateLayeredWindow")
	procSetTimer          = user32.NewProc("SetTimer")
	procKillTimer         = user32.NewProc("KillTimer")
	procPostMessage       = user32.NewProc("PostMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	procSystemParamsInfo  = user32.NewProc("SystemParametersInfoW")
	procGetDpiForSystem   = user32.NewProc("GetDpiForSystem")
	procSetDpiAwarenessCx = user32.NewProc("SetProcessDpiAwarenessContext")
	procLoadCursor        = user32.NewProc("LoadCursorW")
	procGetModuleHandle   = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

const (
	wsPopup           = 0x80000000
	wsExLayered       = 0x00080000
	wsExTopmost       = 0x00000008
	wsExToolWindow    = 0x00000080
	wsExNoActivate    = 0x08000000
	swHide            = 0
	swShowNoActivate  = 4
	swpNoActivate     = 0x0010
	swpNoSize         = 0x0001
	wmTimer           = 0x0113
	wmLButtonUp       = 0x0202
	wmMouseActivate   = 0x0021
	wmApp             = 0x8000
	wmOverlayShow     = wmApp + 1
	wmOverlayHide     = wmApp + 2
	wmOverlayReconfig = wmApp + 3
	maNoActivate      = 3
	ulwAlpha          = 2
	spiGetWorkArea    = 0x0030
	frameMs           = 33
	timerID           = 1
)

type point struct{ x, y int32 }
type size struct{ cx, cy int32 }
type rect struct{ left, top, right, bottom int32 }

type overlayState struct {
	mu      sync.Mutex
	visible bool
	content PillContent
	thread  string    // thread id of what's showing, for the mute button
	env     []float64 // nil while pending (waiting for Kokoro)
	shownAt time.Time
	playAt  time.Time // when audio started, after the pre-roll
}

var (
	ov            overlayState
	ovHwnd        uintptr
	ovScale       = 1.0
	ovSysScale    = 1.0
	ovMonScale    = 1.0
	ovW, ovH      int32 // current bitmap size
	ovPos         point
	ovWork        rect
	ovMemDC       uintptr
	ovBits        unsafe.Pointer
	ovEnabled     bool
	ovPaintFailed bool
	ovBitmap      uintptr
	ovPalette     Palette
	ovSettings    Settings
	ovLayout      PillLayout
	ovTick        int
)

// enableDPIAwareness must run before any window is created so the pill is drawn sharp.
func enableDPIAwareness() {
	if procSetDpiAwarenessCx.Find() == nil {
		procSetDpiAwarenessCx.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	}
}

// overlayInit creates the window. Call on the thread that runs the message loop.
func overlayInit() {
	if procGetDpiForSystem.Find() == nil {
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi > 0 {
			ovSysScale = float64(dpi) / 96
		}
	}
	hinst, _, _ := procGetModuleHandle.Call(0)
	className, _ := syscall.UTF16PtrFromString("VoiceLoopOverlay")
	cursor, _, _ := procLoadCursor.Call(0, 32649) // IDC_HAND
	wc := struct {
		cbSize, style                    uint32
		lpfnWndProc                      uintptr
		cbClsExtra, cbWndExtra           int32
		hInstance, hIcon, hCursor, hbrBg uintptr
		lpszMenuName, lpszClassName      *uint16
		hIconSm                          uintptr
	}{lpfnWndProc: windows.NewCallback(overlayProc), hInstance: hinst, hCursor: cursor, lpszClassName: className}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		log.Printf("overlay: register class: %v", err)
		return
	}
	title, _ := syscall.UTF16PtrFromString("VoiceLoop")
	ovHwnd, _, _ = procCreateWindowEx.Call(
		wsExLayered|wsExTopmost|wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup,
		0, 0, 1, 1, 0, 0, hinst, 0)
	if ovHwnd == 0 {
		log.Printf("overlay: create window failed")
		return
	}
	lockWatch(ovHwnd)
	ovMemDC, _, _ = procCreateCompatibleDC.Call(0)
	overlayConfigure()
	ovEnabled = true
}

// overlayConfigure reads monitor, scale and colours from the settings. Runs on the window's thread.
func overlayConfigure() {
	ovSettings = store.get()
	mon, ok := pickMonitor(ovSettings.Monitor)
	if !ok {
		mon.Scale = ovSysScale
		procSystemParamsInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&mon.work)), 0)
	}
	ovMonScale = mon.Scale
	ovWork = mon.work
	ovScale = mon.Scale * float64(ovSettings.PillSize) / 100
	ovPalette = ovSettings.palette()
	ovW, ovH = 0, 0 // force a fresh bitmap and position on the next paint
	log.Printf("overlay: %s at scale %.2f, %s, thread names %v (fit %v)", mon.Device, ovScale, ovSettings.Anchor, ovSettings.ShowThreadName, ovSettings.FitThreadName)
}

// ensureBitmap (re)creates the drawing bitmap and moves the window when the pill's size changes.
func ensureBitmap(w, h int32) bool {
	if w == ovW && h == ovH && ovBits != nil {
		return true
	}
	bmi := struct {
		size                          uint32
		width, height                 int32
		planes, bitCount              uint16
		compression, sizeImage        uint32
		xPels, yPels, used, important uint32
	}{size: 40, width: w, height: -h, planes: 1, bitCount: 32}
	var bits unsafe.Pointer
	hbm, _, _ := procCreateDIBSection.Call(ovMemDC, uintptr(unsafe.Pointer(&bmi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbm == 0 || bits == nil {
		log.Printf("overlay: CreateDIBSection failed")
		return false
	}
	procSelectObject.Call(ovMemDC, hbm)
	if ovBitmap != 0 {
		procDeleteObject.Call(ovBitmap)
	}
	ovBitmap, ovBits, ovW, ovH = hbm, bits, w, h
	st := ovSettings
	ovPos.x, ovPos.y = placePill(ovWork.left, ovWork.top, ovWork.right, ovWork.bottom, w, h, st.Anchor, st.OffsetX, st.OffsetY, ovMonScale)
	return true
}

// overlayReconfigure applies changed settings; safe to call from any goroutine.
func overlayReconfigure() {
	if ovHwnd != 0 {
		procPostMessage.Call(ovHwnd, wmOverlayReconfig, 0, 0)
	}
}

func overlayProc(hwnd, m, wparam, lparam uintptr) uintptr {
	switch m {
	case wmOverlayShow:
		procSetWindowPos.Call(hwnd, ^uintptr(0), 0, 0, 0, 0, swpNoActivate|swpNoSize|0x0002) // HWND_TOPMOST, no move
		overlayPaint()
		procShowWindow.Call(hwnd, swShowNoActivate)
		procSetTimer.Call(hwnd, timerID, frameMs, 0)
		return 0
	case wmOverlayHide:
		procKillTimer.Call(hwnd, timerID)
		procShowWindow.Call(hwnd, swHide)
		return 0
	case wmOverlayReconfig:
		overlayConfigure()
		ov.mu.Lock()
		visible := ov.visible
		ov.mu.Unlock()
		if visible {
			overlayPaint()
		}
		return 0
	case wmTimer:
		overlayPaint()
		// Every ~0.5 s, take back the top spot from other always-on-top windows.
		// SWP_NOACTIVATE means it never steals keyboard focus.
		ovTick++
		if ovTick%15 == 0 {
			procSetWindowPos.Call(hwnd, ^uintptr(0), 0, 0, 0, 0, swpNoActivate|swpNoSize|0x0002)
		}
		return 0
	case wmWTSSessionChange:
		go onSessionChange(wparam)
		return 0
	case wmMouseActivate:
		return maNoActivate
	case wmLButtonUp:
		pt := image.Pt(int(int16(lparam&0xffff)), int(int16(lparam>>16&0xffff)))
		switch {
		case pt.In(ovLayout.Mute):
			go muteShowing()
		case pt.In(ovLayout.Stop):
			go stopPlayback()
		}
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, m, wparam, lparam)
	return r
}

func overlayPaint() {
	ov.mu.Lock()
	content, env, shownAt, playAt := ov.content, ov.env, ov.shownAt, ov.playAt
	ov.mu.Unlock()
	if !ovSettings.ShowThreadName {
		content.Title = ""
	}
	content.Fit = ovSettings.FitThreadName
	content.MaxTitle = float64(ovSettings.MaxTitle)
	n := nBars

	var levels []float64
	pulse := 0.5 + 0.5*float64(time.Since(shownAt).Milliseconds()%1200)/1200
	if env == nil || time.Now().Before(playAt) {
		levels = pendingLevels(int(time.Since(shownAt).Milliseconds()), n)
	} else {
		levels = barLevels(env, int(time.Since(playAt).Milliseconds()), n)
		pulse = 1
	}
	img, lay := renderOverlay(ovScale, ovPalette, content, levels, pulse)
	if !ensureBitmap(int32(img.Bounds().Dx()), int32(img.Bounds().Dy())) {
		return
	}
	ovLayout = lay

	// RGBA (premultiplied) to BGRA (premultiplied) for the DIB.
	dst := unsafe.Slice((*byte)(ovBits), len(img.Pix))
	for i := 0; i+3 < len(img.Pix); i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3]
	}
	sz := size{ovW, ovH}
	src := point{0, 0}
	blend := [4]byte{0, 0, 255, 1} // AC_SRC_OVER, 0, alpha 255, AC_SRC_ALPHA
	r, _, err := procUpdateLayered.Call(ovHwnd, 0, uintptr(unsafe.Pointer(&ovPos)), uintptr(unsafe.Pointer(&sz)),
		ovMemDC, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	if r == 0 && !ovPaintFailed {
		ovPaintFailed = true
		log.Printf("overlay: UpdateLayeredWindow failed: %v", err)
	}
}

// overlayPending shows the pill straight away while Kokoro prepares the audio.
func overlayPending(j job) {
	if !ovEnabled {
		return
	}
	ov.mu.Lock()
	ov.content = PillContent{Kind: j.Kind, Title: j.Title}
	ov.thread, ov.env, ov.shownAt, ov.visible = j.Thread, nil, time.Now(), true
	ov.mu.Unlock()
	procPostMessage.Call(ovHwnd, wmOverlayShow, 0, 0)
}

// overlayPlaying switches the pill to bars that follow the audio.
func overlayPlaying(j job, env []float64, prerollMs int) {
	if !ovEnabled {
		return
	}
	ov.mu.Lock()
	wasVisible := ov.visible
	if !wasVisible {
		ov.shownAt = time.Now()
	}
	ov.content = PillContent{Kind: j.Kind, Title: j.Title}
	ov.thread, ov.env, ov.visible = j.Thread, env, true
	ov.playAt = time.Now().Add(time.Duration(prerollMs) * time.Millisecond)
	ov.mu.Unlock()
	if !wasVisible {
		procPostMessage.Call(ovHwnd, wmOverlayShow, 0, 0)
	}
}

// overlayShowingThread returns the thread on the pill and marks it muted for display.
func overlayMarkMuted() (thread, title string) {
	ov.mu.Lock()
	defer ov.mu.Unlock()
	ov.content.Muted = true
	return ov.thread, ov.content.Title
}

func overlayHide() {
	if !ovEnabled {
		return
	}
	ov.mu.Lock()
	ov.visible = false
	ov.mu.Unlock()
	procPostMessage.Call(ovHwnd, wmOverlayHide, 0, 0)
}

// messageLoop owns the overlay window and the global hotkeys.
func messageLoop(onHotkey func(id uintptr)) {
	var m msg
	for {
		r, _, _ := procGetMsg.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		if m.message == wmHotkey && m.hwnd == 0 {
			onHotkey(m.wparam)
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}
