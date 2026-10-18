// Command v is a terminal tool for coding together with an AI agent.
package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type mode int

const (
	modeSplash mode = iota // until the first input
	modeChat
	modeView
	modeReview // an agent's proposed edit, waiting for y / n
	modeBrowse
	modeConfig
)

type model struct {
	w, h         int
	mode         mode
	omni         textinput.Model
	menu         *menu
	dismissed    string // omnibox value the menu was closed with Esc for
	tagline      int
	status       string // transient error; replaces the hint until the next Enter
	statusInfo   bool   // status is a notice, not an error
	discardArmed bool   // Esc once more leaves a file with unsaved changes
	where        string // "branch · ~/path" on the right of the status line

	doc          *Doc
	top, left    int
	browser      *Browser
	log          []message
	settings     Settings
	cfg          *configScreen
	agent        *Agent      // nil until the first prompt
	agentReady   bool        // got the init message
	agentStale   bool        // model or effort changed: restart before the next prompt
	sessionID    string      // Claude Code session, for --resume
	busy         bool        // a turn is running
	interrupted  bool        // the running turn was interrupted
	showThinking bool        // Ctrl-O: thinking blocks expanded in the chat
	perm         *permission // tool call waiting for y/n
	choice       int         // highlighted answer in the choice list
	queue        []string    // prompts typed while the agent was busy

	listen tea.Cmd // listens to the agent started at launch

	vmode    viewMode  // flat or diff, kept across files
	rmode    viewMode  // the same for reviews of agent edits
	viewMenu *viewMenu // open view-mode dropdown
	stack    []screen  // screens to return to with Esc

	exitKey string // "ctrl+c" or "ctrl+d" after the first press; a second one quits
	exitSeq int    // invalidates stale disarm ticks
}

func newModel() model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = placeholder
	ti.Focus()
	m := model{omni: ti, tagline: rand.IntN(len(taglines)), where: where(), rmode: viewUnified}
	m.styleOmni()
	return m
}

// styleOmni applies the active theme to the omnibox.
func (m *model) styleOmni() {
	m.omni.PromptStyle = accentStyle
	m.omni.TextStyle = fgStyle
	m.omni.PlaceholderStyle = hintStyle
	m.omni.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(cur.Fg))
}

// where describes the working directory: git branch (if any) and path with ~.
func where() string {
	dir, _ := os.Getwd()
	dir = tildify(dir)
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if branch := strings.TrimSpace(string(out)); err == nil && branch != "" {
		return branch + " · " + dir
	}
	return dir
}

func (m *model) setErr(s string) { m.status, m.statusInfo = s, false }

// flash shows a notice in the status line until the next Enter.
func (m *model) flash(s string) { m.status, m.statusInfo = s, true }

// exitWindow is how long a second Ctrl-C or Ctrl-D still quits.
const exitWindow = time.Second

type disarmMsg int

var keyNames = map[string]string{"ctrl+c": "Ctrl-C", "ctrl+d": "Ctrl-D"}

func (m model) Init() tea.Cmd { return tea.Batch(textinput.Blink, m.listen) }

const placeholder = "type / for commands"

// fileMode reports whether the main area shows a file: opened or under review.
func (m *model) fileMode() bool { return m.mode == modeView || m.mode == modeReview }

// footerRows is the height under the omnibox: the menu, the choices for a
// pending tool call, or the status line.
func (m model) footerRows() int {
	if m.showChoices() {
		return len(m.choices()) + 1
	}
	if m.menu != nil {
		return min(len(m.menu.items), menuRows)
	}
	return 1
}

// bodyRows is the height of the main area: everything except the header
// (view and browse modes), the two lines around the omnibox, and the footer.
func (m model) bodyRows() int {
	rows := m.h - 3 - m.footerRows()
	if m.fileMode() || m.mode == modeBrowse || m.mode == modeConfig {
		rows -= headerRows
	}
	return max(rows, 1)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.omni.Width = m.w - 4
		return m, nil
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.fileMode() {
			m.viewClick(msg.X, msg.Y)
			return m, nil
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.bodyKey("up")
		case tea.MouseButtonWheelDown:
			m.bodyKey("down")
		}
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	case agentEvent:
		if msg.a != m.agent {
			return m, nil // from a session that /reset closed
		}
		m.onAgent(msg.msg)
		return m, m.agent.wait()
	case agentClosedMsg:
		if msg.a == m.agent {
			m.note("Claude Code exited")
			if msg.stderr != "" {
				m.note(msg.stderr)
			}
			m.agent, m.busy, m.perm, m.agentReady = nil, false, nil, false
		}
		return m, nil
	case disarmMsg:
		if int(msg) == m.exitSeq {
			m.exitKey = ""
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.omni, cmd = m.omni.Update(msg)
	return m, cmd
}

func (m model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	empty := m.omni.Value() == ""
	if m.fileMode() {
		switch {
		case m.viewMenu != nil:
			m.viewMenuKey(k)
			return m, nil
		case k == "ctrl+t":
			m.openViewMenu()
			return m, nil
		case m.doc.Ed.On && k != "ctrl+c" && k != "ctrl+o":
			m.exitKey = ""
			return m.editKey(msg)
		}
	}
	// A pending tool call: pick from the choice list with ↑↓ and Enter, or
	// press its key.
	if m.perm != nil && empty && m.viewMenu == nil {
		choices := m.choices()
		switch k {
		case "up", "shift+tab":
			m.choice = (m.choice + len(choices) - 1) % len(choices)
			return m, nil
		case "down", "tab":
			m.choice = (m.choice + 1) % len(choices)
			return m, nil
		case "enter":
			m.choose(choices[m.choice].key)
			return m, nil
		case "esc":
			m.choose("n")
			return m, nil
		}
		for _, c := range choices {
			if k == c.key {
				m.choose(k)
				return m, nil
			}
		}
	}
	if k == "ctrl+o" {
		m.showThinking = !m.showThinking
		return m, nil
	}
	if m.busy && empty && (k == "esc" || k == "ctrl+c") && m.mode == modeChat {
		if err := m.agent.Interrupt(); err != nil {
			m.setErr(err.Error())
		}
		m.note("interrupted")
		m.interrupted = true
		return m, nil
	}
	switch {
	case k == "ctrl+c" && (m.omni.Value() != "" || m.menu != nil):
		// Like Claude Code: Ctrl-C first clears the input.
		m.omni.SetValue("")
		m.menu, m.exitKey = nil, ""
		return m, nil
	case k == "ctrl+c" || (k == "ctrl+d" && m.omni.Value() == ""):
		if m.exitKey == k {
			return m, tea.Quit
		}
		m.exitKey = k
		m.exitSeq++
		seq := m.exitSeq
		return m, tea.Tick(exitWindow, func(time.Time) tea.Msg { return disarmMsg(seq) })
	}
	m.exitKey = ""
	if k == "tab" && m.fileMode() && m.omni.Value() == "" {
		m.openViewMenu()
		return m, nil
	}
	if m.menu != nil {
		switch k {
		case "tab", "down":
			m.moveSel(1)
			return m, nil
		case "shift+tab", "up":
			m.moveSel(-1)
			return m, nil
		case "enter":
			cmd := m.accept()
			return m, cmd
		case "esc":
			m.menu, m.dismissed = nil, m.omni.Value()
			return m, nil
		}
	}
	switch k {
	case "tab":
		m.tab(1)
		return m, nil
	case "shift+tab":
		m.tab(-1)
		return m, nil
	case "enter":
		if m.omni.Value() == "" {
			m.bodyKey(k)
			return m, nil
		}
		v := m.omni.Value()
		m.omni.SetValue("")
		m.status, m.dismissed = "", ""
		return m, m.run(v)
	}
	// With an empty omnibox, navigation keys drive the main area.
	if m.omni.Value() == "" && m.bodyKey(k) {
		return m, nil
	}
	before := m.omni.Value()
	var cmd tea.Cmd
	m.omni, cmd = m.omni.Update(msg)
	if m.omni.Value() != before {
		m.suggest(false)
	}
	return m, cmd
}

// bodyKey handles a navigation key in the main area; it reports whether the key was used.
func (m *model) bodyKey(k string) bool {
	rows := m.bodyRows()
	switch m.mode {
	case modeSplash:
		if k == "esc" {
			m.mode = modeChat
			return true
		}
	case modeConfig:
		return m.configKey(k)
	case modeBrowse:
		b := m.browser
		var err error
		switch k {
		case "up":
			b.Move(-1, rows)
		case "down":
			b.Move(1, rows)
		case "pgup":
			b.Move(-rows+1, rows)
		case "pgdown":
			b.Move(rows-1, rows)
		case "home":
			b.Move(-1<<30, rows)
		case "end":
			b.Move(1<<30, rows)
		case "left", "backspace":
			err = b.Up()
		case "right", "enter":
			var path string
			if path, err = b.Enter(); err == nil && path != "" {
				m.openFile(path)
			}
		case "esc":
			m.pop()
		default:
			return false
		}
		if err != nil {
			m.setErr(err.Error())
		}
		return true
	case modeView, modeReview:
		switch k {
		case "up":
			m.scroll(-1, 0)
		case "down":
			m.scroll(1, 0)
		case "pgup":
			m.scroll(-rows+1, 0)
		case "pgdown":
			m.scroll(rows-1, 0)
		case "left":
			m.scroll(0, -4)
		case "right":
			m.scroll(0, 4)
		case "home":
			m.top, m.left = 0, 0
		case "end":
			m.scroll(1<<30, 0)
		case "enter":
			m.startEdit(-1)
		case "ctrl+s":
			m.save()
		case "esc":
			if m.doc.Ed.Dirty && m.doc.Review == "" && !m.discardArmed {
				m.setErr("unsaved changes · ctrl+s save · esc again to discard")
				m.discardArmed = true
				return true
			}
			m.discardArmed = false
			m.status = ""
			m.pop()
		default:
			return false
		}
		return true
	}
	return false
}

func (m *model) scroll(dy, dx int) {
	m.top = min(max(m.top+dy, 0), max(m.rowCount()-m.bodyRows(), 0))
	m.left = max(m.left+dx, 0)
}

func (m model) View() string {
	if m.w == 0 {
		return ""
	}
	var b strings.Builder
	line := func(s string) {
		b.WriteString(s)
		b.WriteByte('\n')
	}
	pad := func(s string, style lipgloss.Style) string {
		return s + style.Render(strings.Repeat(" ", max(m.w-lipgloss.Width(s), 0)))
	}

	rows := m.bodyRows()
	switch m.mode {
	case modeSplash:
		for _, l := range splash(taglines[m.tagline], m.w, rows) {
			line(l)
		}
	case modeChat:
		for _, l := range m.renderLog(rows) {
			line(l)
		}
	case modeBrowse:
		for _, l := range m.header(pathTitle(m.browser.dir, true), fmt.Sprintf("%d items", len(m.browser.entries)), "") {
			line(l)
		}
		for _, l := range m.browser.Render(m.w, rows) {
			line(l)
		}
	case modeConfig:
		for _, l := range m.header(fgStyle.Bold(true).Render("Settings"), tildify(settingsPath()), "") {
			line(l)
		}
		for _, l := range m.renderConfig(rows) {
			line(l)
		}
	case modeView, modeReview:
		for _, l := range m.renderView(rows) {
			line(l)
		}
	}

	// Omnibox between two lines, like Claude Code's input; footer below.
	sep := sepStyle.Render(strings.Repeat("─", m.w))
	line(sep)
	line(pad(m.omni.View(), bg))
	line(sep)
	if m.menu != nil {
		b.WriteString(strings.Join(m.renderMenu(m.w), "\n"))
		return b.String()
	}
	if m.showChoices() {
		b.WriteString(strings.Join(m.renderChoices(), "\n"))
		b.WriteByte('\n')
	}
	left, style := " "+m.status, errStyle
	if m.statusInfo {
		style = hintStyle
	}
	if m.exitKey != "" {
		left, style = " Press "+keyNames[m.exitKey]+" again to exit", hintStyle
	} else if m.status == "" {
		left, style = " "+m.hint(), hintStyle
	}
	// The model always fits; the branch and path go first when space is short.
	agent := modelName(m.settings.Model) + " · " + m.settings.Effort + " "
	right := agent
	if full := m.where + " · " + agent; lipgloss.Width(left)+lipgloss.Width(full) < m.w {
		right = full
	}
	if lipgloss.Width(right) >= m.w/2 {
		right = ""
	}
	left = runewidth.Truncate(left, m.w-lipgloss.Width(right), "…")
	gap := max(m.w-lipgloss.Width(left)-lipgloss.Width(right), 0)
	b.WriteString(style.Render(left) + bg.Render(strings.Repeat(" ", gap)) + hintStyle.Render(right))
	return b.String()
}

func (m model) hint() string {
	switch {
	case m.fileMode() && m.doc.Ed.On && m.mode == modeReview:
		return "editing the proposal · ctrl+s accept · ctrl+z undo · esc stop editing"
	case m.fileMode() && m.doc.Ed.On:
		return "editing · ctrl+s save · ctrl+z undo · ctrl+t view mode · esc stop"
	case m.perm != nil:
		return "↑↓ choose · enter select · pgup/pgdn scroll"
	case m.busy:
		return "working… · esc to interrupt"
	}
	switch m.mode {
	case modeBrowse:
		return "↑↓ select · enter open · ← parent · esc back"
	case modeView:
		return "↑↓ ←→ scroll · enter edit · tab view mode · esc back"
	case modeChat:
		return "/ commands · ctrl+o thinking · /reset new session"
	case modeConfig:
		return "↑↓ move · enter choose · esc back"
	}
	return "v is for vibing · /q is for exit"
}

func main() {
	themeErr := loadThemes(filepath.Join(configDir(), "themes"))
	s, err := loadSettings()
	t := findTheme(s.Theme)
	if t == nil {
		t = findTheme(defaultTheme)
	}
	applyTheme(t)
	m := newModel()
	m.settings = s
	switch {
	case err != nil:
		m.setErr("settings: " + err.Error())
	case findTheme(s.Theme) == nil:
		m.setErr("settings: unknown theme " + s.Theme)
	case themeErr != nil:
		m.setErr(strings.ReplaceAll(themeErr.Error(), "\n", "; "))
	}
	if len(os.Args) > 1 {
		m.cmdOpen(os.Args[1])
	}
	// Connect to Claude Code right away; it signs in with the user's login.
	m.listen = m.connect()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, err := p.Run()
	if fm, ok := final.(model); ok {
		fm.stopAgent()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
