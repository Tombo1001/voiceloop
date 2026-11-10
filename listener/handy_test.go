package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHandyShortcut(t *testing.T) {
	file := func(binding string) []byte {
		return []byte(`{"settings":{"auto_submit":true,"bindings":{"cancel":{"current_binding":"escape"},` +
			`"transcribe":{"id":"transcribe","default_binding":"ctrl+space","current_binding":"` + binding + `"}}}}`)
	}
	for in, want := range map[string][2]string{
		"ctrl+space":       {"ctrl-space", "Ctrl+Space"},
		"Ctrl+Shift+D":     {"ctrl-shift-d", "Ctrl+Shift+D"},
		"option+space":     {"alt-space", "Alt+Space"},
		"super+control+f9": {"win-ctrl-f9", "Win+Ctrl+F9"},
		"f13":              {"f13", "F13"},
	} {
		a, p, ok := handyShortcutFrom(file(in))
		if !ok || a != want[0] || p != want[1] {
			t.Errorf("%s: got %q %q %v, want %v", in, a, p, ok, want)
		}
	}
	for _, bad := range []string{"", "ctrl+", "ctrl+mousebutton4", "fn+space"} {
		if a, _, ok := handyShortcutFrom(file(bad)); ok {
			t.Errorf("%q should be refused, got %q", bad, a)
		}
	}
	if _, _, ok := handyShortcutFrom([]byte(`not json`)); ok {
		t.Error("bad JSON accepted")
	}
}

func TestHandyFileAndIcons(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	if _, _, ok := handyShortcut(); ok {
		t.Fatal("found Handy in an empty folder")
	}
	if iconFor("ctrl-alt-d") != "" || iconFor("ctrl-space") != "dictate.png" {
		t.Fatal("icons without Handy")
	}
	os.MkdirAll(filepath.Join(dir, "com.pais.handy"), 0o755)
	os.WriteFile(filepath.Join(dir, "com.pais.handy", "settings_store.json"),
		[]byte(`{"settings":{"bindings":{"transcribe":{"current_binding":"ctrl+alt+d"}}}}`), 0o644)
	if a, p, ok := handyShortcut(); !ok || a != "ctrl-alt-d" || p != "Ctrl+Alt+D" {
		t.Fatalf("got %q %q %v", a, p, ok)
	}
	if iconFor("ctrl-alt-d") != "dictate.png" || iconFor("ctrl-alt-d,slash") != "slash-command.png" || iconFor("ctrl-alt-x") != "stop-speech.png" {
		t.Fatal("icons should follow Handy's shortcut")
	}
}
