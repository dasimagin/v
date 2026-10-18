package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// historyMax is how many lines ~/.v/history keeps.
const historyMax = 1000

// history is what the user typed in the omnibox, oldest first, and the
// state of browsing it with ↑↓. Browsing with text typed only shows
// entries that start with it.
type history struct {
	items  []string
	pos    int    // index being shown, -1 when not browsing
	draft  string // what was typed before browsing started
	prefix string
}

func historyPath() string { return filepath.Join(configDir(), "history") }

// loadHistory reads ~/.v/history: one JSON string per line.
func loadHistory() history {
	h := history{pos: -1}
	f, err := os.Open(historyPath())
	if err != nil {
		return h
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var s string
		if json.Unmarshal(sc.Bytes(), &s) == nil && s != "" {
			h.items = append(h.items, s)
		}
	}
	if len(h.items) > historyMax {
		h.items = h.items[len(h.items)-historyMax:]
	}
	return h
}

// add records a submitted line in memory and in the file.
func (h *history) add(s string) {
	h.pos = -1
	s = strings.TrimSpace(s)
	if s == "" || (len(h.items) > 0 && h.items[len(h.items)-1] == s) {
		return
	}
	h.items = append(h.items, s)
	b, _ := json.Marshal(s)
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(historyPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

// older returns the previous entry matching the prefix, starting from the
// current input; ok is false when there is none.
func (h *history) older(current string) (string, bool) {
	if h.pos < 0 {
		h.draft, h.prefix, h.pos = current, current, len(h.items)
	}
	for i := h.pos - 1; i >= 0; i-- {
		if s := h.items[i]; strings.HasPrefix(s, h.prefix) && s != current {
			h.pos = i
			return s, true
		}
	}
	return "", false
}

// newer returns the next entry matching the prefix, or the draft after
// the newest one.
func (h *history) newer() (string, bool) {
	if h.pos < 0 {
		return "", false
	}
	for i := h.pos + 1; i < len(h.items); i++ {
		if strings.HasPrefix(h.items[i], h.prefix) && h.items[i] != h.items[h.pos] {
			h.pos = i
			return h.items[i], true
		}
	}
	h.pos = -1
	return h.draft, true
}

// historyKey handles ↑↓ in the omnibox. It reports whether the key was used:
// in file screens an empty omnibox leaves ↑↓ to scrolling.
func (m *model) historyKey(k string) bool {
	if k != "up" && k != "down" || m.menu != nil || m.perm != nil {
		return false
	}
	browsing := m.hist.pos >= 0
	if !browsing && m.omni.Value() == "" && m.mode != modeChat && m.mode != modeSplash {
		return false
	}
	var s string
	ok := false
	if k == "up" {
		s, ok = m.hist.older(m.omni.Value())
	} else {
		s, ok = m.hist.newer()
	}
	if ok {
		m.setOmni(s)
	}
	return browsing || ok || k == "up"
}
