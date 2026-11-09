package main

// Threads VoiceLoop has heard from, and which are muted. Saved to threads.json.

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"
)

type ThreadInfo struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Muted     bool      `json:"muted"`
	LastHeard time.Time `json:"last_heard"`
}

type threadStore struct {
	mu   sync.Mutex
	path string
	m    map[string]*ThreadInfo
}

var threads = threadStore{m: map[string]*ThreadInfo{}}

const keepThreads = 300 // hard cap on remembered threads; muted ones are always kept

func (t *threadStore) load(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.path = path
	var list []*ThreadInfo
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &list)
	}
	for _, x := range list {
		if x.ID != "" {
			t.m[x.ID] = x
		}
	}
}

// heard records a reply from a thread and refreshes its title (titles change when renamed).
func (t *threadStore) heard(id, title string) {
	if id == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	x := t.m[id]
	if x == nil {
		x = &ThreadInfo{ID: id}
		t.m[id] = x
	}
	if title != "" {
		x.Title = title
	}
	x.LastHeard = time.Now()
	t.saveLocked()
}

func (t *threadStore) setMuted(id, title string, muted bool) ThreadInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	x := t.m[id]
	if x == nil {
		x = &ThreadInfo{ID: id, LastHeard: time.Now()}
		t.m[id] = x
	}
	if title != "" {
		x.Title = title
	}
	x.Muted = muted
	t.saveLocked()
	return *x
}

func (t *threadStore) isMuted(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	x := t.m[id]
	return x != nil && x.Muted
}

// list returns threads, most recently heard first.
func (t *threadStore) list() []ThreadInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sortedLocked()
}

// trim forgets unmuted threads not heard for longer than maxAge. Muted threads are
// kept, otherwise a forgotten muted thread would start speaking again.
func (t *threadStore) trim(maxAge time.Duration) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	cut := time.Now().Add(-maxAge)
	n := 0
	for id, x := range t.m {
		if !x.Muted && x.LastHeard.Before(cut) {
			delete(t.m, id)
			n++
		}
	}
	if n > 0 {
		t.saveLocked()
	}
	return n
}

func (t *threadStore) sortedLocked() []ThreadInfo {
	out := make([]ThreadInfo, 0, len(t.m))
	for _, x := range t.m {
		out = append(out, *x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastHeard.After(out[j].LastHeard) })
	return out
}

func (t *threadStore) saveLocked() {
	all := t.sortedLocked()
	var keep []ThreadInfo
	for i, x := range all {
		if i < keepThreads || x.Muted {
			keep = append(keep, x)
		} else {
			delete(t.m, x.ID)
		}
	}
	if t.path == "" {
		return
	}
	b, _ := json.MarshalIndent(keep, "", "  ")
	_ = os.WriteFile(t.path, b, 0o644)
}
