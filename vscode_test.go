package main

import (
	"os"
	"path/filepath"
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
		{"one-light", Theme{
			Name: "One Light", Bg: "#fafafa", Panel: "#eaeaeb", Fg: "#383a42", Accent: "#526fff",
			Keyword: "#a626a4", Func: "#4078f2", String: "#50a14f", Comment: "#a0a1a7", CommentItalic: true,
		}},
		{"github-light-default", Theme{
			Name: "GitHub Light Default", Bg: "#ffffff", Panel: "#f6f8fa", Fg: "#1f2328", Muted: "#656d76",
			Accent: "#0969da", Accent2: "#8250df", Error: "#cf222e",
			Keyword: "#cf222e", Func: "#8250df", String: "#0a3069", Number: "#0550ae", Type: "#953800",
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
