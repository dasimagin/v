package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// viewMenu is the open view-mode dropdown in the header.
type viewMenu struct{ sel int }

// rowCount is the number of scrollable rows of the open file in the current mode.
func (m *model) rowCount() int {
	switch m.effectiveMode() {
	case viewUnified:
		return len(m.unifiedRows())
	case viewSplit:
		return len(m.splitRows())
	}
	return len(m.doc.Lines)
}

// curMode is the view mode setting for the current screen: reviews of agent
// edits keep their own, so they can default to a diff.
func (m *model) curMode() *viewMode {
	if m.mode == modeReview {
		return &m.rmode
	}
	return &m.vmode
}

// effectiveMode falls back to Flat outside a git repository.
func (m *model) effectiveMode() viewMode {
	if m.doc.Base == nil {
		return viewFlat
	}
	return *m.curMode()
}

func (m *model) unifiedRows() []diffRow {
	if m.doc.unified == nil {
		m.doc.unified = unifiedRows(m.doc)
	}
	return m.doc.unified
}

func (m *model) splitRows() []diffRow {
	if m.doc.split == nil {
		m.doc.split = splitRows(m.doc)
	}
	return m.doc.split
}

// modeLabel is the dropdown button in the top right corner.
func (m *model) modeLabel() string {
	if m.doc.Base == nil {
		return " " + viewModeNames[viewFlat] + " "
	}
	return " " + viewModeNames[*m.curMode()] + " ▾ "
}

// openViewMenu opens the dropdown, or explains why there is none.
func (m *model) openViewMenu() {
	if m.doc.Base == nil {
		m.setErr("not in a git repository: diffs are off")
		return
	}
	m.viewMenu = &viewMenu{sel: int(*m.curMode())}
}

func (m *model) setViewMode(v viewMode) {
	*m.curMode(), m.viewMenu, m.top = v, nil, 0
}

// viewMenuKey handles keys while the dropdown is open.
func (m *model) viewMenuKey(k string) {
	n := len(viewModeNames)
	switch k {
	case "up", "shift+tab":
		m.viewMenu.sel = (m.viewMenu.sel + n - 1) % n
	case "down", "tab":
		m.viewMenu.sel = (m.viewMenu.sel + 1) % n
	case "enter", " ":
		m.setViewMode(viewMode(m.viewMenu.sel))
	case "esc":
		m.viewMenu = nil
	}
}

// menuWidth is the dropdown list width; it is also used for mouse hit tests.
func menuWidth() int {
	w := 0
	for _, n := range viewModeNames {
		w = max(w, lipgloss.Width(n))
	}
	return w + 4
}

// viewClick handles a left click in view mode at screen cell (x, y).
func (m *model) viewClick(x, y int) {
	labelW := lipgloss.Width(m.modeLabel())
	if y == headerTitleRow && x >= m.w-labelW-1 && x < m.w-1 {
		if m.viewMenu != nil {
			m.viewMenu = nil
		} else {
			m.openViewMenu()
		}
		return
	}
	if m.viewMenu != nil {
		if i := y - headerRows; i >= 0 && i < len(viewModeNames) && x >= m.w-menuWidth()-1 {
			m.setViewMode(viewMode(i))
			return
		}
		m.viewMenu = nil
	}
}

// renderView draws the header and rows lines of the open file.
func (m *model) renderView(rows int) []string {
	d := m.doc
	total := m.rowCount()
	title := pathTitle(d.Path, false)
	if d.Review != "" {
		title = buttonOpen.Render(" proposed "+d.Review+" ") + bg.Render("  ") + title
	}
	info := fmt.Sprintf("%s · %d/%d", d.Lang, min(m.top+rows, total), total)
	labelStyle := buttonOff
	switch {
	case d.Base != nil && m.viewMenu != nil:
		labelStyle = buttonOpen
	case d.Base != nil:
		labelStyle = buttonStyle
	}
	out := m.header(title, info, labelStyle.Render(m.modeLabel()))

	var body []string
	switch m.effectiveMode() {
	case viewFlat:
		body = m.renderFlat(rows)
	case viewUnified:
		body = m.renderUnified(rows)
	case viewSplit:
		body = m.renderSplit(rows)
	}
	if m.viewMenu != nil {
		// The list hangs under the button, right-aligned with it.
		mw := menuWidth()
		for i, name := range viewModeNames {
			item := "  " + name + strings.Repeat(" ", mw-2-lipgloss.Width(name))
			st := lipgloss.NewStyle().Background(lipgloss.Color(cur.Panel)).Foreground(lipgloss.Color(cur.Fg))
			if i == m.viewMenu.sel {
				item = "› " + item[2:]
				st = buttonStyle
			}
			if i < len(body) {
				body[i] = ansi.Truncate(body[i], m.w-mw-1, "") + st.Render(item) + bg.Render(" ")
			}
		}
	}
	return append(out, body...)
}

func blank(w int) string { return bg.Render(strings.Repeat(" ", max(w, 0))) }

func (m *model) renderFlat(rows int) []string {
	d := m.doc
	gw := len(fmt.Sprint(len(d.Lines))) + 2
	out := make([]string, rows)
	for i := range out {
		if n := m.top + i; n < len(d.Lines) {
			out[i] = gutterStyle.Render(fmt.Sprintf("%*d ", gw-1, n+1)) + renderLine(d.Lines[n], m.left, m.w-gw)
		} else {
			out[i] = blank(m.w)
		}
	}
	return out
}

// noChanges fills the body with a centered note when the diff is empty.
func (m *model) noChanges(rows int) []string {
	note := "No changes against HEAD"
	if m.doc.Base.New {
		note = "Empty file, not in HEAD yet"
	}
	out := make([]string, rows)
	for i := range out {
		out[i] = blank(m.w)
	}
	out[rows/2] = center(hintStyle.Render(note), m.w)
	return out
}

func number(n, w int) string {
	if n == 0 {
		return strings.Repeat(" ", w)
	}
	return fmt.Sprintf("%*d", w, n)
}

func (m *model) renderUnified(rows int) []string {
	all := m.unifiedRows()
	if len(all) == 0 {
		return m.noChanges(rows)
	}
	nw := len(fmt.Sprint(max(len(m.doc.Lines), len(m.doc.Base.Lines))))
	gw := 2*nw + 4 // "old new ± "
	out := make([]string, rows)
	for i := range out {
		n := m.top + i
		if n >= len(all) {
			out[i] = blank(m.w)
			continue
		}
		r := all[n]
		switch r.left.kind {
		case rowHunk:
			s := gutterStyle.Render(strings.Repeat(" ", gw)) + accentStyle.Render(r.head)
			out[i] = s + blank(m.w-lipgloss.Width(s))
		case rowCtx:
			out[i] = gutterStyle.Render(number(r.oldNo, nw)+" "+number(r.left.no, nw)+"   ") + renderLine(r.left.segs, m.left, m.w-gw)
		case rowAdd:
			out[i] = gutterStyle.Render(number(0, nw)+" "+number(r.left.no, nw)+" ") + addSign.Render("+ ") + renderLineOn(r.left.segs, m.left, m.w-gw, addTint)
		case rowDel:
			out[i] = gutterStyle.Render(number(r.oldNo, nw)+" "+number(0, nw)+" ") + delSign.Render("- ") + renderLineOn(r.left.segs, m.left, m.w-gw, delTint)
		}
	}
	return out
}

func (m *model) renderSplit(rows int) []string {
	all := m.splitRows()
	if len(all) == 0 {
		return m.noChanges(rows)
	}
	nw := len(fmt.Sprint(max(len(m.doc.Lines), len(m.doc.Base.Lines))))
	lw := (m.w - 1) / 2
	rw := m.w - 1 - lw
	half := func(s side, w int) string {
		gw := nw + 3 // "no ± "
		switch s.kind {
		case rowCtx:
			return gutterStyle.Render(number(s.no, nw)+"   ") + renderLine(s.segs, m.left, w-gw)
		case rowAdd:
			return gutterStyle.Render(number(s.no, nw)+" ") + addSign.Render("+ ") + renderLineOn(s.segs, m.left, w-gw, addTint)
		case rowDel:
			return gutterStyle.Render(number(s.no, nw)+" ") + delSign.Render("- ") + renderLineOn(s.segs, m.left, w-gw, delTint)
		}
		return blank(w)
	}
	sep := sepStyle.Render("│")
	out := make([]string, rows)
	for i := range out {
		n := m.top + i
		if n >= len(all) {
			out[i] = blank(lw) + sep + blank(rw)
			continue
		}
		r := all[n]
		if r.left.kind == rowHunk {
			s := gutterStyle.Render(strings.Repeat(" ", nw+3)) + accentStyle.Render(r.head)
			out[i] = s + blank(m.w-lipgloss.Width(s))
			continue
		}
		out[i] = half(r.left, lw) + sep + half(r.right, rw)
	}
	return out
}
