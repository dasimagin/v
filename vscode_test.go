package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinThemes(t *testing.T) {
	if err := loadThemes(""); err != nil {
		t.Fatal(err)
	}
	// Expected values were read from the VS Code extensions' theme files.
	tests := []struct {
		id   string
		want Theme
	}{
		{"one-dark-pro-darker", Theme{
			Name: "One Dark Pro Darker", Bg: "#23272e", Panel: "#1e2227", Sel: "#2c313a", Fg: "#abb2bf",
			Muted: "#7f848e", Gutter: "#495162", Accent: "#61afef", Accent2: "#c678dd", Error: "#e06c75",
			Keyword: "#c678dd", Func: "#61afef", String: "#98c379", Number: "#d19a66", Type: "#e5c07b",
			Comment: "#7f848e", Variable: "#e06c75", Builtin: "#56b6c2",
		}},
		{"one-dark-pro", Theme{
			Name: "One Dark Pro", Bg: "#282c34", Panel: "#21252b", Fg: "#abb2bf", Comment: "#7f848e", CommentItalic: true,
		}},
	}
	for _, tt := range tests {
		got := findTheme(tt.id)
		if got == nil {
			t.Fatalf("theme %s not found", tt.id)
		}
		check := func(field, got, want string) {
			if want != "" && got != want {
				t.Errorf("%s %s = %s, want %s", tt.id, field, got, want)
			}
		}
		w := tt.want
		check("Name", got.Name, w.Name)
		check("Bg", got.Bg, w.Bg)
		check("Panel", got.Panel, w.Panel)
		check("Sel", got.Sel, w.Sel)
		check("Fg", got.Fg, w.Fg)
		check("Muted", got.Muted, w.Muted)
		check("Gutter", got.Gutter, w.Gutter)
		check("Accent", got.Accent, w.Accent)
		check("Accent2", got.Accent2, w.Accent2)
		check("Error", got.Error, w.Error)
		check("Keyword", got.Keyword, w.Keyword)
		check("Func", got.Func, w.Func)
		check("String", got.String, w.String)
		check("Number", got.Number, w.Number)
		check("Type", got.Type, w.Type)
		check("Comment", got.Comment, w.Comment)
		check("Variable", got.Variable, w.Variable)
		check("Builtin", got.Builtin, w.Builtin)
		if w.CommentItalic && !got.CommentItalic {
			t.Errorf("%s: comments should be italic", tt.id)
		}
	}
}

func TestLightThemesAreNotSupported(t *testing.T) {
	if err := loadThemes(""); err != nil {
		t.Fatal(err)
	}
	for _, th := range themes {
		if isLight(th.Bg) {
			t.Errorf("the built-in theme %s is light (%s)", th.Name, th.Bg)
		}
	}
	for _, id := range []string{"one-light", "github-light-default"} {
		if findTheme(id) != nil {
			t.Errorf("%s must be gone", id)
		}
	}
	for hex, want := range map[string]bool{"#ffffff": true, "#fafafa": true, "#f6f8fa": true, "#23272e": false, "#282c34": false, "#0d1117": false, "#808080": false, "oops": false} {
		if isLight(hex) != want {
			t.Errorf("isLight(%s) = %v", hex, !want)
		}
	}
	// A light theme dropped into the user's folder is skipped, with a reason, and the dark ones still load.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bright.json"), []byte(`{"name":"Bright","colors":{"editor.background":"#ffffff","editor.foreground":"#000000"}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "night.json"), []byte(`{"name":"Night","colors":{"editor.background":"#101010","editor.foreground":"#eeeeee"}}`), 0o644)
	err := loadThemes(dir)
	if err == nil || !strings.Contains(err.Error(), "bright.json") || !strings.Contains(err.Error(), "light themes are not supported") {
		t.Errorf("error = %v", err)
	}
	if findTheme("bright") != nil || findTheme("night") == nil {
		t.Error("the light theme is skipped, the dark one is kept")
	}
	loadThemes("") // back to the built-in set for the other tests
}

func TestUserThemeOverridesAndErrors(t *testing.T) {
	dir := t.TempDir()
	user := `{
		// a comment
		"name": "Mine",
		"colors": {"editor.background": "#101010", "editor.foreground": "#eeeeee",},
		"tokenColors": [{"scope": "keyword, storage", "settings": {"foreground": "#ff000080"}}],
	}`
	if err := os.WriteFile(filepath.Join(dir, "mine.json"), []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := loadThemes(dir)
	if err == nil {
		t.Error("broken.json should be reported")
	}
	mine := findTheme("mine")
	if mine == nil {
		t.Fatal("user theme not loaded")
	}
	// keyword.control falls back to the "keyword" rule; 50% red over #101010.
	if mine.Keyword != "#880808" {
		t.Errorf("Keyword = %s, want #880808", mine.Keyword)
	}
	if findTheme(defaultTheme) == nil {
		t.Error("built-in themes should still be there")
	}
	t.Cleanup(func() { _ = loadThemes("") })
}

func TestHexColor(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"#ABCDEF", "#abcdef"},
		{"#abc", "#aabbcc"},
		{"#ffffff00", "#000000"},
		{"#ffffffff", "#ffffff"},
	} {
		if got, ok := hexColor(tt.in, "#000000"); !ok || got != tt.want {
			t.Errorf("hexColor(%q) = %q, %v; want %q", tt.in, got, ok, tt.want)
		}
	}
	if _, ok := hexColor("red", "#000000"); ok {
		t.Error("named colors are not supported")
	}
}
