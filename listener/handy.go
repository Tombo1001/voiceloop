package main

// Handy is the dictation app voiceloop's pad keys start. Its shortcut lives in
// %APPDATA%\com.pais.handy\settings_store.json, so the "Dictate" pad action can follow it
// instead of assuming Ctrl+Space.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func handySettingsPath() string {
	if d := os.Getenv("APPDATA"); d != "" {
		return filepath.Join(d, "com.pais.handy", "settings_store.json")
	}
	return ""
}

// handyShortcut returns Handy's dictation shortcut as a pad action ("ctrl-space") and as
// people write it ("Ctrl+Space"). ok is false when Handy isn't installed, or its shortcut
// is one a pad can't send.
func handyShortcut() (action, shortcut string, ok bool) {
	p := handySettingsPath()
	if p == "" {
		return "", "", false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", "", false
	}
	return handyShortcutFrom(b)
}

func handyShortcutFrom(b []byte) (action, shortcut string, ok bool) {
	var f struct {
		Settings struct {
			Bindings map[string]struct {
				Current string `json:"current_binding"`
			} `json:"bindings"`
		} `json:"settings"`
	}
	if json.Unmarshal(b, &f) != nil {
		return "", "", false
	}
	raw := strings.TrimSpace(f.Settings.Bindings["transcribe"].Current)
	if raw == "" {
		return "", "", false
	}
	alias := map[string]string{
		"control": "ctrl", "option": "alt", "super": "win", "meta": "win", "cmd": "win", "command": "win",
		"windows": "win", "return": "enter", "esc": "escape", "arrowup": "up", "arrowdown": "down",
		"arrowleft": "left", "arrowright": "right", "period": "dot", "backquote": "grave",
	}
	var keys, pretty []string
	for _, k := range strings.Split(strings.ToLower(raw), "+") {
		k = strings.TrimSpace(k)
		if a, found := alias[k]; found {
			k = a
		}
		if k == "" {
			return "", "", false
		}
		keys = append(keys, k)
		if len(k) == 1 {
			pretty = append(pretty, strings.ToUpper(k))
		} else {
			pretty = append(pretty, strings.ToUpper(k[:1])+k[1:])
		}
	}
	action = strings.Join(keys, "-")
	if _, err := parsePadAction(action); err != nil {
		return "", "", false
	}
	return action, strings.Join(pretty, "+"), true
}

// iconFor gives the starter icon for a pad action. Handy's current shortcut counts as Dictate,
// so a pad set to follow a changed shortcut still gets the right key symbols.
func iconFor(action string) string {
	if ic := actionIcons[action]; ic != "" {
		return ic
	}
	if h, _, ok := handyShortcut(); ok && h != "ctrl-space" {
		switch action {
		case h:
			return actionIcons["ctrl-space"]
		case h + ",slash":
			return actionIcons["ctrl-space,slash"]
		}
	}
	return ""
}

// handyState is what the settings panel needs: nil when Handy's shortcut isn't known.
func handyState() map[string]string {
	if a, p, ok := handyShortcut(); ok {
		return map[string]string{"action": a, "shortcut": p}
	}
	return nil
}
