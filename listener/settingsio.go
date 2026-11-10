package main

// Settings export and import. A bundle holds everything in the settings panel,
// voice mode and the muted threads. It never holds config.json (the token and
// listen address belong to this machine), and the pad profile is exported
// next to it as its own XML file.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const settingsBundleFormat = 1

type MutedThread struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

type SettingsBundle struct {
	Format       int           `json:"voiceloop_settings"`
	Exported     time.Time     `json:"exported"`
	AppVersion   string        `json:"app_version"`
	VoiceMode    bool          `json:"voice_mode"`
	Settings     Settings      `json:"settings"`
	MutedThreads []MutedThread `json:"muted_threads"`
}

func currentBundle(voiceOn bool) SettingsBundle {
	b := SettingsBundle{Format: settingsBundleFormat, Exported: time.Now().UTC(), AppVersion: version,
		VoiceMode: voiceOn, Settings: store.get(), MutedThreads: []MutedThread{}}
	b.Settings.Pad = nil
	b.Settings.TTSKey = "" // a speech server's API key stays on this machine, like the token
	for _, t := range threads.list() {
		if t.Muted {
			b.MutedThreads = append(b.MutedThreads, MutedThread{t.ID, t.Title})
		}
	}
	return b
}

func (b SettingsBundle) marshal() []byte {
	out, _ := json.MarshalIndent(b, "", "  ")
	return append(out, '\n')
}

// parseSettingsBundle reads an exported file. Settings missing from it (say,
// from an older voiceloop) take their defaults; out-of-range values are clamped.
func parseSettingsBundle(raw []byte) (SettingsBundle, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	var probe struct {
		Format   *int            `json:"voiceloop_settings"`
		Settings json.RawMessage `json:"settings"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return SettingsBundle{}, fmt.Errorf("not a voiceloop settings file: %v", err)
	}
	if probe.Format == nil || len(probe.Settings) == 0 {
		return SettingsBundle{}, fmt.Errorf("not a voiceloop settings file (exports start with \"voiceloop_settings\")")
	}
	if *probe.Format != settingsBundleFormat {
		return SettingsBundle{}, fmt.Errorf("settings file format %d isn't supported by this voiceloop (it reads format %d)", *probe.Format, settingsBundleFormat)
	}
	b := SettingsBundle{Settings: defaultSettings()}
	if err := json.Unmarshal(raw, &b); err != nil {
		return SettingsBundle{}, fmt.Errorf("the settings in that file don't fit: %v", err)
	}
	b.Settings.Pad = nil
	b.Settings.normalise()
	return b, nil
}

// backupSettings saves the current bundle in settings-backups\ next to the exe
// and returns the file name.
func backupSettings(dir, why string, voiceOn bool) (string, error) {
	d := filepath.Join(dir, "settings-backups")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("voiceloop-settings-%s-before-%s.json", time.Now().Format("2006-01-02-150405"), why)
	return name, os.WriteFile(filepath.Join(d, name), currentBundle(voiceOn).marshal(), 0o644)
}

// applyBundle replaces the settings and the mute list with the bundle's.
func applyBundle(b SettingsBundle) error {
	if b.Settings.TTSKey == "" {
		b.Settings.TTSKey = store.get().TTSKey // exports leave the key out, so keep this machine's
	}
	if err := store.replace(b.Settings); err != nil {
		return err
	}
	want := map[string]string{}
	for _, t := range b.MutedThreads {
		if t.ID != "" {
			want[t.ID] = t.Title
		}
	}
	for _, t := range threads.list() {
		if _, keep := want[t.ID]; t.Muted && !keep {
			threads.setMuted(t.ID, t.Title, false)
		}
	}
	for id, title := range want {
		threads.setMuted(id, title, true)
	}
	return nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func padExportName(p *PadProfile, day string) string {
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(p.Name), "-"), "-")
	if slug == "" {
		slug = "pad"
	}
	return fmt.Sprintf("voiceloop-pad-%s-%s.xml", slug, day)
}
