package main

import (
	"fmt"
	"net/url"
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
		return m.linkPR(arg)
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
var agentOptions = map[string]option{"model": options[2], "effort": options[3]}

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
	available := max(m.w-min(28, m.w/3)-1, 1)
	extra := 0
	for _, s := range []string{branch, pr} {
		if s != "" {
			extra += lipgloss.Width(s) + 3
		}
	}
	model, effort := m.agentParts()
	if agent := model + " · " + effort; available-extra-lipgloss.Width(agent)-3 < 12 {
		model, effort = "", ""
	} else {
		extra += lipgloss.Width(agent) + 3
	}
	path := truncate(tildify(m.workdir), max(available-extra, 1))
	x := max(m.w-lipgloss.Width(path)-extra-1, 0)
	parts := []workspacePart{{kind: "workdir", text: path, x: x}}
	x += lipgloss.Width(path)
	for _, p := range []workspacePart{{kind: "branch", text: branch}, {kind: "pr", text: pr}, {kind: "model", text: model}, {kind: "effort", text: effort, sep: " · "}} {
		if p.text != "" {
			p.x = x + 3
			parts = append(parts, p)
			x = p.x + lipgloss.Width(p.text)
		}
	}
	return parts
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
		if x >= m.w-lipgloss.Width(m.modeLabel())-1 {
			if m.viewMenu != nil {
				m.viewMenu = nil
			} else {
				m.openViewMenu()
			}
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
			}
		}
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
	left, style := " "+m.status, errStyle
	if m.statusInfo {
		style = hintStyle
	}
	if m.exitKey != "" {
		left, style = " Press "+keyNames[m.exitKey]+" again to exit", hintStyle
	} else if m.status == "" {
		left, style = " "+m.hint(), hintStyle
	}
	parts := m.workspaceParts()
	width := max(parts[0].x-1, 0)
	content := style.Render(truncate(left, width))
	if actions := m.footerActions(); len(actions) > 0 {
		var b strings.Builder
		for _, a := range actions {
			b.WriteString(blank(1) + buttonStyle.Render(" "+a.label+" "))
		}
		content = ansi.Truncate(b.String(), width, "…")
	}
	var b strings.Builder
	b.WriteString(content)
	x := lipgloss.Width(content)
	for _, p := range parts {
		if p.sep != "" && p.x-x == lipgloss.Width(p.sep) {
			b.WriteString(hintStyle.Render(p.sep))
		} else {
			b.WriteString(blank(p.x - x))
		}
		st := hintStyle
		if p.kind == "branch" {
			st = accentStyle
		}
		if p.kind == "pr" && m.repo.selectedPR() != nil {
			st = fgStyle
		}
		b.WriteString(st.Render(p.text))
		x = p.x + lipgloss.Width(p.text)
	}
	return ansi.Truncate(b.String()+blank(m.w-x), m.w, "")
}

func (m *model) footerClick(x int) bool {
	if m.perm != nil {
		return false
	}
	width := max(m.workspaceParts()[0].x-1, 0)
	at := 0
	for _, a := range m.footerActions() {
		end := at + lipgloss.Width(a.label) + 3
		if x >= at && x < end && end <= width {
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
