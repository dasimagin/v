package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func configModel(t *testing.T) *model {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // settings are saved there
	m := newModel()
	m.settings = defaultSettings()
	m.w, m.h = 120, 30
	m.cmdConfig("")
	return &m
}

func optionIndex(label string) int {
	for i, o := range options {
		if strings.HasPrefix(o.label, label) {
			return i
		}
	}
	return -1
}

func TestConfigFilterIsFuzzy(t *testing.T) {
	m := configModel(t)
	if got := m.configVisible(); len(got) != len(options) {
		t.Fatalf("no filter: %d of %d", len(got), len(options))
	}
	m.omni.SetValue("bckgrnd") // letters in order, not adjacent
	got := m.configVisible()
	if len(got) == 0 || options[got[0]].label != "Background" {
		t.Errorf("a fuzzy query must find Background first: %v", got)
	}
	m.omni.SetValue("diff")
	var labels []string
	for _, i := range m.configVisible() {
		labels = append(labels, options[i].label)
	}
	if len(labels) < 2 || !strings.HasPrefix(labels[0], "Diff context") {
		t.Errorf("diff: %v", labels)
	}
	m.omni.SetValue("zzzzqq")
	if len(m.configVisible()) != 0 {
		t.Error("nothing should match gibberish")
	}
	lines := m.renderConfig(20)
	if !strings.Contains(ansi.Strip(strings.Join(lines, "\n")), "Nothing matches") {
		t.Error("an empty result says so")
	}
	// Changing the filter starts at the top again, with the dropdown closed.
	m.omni.SetValue("")
	m.syncConfig()
	m.cfg.row, m.cfg.open = 5, true
	m.omni.SetValue("a")
	m.syncConfig()
	if m.cfg.row != 0 || m.cfg.open {
		t.Errorf("row=%d open=%v", m.cfg.row, m.cfg.open)
	}
}

func TestConfigScrollsToTheFocusedOption(t *testing.T) {
	m := configModel(t)
	if len(options) < 8 {
		t.Skip("too few options to scroll")
	}
	const rows = 12 // fits only a few of the options
	first := ansi.Strip(strings.Join(m.renderConfig(rows), "\n"))
	if !strings.Contains(first, options[0].label) || strings.Contains(first, options[len(options)-1].label) {
		t.Fatalf("the top of the list:\n%s", first)
	}
	m.cfg.row = len(options) - 1
	last := ansi.Strip(strings.Join(m.renderConfig(rows), "\n"))
	if !strings.Contains(last, options[len(options)-1].label) || strings.Contains(last, options[0].label) {
		t.Errorf("the last option must be reachable:\n%s", last)
	}
	if m.cfg.top == 0 {
		t.Error("the list must have scrolled")
	}
	// Opening a dropdown on it keeps all its entries in view too.
	m.cfg.open, m.cfg.pick = true, 0
	open := ansi.Strip(strings.Join(m.renderConfig(rows), "\n"))
	o := options[len(options)-1]
	for _, v := range o.values(m) {
		if !strings.Contains(open, v) {
			t.Errorf("entry %q of the open dropdown is off screen:\n%s", v, open)
		}
	}
	if got := len(m.renderConfig(rows)); got != rows {
		t.Errorf("%d rows, want %d", got, rows)
	}
}

func TestConfigRowsAndDropdownsAreClickable(t *testing.T) {
	m := configModel(t)
	idx := optionIndex("Animations")
	m.cfg.row = idx
	m.renderConfig(m.bodyRows()) // positions the list
	lines := m.configLines()
	at := func(kind string, opt int) (int, cfgLine) {
		for i, l := range lines {
			if l.kind == kind && l.opt == opt {
				return i, l
			}
		}
		t.Fatalf("no %s line for option %d", kind, opt)
		return 0, cfgLine{}
	}
	screenY := func(i int) int { return headerRows + i - m.cfg.top }

	// A click on another row's label only takes the focus.
	other := optionIndex("Color theme")
	i, l := at("value", other)
	m.configClick(l.x0-3, screenY(i))
	if m.cfg.row != other || m.cfg.open {
		t.Errorf("row click: row=%d open=%v", m.cfg.row, m.cfg.open)
	}
	// A click on the value button opens its dropdown.
	m.cfg.row = idx
	m.renderConfig(m.bodyRows())
	lines = m.configLines()
	i, l = at("value", idx)
	m.configClick(l.x0+1, screenY(i))
	if !m.cfg.open || m.cfg.row != idx {
		t.Fatalf("the value button must open the dropdown: open=%v row=%d", m.cfg.open, m.cfg.row)
	}
	// A click on an entry picks it and saves.
	m.renderConfig(m.bodyRows())
	lines = m.configLines()
	var off cfgLine
	var offAt int
	for j, ln := range lines {
		if ln.kind == "item" && ln.item == 1 {
			off, offAt = ln, j
		}
	}
	if off.kind != "item" {
		t.Fatal("no dropdown entries")
	}
	m.configClick(off.x0+2, screenY(offAt))
	if m.cfg.open || m.settings.Animate {
		t.Errorf("picking Off: open=%v animate=%v", m.cfg.open, m.settings.Animate)
	}
	// A click elsewhere closes an open dropdown and keeps the old value.
	m.settings.Animate = true
	m.cfg.row = idx
	m.configKey("enter")
	m.renderConfig(m.bodyRows())
	lines = m.configLines()
	ib, _ := at("blank", optionIndex("Color theme"))
	m.configClick(2, screenY(ib))
	if m.cfg.open || !m.settings.Animate {
		t.Errorf("a click elsewhere: open=%v animate=%v", m.cfg.open, m.settings.Animate)
	}
}

func TestConfigKeysWithAFilter(t *testing.T) {
	m := configModel(t)
	m.omni.SetValue("anim")
	m.syncConfig()
	m.renderConfig(m.bodyRows())
	// Enter opens the matched option even though the omnibox holds text.
	next, _ := m.key(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	if !nm.cfg.open || options[nm.configVisible()[nm.cfg.row]].label != "Animations" {
		t.Fatalf("enter with a filter: open=%v", nm.cfg.open)
	}
	// With the dropdown open, typing does not change the filter, and Esc closes it.
	next, _ = nm.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	nm = next.(model)
	if nm.omni.Value() != "anim" {
		t.Errorf("typing while a dropdown is open must be ignored: %q", nm.omni.Value())
	}
	next, _ = nm.key(tea.KeyMsg{Type: tea.KeyEsc})
	nm = next.(model)
	if nm.cfg.open || nm.mode != modeConfig {
		t.Fatal("esc closes the dropdown, nothing more")
	}
	// Esc then clears the filter; one more leaves the screen.
	next, _ = nm.key(tea.KeyMsg{Type: tea.KeyEsc})
	nm = next.(model)
	if nm.omni.Value() != "" || nm.mode != modeConfig {
		t.Errorf("esc clears the filter first: %q mode=%v", nm.omni.Value(), nm.mode)
	}
	next, _ = nm.key(tea.KeyMsg{Type: tea.KeyEsc})
	if next.(model).mode == modeConfig {
		t.Error("esc with no filter and no dropdown leaves the settings")
	}
}

func TestLongDescriptionsWrapInsteadOfRunningOffScreen(t *testing.T) {
	m := configModel(t)
	m.w = 90
	for _, l := range m.configLines() {
		if w := ansi.StringWidth(l.text); w > m.w {
			t.Fatalf("a %s line is %d wide on a %d-wide screen: %q", l.kind, w, m.w, ansi.Strip(l.text))
		}
	}
	// The long Background description is still all there, over a few lines.
	var text []string
	for _, l := range m.configLines() {
		if l.opt == optionIndex("Background") && l.kind == "desc" {
			text = append(text, strings.TrimSpace(ansi.Strip(l.text)))
		}
	}
	if len(text) < 2 || !strings.Contains(strings.Join(text, " "), "restores it on exit") {
		t.Errorf("description = %q", text)
	}
}

func TestOldCodexSettingsAreIgnored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".v"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := `{"theme":"one-light","model":"claude-sonnet-5-5","effort":"low","agent":"codex","codex_model":"gpt-x","codex_effort":"high"}`
	if err := os.WriteFile(filepath.Join(home, ".v", "settings.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := loadSettings()
	if err != nil {
		t.Fatalf("an old settings file must still load: %v", err)
	}
	if s.Model != "claude-sonnet-5-5" || s.Effort != "low" || s.Iterations != 5 {
		t.Errorf("settings = %+v", s)
	}
	// The saved theme is light and gone: the startup falls back to the default one.
	if findTheme(s.Theme) != nil {
		t.Errorf("theme %q must be gone", s.Theme)
	}
	for _, o := range options {
		if o.label == "Agent" {
			t.Error("there is no Agent option any more")
		}
	}
	m := newModel()
	m.settings = s
	if got := m.agentLabel(); got != "Sonnet 5.5 · low" {
		t.Errorf("label = %q", got)
	}
}

func TestOldReviewAndApplyModesBecomeOne(t *testing.T) {
	for _, c := range []struct{ file, want string }{
		{`{"review_mode":"auto","apply_mode":"manual"}`, reviewManual},
		{`{"review_mode":"auto","apply_mode":"auto"}`, reviewAuto},
		{`{"mode":"auto","apply_mode":"manual"}`, reviewAuto},
		{`{}`, reviewAuto},
	} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if err := os.MkdirAll(filepath.Join(home, ".v"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".v", "settings.json"), []byte(c.file), 0o644); err != nil {
			t.Fatal(err)
		}
		if s, _ := loadSettings(); s.Mode != c.want {
			t.Errorf("%s: mode %q, want %q", c.file, s.Mode, c.want)
		}
	}
}
