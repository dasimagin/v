package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// reviewTools are the tool calls shown as a diff before the user answers.
var reviewTools = map[string]bool{"Edit": true, "MultiEdit": true, "Write": true}

// proposedDoc builds the file as it would be after the agent's tool call,
// with the current file as its diff base. Nothing is written.
func proposedDoc(tool string, in map[string]any) (*Doc, error) {
	path, _ := in["file_path"].(string)
	if path == "" {
		return nil, errors.New("no file_path")
	}
	cur, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var next string
	switch tool {
	case "Write":
		next, _ = in["content"].(string)
	case "Edit":
		if next, err = applyEdit(string(cur), in); err != nil {
			return nil, err
		}
	case "MultiEdit":
		next = string(cur)
		edits, _ := in["edits"].([]any)
		for i, e := range edits {
			em, _ := e.(map[string]any)
			if next, err = applyEdit(next, em); err != nil {
				return nil, fmt.Errorf("edit %d: %w", i+1, err)
			}
		}
	}

	lang, lines, err := highlight(path, next)
	if err != nil {
		return nil, err
	}
	base := &Base{New: true}
	if exists {
		_, old, err := highlight(path, string(cur))
		if err != nil {
			return nil, err
		}
		base = &Base{Lines: old, Text: texts(old)}
	}
	return &Doc{Path: path, Lang: lang, Lines: lines, Text: texts(lines), Base: base, Review: tool}, nil
}

// applyEdit replaces old_string with new_string the way Claude Code's Edit does.
func applyEdit(text string, in map[string]any) (string, error) {
	old, _ := in["old_string"].(string)
	repl, _ := in["new_string"].(string)
	all, _ := in["replace_all"].(bool)
	if old == "" {
		return repl + text, nil
	}
	if !strings.Contains(text, old) {
		return "", errors.New("old_string not found in the file")
	}
	if all {
		return strings.ReplaceAll(text, old, repl), nil
	}
	return strings.Replace(text, old, repl, 1), nil
}

// openReview shows the proposed change on top of the screen stack.
func (m *model) openReview(tool string, in map[string]any) {
	d, err := proposedDoc(tool, in)
	if err != nil {
		m.note("  ⎿ can't show the diff: " + err.Error())
		return
	}
	m.push()
	m.doc, m.top, m.left, m.mode = d, 0, 0, modeReview
}
