package main

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type repoItem struct {
	label, detail string
	pr            pullRequest
	index         int // value index, for the model and effort menus
}

type repoMenu struct {
	kind, query string
	items       []repoItem
	sel         int
}

func (mu *repoMenu) filtered() []repoItem {
	var out []repoItem
	for _, it := range mu.items {
		if strings.Contains(strings.ToLower(it.label+" "+it.detail), strings.ToLower(mu.query)) {
			out = append(out, it)
		}
	}
	return out
}

func (m *model) cmdSwitch(arg string) tea.Cmd { return m.openPicker("switch", arg) }

func (m *model) cmdPR(arg string) tea.Cmd {
	if m.repo == nil || m.repo.Branch == "" {
		m.setErr("select a Git branch to link a PR")
		return nil
	}
	if arg == "auto" {
		if m.repo.Link == "" {
			return m.refreshRepository(true)
		}
		return m.linkPR("")
	}
	if arg != "" {
		if n, err := strconv.Atoi(strings.TrimPrefix(arg, "#")); err == nil && n > 0 {
			repo := githubRepository(m.repo.Target)
			if repo == "" {
				m.setErr("use a full PR URL for this remote")
				return nil
			}
			arg = fmt.Sprintf("https://github.com/%s/pull/%d", repo, n)
		}
		u, err := url.Parse(arg)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			m.setErr("usage: /pr <URL, number, or auto>")
			return nil
		}
		return m.checkoutPR(arg)
	}
	prs := m.repo.pullRequests()
	if len(prs) == 1 || (len(prs) > 1 && m.repo.Link != "") {
		return m.openPR(*m.repo.selectedPR())
	}
	if len(prs) <= 1 {
		switch {
		case m.prLoading:
			m.flash("looking up PRs…")
		case m.repo.PRError != "":
			m.setErr(m.repo.PRError)
		default:
			m.flash("no open PR for this branch · /pr <url> to link one")
		}
		return nil
	}
	mu := &repoMenu{kind: "pr"}
	for _, p := range prs {
		detail := p.Title
		if p.BaseRefName != "" {
			detail += " → " + p.BaseRefName
		}
		if p.IsDraft {
			detail += " · draft"
		}
		if p.URL == m.repo.Link {
			detail += " · linked"
			mu.sel = len(mu.items)
		}
		mu.items = append(mu.items, repoItem{label: prLabel(p), detail: detail, pr: p})
	}
	m.repoMenu, m.menu, m.viewMenu = mu, nil, nil
	return nil
}

func prLabel(p pullRequest) string {
	if p.Number > 0 {
		return fmt.Sprintf("PR #%d", p.Number)
	}
	return "linked PR"
}

func (m *model) repoMenuKey(msg tea.KeyMsg) tea.Cmd {
	mu := m.repoMenu
	items := mu.filtered()
	n := len(items)
	switch msg.String() {
	case "esc", "ctrl+c":
		m.repoMenu = nil
	case "up", "shift+tab":
		if n > 0 {
			mu.sel = (mu.sel + n - 1) % n
		}
	case "down", "tab":
		if n > 0 {
			mu.sel = (mu.sel + 1) % n
		}
	case "enter":
		if n > 0 {
			it := items[mu.sel]
			m.repoMenu = nil
			if o, ok := agentOptions[mu.kind]; ok {
				changed := o.get(m) != it.index
				_ = o.set(m, it.index)
				m.saveOption(o, changed)
				return nil
			}
			return m.linkPR(it.pr.URL)
		}
	case "backspace":
		r := []rune(mu.query)
		if len(r) > 0 {
			mu.query = string(r[:len(r)-1])
			mu.sel = 0
		}
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			mu.query += string(msg.Runes)
			mu.sel = 0
		}
	}
	return nil
}

type workspacePart struct {
	kind, text string
	x          int
	sep        string // drawn in the gap before the part
}

// agentOptions are the settings that open as dropdowns from the status line.
var agentOptions = map[string]option{"model": optionNamed("Model"), "effort": optionNamed("Effort")}

func optionNamed(label string) option {
	for _, o := range options {
		if o.label == label {
			return o
		}
	}
	panic("no option " + label)
}

func (m *model) cmdAgentMenu(kind string) tea.Cmd {
	o := agentOptions[kind]
	mu := &repoMenu{kind: kind, sel: o.get(m)}
	for i, name := range o.values(m) {
		mu.items = append(mu.items, repoItem{label: name, index: i})
	}
	m.repoMenu, m.menu, m.viewMenu = mu, nil, nil
	return nil
}

func (m *model) workspaceParts() []workspacePart {
	branch, pr := "", ""
	if r := m.repo; r != nil {
		branch = r.Branch
		if branch == "" {
			branch = "detached @" + r.Head
		}
		if m.repoChanging {
			branch += "…"
		}
		if r.Branch != "" {
			pr = "no PR"
			if p := r.selectedPR(); p != nil {
				pr = prLabel(*p)
				if len(r.pullRequests()) > 1 {
					pr += " ▾"
				}
			} else if m.prLoading {
				pr = "PR…"
			} else if r.PRError != "" {
				pr = "PR unavailable"
			}
		}
	}
	branch = truncate(branch, max(m.w/4, 4))
	pr = truncate(pr, max(m.w/5, 4))
	rightExtra := 0 // the branch and the PR, with the gaps before them
	for _, s := range []string{branch, pr} {
		if s != "" {
			rightExtra += lipgloss.Width(s) + 3
		}
	}
	// Model and effort sit on the left; they give way to the path when the
	// window is narrow, and the text between the two blocks is last to be served.
	model, effort := m.agentParts()
	leftW := 1 + lipgloss.Width(model) + 3 + lipgloss.Width(effort)
	const textMin = 9 // room for "? help"
	pathMax := m.w - 1 - rightExtra - leftW - 3 - textMin
	if pathMax < 10 {
		model, effort, leftW = "", "", 0
		pathMax = m.w - 1 - rightExtra - 1
	}
	path := truncate(tildify(m.workdir), max(pathMax, 1))
	x := max(m.w-lipgloss.Width(path)-rightExtra-1, 0)
	parts := []workspacePart{{kind: "workdir", text: path, x: x}}
	x += lipgloss.Width(path)
	for _, p := range []workspacePart{{kind: "branch", text: branch}, {kind: "pr", text: pr}} {
		if p.text != "" {
			p.x = x + 3
			parts = append(parts, p)
			x = p.x + lipgloss.Width(p.text)
		}
	}
	if model != "" {
		parts = append(parts, workspacePart{kind: "model", text: model, x: 1})
		parts = append(parts, workspacePart{kind: "effort", text: effort, x: 1 + lipgloss.Width(model) + 3, sep: " · "})
	}
	return parts
}

// statusRegion is the stretch of the bottom line between the model and the
// path, where notices, the file's buttons and the "? help" hint appear.
func (m *model) statusRegion() (x0, width int) {
	x0, right := 1, m.w
	for _, p := range m.workspaceParts() {
		switch p.kind {
		case "effort":
			x0 = p.x + lipgloss.Width(p.text) + 3
		case "workdir":
			right = p.x
		}
	}
	return x0, max(right-2-x0, 0)
}

// showHint reports whether the bottom line shows its "? help" hint: nothing
// else wants the space.
func (m *model) showHint() bool {
	return m.status == "" && m.exitKey == "" && len(m.footerActions()) == 0
}

func (m *model) repoMenuLayout() (x, width int) {
	width = 30
	for _, it := range m.repoMenu.items {
		width = max(width, lipgloss.Width(it.label+it.detail)+6)
	}
	width = min(width, max(m.w-2, 1), 72)
	x = 1
	for _, p := range m.workspaceParts() {
		if p.kind == m.repoMenu.kind {
			x = p.x
		}
	}
	return min(x, max(m.w-width-1, 0)), width
}

func (m *model) renderRepoMenu() []string {
	mu := m.repoMenu
	if mu == nil {
		return nil
	}
	items := mu.filtered()
	n := min(menuRows, len(items))
	top := max(mu.sel-n+1, 0)
	_, width := m.repoMenuLayout()
	panel := bg.Background(lipgloss.Color(cur.Panel))
	row := func(s string, selected bool) string {
		st := panel.Foreground(lipgloss.Color(cur.Fg))
		if selected {
			st = st.Foreground(lipgloss.Color(cur.Accent)).Bold(true)
		}
		s = truncate(s, max(width-1, 1))
		return st.Render(s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0)))
	}
	title := map[string]string{"pr": "Pull requests", "model": "Model", "effort": "Effort"}[mu.kind]
	if title == "" {
		title = "Branches"
	}
	out := []string{row(" "+title+" · type to filter: "+mu.query, false)}
	for i := top; i < top+n; i++ {
		mark := "   "
		if i == mu.sel {
			mark = " › "
		}
		out = append(out, row(mark+items[i].label+"   "+items[i].detail, i == mu.sel))
	}
	if n == 0 {
		out = append(out, row("   No matches", false))
	}
	return out
}

func (m *model) repoMenuTop() int {
	rows := 1 + max(1, min(menuRows, len(m.repoMenu.filtered())))
	return max(m.h-1-rows, 0)
}

func (m *model) workspaceClick(x, y int) (bool, tea.Cmd) {
	if y == toolbarRow {
		if m.mode == modePick && m.picker != nil && m.picker.kind == "prdiff" {
			right := m.w - lipgloss.Width(m.modeLabel()) - 2 // the gap before the view-mode button
			sx, _ := m.submitMenuLayout()
			if x >= sx && x < sx+lipgloss.Width(m.submitButton()) {
				if m.submitMenu != nil {
					m.submitMenu = nil
				} else {
					m.openSubmitMenu()
				}
				return true, nil
			}
			if x < right && x >= right-lipgloss.Width(m.listButton()) {
				if m.listMenu != nil {
					m.listMenu = nil
				} else {
					m.openListMenu()
				}
				return true, nil
			}
		}
		if x >= m.w-lipgloss.Width(m.modeLabel())-1 {
			if m.viewMenu != nil {
				m.viewMenu = nil
			} else {
				m.openViewMenu()
			}
			return true, nil
		}
	}
	if y == m.h-1 && m.showHint() && m.w > 0 {
		if x0, width := m.statusRegion(); x >= x0 && x < x0+min(lipgloss.Width(helpHint), width) {
			m.help = true
			return true, nil
		}
	}
	if y == m.h-1 {
		for _, p := range m.workspaceParts() {
			if x < p.x || x >= p.x+lipgloss.Width(p.text) {
				continue
			}
			if m.repoMenu != nil && m.repoMenu.kind == p.kind {
				m.repoMenu = nil
				return true, nil
			}
			m.viewMenu = nil
			switch p.kind {
			case "branch":
				return true, m.cmdSwitch("")
			case "pr":
				return true, m.cmdPR("")
			case "workdir":
				return true, m.openPicker("workdir", "")
			case "model", "effort":
				if m.perm == nil {
					return true, m.cmdAgentMenu(p.kind)
				}
				return true, nil
			case "help":
				m.help = true
				return true, nil
			}
		}
	}
	if m.submitMenu != nil {
		left, width := m.submitMenuLayout()
		if i := y - toolbarRow - 1; i >= 0 && i < len(verdicts) && x >= left && x < left+width {
			m.submitMenu.sel = i
			return true, m.submitMenuKey("enter")
		}
		m.submitMenu = nil
		return true, nil
	}
	if m.listMenu != nil {
		left, width := m.listMenuLayout()
		if i := y - toolbarRow - 1; i >= 0 && i < len(prListNames) && x >= left && x < left+width {
			m.listMenu.sel = i
			m.listMenuKey("enter")
		} else {
			m.listMenu = nil
		}
		return true, nil
	}
	if m.viewMenu != nil {
		if i := y - toolbarRow - 1; i >= 0 && i < len(viewModeNames) && x >= m.w-menuWidth()-1 {
			m.setViewMode(viewMode(i))
		} else {
			m.viewMenu = nil
		}
		return true, nil
	}
	if mu := m.repoMenu; mu != nil {
		items := mu.filtered()
		n := min(menuRows, len(items))
		first := m.repoMenuTop() + 1 // one filter row above the choices
		left, width := m.repoMenuLayout()
		if y >= first && y < first+n && x >= left && x < left+width {
			mu.sel = max(mu.sel-n+1, 0) + y - first
			return true, m.repoMenuKey(tea.KeyMsg{Type: tea.KeyEnter})
		}
		m.repoMenu = nil
		return true, nil
	}
	if y == m.h-1 {
		return m.footerClick(x), nil
	}
	return false, nil
}

// Repository menus open above the status line; the view menu opens below the header.
func (m *model) overlayMenus(view string) string {
	lines := strings.Split(view, "\n")
	put := func(x, y int, s string) {
		if y >= len(lines)-1 {
			return
		}
		width := lipgloss.Width(s)
		lines[y] = ansi.Cut(lines[y], 0, x) + s + ansi.Cut(lines[y], x+width, m.w)
	}
	if m.repoMenu != nil {
		x, _ := m.repoMenuLayout()
		for i, line := range m.renderRepoMenu() {
			put(x, m.repoMenuTop()+i, line)
		}
	}
	if m.submitMenu != nil {
		x, width := m.submitMenuLayout()
		for i, v := range verdicts {
			mark, st := "  ", bg.Background(lipgloss.Color(cur.Panel)).Foreground(lipgloss.Color(cur.Fg))
			if i == m.submitMenu.sel {
				mark, st = "› ", buttonStyle
			}
			text := truncate(mark+v.label, width)
			put(x, toolbarRow+1+i, st.Render(text+strings.Repeat(" ", max(width-lipgloss.Width(text), 0))))
		}
	}
	if m.listMenu != nil {
		x, width := m.listMenuLayout()
		for i, name := range prListNames {
			mark, st := "  ", bg.Background(lipgloss.Color(cur.Panel)).Foreground(lipgloss.Color(cur.Fg))
			if i == m.listMenu.sel {
				mark, st = "› ", buttonStyle
			}
			text := truncate(mark+name, width)
			put(x, toolbarRow+1+i, st.Render(text+strings.Repeat(" ", max(width-lipgloss.Width(text), 0))))
		}
	}
	if m.viewMenu != nil {
		width := min(menuWidth(), m.w)
		x := max(m.w-width-1, 0)
		for i, name := range viewModeNames {
			mark, st := "  ", bg.Background(lipgloss.Color(cur.Panel)).Foreground(lipgloss.Color(cur.Fg))
			if i == m.viewMenu.sel {
				mark, st = "› ", buttonStyle
			}
			text := truncate(mark+name, width)
			put(x, toolbarRow+1+i, st.Render(text+strings.Repeat(" ", max(width-lipgloss.Width(text), 0))))
		}
	}
	if m.help {
		lines = m.overlayHelp(lines)
	}
	return strings.Join(lines, "\n")
}

type footerAction struct{ label, key string }

func (m *model) footerActions() []footerAction {
	if !m.fileMode() || m.showChoices() || m.status != "" || m.exitKey != "" {
		return nil
	}
	if m.doc.Ed.On {
		save := "Save ^S"
		if m.mode == modeReview {
			save = "Accept ^S"
		}
		return []footerAction{{save, "save"}, {"Undo ^Z", "undo"}, {"Redo ^Y", "redo"}, {"Done esc", "done"}}
	}
	return []footerAction{{"Edit ↵", "edit"}, {"Save ^S", "save"}, {"Back esc", "back"}}
}

func (m *model) renderStatusLine() string {
	text, style := m.status, errStyle
	if m.statusInfo {
		style = hintStyle
	}
	if m.exitKey != "" {
		text, style = "Press "+keyNames[m.exitKey]+" again to exit", hintStyle
	}
	parts := m.workspaceParts()
	sort.SliceStable(parts, func(i, j int) bool { return parts[i].x < parts[j].x })
	x0, width := m.statusRegion()
	content := style.Render(truncate(text, width))
	switch actions := m.footerActions(); {
	case len(actions) > 0:
		var b strings.Builder
		for i, a := range actions {
			if i > 0 {
				b.WriteString(blank(1))
			}
			b.WriteString(buttonStyle.Render(" " + a.label + " "))
		}
		content = ansi.Truncate(b.String(), width, "…")
	case m.showHint() && width >= lipgloss.Width(helpHint):
		content = hintStyle.Render(helpHint)
	}
	var b strings.Builder
	x := 0
	put := func(at int, s string) {
		b.WriteString(blank(at - x))
		b.WriteString(s)
		x = max(x, at) + lipgloss.Width(s)
	}
	placed := false
	for _, p := range parts {
		if !placed && p.x > x0 {
			put(x0, content)
			placed = true
		}
		if p.sep != "" && p.x-x == lipgloss.Width(p.sep) {
			b.WriteString(hintStyle.Render(p.sep))
			x = p.x
		}
		st := hintStyle
		if p.kind == "branch" {
			st = accentStyle
		}
		if p.kind == "pr" && m.repo.selectedPR() != nil {
			st = fgStyle
		}
		put(p.x, st.Render(p.text))
	}
	return ansi.Truncate(b.String()+blank(m.w-x), m.w, "")
}

func (m *model) footerClick(x int) bool {
	if m.perm != nil {
		return false
	}
	x0, width := m.statusRegion()
	at := x0
	for _, a := range m.footerActions() {
		end := at + lipgloss.Width(a.label) + 2
		if x >= at && x < end && end-x0 <= width {
			switch a.key {
			case "save":
				m.save()
			case "edit":
				m.startEdit(-1)
			case "undo":
				m.doc.Undo()
				m.follow()
			case "redo":
				m.doc.Redo()
				m.follow()
			case "done":
				m.stopEdit()
			case "back":
				m.bodyKey("esc")
			}
			return true
		}
		at = end
	}
	return false
}
