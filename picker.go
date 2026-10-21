package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type pickItem struct {
	value, label string
	branch       gitBranch
	dir, current bool
	resolved     bool // a PR thread that is closed
	score        int
}

// workspacePicker uses the omnibox as its query and the main area as its list.
// Tab fills candidates without replacing the query that produced that list.
type workspacePicker struct {
	kind, query, dir string
	items            []pickItem
	sel, top         int
	cycling          bool
	want             string // directory to highlight after the next rebuild
	err              string
	commits          []commit // /show
	loaded           bool
	pscroll          int // preview scroll
	pkey             string
	pr               *prData // /pr
	loadErr          string
	list             string // "files" or "comments" on the left
	focus            int    // row of the preview to bring into view
	compose          *composeState
	hits             map[int][]prHit // clickable spots in the PR preview, by row
	rowLines         map[int]int     // diff line of each numbered preview row
	seq              int
}

func pickerCommand(input string) (kind, query string) {
	name, query, _ := strings.Cut(input, " ")
	switch name {
	case "/switch", "/branch":
		return "switch", query
	case "/workdir":
		return "workdir", query
	case "/open":
		return "open", query
	}
	return "", ""
}

func (m *model) openPicker(kind, query string) tea.Cmd {
	if m.perm != nil {
		m.setErr("answer the agent's request before choosing a workspace")
		return nil
	}
	m.setOmni("/" + kind + " " + query)
	m.syncPicker()
	return m.omni.Focus()
}

// syncPicker is called after an omnibox edit, including command completion.
func (m *model) syncPicker() bool {
	kind, query := pickerCommand(m.omni.Value())
	if m.mode == modePick && m.picker != nil && m.picker.bare() {
		kind, query = m.picker.kind, m.omni.Value() // the command was entered; the omnibox now searches
	}
	if kind == "" || m.perm != nil {
		if m.picker != nil && m.mode == modePick {
			m.closePicker(false)
		}
		return false
	}
	if m.mode != modePick {
		m.push()
		m.mode = modePick
		m.picker = nil
	}
	if m.picker == nil || m.picker.kind != kind {
		m.picker = &workspacePicker{kind: kind}
	}
	p := m.picker
	if p.query != query {
		p.sel, p.top = 0, 0
	}
	p.query, p.cycling = query, false
	m.menu, m.repoMenu, m.viewMenu = nil, nil, nil
	m.status, m.exitKey = "", ""
	m.omni.Focus()
	m.rebuildPicker()
	return true
}

func (m *model) closePicker(clear bool) {
	if m.mode == modePick {
		m.pop()
	}
	m.listMenu = nil
	m.picker = nil
	if clear {
		m.omni.SetValue("")
		m.menu = nil
	}
	if m.fileMode() && m.doc.Ed.On {
		m.omni.Blur()
	}
}

func (m *model) rebuildPicker() {
	p := m.picker
	if p == nil {
		return
	}
	selected := ""
	if p.sel < len(p.items) {
		selected = p.items[p.sel].value
	}
	p.items, p.err = nil, ""
	if p.kind == "switch" {
		p.dir = m.workdir
		if m.repo == nil {
			p.err = "Workdir is not in a Git repository"
			if m.repoLoading {
				p.err = "Reading Git branches…"
			}
		} else {
			for _, b := range m.repo.Branches {
				score, ok := fuzzyScore(p.query, b.Ref)
				if !ok {
					continue
				}
				current := !b.Remote && b.Name == m.repo.Branch
				if current {
					if p.query == "" {
						score++
					}
				}
				p.items = append(p.items, pickItem{value: b.Ref, label: b.Ref, branch: b, score: score, current: current})
			}
		}
	} else if p.kind == "prdiff" {
		p.dir = m.workdir
		if d := p.pr; d == nil {
			p.err = p.loadErr
		} else {
			p.prItems(d)
		}
	} else if p.kind == "show" {
		p.dir = m.workdir
		if !p.loaded {
			p.loaded = true
			var err error
			if p.commits, err = loadCommits(m.workdir); err != nil {
				p.err = "No history here · " + err.Error()
			}
		}
		for _, c := range p.commits {
			score, ok := fuzzyScore(p.query, c.Short+" "+c.Subject+" "+c.Author)
			if ok {
				p.items = append(p.items, pickItem{value: c.Hash, label: c.Short + "  " + c.Subject, score: score})
			}
		}
	} else {
		m.directoryChoices()
	}
	sort.SliceStable(p.items, func(i, j int) bool {
		a, b := p.items[i], p.items[j]
		if a.score != b.score {
			return a.score > b.score
		}
		return a.dir && !b.dir
	})
	p.sel = min(p.sel, max(len(p.items)-1, 0))
	if p.want != "" {
		for i, it := range p.items {
			if it.value == p.want {
				p.sel = i
				break
			}
		}
		p.want = ""
	} else if p.cycling { // keep a Tab cycle stable when background Git metadata refreshes
		for i, it := range p.items {
			if it.value == selected {
				p.sel = i
				break
			}
		}
	}
	m.movePicker(0, false)
}

func (m *model) directoryChoices() {
	p := m.picker
	query := p.query
	if query == "~" {
		query = "~/"
	}
	parent, needle := filepath.Split(query)
	p.dir = workPath(m.workdir, parent)
	seen := map[string]bool{}
	add := func(path, label, match string, isDir bool) {
		path = filepath.Clean(path)
		if seen[path] {
			return
		}
		score, ok := fuzzyScore(needle, match)
		if !ok {
			return
		}
		seen[path] = true
		p.items = append(p.items, pickItem{value: path, label: label, score: score, dir: isDir, current: path == m.workdir})
	}
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		p.err = err.Error()
		return
	}
	for _, de := range entries {
		name := de.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(needle, ".") {
			continue
		}
		path := filepath.Join(p.dir, name)
		isDir := de.IsDir()
		if de.Type()&os.ModeSymlink != 0 {
			fi, err := os.Stat(path)
			isDir = err == nil && fi.IsDir()
		}
		switch {
		case isDir:
			add(path, name+"/", name, true)
		case p.kind == "open":
			add(path, name, name, false)
		}
	}
}

// fuzzyScore matches a case-insensitive subsequence. Consecutive letters and
// word/path boundaries rank above scattered matches; no extra dependency needed.
func fuzzyScore(query, value string) (int, bool) {
	var q []rune
	for _, r := range strings.ToLower(strings.TrimSpace(query)) {
		if !unicode.IsSpace(r) {
			q = append(q, r)
		}
	}
	if len(q) == 0 {
		return 0, true
	}
	v := []rune(strings.ToLower(value))
	at, last, score := 0, -2, 0
	for _, want := range q {
		for at < len(v) && v[at] != want {
			at++
		}
		if at == len(v) {
			return 0, false
		}
		score += 10
		if at == last+1 {
			score += 20
		}
		if at == 0 || strings.ContainsRune("/_.- ", v[at-1]) {
			score += 15
		}
		score -= at - last - 1
		last, at = at, at+1
	}
	if strings.HasPrefix(string(v), string(q)) {
		score += 40
	}
	return score - len(v) + len(q), true
}

func (m *model) movePicker(delta int, wrap bool) {
	p := m.picker
	n := len(p.items)
	if n == 0 {
		p.sel, p.top = 0, 0
		return
	}
	if wrap {
		p.sel = (p.sel + delta%n + n) % n
	} else {
		p.sel = min(max(p.sel+delta, 0), n-1)
	}
	rows := m.bodyRows()
	p.top = min(max(p.top, p.sel-rows+1), p.sel)
}

func (m *model) choosePicker() tea.Cmd {
	p := m.picker
	switch p.kind {
	case "open":
		return m.chooseFile()
	case "show", "prdiff":
		return nil
	}
	if m.repoChanging || len(p.items) == 0 || !m.canSwitchWorkspace() {
		return nil
	}
	it, kind := p.items[p.sel], p.kind
	m.hist.add("/" + kind + " " + it.value)
	m.closePicker(true)
	m.status = ""
	if kind == "switch" {
		return m.switchBranch(it.branch)
	}
	return m.cmdWorkdir(it.value)
}

// chooseFile enters a directory or opens a file. With no match, a typed name is created.
func (m *model) chooseFile() tea.Cmd {
	p := m.picker
	if len(p.items) == 0 {
		if p.err != "" || strings.TrimSpace(p.query) == "" {
			return nil
		}
		m.hist.add("/open " + p.query)
		m.closePicker(true)
		m.openPath(workPath(m.workdir, p.query))
		return nil
	}
	it := p.items[p.sel]
	if it.dir {
		m.browse(it.value, "")
		return nil
	}
	m.hist.add("/open " + it.value)
	m.closePicker(true)
	m.openPath(it.value)
	return nil
}

func (m *model) pickerKey(msg tea.KeyMsg) tea.Cmd {
	p := m.picker
	if p.kind == "prdiff" {
		if p.compose != nil {
			return m.composeKey(msg)
		}
		switch msg.String() {
		case "ctrl+r":
			return m.startCompose("reply")
		case "ctrl+n":
			return m.startCompose("new")
		case "ctrl+a":
			if len(p.items) == 0 {
				return nil
			}
			if a, b, ok := strings.Cut(p.items[p.sel].value, ":"); ok {
				return m.applyThread(atoi(a), atoi(b))
			}
			m.flash("select a thread in the Comments list, or click Apply on it")
			return nil
		case "ctrl+e":
			if len(p.items) == 0 {
				return nil
			}
			if a, b, ok := strings.Cut(p.items[p.sel].value, ":"); ok {
				fi, ti := atoi(a), atoi(b)
				if p.pr != nil && fi < len(p.pr.Files) && ti < len(p.pr.Files[fi].Threads) {
					return m.toggleThread(fi, ti)
				}
			}
			return nil
		}
	}
	switch msg.String() {
	case "esc", "ctrl+c":
		m.closePicker(true)
		return nil
	case "enter":
		return m.choosePicker()
	case "up", "down", "pgup", "pgdown":
		d := map[string]int{"up": -1, "down": 1, "pgup": -m.bodyRows(), "pgdown": m.bodyRows()}[msg.String()]
		m.movePicker(d, false)
		if p.cycling && len(p.items) > 0 {
			m.setOmni("/" + p.kind + " " + p.items[p.sel].value)
		}
		return nil
	case "tab", "shift+tab":
		if len(p.items) == 0 {
			return nil
		}
		d := 1
		if msg.String() == "shift+tab" {
			d = -1
		}
		if p.cycling || d < 0 {
			m.movePicker(d, true)
		}
		if p.bare() {
			m.movePicker(d, true)
			return nil
		}
		p.cycling = true
		m.setOmni("/" + p.kind + " " + p.items[p.sel].value)
		return nil
	case "ctrl+l":
		if p.kind == "prdiff" {
			m.setPRList(map[string]string{"files": "comments", "comments": "files"}[p.list])
			return nil
		}
	case "shift+up", "shift+down":
		p.pscroll += 3 * map[string]int{"shift+up": -1, "shift+down": 1}[msg.String()]
		return nil
	case "right", "alt+right":
		if p.browsable() && len(p.items) > 0 && m.omniAtEnd() {
			if it := p.items[p.sel]; it.dir {
				m.browse(it.value, "")
			} else {
				return m.chooseFile()
			}
			return nil
		}
	case "left", "alt+left":
		if p.browsable() && m.omniAtEnd() && (msg.String() == "alt+left" || p.query == "" || strings.HasSuffix(p.query, "/")) {
			if up := filepath.Dir(p.dir); up != p.dir {
				m.browse(up, p.dir)
			}
			return nil
		}
	}
	before := m.omni.Value()
	var cmd tea.Cmd
	m.omni, cmd = m.omni.Update(msg)
	if m.omni.Value() != before {
		p.sel, p.top, p.cycling = 0, 0, false
		m.suggest(false)
	}
	return cmd
}

// bare screens are opened by their command alone; the omnibox then only searches.
func (p *workspacePicker) bare() bool { return p.kind == "show" || p.kind == "prdiff" }

func (m *model) openBare(kind string) (*workspacePicker, tea.Cmd) {
	m.setOmni("")
	if m.mode != modePick {
		m.push()
		m.mode = modePick
	}
	m.picker = &workspacePicker{kind: kind}
	m.menu, m.repoMenu, m.viewMenu = nil, nil, nil
	m.status, m.exitKey = "", ""
	return m.picker, m.omni.Focus()
}

// cmdShow opens the history screen; the omnibox is then its search field.
func (m *model) cmdShow(string) tea.Cmd {
	if m.perm != nil {
		m.setErr("answer the agent's request before browsing history")
		return nil
	}
	_, focus := m.openBare("show")
	m.rebuildPicker()
	return focus
}

var prSeq int

// openPR reads the PR from GitHub and shows its files, diff and comments.
func (m *model) openPR(pr pullRequest) tea.Cmd {
	if m.perm != nil {
		m.setErr("answer the agent's request before reading a PR")
		return nil
	}
	if repo, _ := parsePRURL(pr.URL); repo == "" {
		m.note(pr.Title + "\n" + pr.URL)
		m.flash("only github.com PRs can be read here · " + pr.URL)
		return nil
	}
	p, focus := m.openBare("prdiff")
	prSeq++
	p.seq, p.loadErr, p.list = prSeq, "Loading "+prLabel(pr)+"…", "files"
	m.rebuildPicker()
	dir, url, seq := m.repo.Root, pr.URL, p.seq
	return tea.Batch(focus, func() tea.Msg {
		d, err := fetchPR(dir, url)
		return prDiffMsg{seq, d, err}
	})
}

func (m *model) onPRDiff(msg prDiffMsg) tea.Cmd {
	p := m.picker
	if p == nil || p.kind != "prdiff" || p.seq != msg.seq {
		return nil
	}
	if msg.err != nil {
		p.loadErr = msg.err.Error()
	} else {
		p.pr = msg.data
	}
	m.rebuildPicker()
	return nil
}

func (p *workspacePicker) browsable() bool { return p.kind == "workdir" || p.kind == "open" }

func (m *model) omniAtEnd() bool { return m.omni.Position() >= len([]rune(m.omni.Value())) }

// browse lists dir's subdirectories and highlights focus, so Left then Enter picks the directory just left.
func (m *model) browse(dir, focus string) {
	m.setOmni("/" + m.picker.kind + " " + strings.TrimSuffix(dir, string(filepath.Separator)) + string(filepath.Separator))
	m.syncPicker()
	m.picker.want = focus
	m.rebuildPicker()
}

func (m *model) renderPicker(rows int) []string {
	p := m.picker
	m.movePicker(0, false)
	title := "Switch branch"
	switch p.kind {
	case "workdir":
		title = "Choose workdir"
	case "open":
		title = "Open file"
	case "show":
		title = "Branch history"
	case "prdiff":
		title = "Pull request"
		if p.pr != nil {
			title = fmt.Sprintf("PR #%d", p.pr.Number)
		}
	}
	info := fmt.Sprintf("%s · %d matches", tildify(p.dir), len(p.items))
	if p.compose != nil {
		info = "writing a comment"
	}
	out := m.header(fgStyle.Bold(true).Render(title), info, "")
	lw := m.pickerListWidth()
	var left []string
	if len(p.items) == 0 {
		note := "No matches · keep typing or edit the query"
		if p.err != "" {
			note = p.err
		}
		left = append(left, hintStyle.Render(truncate("  "+note, lw)))
	}
	for i := p.top; i < len(p.items) && len(left) < rows; i++ {
		it := p.items[i]
		mark, st := "  ", fgStyle
		switch {
		case it.current:
			st = bg.Foreground(lipgloss.Color(cur.Accent2))
		case it.dir:
			st = accentStyle
		case it.branch.Remote, it.resolved:
			st = hintStyle
		case strings.HasPrefix(it.label, "."):
			st = hintStyle
		}
		if i == p.sel {
			mark, st = "› ", st.Background(lipgloss.Color(cur.Sel)).Bold(true)
		}
		text := truncate(" "+mark+it.label, lw)
		left = append(left, st.Render(text+strings.Repeat(" ", max(lw-lipgloss.Width(text), 0))))
	}
	for len(left) < rows {
		left = append(left, blank(lw))
	}
	if lw >= m.w {
		return append(out, left...)
	}
	right := m.previewPane(m.w-lw-1, rows)
	for i := range left {
		out = append(out, left[i]+sepStyle.Render("│")+right[i])
	}
	return out
}

// pickerListWidth leaves the right side to the preview when the window is wide enough.
func (m *model) pickerListWidth() int {
	need := 100
	if m.picker != nil && m.picker.kind == "prdiff" {
		need = 70 // the diff is the point of this screen
	}
	if m.w < need {
		return m.w
	}
	return max(m.w*2/5, 40)
}

// pickerPreview is a read-only look at the highlighted item.
func (m *model) pickerPreview() previewLines {
	p := m.picker
	if len(p.items) == 0 {
		return nil
	}
	it := p.items[p.sel]
	switch p.kind {
	case "switch":
		return cachedPreview("log:"+m.workdir+":"+it.branch.Ref, previewTTL, func() previewLines { return branchPreview(m.workdir, it.branch.Ref) })
	case "prdiff":
		return p.prPreview(it.value, m.w-m.pickerListWidth()-2)
	case "show":
		return cachedPreview("commit:"+m.workdir+":"+it.value, 0, func() previewLines { return commitPreview(m.workdir, it.value) })
	}
	key, load := pathPreview(it.value)
	if key == "" {
		return load()
	}
	return cachedPreview(key, 0, load)
}

func (m *model) previewPane(width, rows int) []string {
	p := m.picker
	lines := m.pickerPreview()
	if k := p.kind + "\x00"; len(p.items) > 0 {
		if k += p.items[p.sel].value; k != p.pkey {
			p.pkey, p.pscroll = k, max(p.focus-2, 0)
		}
	}
	p.pscroll = min(max(p.pscroll, 0), max(len(lines)-rows, 0))
	out := make([]string, rows)
	for i := range out {
		if n := p.pscroll + i; n < len(lines) {
			if raw := lines[n]; len(raw) == 1 && raw[0].tok == tokRaw {
				text := ansi.Truncate(raw[0].text, width-1, "")
				out[i] = text + blank(width-lipgloss.Width(text))
			} else if c := p.compose; c != nil && c.kind == "line" && c.line != 0 && p.rowLines[n] >= c.start && p.rowLines[n] <= c.line {
				// the lines a comment being written will apply to
				tint := lipgloss.Color(cur.Sel)
				out[i] = bg.Background(tint).Render(" ") + renderLineOn(raw, 0, width-1, tint, -1)
			} else {
				out[i] = bg.Render(" ") + renderLine(raw, 0, width-1)
			}
		} else {
			out[i] = blank(width)
		}
	}
	return out
}

// prItems fills the left list of a PR screen: files, or comments.
func (p *workspacePicker) prItems(d *prData) {
	if p.list != "comments" {
		if score, ok := fuzzyScore(p.query, "conversation"); ok {
			p.items = append(p.items, pickItem{value: "", label: "Conversation", score: score, current: true})
		}
		for _, f := range d.Files {
			score, ok := fuzzyScore(p.query, f.Path)
			if !ok {
				continue
			}
			label := f.Path
			if n := len(f.Threads); n > 0 {
				label += fmt.Sprintf(" [%d]", n)
			}
			p.items = append(p.items, pickItem{value: f.Path, label: label, score: score})
		}
		return
	}
	if score, ok := fuzzyScore(p.query, "conversation"); ok {
		p.items = append(p.items, pickItem{value: "", label: "Conversation", score: score, current: true})
	}
	for fi, f := range d.Files {
		for ti, t := range f.Threads {
			line := "outdated"
			if t.Line > 0 {
				line = fmt.Sprint(t.Line)
			}
			label := fmt.Sprintf("%s:%s  %s", filepath.Base(f.Path), line, t.summary())
			var all strings.Builder
			for _, c := range t.Comments {
				all.WriteString(" " + c.Body)
			}
			if score, ok := fuzzyScore(p.query, f.Path+all.String()); ok {
				p.items = append(p.items, pickItem{value: fmt.Sprintf("%d:%d", fi, ti), label: label, score: score, resolved: t.Resolved})
			}
		}
	}
	if len(p.items) == 0 && p.query == "" {
		p.err = "No comments here"
	}
}

// listButton is the header button that picks what the left list shows; it
// sits left of the view-mode button. Clicking it, or ctrl+l, opens the menu.
func (m *model) listButton() string {
	p := m.picker
	label := "Files"
	if p.pr != nil {
		n := 0
		for _, f := range p.pr.Files {
			n += len(f.Threads)
		}
		if p.list == "comments" {
			label = fmt.Sprintf("Comments [%d]", n)
		} else {
			label = fmt.Sprintf("Files [%d]", len(p.pr.Files))
		}
	}
	st := buttonStyle
	if m.listMenu != nil {
		st = buttonOpen
	}
	return st.Render(" " + label + " ▾ ")
}

var prListNames = []string{"Files", "Comments"}

// setPRList switches the left list of the PR screen between files and comments.
func (m *model) setPRList(list string) {
	p := m.picker
	if list == "" || list == p.list {
		return
	}
	p.list, p.sel, p.top = list, 0, 0
	m.listMenu = nil
	m.rebuildPicker()
}

// openListMenu drops the Files/Comments menu below its header button.
func (m *model) openListMenu() {
	m.repoMenu, m.menu, m.viewMenu = nil, nil, nil
	sel := 0
	if m.picker.list == "comments" {
		sel = 1
	}
	m.listMenu = &viewMenu{sel: sel}
}

func (m *model) listMenuKey(k string) {
	n := len(prListNames)
	switch k {
	case "up", "shift+tab", "down", "tab":
		m.listMenu.sel = (m.listMenu.sel + 1) % n // two entries: any move toggles
	case "enter", " ":
		m.setPRList([]string{"files", "comments"}[m.listMenu.sel])
		m.listMenu = nil
	case "esc", "ctrl+l":
		m.listMenu = nil
	}
}

// listMenuLayout is where the menu opens: under the list button, left of the view-mode one.
func (m *model) listMenuLayout() (x, width int) {
	width = 4
	for _, n := range prListNames {
		width = max(width, lipgloss.Width(n)+4)
	}
	right := m.w - lipgloss.Width(m.modeLabel()) - 2
	return max(right-lipgloss.Width(m.listButton()), 0), width
}
