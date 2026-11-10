//go:build windows

// VoiceLoop listener: receives Claude Code replies from the hook, turns
// them into speech with Kokoro (or any OpenAI-style speech server), and plays them. Tray icon shows voice mode on or off.
package main

import (
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
	"voiceloop/listener/logo"
)

// job is one thing to say. Kind is "speech", "question" or "approval".
type job struct {
	Kind, Text string
	Thread     string // T3 thread id (or Claude session id when not from T3)
	Title      string // thread name for the pill
	Active     bool   // the thread the user last typed into
	Force      bool   // the user asked for this (/resend): play whatever the mode, mute or voice setting
	gen        int64
}

var (
	cfg     Config
	baseDir string

	mu      sync.Mutex
	voiceOn bool
	queue   = make(chan job, 8)
	prio    = make(chan job, 4) // questions and approvals jump the queue in "all threads" mode
	gen     atomic.Int64        // bumped by every stop; jobs from an older generation are dropped

	mToggle *systray.MenuItem

	winmm         = windows.NewLazySystemDLL("winmm.dll")
	user32        = windows.NewLazySystemDLL("user32.dll")
	procRegHotKey = user32.NewProc("RegisterHotKey")
	procGetMsg    = user32.NewProc("GetMessageW")
)

const (
	modAlt      = 0x0001
	modCtrl     = 0x0002
	modNoRepeat = 0x4000
	wmHotkey    = 0x0312
	hkToggle    = 1
	hkStop      = 2
)

func main() {
	if !claimSingleInstance() {
		return // the running copy opens Settings instead
	}
	enableDPIAwareness()
	exe, _ := os.Executable()
	baseDir = filepath.Dir(exe)

	lf, err := os.OpenFile(filepath.Join(baseDir, "voiceloop.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		log.SetOutput(lf)
	}
	if err := loadConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	store.load(filepath.Join(baseDir, "settings.json"), cfg)
	threads.load(filepath.Join(baseDir, "threads.json"))
	if err := installIcons(iconsDir()); err != nil {
		log.Printf("icons: %v", err)
	}
	if migrated, err := pads.load(baseDir, store.get().Pad); err != nil {
		log.Printf("macropad: %v", err)
	} else if migrated {
		_, _ = store.update([]byte(`{"pad":null}`))
		log.Printf("macropad: key map moved from settings.json into pad-profile.xml")
	}
	voiceOn = loadState()
	log.Printf("starting, voice mode %v, listening on %s", voiceOn, cfg.Listen)

	go httpServer()
	go settingsServer()
	if !launchedInBackground() {
		go func() { time.Sleep(700 * time.Millisecond); openSettings() }()
	}
	go speaker()
	go hotkeys()
	systray.Run(onReady, func() {})
}

func loadConfig() error {
	c, created, err := ensureConfig(baseDir)
	if err != nil {
		return err
	}
	cfg = c
	if created {
		log.Printf("first run: wrote config.json with a new token (Settings, About shows it)")
	}
	return nil
}

func statePath() string { return filepath.Join(baseDir, "state.json") }

// loadState is the saved voice mode. A first run (no state.json yet) starts with it on,
// so a new install speaks without anyone having to find Ctrl+Alt+V.
func loadState() bool {
	s := struct{ VoiceOn bool }{VoiceOn: true}
	if b, err := os.ReadFile(statePath()); err == nil {
		s.VoiceOn = false
		_ = json.Unmarshal(b, &s)
	}
	return s.VoiceOn
}

func setVoice(on bool) {
	mu.Lock()
	voiceOn = on
	mu.Unlock()
	b, _ := json.Marshal(struct{ VoiceOn bool }{on})
	_ = os.WriteFile(statePath(), b, 0o644)
	if !on {
		stopPlayback()
	}
	refreshTray()
	if on {
		beep("on")
	} else {
		beep("off")
	}
	log.Printf("voice mode %v", on)
}

func isOn() bool { mu.Lock(); defer mu.Unlock(); return voiceOn }

// ---------- tray ----------

func onReady() {
	systray.SetTitle("VoiceLoop")
	mToggle = systray.AddMenuItem("", "")
	mStop := systray.AddMenuItem("Stop speaking (Ctrl+Alt+X)", "")
	mSettings := systray.AddMenuItem("Settings…", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "")
	refreshTray()
	// Double-click the tray icon to open Settings; right-click still shows the menu.
	var lastTap time.Time
	var tapMu sync.Mutex
	systray.SetOnTapped(func() {
		tapMu.Lock()
		double := time.Since(lastTap) < 500*time.Millisecond
		lastTap = time.Now()
		if double {
			lastTap = time.Time{}
		}
		tapMu.Unlock()
		if double {
			openSettings()
		}
	})
	go func() {
		for {
			select {
			case <-mToggle.ClickedCh:
				setVoice(!isOn())
			case <-mStop.ClickedCh:
				stopPlayback()
			case <-mSettings.ClickedCh:
				openSettings()
			case <-mQuit.ClickedCh:
				stopPlayback()
				systray.Quit()
				os.Exit(0)
			}
		}
	}()
}

func refreshTray() {
	if mToggle == nil {
		return
	}
	if isOn() {
		systray.SetIcon(logo.ICO(logo.Teal, 16, 20, 24, 32, 48))
		systray.SetTooltip("VoiceLoop: voice mode ON")
		mToggle.SetTitle("Turn voice mode off (Ctrl+Alt+V)")
	} else {
		systray.SetIcon(logo.ICO(logo.Grey, 16, 20, 24, 32, 48))
		systray.SetTooltip("VoiceLoop: voice mode off")
		mToggle.SetTitle("Turn voice mode on (Ctrl+Alt+V)")
	}
}

// ---------- hotkeys ----------

type msg struct {
	hwnd    uintptr
	message uint32
	wparam  uintptr
	lparam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

func hotkeys() {
	runtime.LockOSThread()
	if r, _, err := procRegHotKey.Call(0, hkToggle, modCtrl|modAlt|modNoRepeat, 'V'); r == 0 {
		log.Printf("hotkey Ctrl+Alt+V unavailable: %v", err)
	}
	if r, _, err := procRegHotKey.Call(0, hkStop, modCtrl|modAlt|modNoRepeat, 'X'); r == 0 {
		log.Printf("hotkey Ctrl+Alt+X unavailable: %v", err)
	}
	overlayInit()
	messageLoop(func(id uintptr) {
		switch id {
		case hkToggle:
			go setVoice(!isOn())
		case hkStop:
			go stopPlayback()
		}
	})
}

// ---------- HTTP ----------

func authed(r *http.Request) bool {
	want := "Bearer " + cfg.Token
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(want)) == 1
}

func httpServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/speak", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !authed(r) {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		var body job
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		switch body.Kind {
		case "", "speech":
			body.Kind = "speech"
		case "question", "approval":
		default:
			http.Error(w, "bad kind", http.StatusBadRequest)
			return
		}
		if body.Kind == "speech" && body.Text == "" {
			http.Error(w, "no text", http.StatusBadRequest)
			return
		}
		st := store.get()
		threads.heard(body.Thread, body.Title)
		if lockedMute() {
			// The PC is locked and the user has walked away: say nothing, and don't save it for later.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Is this thread one we speak? Muted threads never are; in "current" mode only
		// the thread the user last typed into is. Questions and approvals from other threads
		// still chime, without speech, since those threads are waiting on the user.
		wanted := body.Force || !threads.isMuted(body.Thread) && (st.ThreadMode == "all" || body.Active || body.Thread == "")
		if !wanted {
			if body.Kind == "speech" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			body.Text = ""
		}
		if !isOn() && !body.Force {
			if body.Kind == "speech" || !st.ChimesWhenOff {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			body.Text = "" // voice off: chime only
		}
		body.Text = limitText(body.Text, st.CharLimit)
		if st.ThreadMode == "current" || body.Force {
			// One thread at a time: the newest thing replaces anything queued or playing.
			drain()
			stopPlayback()
		}
		body.gen = gen.Load()
		ch := queue
		if body.Kind != "speech" && st.ThreadMode == "all" {
			ch = prio
		}
		select {
		case ch <- body:
		default:
			log.Printf("queue full, dropped a %s from %q", body.Kind, body.Title)
		}
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/mute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !authed(r) {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		var b struct {
			Thread, Title string
			Muted         bool
		}
		if json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&b) != nil || b.Thread == "" {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		t := threads.setMuted(b.Thread, b.Title, b.Muted)
		log.Printf("thread %q muted=%v (from /mute)", t.Title, t.Muted)
		json.NewEncoder(w).Encode(t)
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost {
			var body struct{ VoiceOn bool }
			if json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body) == nil {
				setVoice(body.VoiceOn)
			}
		}
		json.NewEncoder(w).Encode(map[string]bool{"voice_on": isOn()})
	})
	log.Fatal(http.ListenAndServe(cfg.Listen, mux))
}

func drain() {
	for {
		select {
		case <-queue:
		case <-prio:
		default:
			return
		}
	}
}

// nextJob takes a question or approval first, then replies in arrival order.
func nextJob() job {
	select {
	case j := <-prio:
		return j
	default:
	}
	select {
	case j := <-prio:
		return j
	case j := <-queue:
		return j
	}
}

// muteShowing is the pill's "Mute thread" button: mute whatever is speaking,
// show "Muted" for a moment, then stop.
func muteShowing() {
	thread, title := overlayMarkMuted()
	if thread == "" {
		stopPlayback()
		return
	}
	threads.setMuted(thread, title, true)
	log.Printf("thread %q muted from the pill", title)
	g := gen.Load()
	time.Sleep(900 * time.Millisecond)
	if gen.Load() == g {
		stopPlayback()
	}
}

// ---------- speech ----------

func speaker() {
	for {
		j := nextJob()
		if j.gen != gen.Load() || lockedMute() || (!j.Force && j.Thread != "" && threads.isMuted(j.Thread) && j.Kind == "speech") {
			continue
		}
		overlayPending(j)
		if j.Kind != "speech" {
			pcm := chimePCM(j.Kind)
			play(wavFromPCM(pcm, toneRate), j, store.get().PreRollMs, true)
		}
		if j.Text != "" && j.gen == gen.Load() {
			if j.Kind != "speech" {
				overlayPending(j)
			}
			speakChunks(j)
		}
		if j.gen == gen.Load() && len(queue) == 0 && len(prio) == 0 {
			overlayHide()
		}
	}
}

// speakChunks synthesises the text a chunk at a time and plays each chunk while
// the next one is being made, so long replies start talking within a second or two.
func speakChunks(j job) {
	chunks := splitChunks(j.Text, 140, 200)
	ready := make(chan []byte, 1)
	go func() {
		defer close(ready)
		for i, c := range chunks {
			if j.gen != gen.Load() {
				return
			}
			start := time.Now()
			wav, err := synth(c)
			// The speech server sometimes restarts mid-reply (for example when it hits a container memory cap) and
			// takes 10-20 s to come back, so keep trying for a while.
			for _, wait := range []time.Duration{4 * time.Second, 7 * time.Second, 10 * time.Second} {
				if err == nil || j.gen != gen.Load() {
					break
				}
				log.Printf("tts chunk %d: %v (retrying in %v)", i+1, err, wait)
				time.Sleep(wait)
				wav, err = synth(c)
			}
			if err != nil {
				log.Printf("tts chunk %d: %v", i+1, err)
				return
			}
			log.Printf("tts %s chunk %d/%d, %d chars in %v", j.Kind, i+1, len(chunks), len(c), time.Since(start).Round(time.Millisecond))
			select {
			case ready <- wav:
			case <-time.After(5 * time.Minute):
				return
			}
		}
	}()
	first := true
	for wav := range ready {
		if j.gen != gen.Load() {
			// Drain so the synth goroutine can finish.
			continue
		}
		preroll := 0
		if first {
			preroll = store.get().PreRollMs // only the first chunk needs the Bluetooth wake-up silence
			first = false
		}
		play(wav, j, preroll, false)
	}
}

func synth(text string) ([]byte, error) {
	st := store.get()
	return synthWith(st.server(), st.Voice, st.Speed, text)
}

// play blocks until the audio ends or is stopped, showing the overlay with kind's colour.
func play(raw []byte, j job, prerollMs int, chime bool) {
	g := j.gen
	wav, _, err := fixWav(raw, prerollMs)
	if err != nil {
		log.Printf("wav: %v", err)
		return
	}
	if g != gen.Load() {
		return
	}
	rate := int(binary.LittleEndian.Uint32(wav[24:28]))
	pcm := wav[44:]
	overlayPlaying(j, envelope(pcm, rate), prerollMs)
	st := store.get()
	vol := st.SpeechVolume
	if chime {
		vol = st.ChimeVolume
	}
	if err := playPCM(pcm, rate, vol, st.OutputDevice); err != nil {
		log.Printf("audio: %v", err)
	}
}

// stopPlayback silences audio, drops queued and in-flight jobs, and hides the overlay.
func stopPlayback() {
	gen.Add(1)
	stopAudio()
	overlayHide()
}

// beep plays a short chime without the overlay (voice mode on/off feedback, chime tests).
func beep(kind string) {
	st := store.get()
	pcm := chimePCM(kind)
	silence := make([]byte, toneRate*2*st.PreRollMs/1000&^1)
	go playPCM(append(silence, pcm...), toneRate, st.ChimeVolume, st.OutputDevice)
}
