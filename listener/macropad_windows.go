//go:build windows

package main

// Talks to the macropad through Windows' own HID driver: no extra driver, no
// admin rights. Windows locks a pad's keyboard and mouse interfaces, but its
// vendor-defined interface (usage page 0xFF00), which is the programming
// channel, is open to ordinary programs.

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	hidDLL             = windows.NewLazySystemDLL("hid.dll")
	procHidGetHidGuid  = hidDLL.NewProc("HidD_GetHidGuid")
	procHidGetPreparse = hidDLL.NewProc("HidD_GetPreparsedData")
	procHidFreePrepars = hidDLL.NewProc("HidD_FreePreparsedData")
	procHidPGetCaps    = hidDLL.NewProc("HidP_GetCaps")

	padMu sync.Mutex // one upload at a time
)

type hidCaps struct {
	Usage, UsagePage                uint16
	InputLen, OutputLen, FeatureLen uint16
	_                               [17]uint16
	_                               [10]uint16 // link and value counts, unused
}

// PadInfo is what the settings page shows about the pad.
type PadInfo struct {
	Found bool   `json:"found"`
	Model string `json:"model,omitempty"`
	Error string `json:"error,omitempty"`
}

// findPad returns the device path of the pad's programming interface, found by
// the USB IDs, usage page and report length in the active profile.
func findPad() (string, error) {
	u := pads.get().USB
	vid, _ := parseHex16(u.VendorID)
	pid, _ := parseHex16(u.ProductID)
	page, _ := parseHex16(u.UsagePage)
	var g windows.GUID
	procHidGetHidGuid.Call(uintptr(unsafe.Pointer(&g)))
	paths, err := windows.CM_Get_Device_Interface_List("", &g, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
	if err != nil {
		return "", err
	}
	want := fmt.Sprintf("vid_%04x&pid_%04x", vid, pid)
	for _, p := range paths {
		if !strings.Contains(strings.ToLower(p), want) {
			continue
		}
		h, err := openHID(p)
		if err != nil {
			continue // the keyboard and mouse interfaces are locked by Windows; that's expected
		}
		caps, ok := hidGetCaps(h)
		windows.CloseHandle(h)
		if ok && caps.UsagePage == page && int(caps.OutputLen) == u.ReportLength {
			return p, nil
		}
	}
	return "", fmt.Errorf("no pad found")
}

func openHID(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
}

func hidGetCaps(h windows.Handle) (hidCaps, bool) {
	var pre uintptr
	if r, _, _ := procHidGetPreparse.Call(uintptr(h), uintptr(unsafe.Pointer(&pre))); r == 0 {
		return hidCaps{}, false
	}
	defer procHidFreePrepars.Call(pre)
	var c hidCaps
	r, _, _ := procHidPGetCaps.Call(pre, uintptr(unsafe.Pointer(&c)))
	return c, r == 0x110000 // HIDP_STATUS_SUCCESS
}

func padStatus() PadInfo {
	if _, err := findPad(); err != nil {
		return PadInfo{}
	}
	return PadInfo{Found: true, Model: pads.get().Name}
}

// uploadPad writes the active profile's key map (and light mode) to the pad.
func uploadPad() error {
	p := pads.get()
	msgs, err := padMessagesFor(p)
	if err != nil {
		return err
	}
	log.Printf("macropad upload: %d reports (%s)", len(msgs), p.Name)
	return padWrite(msgs, p.USB.ReportLength)
}

// padLights changes just the LED effect.
func padLights(mode int) error {
	log.Printf("macropad lights: mode %d", mode)
	p := pads.get()
	return padWrite(padEncoders[p.Protocol].lights(mode), p.USB.ReportLength)
}

func padWrite(msgs [][]byte, reportLen int) error {
	padMu.Lock()
	defer padMu.Unlock()
	path, err := findPad()
	if err != nil {
		return fmt.Errorf("plug the pad in first")
	}
	h, err := openHID(path)
	if err != nil {
		return fmt.Errorf("couldn't open the pad: %v", err)
	}
	defer windows.CloseHandle(h)
	// Each protocol message already starts with the report ID (0x03 on the 8890),
	// so it goes out as-is, padded to the report length. The all-zero wake-up message the
	// libusb tool sends first isn't a valid report through Windows' HID driver;
	// the pad doesn't need it here.
	buf := make([]byte, reportLen)
	for i, m := range msgs {
		if m[0] == 0 {
			continue
		}
		clear(buf)
		copy(buf, m)
		var n uint32
		if err := windows.WriteFile(h, buf, &n, nil); err != nil || int(n) != reportLen {
			return fmt.Errorf("writing to the pad failed at message %d of %d: %v", i+1, len(msgs), err)
		}
		time.Sleep(5 * time.Millisecond) // the pad's chip is slow; don't flood it
	}
	return nil
}
