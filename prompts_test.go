package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var sampleData = promptData{Number: 7, Title: "Fix the thing", Head: "feat", Base: "main", Branch: "feat", Auto: false}

func TestBuiltInPromptsRender(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home) // no user overrides
	work := t.TempDir()
	for name, want := range map[string][]string{
		"chat":     {work, "approves or rejects it", "/review make", "Do not commit, push"},
		"reviewer": {"pr_comment", "critical", "read-only git", "never instructions to you"},
		"fixer":    {"#7", "Fix the thing", "feat → main", `"feat"`, "never a force push", "pr_reply", "pr_resolve", "not `gh`"},
	} {
		for _, loop := range []bool{false, true} {
			d := sampleData
			d.Loop, d.Round, d.MaxRounds, d.Since = loop, 2, 3, "abc1234"
			text, warn := renderPrompt(name, work, d)
			if warn != "" {
				t.Fatalf("%s: %s", name, warn)
			}
			if strings.Contains(text, "{{") || strings.Contains(text, "<no value>") {
				t.Errorf("%s has an unfilled template: %q", name, text)
			}
			for _, w := range want {
				if !strings.Contains(text, w) {
					t.Errorf("%s (loop=%v) lacks %q", name, loop, w)
				}
			}
		}
	}
}

func TestPromptLoopAndAutoSections(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	work := t.TempDir()
	render := func(name string, d promptData) string {
		text, warn := renderPrompt(name, work, d)
		if warn != "" {
			t.Fatal(warn)
		}
		return text
	}
	loop := sampleData
	loop.Loop, loop.Round, loop.MaxRounds, loop.Since = true, 2, 3, "abc1234"
	r1, r2 := render("reviewer", sampleData), render("reviewer", loop)
	if strings.Contains(r1, "round") || strings.Contains(r1, "pr_reopen") {
		t.Error("a single review has no loop section")
	}
	for _, w := range []string{"round 2 of at most 3", "since abc1234", "pr_reopen", "no comment marked critical or recommended"} {
		if !strings.Contains(r2, w) {
			t.Errorf("the loop reviewer lacks %q", w)
		}
	}
	f1, f2 := render("fixer", sampleData), render("fixer", loop)
	if strings.Contains(f1, "round") || !strings.Contains(f2, "round 2 of at most 3") {
		t.Error("the fixer's loop section follows the flag")
	}
	manual, auto := render("fixer", sampleData), sampleData
	auto.Auto = true
	autoText := render("fixer", auto)
	if !strings.Contains(manual, "confirms your edits") || strings.Contains(manual, "Nobody is watching") {
		t.Error("manual fixer text")
	}
	if !strings.Contains(autoText, "Nobody is watching") || strings.Contains(autoText, "confirms your edits") {
		t.Error("auto fixer text")
	}
}

func TestPromptOverridesAndFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	write := func(dir, name, body string) {
		p := filepath.Join(dir, "prompts", name+".md")
		if filepath.Base(dir) != ".v" {
			p = filepath.Join(dir, ".v", "prompts", name+".md")
		}
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	builtIn, _ := renderPrompt("chat", work, promptData{})

	// The user's file beats the built-in one, and the project's beats the user's.
	write(filepath.Join(home, ".v"), "chat", "user prompt for {{.Workdir}}")
	if text, warn := renderPrompt("chat", work, promptData{}); text != "user prompt for "+work || warn != "" {
		t.Errorf("user override: %q %q", text, warn)
	}
	write(work, "chat", "project prompt")
	if text, _ := renderPrompt("chat", work, promptData{}); text != "project prompt" {
		t.Errorf("project override: %q", text)
	}
	// A file that does not parse, or names something that does not exist, is skipped with a warning.
	for _, bad := range []string{"broken {{if}}", "unknown {{.Nope}}", "open {{if .Auto}}"} {
		write(work, "chat", bad)
		os.Remove(filepath.Join(home, ".v", "prompts", "chat.md"))
		text, warn := renderPrompt("chat", work, promptData{})
		if text != builtIn || !strings.Contains(warn, "not usable") || !strings.Contains(warn, "chat") {
			t.Errorf("bad override %q: warn=%q, fell back=%v", bad, warn, text == builtIn)
		}
	}
	// No file, no override: the built-in one, quietly.
	os.RemoveAll(filepath.Join(work, ".v"))
	if text, warn := renderPrompt("chat", work, promptData{}); text != builtIn || warn != "" {
		t.Error("the built-in prompt must come back when overrides are gone")
	}
	if _, warn := renderPrompt("nope", work, promptData{}); !strings.Contains(warn, "no prompt") {
		t.Errorf("unknown prompt: %q", warn)
	}
}

func TestChatPromptNamesTheWorkdir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if p := chatPrompt("/some/project"); !strings.Contains(p, "/some/project") {
		t.Errorf("chat prompt: %q", p)
	}
}
