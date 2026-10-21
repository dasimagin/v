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
	Theme  string `json:"theme"`
	Model  string `json:"model"`  // Claude Code --model
	Effort string `json:"effort"` // Claude Code --effort

	DiffBefore int `json:"diff_before"` // lines of context before each change, in every diff
	DiffAfter  int `json:"diff_after"`  // and after it

	Agent       string `json:"agent"`        // "claude" or "codex"
	CodexModel  string `json:"codex_model"`  // "" for Codex's default
	CodexEffort string `json:"codex_effort"` // "" for the model's default
}

func defaultSettings() Settings {
	return Settings{Theme: defaultTheme, Model: "claude-opus-5-5", Effort: "high", Agent: "claude", DiffBefore: 3, DiffAfter: 3}
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
