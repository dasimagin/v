package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Settings is the user config in ~/.v/settings.json.
type Settings struct {
	Theme    string `json:"theme"`
	UITheme  string `json:"ui_theme"`  // the graphical UI: a theme ID, or "light"
	UIDetail string `json:"ui_detail"` // how much of the agent's work the chat shows: answer, edit, commands or thinking
	Model    string `json:"model"`     // Claude Code --model
	Effort   string `json:"effort"`    // Claude Code --effort

	Mode       string `json:"mode"`                // "manual": the agents ask about every step; "auto": they review, apply, commit, push and reply on their own
	Iterations int    `json:"converge_iterations"` // the most rounds of review and fix that /converge runs
	Background string `json:"background"`          // "theme" (default), "terminal" (leave it to the terminal) or "sync" (the theme's, also set on the terminal)
	Animate    bool   `json:"animate"`             // the brand shimmers
	PRComments string `json:"pr_comments"`         // "pending" (default): comments written on the PR screen wait in your pending review until you submit it; "immediate": they are published at once

	DiffBefore int `json:"diff_before"` // lines of context before each change, in every diff
	DiffAfter  int `json:"diff_after"`  // and after it

}

func defaultSettings() Settings {
	return Settings{Theme: defaultTheme, Model: "claude-opus-5-5", Effort: "high", DiffBefore: 3, DiffAfter: 3, Mode: reviewAuto, PRComments: "pending", Animate: true, Iterations: 5, Background: "theme", UITheme: defaultTheme, UIDetail: "commands"}
}

// models are the choices for the agent's model, best first.
var models = []struct{ ID, Name string }{
	{"claude-fable-5-1", "Fable 5.1"},
	{"claude-opus-5-5", "Opus 5.5"},
	{"claude-sonnet-5-5", "Sonnet 5.5"},
	{"claude-haiku-5-5", "Haiku 5.5"},
}

var efforts = []string{"low", "medium", "high", "xhigh", "max"}

// modelName is the display name of a model ID; unknown IDs show as is.
func modelName(id string) string {
	for _, m := range models {
		if m.ID == id {
			return m.Name
		}
	}
	return id
}

// configDir is ~/.v: settings.json and the user's themes/ live there.
func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".v")
}

func settingsPath() string { return filepath.Join(configDir(), "settings.json") }

// loadSettings reads the config, writing the defaults on first run. On a
// broken file it returns the defaults and the error, and leaves the file alone.
func loadSettings() (Settings, error) {
	s := defaultSettings()
	b, err := os.ReadFile(settingsPath())
	if errors.Is(err, fs.ErrNotExist) {
		return s, s.save()
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return defaultSettings(), err
	}
	// Before there was one mode, review and apply had one each: manual in either stays manual.
	var old struct {
		Mode       string `json:"mode"`
		ReviewMode string `json:"review_mode"`
		ApplyMode  string `json:"apply_mode"`
	}
	if json.Unmarshal(b, &old) == nil && old.Mode == "" && (old.ReviewMode == reviewManual || old.ApplyMode == reviewManual) {
		s.Mode = reviewManual
	}
	return s, nil
}

// save writes the config atomically.
func (s Settings) save() error {
	path := settingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
