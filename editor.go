package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// pos is a place in a document: line index and rune index in that line.
type pos struct{ Line, Col int }

type snapshot struct {
	text []string
	cur  pos
}

// Editor is the editing state of a Doc.
type Editor struct {
	On    bool // keys go to the buffer and the cursor is shown
	Cur   pos
	Dirty bool // changed since loaded or saved

	want       int // display column that up/down try to keep
	undo, redo []snapshot
	lastKind   string // "type" while typing on one line, to group undo steps
	lastLine   int
}

func (d *Doc) line(i int) string { return d.Text[i] }

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// clamp keeps the cursor inside the text.
func (d *Doc) clamp() {
	e := &d.Ed
	e.Cur.Line = min(max(e.Cur.Line, 0), len(d.Text)-1)
	e.Cur.Col = min(max(e.Cur.Col, 0), runeLen(d.line(e.Cur.Line)))
}

// checkpoint saves an undo step before a change. Typing on the same line
// joins the previous step, so undo removes a word or a line of typing at once.
func (d *Doc) checkpoint(kind string) {
	e := &d.Ed
	if kind == "type" && e.lastKind == "type" && e.lastLine == e.Cur.Line {
		return
	}
	e.undo = append(e.undo, snapshot{append([]string(nil), d.Text...), e.Cur})
	e.redo = nil
	e.lastKind, e.lastLine = kind, e.Cur.Line
}

// changed refreshes everything that depends on the text.
func (d *Doc) changed() {
	d.Ed.Dirty = true
	d.clamp()
	_ = d.rehighlight()
}

func (d *Doc) Undo() { d.swapHistory(&d.Ed.undo, &d.Ed.redo) }
func (d *Doc) Redo() { d.swapHistory(&d.Ed.redo, &d.Ed.undo) }

func (d *Doc) swapHistory(from, to *[]snapshot) {
	if len(*from) == 0 {
		return
	}
	s := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	*to = append(*to, snapshot{append([]string(nil), d.Text...), d.Ed.Cur})
	d.Text, d.Ed.Cur = s.text, s.cur
	d.Ed.lastKind = ""
	d.changed()
}

// Insert types s at the cursor; s may hold newlines (a paste).
func (d *Doc) Insert(s string) {
	if s == "" {
		return
	}
	kind := "type"
	if strings.Contains(s, "\n") || runeLen(s) > 1 {
		kind = "paste"
	}
	d.checkpoint(kind)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	e := &d.Ed
	l := []rune(d.line(e.Cur.Line))
	before, after := string(l[:e.Cur.Col]), string(l[e.Cur.Col:])
	parts := strings.Split(s, "\n")
	parts[0] = before + parts[0]
	last := len(parts) - 1
	e.Cur = pos{e.Cur.Line + last, runeLen(parts[last])}
	parts[last] += after
	d.Text = append(d.Text[:e.Cur.Line-last], append(parts, d.Text[e.Cur.Line-last+1:]...)...)
	d.changed()
	d.keepCol()
}

// Newline splits the line at the cursor, keeping its indentation.
func (d *Doc) Newline() {
	l := d.line(d.Ed.Cur.Line)
	indent := l[:len(l)-len(strings.TrimLeftFunc(l, unicode.IsSpace))]
	if runeLen(indent) > d.Ed.Cur.Col {
		indent = string([]rune(indent)[:d.Ed.Cur.Col])
	}
	d.Insert("\n" + indent)
}

// Backspace deletes before the cursor, joining lines at column 0.
func (d *Doc) Backspace() {
	e := &d.Ed
	if e.Cur.Col == 0 && e.Cur.Line == 0 {
		return
	}
	d.checkpoint("delete")
	if e.Cur.Col == 0 {
		prev := d.line(e.Cur.Line - 1)
		d.Text[e.Cur.Line-1] = prev + d.line(e.Cur.Line)
		d.Text = append(d.Text[:e.Cur.Line], d.Text[e.Cur.Line+1:]...)
		e.Cur = pos{e.Cur.Line - 1, runeLen(prev)}
	} else {
		l := []rune(d.line(e.Cur.Line))
		d.Text[e.Cur.Line] = string(l[:e.Cur.Col-1]) + string(l[e.Cur.Col:])
		e.Cur.Col--
	}
	d.changed()
	d.keepCol()
}

// Delete deletes under the cursor, joining with the next line at the end.
func (d *Doc) Delete() {
	e := &d.Ed
	l := []rune(d.line(e.Cur.Line))
	if e.Cur.Col == len(l) && e.Cur.Line == len(d.Text)-1 {
		return
	}
	d.checkpoint("delete")
	if e.Cur.Col == len(l) {
		d.Text[e.Cur.Line] += d.line(e.Cur.Line + 1)
		d.Text = append(d.Text[:e.Cur.Line+1], d.Text[e.Cur.Line+2:]...)
	} else {
		d.Text[e.Cur.Line] = string(l[:e.Cur.Col]) + string(l[e.Cur.Col+1:])
	}
	d.changed()
}

// indentUnit is what Tab inserts: a tab if the file indents with tabs (or
// is Go), otherwise four spaces.
func (d *Doc) indentUnit() string {
	tabs, spaces := 0, 0
	for _, l := range d.Text {
		switch {
		case strings.HasPrefix(l, "\t"):
			tabs++
		case strings.HasPrefix(l, "  "):
			spaces++
		}
	}
	if tabs >= spaces && (tabs > 0 || d.Lang == "Go" || d.Lang == "makefile") {
		return "\t"
	}
	return "    "
}

// keepCol remembers the cursor's display column for up/down moves.
func (d *Doc) keepCol() {
	d.Ed.want = displayCol(d.line(d.Ed.Cur.Line), d.Ed.Cur.Col)
}

// Move moves the cursor by lines (dy) or runes (dx); horizontal moves wrap
// across lines.
func (d *Doc) Move(dy, dx int) {
	e := &d.Ed
	e.lastKind = ""
	if dy != 0 {
		e.Cur.Line = min(max(e.Cur.Line+dy, 0), len(d.Text)-1)
		e.Cur.Col = runeAt(d.line(e.Cur.Line), e.want)
		return
	}
	e.Cur.Col += dx
	switch {
	case e.Cur.Col < 0 && e.Cur.Line > 0:
		e.Cur.Line--
		e.Cur.Col = runeLen(d.line(e.Cur.Line))
	case e.Cur.Col > runeLen(d.line(e.Cur.Line)) && e.Cur.Line < len(d.Text)-1:
		e.Cur.Line++
		e.Cur.Col = 0
	}
	d.clamp()
	d.keepCol()
}

// Home goes to the first non-blank character, or to column 0 if already there.
func (d *Doc) Home() {
	l := d.line(d.Ed.Cur.Line)
	first := runeLen(l) - runeLen(strings.TrimLeftFunc(l, unicode.IsSpace))
	if d.Ed.Cur.Col == first {
		first = 0
	}
	d.Ed.Cur.Col = first
	d.keepCol()
}

func (d *Doc) End() {
	d.Ed.Cur.Col = runeLen(d.line(d.Ed.Cur.Line))
	d.keepCol()
}
