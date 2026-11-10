package main

// User settings, edited in the settings panel and saved to settings.json next to the exe.
// config.json holds the token and listen address. Updates never overwrite settings.json.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Config is config.json. VoiceLoop writes it on first run with a random token. Only listen
// and token are used after that; the other fields seed settings.json on first run.
type Config struct {
	Listen    string  `json:"listen"`     // e.g. ":8765"
	Token     string  `json:"token"`      // shared secret; the hook sends it as a bearer token
	KokoroURL string  `json:"kokoro_url"` // e.g. http://127.0.0.1:8880
	Voice     string  `json:"voice"`      // e.g. bf_emma
	Speed     float64 `json:"speed"`      // 1.0 = normal
	PreRollMs int     `json:"preroll_ms"` // silence before speech, covers the Bluetooth profile switch
	// Play question/approval chimes (without speech) even when voice mode is off.
	ChimesWhenOff bool `json:"chimes_when_off"`
}

type Settings struct {
	// Voice
	KokoroURL string  `json:"kokoro_url"`  // the speech server; the name is from when only Kokoro worked
	TTSModel  string  `json:"tts_model"`   // model name sent to the server; "kokoro" for Kokoro-FastAPI
	TTSKey    string  `json:"tts_api_key"` // optional bearer token for the server; never exported
	Voice     string  `json:"voice"`
	Speed     float64 `json:"speed"`
	CharLimit int     `json:"speech_char_limit"` // longest reply read out, in characters; 0 = no limit

	// Sound
	OutputDevice   string `json:"output_device"` // waveOut device name; "" = Windows default
	SpeechVolume   int    `json:"speech_volume"` // 0..100
	ChimeVolume    int    `json:"chime_volume"`  // 0..100
	ChimesWhenOff  bool   `json:"chimes_when_off"`
	PreRollMs      int    `json:"preroll_ms"`
	MuteWhenLocked bool   `json:"mute_when_locked"` // silent while Windows is locked

	// Threads
	ThreadMode      string `json:"thread_mode"`       // "current": only the thread last typed into; "all": every unmuted thread
	ThreadTrimHours int    `json:"thread_trim_hours"` // forget unmuted threads not heard for this long (1..72)

	// Pill
	ShowThreadName bool   `json:"show_thread_name"`
	FitThreadName  bool   `json:"fit_thread_name"` // grow the pill to show the whole name
	MaxTitle       int    `json:"max_title"`       // longest name in px (100% scale) when not fitting
	SpeechColour   string `json:"speech_colour"`
	QuestionColour string `json:"question_colour"`
	ApprovalColour string `json:"approval_colour"`
	PillColour     string `json:"pill_colour"`
	PillOpacity    int    `json:"pill_opacity"` // 30..100
	PillSize       int    `json:"pill_size"`    // percent, 75..150
	Monitor        string `json:"monitor"`      // e.g. \\.\DISPLAY2; "" = primary
	Anchor         string `json:"anchor"`       // top-left, top-centre, ..., bottom-right
	OffsetX        int    `json:"offset_x"`     // px at 100% scaling, from the anchored edge
	OffsetY        int    `json:"offset_y"`

	// Macropad key map from VoiceLoop 0.5.x. Moved into pad-profile.xml on first
	// start of 0.6, then cleared.
	Pad *PadMap `json:"pad,omitempty"`
}

// server is where this machine's speech comes from.
func (s Settings) server() ttsServer {
	return ttsServer{URL: s.KokoroURL, Model: s.TTSModel, Key: s.TTSKey}
}

func defaultSettings() Settings {
	return Settings{
		TTSModel: "kokoro", Voice: "bf_emma", Speed: 1.05, CharLimit: 600,
		SpeechVolume: 100, ChimeVolume: 70, PreRollMs: 400, MuteWhenLocked: true,
		SpeechColour: "#4fd1c5", QuestionColour: "#f6b24e", ApprovalColour: "#ff6b6b",
		PillColour: "#2a2a2c", PillOpacity: 95, PillSize: 100,
		Anchor: "bottom-centre", OffsetY: 118,
		ThreadMode: "current", ThreadTrimHours: 24, ShowThreadName: true, MaxTitle: 260,
	}
}

var anchors = []string{"top-left", "top-centre", "top-right", "bottom-left", "bottom-centre", "bottom-right"}

// normalise clamps values into range and fills anything invalid with defaults.
func (s *Settings) normalise() {
	d := defaultSettings()
	clampI := func(v *int, lo, hi int) {
		if *v < lo {
			*v = lo
		}
		if *v > hi {
			*v = hi
		}
	}
	clampI(&s.SpeechVolume, 0, 100)
	clampI(&s.ChimeVolume, 0, 100)
	clampI(&s.PreRollMs, 0, 2000)
	clampI(&s.PillOpacity, 30, 100)
	clampI(&s.PillSize, 75, 150)
	clampI(&s.OffsetX, -2000, 2000)
	clampI(&s.OffsetY, -2000, 2000)
	clampI(&s.MaxTitle, 100, 900)
	clampI(&s.CharLimit, 0, 50000) // 0 is allowed: no limit
	if s.ThreadTrimHours == 0 {
		s.ThreadTrimHours = 24
	}
	clampI(&s.ThreadTrimHours, 1, 72)
	if s.ThreadMode != "all" {
		s.ThreadMode = "current"
	}
	if s.Speed < 0.5 || s.Speed > 2 {
		s.Speed = d.Speed
	}
	if s.Voice == "" {
		s.Voice = d.Voice
	}
	s.KokoroURL = strings.TrimRight(strings.TrimSpace(s.KokoroURL), "/")
	if s.TTSModel = strings.TrimSpace(s.TTSModel); s.TTSModel == "" {
		s.TTSModel = "kokoro"
	}
	s.TTSKey = strings.TrimSpace(s.TTSKey)
	for _, c := range []*string{&s.SpeechColour, &s.QuestionColour, &s.ApprovalColour, &s.PillColour} {
		if _, err := parseHex(*c); err != nil {
			*c = ""
		}
	}
	if s.SpeechColour == "" {
		s.SpeechColour = d.SpeechColour
	}
	if s.QuestionColour == "" {
		s.QuestionColour = d.QuestionColour
	}
	if s.ApprovalColour == "" {
		s.ApprovalColour = d.ApprovalColour
	}
	if s.PillColour == "" {
		s.PillColour = d.PillColour
	}
	ok := false
	for _, a := range anchors {
		ok = ok || a == s.Anchor
	}
	if !ok {
		s.Anchor = d.Anchor
	}
}

func parseHex(h string) (color.NRGBA, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "#")
	if len(h) != 6 {
		return color.NRGBA{}, fmt.Errorf("bad colour %q", h)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.NRGBA{}, err
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}, nil
}

// Palette is what the renderer needs from the settings.
type Palette struct {
	Speech, Question, Approval, Background color.NRGBA
}

func (s Settings) palette() Palette {
	p := Palette{}
	p.Speech, _ = parseHex(s.SpeechColour)
	p.Question, _ = parseHex(s.QuestionColour)
	p.Approval, _ = parseHex(s.ApprovalColour)
	p.Background, _ = parseHex(s.PillColour)
	p.Background.A = uint8(255 * s.PillOpacity / 100)
	return p
}

// ---------- store ----------

type settingsStore struct {
	mu   sync.Mutex
	path string
	s    Settings
}

var store settingsStore

// loadSettings reads settings.json; legacy fields from config.json seed a first run.
func (st *settingsStore) load(path string, legacy Config) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.path = path
	st.s = defaultSettings()
	if legacy.KokoroURL != "" {
		st.s.KokoroURL = legacy.KokoroURL
	}
	if legacy.Voice != "" {
		st.s.Voice = legacy.Voice
	}
	if legacy.Speed != 0 {
		st.s.Speed = legacy.Speed
	}
	if legacy.PreRollMs != 0 {
		st.s.PreRollMs = legacy.PreRollMs
	}
	st.s.ChimesWhenOff = legacy.ChimesWhenOff
	if b, err := os.ReadFile(path); err == nil {
		// Notepad and PowerShell 5 save UTF-8 with a byte-order mark, which encoding/json rejects.
		b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
		if err := json.Unmarshal(b, &st.s); err != nil {
			log.Printf("settings.json unreadable, using defaults: %v", err)
		}
	}
	st.s.normalise()
}

func (st *settingsStore) get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s
}

// replace swaps in a whole set of settings (settings import) and saves them.
func (st *settingsStore) replace(next Settings) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	next.normalise()
	b, _ := json.MarshalIndent(next, "", "  ")
	if st.path != "" {
		if err := os.WriteFile(st.path, b, 0o644); err != nil {
			return err
		}
	}
	st.s = next
	return nil
}

// update merges a partial JSON object into the settings, normalises and saves them.
func (st *settingsStore) update(patch []byte) (Settings, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	next := st.s
	if err := json.Unmarshal(patch, &next); err != nil {
		return st.s, err
	}
	next.normalise()
	b, _ := json.MarshalIndent(next, "", "  ")
	if st.path != "" {
		if err := os.WriteFile(st.path, b, 0o644); err != nil {
			return st.s, err
		}
	}
	st.s = next
	return next, nil
}

// placePill returns the pill's top-left corner inside a monitor's work area.
// Offsets are in 100%-scale pixels, measured inwards from the anchored edges
// (OffsetX shifts right of centre for the centre anchors).
func placePill(left, top, right, bottom, w, h int32, anchor string, offX, offY int, scale float64) (int32, int32) {
	ox, oy := int32(float64(offX)*scale), int32(float64(offY)*scale)
	var x, y int32
	switch {
	case strings.HasSuffix(anchor, "left"):
		x = left + ox
	case strings.HasSuffix(anchor, "right"):
		x = right - w - ox
	default:
		x = left + (right-left-w)/2 + ox
	}
	if strings.HasPrefix(anchor, "top") {
		y = top + oy
	} else {
		y = bottom - h - oy
	}
	// Keep it on screen whatever the offsets say.
	x = max(left, min(x, right-w))
	y = max(top, min(y, bottom-h))
	return x, y
}
