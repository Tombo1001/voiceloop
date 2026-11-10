//go:build windows

package main

// Settings panel: a small web UI served on 127.0.0.1 and opened in an Edge app window.

import (
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"voiceloop/listener/logo"
)

const settingsAddr = "127.0.0.1:8766"

//go:embed ui.html
var uiHTML []byte

var uiKey = func() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}()

func settingsServer() {
	mux := http.NewServeMux()
	guard := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Loopback only, and only pages that were opened with this run's key.
			if r.Host != settingsAddr || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-VL-Key")), []byte(uiKey)) != 1 {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			h(w, r)
		}
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || r.Host != settingsAddr {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(uiHTML)
	})
	favicon := logo.ICO(logo.Teal, 16, 24, 32, 48, 64, 256)
	logoPNG := logo.PNG(128, logo.Teal)
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		w.Write(favicon)
	})
	icon192, icon512 := logo.PNG(192, logo.Teal), logo.PNG(512, logo.Teal)
	mux.HandleFunc("/icon-192.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(icon192)
	})
	mux.HandleFunc("/icon-512.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(icon512)
	})
	mux.HandleFunc("/manifest.webmanifest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Write([]byte(`{"name":"voiceloop","short_name":"voiceloop","display":"standalone",` +
			`"background_color":"#1d1d1f","theme_color":"#1d1d1f",` +
			`"icons":[{"src":"/icon-192.png","sizes":"192x192","type":"image/png"},` +
			`{"src":"/icon-512.png","sizes":"512x512","type":"image/png"}]}`))
	})
	mux.HandleFunc("/logo.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(logoPNG)
	})
	// The settings page keeps this stream open. When voiceloop quits or restarts the
	// connection drops, and the page closes its window. EventSource can't send headers,
	// so the key comes in the query string here.
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Host != settingsAddr || subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("k")), []byte(uiKey)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no streaming", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, "event: hello\ndata: %s\n\n", version)
		fl.Flush()
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-t.C:
				fmt.Fprint(w, ": ping\n\n")
				fl.Flush()
			}
		}
	})
	mux.HandleFunc("/api/state", guard(func(w http.ResponseWriter, r *http.Request) {
		writeState(w)
	}))
	mux.HandleFunc("/api/settings", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "post only", http.StatusMethodNotAllowed)
			return
		}
		patch, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
		if _, err := store.update(patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		overlayReconfigure()
		writeState(w)
	}))
	mux.HandleFunc("/api/voice", guard(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ On bool }
		if json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&b) == nil {
			setVoice(b.On)
		}
		writeState(w)
	}))
	mux.HandleFunc("/api/test-speech", guard(func(w http.ResponseWriter, r *http.Request) {
		// The panel sends what's in the boxes, which may not be saved yet.
		var b struct{ URL, Model, Key, Voice string }
		_ = json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&b)
		srv := ttsServer{URL: strings.TrimRight(strings.TrimSpace(b.URL), "/"), Model: strings.TrimSpace(b.Model), Key: strings.TrimSpace(b.Key)}
		json.NewEncoder(w).Encode(testSpeechServer(srv, firstNonEmpty(strings.TrimSpace(b.Voice), store.get().Voice)))
	}))
	mux.HandleFunc("/api/preview", guard(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Kind string }
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&b)
		text := map[string]string{
			"speech":   "This is how Claude's replies will sound.",
			"question": "Question. Is this where you want the pill? Options: yes, or move it.",
			"approval": "Approval needed. Claude needs your permission to run a command.",
		}[b.Kind]
		if text == "" {
			http.Error(w, "bad kind", http.StatusBadRequest)
			return
		}
		drain()
		stopPlayback()
		// Show the most recently heard thread's name, so the preview looks like the real thing.
		title := "Example thread"
		for _, t := range threads.list() {
			if t.Title != "" {
				title = t.Title
				break
			}
		}
		queue <- job{Kind: b.Kind, Text: text, Title: title, Thread: "", gen: gen.Load()}
		w.Write([]byte(`{"ok":true}`))
	}))
	mux.HandleFunc("/api/beep", guard(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Kind string }
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&b)
		if chimePCM(b.Kind) == nil {
			http.Error(w, "bad kind", http.StatusBadRequest)
			return
		}
		beep(b.Kind)
		w.Write([]byte(`{"ok":true}`))
	}))
	mux.HandleFunc("/api/thread", guard(func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID    string
			Muted bool
		}
		if json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&b) != nil || b.ID == "" {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		threads.setMuted(b.ID, "", b.Muted)
		writeState(w)
	}))
	padState := func(w http.ResponseWriter, extra map[string]any) {
		out := map[string]any{"profile": pads.get(), "status": padStatus()}
		for k, v := range extra {
			out[k] = v
		}
		json.NewEncoder(w).Encode(out)
	}
	padErr := func(w http.ResponseWriter, err error) {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
	}
	mux.HandleFunc("/api/pad", guard(func(w http.ResponseWriter, r *http.Request) {
		padState(w, nil)
	}))
	mux.HandleFunc("/api/pad/bind", guard(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Control, Action string }
		_ = json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&b)
		if _, err := pads.update(func(p *PadProfile) error { return p.SetAction(b.Control, strings.TrimSpace(b.Action)) }); err != nil {
			padErr(w, err)
			return
		}
		padState(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/pad/layout", guard(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ KnobSide string }
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&b)
		if _, err := pads.update(func(p *PadProfile) error { p.Mapping.KnobSide = b.KnobSide; return nil }); err != nil {
			padErr(w, err)
			return
		}
		padState(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/pad/lights", guard(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Mode int }
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&b)
		if _, err := pads.update(func(p *PadProfile) error { p.Mapping.Lights = b.Mode; return nil }); err != nil {
			padErr(w, err)
			return
		}
		if err := padLights(b.Mode); err != nil {
			padState(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		padState(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/pad/upload", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "post only", http.StatusMethodNotAllowed)
			return
		}
		if err := uploadPad(); err != nil {
			log.Printf("macropad upload: %v", err)
			padErr(w, err)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/pad/check", guard(func(w http.ResponseWriter, r *http.Request) {
		// Check an action string as the user types it, with the active pad's limits.
		var b struct{ Action string }
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&b)
		if _, err := pads.get().parseAction(b.Action); err != nil {
			padErr(w, err)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/pad/export", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "post only", http.StatusMethodNotAllowed)
			return
		}
		p := pads.get()
		path := saveDialog("Export pad profile", padExportName(p, time.Now().Format("2006-01-02")),
			"Pad profile (*.xml)", "*.xml", "xml")
		if path == "" {
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "cancelled": true})
			return
		}
		out, err := ParsePadProfile(p.Marshal()) // a copy, so the icons go into the file only
		if err != nil {
			padErr(w, err)
			return
		}
		out.embedIcons(iconsDir())
		if err := os.WriteFile(path, out.Marshal(), 0o644); err != nil {
			padErr(w, fmt.Errorf("couldn't save the pad profile: %v", err))
			return
		}
		exported(path)
		log.Printf("pad profile exported to %s", path)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "pad": path})
	}))
	// Key symbols. The preview is an <img>, which can't send headers, so the key
	// comes in the query string (as for /api/events).
	mux.HandleFunc("/api/pad/symbol.png", func(w http.ResponseWriter, r *http.Request) {
		if r.Host != settingsAddr || subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("k")), []byte(uiKey)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		g, err := symbolGrid(pads.get(), iconsDir(), r.URL.Query().Get("control"))
		if err != nil || g == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(gridPNG(g))
	})
	mux.HandleFunc("/api/pad/symbol/choose", guard(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Control string }
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&b)
		path := openDialog("Choose a symbol for "+b.Control, iconsDir(), "Images (*.png, *.jpg)", "*.png;*.jpg;*.jpeg")
		if path == "" {
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "cancelled": true})
			return
		}
		name, err := adoptIcon(iconsDir(), path)
		if err != nil {
			padErr(w, err)
			return
		}
		if _, err := pads.update(func(p *PadProfile) error { p.SetSymbol(b.Control, name); return nil }); err != nil {
			padErr(w, err)
			return
		}
		padState(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/pad/symbol/clear", guard(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Control string }
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&b)
		pads.update(func(p *PadProfile) error { p.SetSymbol(b.Control, ""); return nil })
		padState(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/pad/symbols/match", guard(func(w http.ResponseWriter, r *http.Request) {
		n := 0
		pads.update(func(p *PadProfile) error { n = p.MatchSymbols(); return nil })
		padState(w, map[string]any{"ok": true, "matched": n})
	}))
	mux.HandleFunc("/api/pad/symbols/settings", guard(func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Shape       string
			Size        float64
			Layers      int
			LayerHeight float64
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&b)
		if _, err := pads.update(func(p *PadProfile) error {
			p.Symbols.Shape, p.Symbols.Size, p.Symbols.Layers, p.Symbols.LayerHeight = b.Shape, b.Size, b.Layers, b.LayerHeight
			return nil
		}); err != nil {
			padErr(w, err)
			return
		}
		padState(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/pad/symbols/folder", guard(func(w http.ResponseWriter, r *http.Request) {
		_ = exec.Command("explorer.exe", iconsDir()).Start()
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/pad/symbols/export", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "post only", http.StatusMethodNotAllowed)
			return
		}
		p := pads.get()
		stl, n, err := symbolsPlate(p, iconsDir())
		if err != nil {
			padErr(w, err)
			return
		}
		path := saveDialog("Export key symbols for 3D printing",
			fmt.Sprintf("voiceloop-key-symbols-%gmm-%s.stl", p.Symbols.Size, time.Now().Format("2006-01-02")),
			"STL for 3D printing (*.stl)", "*.stl", "stl")
		if path == "" {
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "cancelled": true})
			return
		}
		if err := os.WriteFile(path, stl, 0o644); err != nil {
			padErr(w, fmt.Errorf("couldn't save the STL: %v", err))
			return
		}
		exported(path)
		log.Printf("key symbols exported: %d symbols, %d bytes, %s", n, len(stl), path)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "stl": path, "count": n})
	}))
	mux.HandleFunc("/api/reveal", guard(func(w http.ResponseWriter, r *http.Request) {
		// Open Explorer on a file this run exported, with the file selected. Only
		// paths voiceloop wrote itself are accepted.
		var b struct{ Path string }
		_ = json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&b)
		if !wasExported(b.Path) {
			http.Error(w, "unknown file", http.StatusBadRequest)
			return
		}
		cmd := exec.Command("explorer.exe", "/select,"+b.Path)
		_ = cmd.Start()
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/pad/import", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "post only", http.StatusMethodNotAllowed)
			return
		}
		b, _ := io.ReadAll(io.LimitReader(r.Body, 16<<20)) // room for a few embedded icons
		p, err := ParsePadProfile(b)
		if err != nil {
			padErr(w, err)
			return
		}
		if n, err := p.adoptEmbeddedIcons(iconsDir()); err != nil {
			padErr(w, fmt.Errorf("couldn't save the profile's icons: %v", err))
			return
		} else if n > 0 {
			log.Printf("macropad: saved %d icon(s) from the imported profile", n)
		}
		backup, err := pads.replace(p, "import")
		if err != nil {
			padErr(w, err)
			return
		}
		log.Printf("macropad: imported %q, previous profile saved as %s", p.Name, backup)
		padState(w, map[string]any{"ok": true, "backup": backup})
	}))
	mux.HandleFunc("/api/pad/reset", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "post only", http.StatusMethodNotAllowed)
			return
		}
		p, _ := ParsePadProfile(defaultPadXML)
		backup, err := pads.replace(p, "reset")
		if err != nil {
			padErr(w, err)
			return
		}
		log.Printf("macropad: reset to the built-in pad, previous profile saved as %s", backup)
		padState(w, map[string]any{"ok": true, "backup": backup})
	}))
	mux.HandleFunc("/api/settings/export", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "post only", http.StatusMethodNotAllowed)
			return
		}
		day := time.Now().Format("2006-01-02")
		path := saveDialog("Export voiceloop settings", "voiceloop-settings-"+day+".json",
			"voiceloop settings (*.json)", "*.json", "json")
		if path == "" {
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "cancelled": true})
			return
		}
		if err := os.WriteFile(path, currentBundle(isOn()).marshal(), 0o644); err != nil {
			padErr(w, fmt.Errorf("couldn't save the settings: %v", err))
			return
		}
		// The pad profile goes next to it, as its own file.
		p := pads.get()
		padPath := filepath.Join(filepath.Dir(path), padExportName(p, day))
		if err := os.WriteFile(padPath, p.Marshal(), 0o644); err != nil {
			padErr(w, fmt.Errorf("the settings were saved, but the pad profile wasn't: %v", err))
			return
		}
		exported(path)
		exported(padPath)
		log.Printf("settings exported to %s (pad profile %s)", path, filepath.Base(padPath))
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "settings": path, "pad": padPath})
	}))
	mux.HandleFunc("/api/settings/import", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "post only", http.StatusMethodNotAllowed)
			return
		}
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 512<<10))
		b, err := parseSettingsBundle(raw)
		if err != nil {
			padErr(w, err)
			return
		}
		backup, err := backupSettings(baseDir, "import", isOn())
		if err != nil {
			padErr(w, fmt.Errorf("couldn't save a copy of the current settings first, so nothing was changed: %v", err))
			return
		}
		if err := applyBundle(b); err != nil {
			padErr(w, err)
			return
		}
		if b.VoiceMode != isOn() {
			setVoice(b.VoiceMode)
		}
		overlayReconfigure()
		log.Printf("settings imported (exported %s by %s), previous settings saved as %s", b.Exported.Format(time.RFC3339), b.AppVersion, backup)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "backup": backup})
	}))
	mux.HandleFunc("/api/stop", guard(func(w http.ResponseWriter, r *http.Request) {
		stopPlayback()
		w.Write([]byte(`{"ok":true}`))
	}))
	srv := &http.Server{Addr: settingsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		log.Printf("settings server: %v", err)
	}
}

func writeState(w http.ResponseWriter) {
	exe, _ := os.Executable()
	json.NewEncoder(w).Encode(map[string]any{
		"settings": store.get(),
		"monitors": monitors(),
		"devices":  outputDevices(),
		"voice_on": isOn(),
		"threads":  threadsTrimmed(),
		"version":  version,
		"folder":   filepath.Dir(exe),
		"pairing": map[string]any{
			"token": cfg.Token,
			"urls":  hookURLs(cfg.Listen),
		},
		"handy": handyState(),
	})
}

func iconsDir() string { return filepath.Join(baseDir, "icons") }

// Files exported this run, which /api/reveal may show in Explorer.
var (
	exportsMu sync.Mutex
	exportsW  = map[string]bool{}
)

func exported(path string) {
	exportsMu.Lock()
	exportsW[strings.ToLower(path)] = true
	exportsMu.Unlock()
}
func wasExported(path string) bool {
	exportsMu.Lock()
	defer exportsMu.Unlock()
	return path != "" && exportsW[strings.ToLower(path)]
}

// openSettings opens the panel in an Edge app window (no tabs or address bar),
// falling back to the default browser.
var (
	openMu   sync.Mutex
	lastOpen time.Time
)

func openSettings() {
	openMu.Lock()
	if time.Since(lastOpen) < 2*time.Second {
		openMu.Unlock()
		return
	}
	lastOpen = time.Now()
	openMu.Unlock()
	url := "http://" + settingsAddr + "/?k=" + uiKey
	for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
		edge := filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe")
		if _, err := os.Stat(edge); err == nil {
			cmd := exec.Command(edge, "--app="+url, "--window-size=980,860", "--no-first-run")
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			if cmd.Start() == nil {
				return
			}
		}
	}
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
}

// threadsTrimmed applies the "forget after N hours" setting, then lists what's left.
func threadsTrimmed() []ThreadInfo {
	threads.trim(time.Duration(store.get().ThreadTrimHours) * time.Hour)
	return threads.list()
}
