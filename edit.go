package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// editKey handles a key while the open file or proposal is being edited.
func (m model) editKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := m.doc
	page := max(m.bodyRows()-1, 1)
	switch msg.String() {
	case "esc":
		m.stopEdit()
		return m, nil
	case "ctrl+s":
		m.save()
		return m, nil
	case "ctrl+z":
		d.Undo()
	case "ctrl+y":
		d.Redo()
	case "up":
		d.Move(-1, 0)
	case "down":
		d.Move(1, 0)
	case "left":
		d.Move(0, -1)
	case "right":
		d.Move(0, 1)
	case "pgup":
		d.Move(-page, 0)
	case "pgdown":
		d.Move(page, 0)
	case "home", "ctrl+a":
		d.Home()
	case "end", "ctrl+e":
		d.End()
	case "ctrl+home":
		d.Ed.Cur = pos{}
		d.keepCol()
	case "ctrl+end":
		d.Ed.Cur = pos{len(d.Text) - 1, runeLen(d.Text[len(d.Text)-1])}
		d.keepCol()
	case "enter":
		d.Newline()
	case "backspace":
		d.Backspace()
	case "delete":
		d.Delete()
	case "tab":
		d.Insert(d.indentUnit())
	default:
		switch {
		case msg.Alt:
		case msg.Type == tea.KeySpace:
			d.Insert(" ")
		case msg.Type == tea.KeyRunes:
			d.Insert(string(msg.Runes)) // typing, or a whole paste
		}
	}
	m.follow()
	return m, nil
}

// firstVisibleLine is the first working-copy line on screen, for placing
// the cursor when editing starts.
func (m *model) firstVisibleLine() int {
	var rows []diffRow
	right := false
	switch m.effectiveMode() {
	case viewFlat:
		return min(m.top, len(m.doc.Text)-1)
	case viewUnified:
		rows = m.unifiedRows()
	case viewSplit:
		rows, right = m.splitRows(), true
	}
	for i := m.top; i < len(rows); i++ {
		s := rows[i].left
		if right {
			s = rows[i].right
		}
		if s.kind == rowCtx || s.kind == rowAdd {
			return s.no - 1
		}
	}
	return 0
}

// firstChangedLine is the first line the proposal adds or changes, or -1.
func (m *model) firstChangedLine() int {
	if m.doc.Base == nil {
		return -1
	}
	rows, _ := unifiedRows(m.doc)
	for _, r := range rows {
		if r.left.kind == rowAdd {
			return r.left.no - 1
		}
	}
	return -1
}

// startEdit turns editing on with the cursor on line, -1 for the first
// line on screen. Diffs switch to whole-file context, so the layout is
// rebuilt and scrolled to keep the cursor in view.
func (m *model) startEdit(line int) {
	d := m.doc
	if line < 0 && m.mode == modeReview {
		line = m.firstChangedLine()
	}
	if line < 0 {
		line = m.firstVisibleLine()
	}
	d.Ed.On = true
	m.omni.Blur() // keys go to the file; the omnibox cursor would mislead
	d.unified, d.split = nil, nil
	d.Ed.Cur = pos{line, 0}
	d.clamp()
	d.keepCol()
	m.discardArmed = false
	m.followCentered()
}

func (m *model) stopEdit() {
	d := m.doc
	line := d.Ed.Cur.Line
	d.Ed.On = false
	m.omni.Focus()
	d.unified, d.split = nil, nil
	m.top = 0
	// Back in hunk view, scroll to where the cursor was if it is shown.
	if row := m.rowOf(line); row >= 0 {
		m.top = max(row-m.bodyRows()/2, 0)
	}
}

// rowOf is the screen row (before scrolling) of working-copy line n, or -1.
func (m *model) rowOf(n int) int {
	switch m.effectiveMode() {
	case viewUnified:
		m.unifiedRows()
		return m.doc.unifiedAt[n]
	case viewSplit:
		m.splitRows()
		return m.doc.splitAt[n]
	}
	return n
}

// numWidth is the width of line numbers in the current view.
func (m *model) numWidth() int {
	n := len(m.doc.Text)
	if m.doc.Base != nil {
		n = max(n, len(m.doc.Base.Text))
	}
	return len(fmt.Sprint(n))
}

// textX is the screen column where working-copy text starts in the
// current view, and how wide that text area is.
func (m *model) textX() (x, width int) {
	nw := m.numWidth()
	switch m.effectiveMode() {
	case viewUnified:
		return 2*nw + 4, m.w - (2*nw + 4)
	case viewSplit:
		lw := (m.w - 1) / 2
		return lw + 1 + nw + 3, m.w - 1 - lw - (nw + 3)
	}
	gw := len(fmt.Sprint(len(m.doc.Text))) + 2
	return gw, m.w - gw
}

// follow scrolls so the cursor stays on screen.
func (m *model) follow() {
	d := m.doc
	rows := m.bodyRows()
	if row := m.rowOf(d.Ed.Cur.Line); row >= 0 {
		if row < m.top {
			m.top = row
		} else if row >= m.top+rows {
			m.top = row - rows + 1
		}
	}
	_, w := m.textX()
	col := displayCol(d.Text[d.Ed.Cur.Line], d.Ed.Cur.Col)
	if col < m.left {
		m.left = col
	} else if col >= m.left+w-1 {
		m.left = col - w + 2
	}
}

// followCentered scrolls the cursor line to the middle of the screen.
func (m *model) followCentered() {
	if row := m.rowOf(m.doc.Ed.Cur.Line); row >= 0 {
		m.top = max(row-m.bodyRows()/2, 0)
	}
	m.follow()
}

// clickText places the cursor where the user clicked in the file body and
// starts editing. y is the row in the body, x the screen column.
func (m *model) clickText(x, y int) {
	row := m.top + y
	line := -1
	tx, _ := m.textX()
	switch m.effectiveMode() {
	case viewFlat:
		if row < len(m.doc.Text) {
			line = row
		}
	case viewUnified:
		if rows := m.unifiedRows(); row < len(rows) {
			if s := rows[row].left; s.kind == rowCtx || s.kind == rowAdd {
				line = s.no - 1
			}
		}
	case viewSplit:
		if rows := m.splitRows(); row < len(rows) && x >= tx-m.numWidth()-3 {
			if s := rows[row].right; s.kind == rowCtx || s.kind == rowAdd {
				line = s.no - 1
			}
		}
	}
	if line < 0 {
		return
	}
	col := runeAt(m.doc.Text[line], max(x-tx, 0)+m.left)
	if !m.doc.Ed.On {
		m.startEdit(line)
	}
	m.doc.Ed.Cur = pos{line, col}
	m.doc.keepCol()
	m.follow()
}

// save writes the open file, or accepts the proposal under review with the
// user's changes.
func (m *model) save() {
	d := m.doc
	if d.Review != "" {
		if m.perm != nil {
			m.answerPermission(true, "")
		}
		return
	}
	perm := os.FileMode(0o644)
	if fi, err := os.Stat(d.Path); err == nil {
		perm = fi.Mode().Perm()
	}
	if err := os.WriteFile(d.Path, []byte(d.Contents()), perm); err != nil {
		m.setErr("save: " + err.Error())
		return
	}
	d.Ed.Dirty = false
	m.discardArmed = false
	m.flash("saved " + shortPath(d.Path))
}
