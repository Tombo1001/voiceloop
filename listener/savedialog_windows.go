//go:build windows

package main

// A native Windows "Save As" dialog, opened by voiceloop itself so an export
// can write two files (settings and pad profile) into the folder you pick.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetSaveFileName = windows.NewLazySystemDLL("comdlg32.dll").NewProc("GetSaveFileNameW")
	procGetOpenFileName = windows.NewLazySystemDLL("comdlg32.dll").NewProc("GetOpenFileNameW")
	procGetForegroundW  = user32.NewProc("GetForegroundWindow")
	procCoInitializeEx  = windows.NewLazySystemDLL("ole32.dll").NewProc("CoInitializeEx")
	procCoUninitialize  = windows.NewLazySystemDLL("ole32.dll").NewProc("CoUninitialize")
)

type openFileName struct {
	structSize    uint32
	owner         uintptr
	instance      uintptr
	filter        *uint16
	customFilter  *uint16
	maxCustFilter uint32
	filterIndex   uint32
	file          *uint16
	maxFile       uint32
	fileTitle     *uint16
	maxFileTitle  uint32
	initialDir    *uint16
	title         *uint16
	flags         uint32
	fileOffset    uint16
	fileExtension uint16
	defExt        *uint16
	custData      uintptr
	hook          uintptr
	templateName  *uint16
	reserved      uintptr
	reservedDword uint32
	flagsEx       uint32
}

// saveDialog asks where to save a file. It returns "" if the user cancels.
// The dialog belongs to the window in front (the settings window), so it
// opens on top of it.
func saveDialog(title, defaultName, filterName, pattern, ext string) string {
	home, _ := os.UserHomeDir()
	return fileDialog(false, title, defaultName, filepath.Join(home, "Documents"), filterName, pattern, ext)
}

// openDialog asks for an existing file, starting in dir. "" if cancelled.
func openDialog(title, dir, filterName, pattern string) string {
	return fileDialog(true, title, "", dir, filterName, pattern, "")
}

func fileDialog(open bool, title, defaultName, startDir, filterName, pattern, ext string) string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procCoInitializeEx.Call(0, 0x2) // COINIT_APARTMENTTHREADED, which the Explorer-style dialog wants
	defer procCoUninitialize.Call()

	buf := make([]uint16, 1024)
	copy(buf, syscall.StringToUTF16(defaultName))
	// The filter is "name\0pattern\0\0", which StringToUTF16 refuses (it panics on NULs).
	filter := append(utf16.Encode([]rune(filterName+"\x00"+pattern+"\x00")), 0)
	t, _ := syscall.UTF16PtrFromString(title)
	var e *uint16
	if ext != "" {
		e, _ = syscall.UTF16PtrFromString(ext)
	}
	dir, _ := syscall.UTF16PtrFromString(startDir)
	owner, _, _ := procGetForegroundW.Call()
	ofn := openFileName{
		owner: owner, filter: &filter[0], filterIndex: 1,
		file: &buf[0], maxFile: uint32(len(buf)), initialDir: dir, title: t, defExt: e,
		// OFN_NOCHANGEDIR | OFN_PATHMUSTEXIST | OFN_EXPLORER
		flags: 0x8 | 0x800 | 0x80000,
	}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))
	proc := procGetSaveFileName
	if open {
		ofn.flags |= 0x1000 // OFN_FILEMUSTEXIST
		proc = procGetOpenFileName
	} else {
		ofn.flags |= 0x2 // OFN_OVERWRITEPROMPT
	}
	if r, _, _ := proc.Call(uintptr(unsafe.Pointer(&ofn))); r == 0 {
		return ""
	}
	return strings.TrimSpace(syscall.UTF16ToString(buf))
}
