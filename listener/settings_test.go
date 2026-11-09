package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPlacePill(t *testing.T) {
	// 4K monitor at 125%, taskbar 60 px: work area 0,0-3840,2100. Pill 295x65.
	x, y := placePill(0, 0, 3840, 2100, 295, 65, "bottom-centre", 0, 118, 1.25)
	if x != 1772 || y != 1888 {
		t.Fatalf("bottom-centre: got %d,%d want 1772,1888 (where it sits today)", x, y)
	}
	// Secondary monitor to the right, top-left anchor with 20 px margins.
	x, y = placePill(3840, 0, 5760, 1040, 236, 52, "top-left", 20, 20, 1)
	if x != 3860 || y != 20 {
		t.Fatalf("top-left on second screen: got %d,%d", x, y)
	}
	// Silly offsets are clamped onto the screen.
	x, y = placePill(0, 0, 1920, 1040, 236, 52, "bottom-right", -5000, 9000, 1)
	if x != 1920-236 || y != 0 {
		t.Fatalf("clamp: got %d,%d", x, y)
	}
}

func TestSettingsStore(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	var st settingsStore
	st.load(p, Config{KokoroURL: "http://127.0.0.1:8880/", Voice: "bm_george", Speed: 1.1})
	s := st.get()
	if s.KokoroURL != "http://127.0.0.1:8880" || s.Voice != "bm_george" || s.ChimeVolume != 70 {
		t.Fatalf("first-run seed wrong: %+v", s)
	}
	s, err := st.update([]byte(`{"chime_volume": 250, "speech_colour": "nonsense", "anchor": "top-right", "pill_size": 120}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.ChimeVolume != 100 || s.SpeechColour != "#4fd1c5" || s.Anchor != "top-right" || s.PillSize != 120 {
		t.Fatalf("normalise wrong: %+v", s)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("settings.json not written")
	}
	var again settingsStore
	again.load(p, Config{KokoroURL: "http://ignored"})
	if again.get().Anchor != "top-right" || again.get().KokoroURL != "http://127.0.0.1:8880" {
		t.Fatalf("saved settings should win over config.json: %+v", again.get())
	}
}

func TestThreadStore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "threads.json")
	var ts threadStore
	ts.m = map[string]*ThreadInfo{}
	ts.load(p)
	ts.heard("a", "First thread")
	ts.heard("b", "Second")
	ts.setMuted("a", "", true)
	ts.heard("a", "First thread, renamed")
	if !ts.isMuted("a") || ts.isMuted("b") || ts.isMuted("nope") {
		t.Fatal("mute state wrong")
	}
	l := ts.list()
	if len(l) != 2 || l[0].ID != "a" || l[0].Title != "First thread, renamed" {
		t.Fatalf("list wrong: %+v", l)
	}
	var again threadStore
	again.m = map[string]*ThreadInfo{}
	again.load(p)
	if !again.isMuted("a") {
		t.Fatal("mute not saved")
	}
}

func TestThreadTrim(t *testing.T) {
	var ts threadStore
	ts.m = map[string]*ThreadInfo{}
	ts.heard("fresh", "Fresh")
	ts.heard("old", "Old")
	ts.heard("oldmuted", "Old but muted")
	ts.setMuted("oldmuted", "", true)
	ts.m["old"].LastHeard = time.Now().Add(-30 * time.Hour)
	ts.m["oldmuted"].LastHeard = time.Now().Add(-30 * time.Hour)
	if n := ts.trim(24 * time.Hour); n != 1 {
		t.Fatalf("trimmed %d, want 1", n)
	}
	if _, ok := ts.m["old"]; ok {
		t.Fatal("old unmuted thread should be gone")
	}
	if _, ok := ts.m["oldmuted"]; !ok {
		t.Fatal("muted thread must be kept")
	}
	var s Settings = defaultSettings()
	s.ThreadTrimHours = 500
	s.normalise()
	if s.ThreadTrimHours != 72 {
		t.Fatalf("trim hours not clamped: %d", s.ThreadTrimHours)
	}
}

func TestMuteWhenLockedDefault(t *testing.T) {
	// A settings.json from before v0.4.8 has no mute_when_locked key: it should come up on.
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, []byte(`{"voice":"bf_isabella","chimes_when_off":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var st settingsStore
	st.load(p, Config{})
	if !st.get().MuteWhenLocked {
		t.Fatal("mute_when_locked should default to on")
	}
	s, _ := st.update([]byte(`{"mute_when_locked": false}`))
	if s.MuteWhenLocked {
		t.Fatal("turning it off didn't stick")
	}
}

func TestSettingsWithBOM(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, []byte("\xef\xbb\xbf{\"offset_y\": 73}"), 0o644); err != nil {
		t.Fatal(err)
	}
	var st settingsStore
	st.load(p, Config{})
	if st.get().OffsetY != 73 {
		t.Fatalf("BOM file not read: offset_y %d", st.get().OffsetY)
	}
}
