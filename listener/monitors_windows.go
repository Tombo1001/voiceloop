//go:build windows

package main

import (
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	procEnumDisplayDevices  = user32.NewProc("EnumDisplayDevicesW")
	procGetDpiForMonitor    = windows.NewLazySystemDLL("shcore.dll").NewProc("GetDpiForMonitor")
)

type Monitor struct {
	Device  string  `json:"device"` // \\.\DISPLAY1
	Name    string  `json:"name"`   // e.g. LEN L28u-30
	Primary bool    `json:"primary"`
	Width   int32   `json:"width"` // physical pixels
	Height  int32   `json:"height"`
	Scale   float64 `json:"scale"` // 1.25 = 125%
	work    rect
}

type monitorInfoEx struct {
	cbSize  uint32
	monitor rect
	work    rect
	flags   uint32
	device  [32]uint16
}

type displayDevice struct {
	cb           uint32
	deviceName   [32]uint16
	deviceString [128]uint16
	stateFlags   uint32
	deviceID     [128]uint16
	deviceKey    [128]uint16
}

var (
	monMu   sync.Mutex
	monAcc  []Monitor
	monCbFn = windows.NewCallback(monitorEnumProc) // created once: Windows callbacks can't be freed
)

func monitors() []Monitor {
	monMu.Lock()
	defer monMu.Unlock()
	monAcc = nil
	procEnumDisplayMonitors.Call(0, 0, monCbFn, 0)
	return append([]Monitor(nil), monAcc...)
}

func monitorEnumProc(hmon, hdc, lprc, data uintptr) uintptr {
	{
		mi := monitorInfoEx{}
		mi.cbSize = uint32(unsafe.Sizeof(mi))
		if r, _, _ := procGetMonitorInfo.Call(hmon, uintptr(unsafe.Pointer(&mi))); r == 0 {
			return 1
		}
		m := Monitor{
			Device:  syscall.UTF16ToString(mi.device[:]),
			Primary: mi.flags&1 != 0,
			Width:   mi.monitor.right - mi.monitor.left,
			Height:  mi.monitor.bottom - mi.monitor.top,
			Scale:   1,
			work:    mi.work,
		}
		var dx, dy uint32
		if procGetDpiForMonitor.Find() == nil {
			if r, _, _ := procGetDpiForMonitor.Call(hmon, 0, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); r == 0 && dx > 0 {
				m.Scale = float64(dx) / 96
			}
		}
		dd := displayDevice{}
		dd.cb = uint32(unsafe.Sizeof(dd))
		dev, _ := syscall.UTF16PtrFromString(m.Device)
		if r, _, _ := procEnumDisplayDevices.Call(uintptr(unsafe.Pointer(dev)), 0, uintptr(unsafe.Pointer(&dd)), 0); r != 0 {
			m.Name = syscall.UTF16ToString(dd.deviceString[:])
		}
		if m.Name == "" || m.Name == "Generic PnP Monitor" {
			m.Name = m.Device
		}
		monAcc = append(monAcc, m)
		return 1
	}
}

// pickMonitor returns the monitor named in settings, or the primary one.
func pickMonitor(device string) (Monitor, bool) {
	ms := monitors()
	for _, m := range ms {
		if device != "" && m.Device == device {
			return m, true
		}
	}
	for _, m := range ms {
		if m.Primary {
			return m, true
		}
	}
	if len(ms) > 0 {
		return ms[0], true
	}
	return Monitor{}, false
}
