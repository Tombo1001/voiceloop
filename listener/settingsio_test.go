package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsBundleRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store.load(filepath.Join(dir, "settings.json"), Config{})
	threads = threadStore{m: map[string]*ThreadInfo{}}
	threads.load(filepath.Join(dir, "threads.json"))
	store.update([]byte(`{"offset_y": 73, "voice": "bf_isabella", "pill_size": 110}`))
	threads.setMuted("t-1", "Noisy thread", true)
	threads.setMuted("t-2", "Quiet thread", false)

	raw := currentBundle(true).marshal()
	if strings.Contains(string(raw), "token") {
		t.Fatal("an export mentions the token")
	}
	b, err := parseSettingsBundle(raw)
	if err != nil {
		t.Fatal(err)
	}
	if b.Settings.OffsetY != 73 || b.Settings.Voice != "bf_isabella" || !b.VoiceMode || len(b.MutedThreads) != 1 {
		t.Fatalf("round trip: %+v", b)
	}

	// Change things, back them up, then import the export: everything returns.
	store.update([]byte(`{"offset_y": 200, "voice": "bm_george"}`))
	threads.setMuted("t-1", "", false)
	threads.setMuted("t-2", "", true)
	name, err := backupSettings(dir, "import", false)
	if err != nil || !strings.HasSuffix(name, "-before-import.json") {
		t.Fatalf("backup %q: %v", name, err)
	}
	saved, _ := os.ReadFile(filepath.Join(dir, "settings-backups", name))
	if !strings.Contains(string(saved), `"bm_george"`) {
		t.Fatal("backup doesn't hold the settings from before the import")
	}
	if err := applyBundle(b); err != nil {
		t.Fatal(err)
	}
	if s := store.get(); s.OffsetY != 73 || s.Voice != "bf_isabella" {
		t.Fatalf("import didn't restore settings: %+v", s)
	}
	if !threads.isMuted("t-1") || threads.isMuted("t-2") {
		t.Fatal("import didn't restore the mute list")
	}
}

func TestSettingsBundleBadFiles(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`<padProfile/>`, "not a voiceloop settings file"},
		{`{"voice": "x"}`, "not a voiceloop settings file"},
		{`{"voiceloop_settings": 9, "settings": {}}`, "format 9 isn't supported"},
		{`{"voiceloop_settings": 1, "settings": {"offset_y": "high"}}`, "don't fit"},
	} {
		if _, err := parseSettingsBundle([]byte(c.in)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.in, err, c.want)
		}
	}
	// Missing settings take defaults; silly values are clamped; a BOM is fine.
	b, err := parseSettingsBundle([]byte("\xef\xbb\xbf{\"voiceloop_settings\": 1, \"settings\": {\"chime_volume\": 900}}"))
	if err != nil || b.Settings.ChimeVolume != 100 || b.Settings.Voice != "bf_emma" {
		t.Fatalf("defaults/clamp: %v %+v", err, b.Settings)
	}
}

func TestPadExportName(t *testing.T) {
	p, _ := ParsePadProfile(defaultPadXML)
	if n := padExportName(p, "2026-11-09"); n != "voiceloop-pad-3-keys-knob-ch57x-2026-11-09.xml" {
		t.Fatal(n)
	}
}
