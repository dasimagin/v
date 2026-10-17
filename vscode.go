package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// vscodeTheme is the part of a VS Code color theme file that v uses.
type vscodeTheme struct {
	Name        string         `json:"name"`
	Colors      map[string]any `json:"colors"` // values are usually strings; skip the rest
	TokenColors []tokenRule    `json:"tokenColors"`
}

type tokenRule struct {
	Scope    scopeList `json:"scope"`
	Settings struct {
		Foreground string `json:"foreground"`
		FontStyle  string `json:"fontStyle"`
	} `json:"settings"`
}

// scopeList accepts both "a, b" and ["a", "b"].
type scopeList []string

func (s *scopeList) UnmarshalJSON(b []byte) error {
	var list []string
	if err := json.Unmarshal(b, &list); err == nil {
		*s = list
		return nil
	}
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}
	for _, p := range strings.Split(str, ",") {
		*s = append(*s, strings.TrimSpace(p))
	}
	return nil
}

// parseVSCodeTheme turns a VS Code theme file (JSON with comments) into a Theme.
func parseVSCodeTheme(id string, data []byte) (*Theme, error) {
	var vt vscodeTheme
	if err := json.Unmarshal(stripJSONC(data), &vt); err != nil {
		return nil, err
	}
	color := func(k string) string { s, _ := vt.Colors[k].(string); return s }
	bgc, ok := hexColor(color("editor.background"), "#000000")
	if !ok {
		return nil, fmt.Errorf("no editor.background")
	}
	ui := func(keys ...string) string {
		for _, k := range keys {
			if c, ok := hexColor(color(k), bgc); ok {
				return c
			}
		}
		return ""
	}
	tok := func(scope string) (string, bool) {
		r := resolveScope(vt.TokenColors, scope)
		if r == nil {
			return "", false
		}
		c, _ := hexColor(r.Settings.Foreground, bgc)
		return c, strings.Contains(r.Settings.FontStyle, "italic")
	}
	fg := ui("editor.foreground", "foreground")
	if fg == "" {
		fg = "#cccccc"
	}
	or := func(c, def string) string {
		if c == "" {
			return def
		}
		return c
	}

	t := &Theme{ID: id, Name: or(vt.Name, id), Bg: bgc, Fg: fg}
	t.Keyword, _ = tok("keyword.control")
	t.Func, _ = tok("entity.name.function")
	t.String, _ = tok("string.quoted.double")
	t.Number, _ = tok("constant.numeric")
	t.Type, _ = tok("entity.name.type")
	t.Constant, _ = tok("constant.language")
	t.Comment, t.CommentItalic = tok("comment.line")
	t.Operator, _ = tok("keyword.operator")
	t.Variable, _ = tok("variable.other")
	t.Builtin, _ = tok("support.function")
	t.Tag, _ = tok("entity.name.tag")
	t.Attr, _ = tok("entity.other.attribute-name")
	t.Escape, _ = tok("constant.character.escape")
	t.Added, _ = tok("markup.inserted")
	t.Deleted, _ = tok("markup.deleted")
	for _, c := range []*string{&t.Keyword, &t.Func, &t.String, &t.Number, &t.Type, &t.Constant,
		&t.Comment, &t.Operator, &t.Variable, &t.Builtin, &t.Tag, &t.Attr, &t.Escape} {
		*c = or(*c, fg)
	}

	t.Panel = or(ui("sideBar.background"), bgc)
	t.Sel = or(ui("list.activeSelectionBackground", "editor.lineHighlightBackground", "editor.selectionBackground"), t.Panel)
	// Some themes set descriptionForeground to the text color; hints need a dimmer one.
	t.Muted = ui("descriptionForeground")
	if t.Muted == "" || t.Muted == fg {
		t.Muted = t.Comment
	}
	t.Gutter = or(ui("editorLineNumber.foreground"), t.Muted)
	t.Accent = or(ui("textLink.foreground", "focusBorder"), t.Func)
	t.Accent2 = t.Func
	if t.Accent2 == t.Accent {
		t.Accent2 = t.Keyword
	}
	t.Error = or(ui("errorForeground"), or(t.Deleted, or(ui("editorError.foreground"), "#ff0000")))
	t.Added = or(t.Added, t.String)
	t.Deleted = or(t.Deleted, t.Error)
	return t, nil
}

// resolveScope finds the rule that colors scope: the longest selector that is
// the scope itself or a dot-prefix of it; later rules win ties. Selectors with
// a space (descendant selectors) are skipped.
func resolveScope(rules []tokenRule, scope string) *tokenRule {
	var best *tokenRule
	bestLen := -1
	for i := range rules {
		r := &rules[i]
		if r.Settings.Foreground == "" {
			continue
		}
		for _, sel := range r.Scope {
			if sel == "" || strings.Contains(sel, " ") {
				continue
			}
			if (scope == sel || strings.HasPrefix(scope, sel+".")) && len(sel) >= bestLen {
				best, bestLen = r, len(sel)
			}
		}
	}
	return best
}

// hexColor normalizes "#rgb", "#rgba", "#rrggbb" or "#rrggbbaa" to "#rrggbb",
// blending any alpha over bg.
func hexColor(c, bg string) (string, bool) {
	c = strings.TrimPrefix(strings.TrimSpace(c), "#")
	if len(c) == 3 || len(c) == 4 {
		var b strings.Builder
		for _, r := range c {
			b.WriteRune(r)
			b.WriteRune(r)
		}
		c = b.String()
	}
	if len(c) != 6 && len(c) != 8 {
		return "", false
	}
	v, err := strconv.ParseUint(c, 16, 32)
	if err != nil {
		return "", false
	}
	if len(c) == 6 {
		return "#" + strings.ToLower(c), true
	}
	under, err := strconv.ParseUint(strings.TrimPrefix(bg, "#"), 16, 32)
	if err != nil {
		return "", false
	}
	a := float64(v&0xff) / 255
	var out uint64
	for shift := 24; shift >= 8; shift -= 8 {
		fgc := float64(v >> shift & 0xff)
		bgc := float64(under >> (shift - 8) & 0xff)
		out = out<<8 | uint64(fgc*a+bgc*(1-a)+0.5)
	}
	return fmt.Sprintf("#%06x", out), true
}

// stripJSONC removes // and /* */ comments and trailing commas outside strings.
func stripJSONC(b []byte) []byte {
	out := make([]byte, 0, len(b))
	inStr := false
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case inStr:
			out = append(out, c)
			if c == '\\' && i+1 < len(b) {
				i++
				out = append(out, b[i])
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			for i < len(b) && b[i] != '\n' {
				i++
			}
			i--
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			i += 2
			for i+1 < len(b) && (b[i] != '*' || b[i+1] != '/') {
				i++
			}
			i++
		case c == '}' || c == ']':
			// Drop a trailing comma before the closing bracket.
			j := len(out) - 1
			for j >= 0 && (out[j] == ' ' || out[j] == '\t' || out[j] == '\n' || out[j] == '\r') {
				j--
			}
			if j >= 0 && out[j] == ',' {
				out = append(out[:j], out[j+1:]...)
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}
