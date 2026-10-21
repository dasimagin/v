package main

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// The agents' instructions are Markdown files: built in, and replaceable per
// project or per user. See prompts/README.md.
//
//go:embed prompts/*.md
var promptFiles embed.FS

// promptData is what the prompt templates can use.
type promptData struct {
	Workdir, Title, Head, Base, Branch string
	Number                             int
	Auto                               bool
	Loop                               bool
	Round, MaxRounds                   int
	Since                              string
}

// promptSources are the places a prompt is looked for, best first.
func promptSources(workdir, name string) []string {
	return []string{
		filepath.Join(workdir, ".v", "prompts", name+".md"),
		filepath.Join(configDir(), "prompts", name+".md"),
	}
}

// renderPrompt builds the prompt called name. A file of the user's that does
// not read or does not parse is skipped, and warn says why; the built-in one
// always works.
func renderPrompt(name, workdir string, d promptData) (text, warn string) {
	d.Workdir = workdir
	for _, path := range promptSources(workdir, name) {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		out, err := execPrompt(string(b), d)
		if err != nil {
			warn = fmt.Sprintf("%s is not usable (%v); using the built-in %s prompt", path, err, name)
			continue
		}
		return out, warn
	}
	b, err := promptFiles.ReadFile("prompts/" + name + ".md")
	if err != nil {
		return "", fmt.Sprintf("no prompt called %s", name)
	}
	out, err := execPrompt(string(b), d)
	if err != nil { // a bug in the built-in file, caught by a test
		return string(b), err.Error()
	}
	return out, warn
}

func execPrompt(src string, d promptData) (string, error) {
	t, err := template.New("prompt").Option("missingkey=error").Parse(src)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, d); err != nil {
		return "", err
	}
	return strings.TrimSpace(b.String()), nil
}

// chatPrompt is the chat agent's system prompt for a working directory.
func chatPrompt(workdir string) string {
	text, _ := renderPrompt("chat", workdir, promptData{})
	return text
}
