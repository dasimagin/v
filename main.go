// Command v is a terminal tool for coding together with an AI agent.
package main

import (
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mode int

const (
	modeSplash mode = iota // until the first input
	modeChat
	modeView
	modeReview // an agent's proposed edit, waiting for y / n
	modeConfig
	modeSessions // the /resume list
	modePick     // /switch, /workdir and /open, queried through the omnibox
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

	workdir                              string // absolute path, independent of the process cwd
	workspaceNotice                      string // context change sent with the next prompt
	repo                                 *repository
	repoMenu                             *repoMenu
	picker                               *workspacePicker
	rv                                   *reviewFlow   // /review or /apply in progress
	runScr                               *runScreen    // the run on screen, if any
	loop                                 *convergeLoop // /converge in progress
	hub                                  *applyHub     // points the chat agent's pr_* tools at the /apply run
	toolsOn                              bool          // an apply run is open: its agent has the hub's tools
	agentHasTools                        bool          // the running agent was started with tools
	deferred                             tea.Cmd       // work an event handler hands to Update
	repoSeq                              int
	repoLoading, prLoading, repoChanging bool

	doc          *Doc
	top, left    int
	log          []message
	settings     Settings
	cfg          *configScreen
	agent        backend         // nil until started
	agentReady   bool            // got the init message
	agentStale   bool            // model, effort or workdir changed: resume before the next prompt
	sessionID    string          // Claude Code session, for --resume
	resumeNext   bool            // the next start continues sessionID (after /resume)
	sessions     *sessionsScreen // the /resume list
	hist         history         // omnibox history, browsed with ↑↓
	busy         bool            // a turn is running
	interrupted  bool            // the running turn was interrupted
	showThinking bool            // Ctrl-O: thinking blocks expanded in the chat
	perm         *permission     // tool call waiting for y/n
	choice       int             // highlighted answer in the choice list
	noting       bool            // Tab during a question: the omnibox takes a note for the answer
	queue        []string        // prompts typed while the agent was busy

	listen tea.Cmd // listens to the agent started at launch

	vmode      viewMode  // flat or diff, kept across files
	rmode      viewMode  // the same for reviews of agent edits
	viewMenu   *viewMenu // open view-mode dropdown
	listMenu   *viewMenu // open Files/Comments dropdown of the PR screen
	help       bool      // the help of the screen on top is open
	submitMenu *viewMenu // open verdict dropdown (Comment, Approve, Request changes, Discard)
	stack      []screen  // screens to return to with Esc

	exitKey string // "ctrl+c" or "ctrl+d" after the first press; a second one quits
	exitSeq int    // invalidates stale disarm ticks
}

func newModel() model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = placeholder
	ti.Focus()
	dir, _ := os.Getwd()
	m := model{omni: ti, tagline: rand.IntN(len(taglines)), workdir: dir, repoLoading: true, rmode: viewUnified, hub: &applyHub{}}
	m.styleOmni()
	m.hist = loadHistory()
	return m
}

// styleOmni applies the active theme to the omnibox.
func (m *model) styleOmni() {
	m.omni.PromptStyle = accentStyle
	m.omni.TextStyle = fgStyle
	m.omni.PlaceholderStyle = hintStyle
	m.omni.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(cur.Fg))
}

func (m *model) setErr(s string) { m.status, m.statusInfo = s, false }

// flash shows a notice in the status line until the next Enter.
func (m *model) flash(s string) { m.status, m.statusInfo = s, true }

// exitWindow is how long a second Ctrl-C or Ctrl-D still quits.
const exitWindow = time.Second

type disarmMsg int

var keyNames = map[string]string{"ctrl+c": "Ctrl-C", "ctrl+d": "Ctrl-D"}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.listen, repositoryCommand(m.workdir, m.repoSeq, true), repositoryTick(), brandTick(m.settings.Animate))
}

const placeholder = "type / for commands"

// fileMode reports whether the main area shows a file: opened or under review.
func (m *model) fileMode() bool { return m.mode == modeView || m.mode == modeReview }

// footerRows is the height under the omnibox: the menu, the choices for a
// pending tool call, or the status line.
func (m model) footerRows() int {
	rows := 1 // one status line; PR menus overlay the body
	if m.showChoices() {
		return rows + len(m.choices()) + 1
	}
	if m.menu != nil {
		return rows + min(len(m.menu.items), menuRows)
	}
	return rows
}

// bodyRows is the height of the main area: everything except the header
// (view and browse modes), the two lines around the omnibox, and the footer.
func (m model) bodyRows() int {
	rows := m.h - 3 - m.footerRows() - headerRows
	return max(rows, 1)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.omni.Width = m.w - 4
		return m, nil
	case tea.MouseMsg:
		if m.help && msg.Action == tea.MouseActionPress {
			m.help = false // a click closes the help
			return m, nil
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if handled, cmd := m.workspaceClick(msg.X, msg.Y); handled {
				return m, cmd
			}
			if m.mode == modePick && m.picker.kind == "prdiff" && msg.Y >= headerRows && msg.Y < headerRows+m.bodyRows() && msg.X > m.pickerListWidth() {
				return m, m.previewClick(msg.X, msg.Y)
			}
			if m.mode == modePick && msg.Y >= headerRows && msg.Y < headerRows+m.bodyRows() && msg.X < m.pickerListWidth() {
				if i := m.picker.top + msg.Y - headerRows; i < len(m.picker.items) {
					m.picker.sel = i
					return m, m.choosePicker()
				}
				return m, nil
			}
		}
		if m.repoMenu != nil {
			if msg.Button == tea.MouseButtonWheelUp {
				return m, m.repoMenuKey(tea.KeyMsg{Type: tea.KeyUp})
			}
			if msg.Button == tea.MouseButtonWheelDown {
				return m, m.repoMenuKey(tea.KeyMsg{Type: tea.KeyDown})
			}
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.fileMode() {
			m.viewClick(msg.X, msg.Y)
			return m, nil
		}
		if m.mode == modePick && msg.X >= m.pickerListWidth() {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.picker.pscroll -= 3
			case tea.MouseButtonWheelDown:
				m.picker.pscroll += 3
			}
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
	case repositoryMsg, pullRequestsMsg, branchSwitchedMsg, prLinkedMsg, repositoryTickMsg:
		return m, m.onRepository(msg)
	case prDiffMsg:
		return m, m.onPRDiff(msg)
	case applyReadyMsg:
		return m, m.onApplyReady(msg)
	case prResolvedMsg:
		m.onPRResolved(msg)
		return m, nil
	case prSubmitMsg:
		return m, m.onPRSubmitted(msg)
	case brandTickMsg:
		if m.settings.Animate {
			brandPhase += brandStep
		}
		return m, brandTick(m.settings.Animate)
	case prDeletedMsg:
		return m, m.onPRDeleted(msg)
	case prComposedMsg:
		return m, m.onPRComposed(msg)
	case convergeReadyMsg:
		return m, m.onConvergeReady(msg)
	case convergePostedMsg:
		return m, m.onConvergePosted(msg)
	case convergePhaseMsg:
		return m, m.onConvergePhase(msg)
	case convergeFixedMsg:
		return m, m.onConvergeFixed(msg)
	case reviewReadyMsg:
		return m, m.onReviewReady(msg)
	case reviewAskMsg:
		return m, m.onReviewAsk(msg)
	case reviewShownMsg:
		return m, m.onReviewShown(msg)
	case reviewEndMsg:
		return m, nil
	case reviewPostedMsg:
		m.onReviewPosted(msg)
		return m, nil
	case reviewActionMsg:
		if msg.err != nil {
			m.setErr(msg.err.Error())
		} else {
			m.note(msg.text)
		}
		return m, nil
	case agentEvent:
		if msg.a != m.agent {
			return m, nil // from a session that /reset closed
		}
		m.onEvent(msg.e)
		next := m.agent.wait()
		if m.deferred != nil {
			next, m.deferred = tea.Batch(next, m.deferred), nil
		}
		return m, next
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
	if m.help { // any key closes the help; the key itself is not used
		m.help = false
		return m, nil
	}
	if k == "?" && m.wantsHelp() {
		m.help = true
		return m, nil
	}
	if m.repoMenu != nil {
		return m, m.repoMenuKey(msg)
	}
	if m.viewMenu != nil {
		m.viewMenuKey(k)
		return m, nil
	}
	if m.listMenu != nil {
		m.listMenuKey(k)
		return m, nil
	}
	if m.submitMenu != nil {
		return m, m.submitMenuKey(k)
	}
	if k == "ctrl+t" {
		m.openViewMenu()
		return m, nil
	}
	if m.rv != nil && !m.rv.waiting() && m.perm == nil && m.mode == modeChat && m.omni.Value() == "" && k == "esc" {
		m.reviewEsc() // stop a running review or apply
		return m, nil
	}
	if m.runScr != nil && m.rv == nil && !m.busy && m.perm == nil && m.mode == modeChat && m.omni.Value() == "" && k == "esc" {
		m.leaveRun() // the run is over: back to the chat
		return m, nil
	}
	if !m.showChoices() {
		switch k {
		case "ctrl+b":
			return m, m.cmdSwitch("")
		case "ctrl+p":
			return m, m.cmdPR("")
		}
	}
	if m.mode == modePick {
		return m, m.pickerKey(msg)
	}
	empty := m.omni.Value() == ""
	if m.fileMode() {
		switch {
		case m.doc.Ed.On && k != "ctrl+c" && k != "ctrl+o":
			m.exitKey = ""
			return m.editKey(msg)
		}
	}
	// A pending tool call: pick an answer with ↑↓ and Enter, or its letter.
	// Tab opens the omnibox for a note that goes with the answer.
	if (m.perm != nil || m.reviewAsking()) && m.viewMenu == nil {
		choices := m.choices()
		move := func(d int) { m.choice = (m.choice + d + len(choices)) % len(choices) }
		if m.noting {
			switch k {
			case "up":
				move(-1)
			case "down":
				move(1)
			case "enter":
				note := strings.TrimSpace(m.omni.Value())
				m.omni.SetValue("")
				m.noting = false
				m.choose(choices[m.choice].key, note)
			case "esc":
				m.omni.SetValue("")
				m.noting = false
				m.omni.Blur()
			default:
				var cmd tea.Cmd
				m.omni, cmd = m.omni.Update(msg)
				return m, cmd
			}
			return m, nil
		}
		switch k {
		case "up", "shift+tab":
			move(-1)
			return m, nil
		case "down":
			move(1)
			return m, nil
		case "tab":
			m.noting = true
			return m, m.omni.Focus()
		case "enter":
			m.choose(choices[m.choice].key, "")
			return m, nil
		case "esc":
			m.choose("n", "")
			return m, nil
		}
		for _, c := range choices {
			if k == c.key {
				m.choose(k, "")
				return m, nil
			}
		}
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			return m, nil // letters are answers here, not omnibox input
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
		m.hist.add(v)
		return m, m.run(v)
	}
	if m.historyKey(k) {
		return m, nil
	}
	// With an empty omnibox, navigation keys drive the main area.
	if m.omni.Value() == "" && m.bodyKey(k) {
		return m, nil
	}
	before := m.omni.Value()
	var cmd tea.Cmd
	m.omni, cmd = m.omni.Update(msg)
	if m.omni.Value() != before {
		m.hist.pos = -1 // typing ends history browsing
		m.suggest(false)
	}
	return m, cmd
}

// bodyKey handles a navigation key in the main area; it reports whether the key was used.
func (m *model) bodyKey(k string) bool {
	rows := m.bodyRows()
	switch m.mode {
	case modePick:
		switch k {
		case "up":
			m.movePicker(-1, false)
		case "down":
			m.movePicker(1, false)
		case "esc":
			m.closePicker(true)
		default:
			return false
		}
		return true
	case modeSplash:
		if k == "esc" {
			m.mode = modeChat
			return true
		}
	case modeConfig:
		return m.configKey(k)
	case modeSessions:
		return m.sessionsKey(k)
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
	case modePick:
		for _, l := range m.renderPicker(rows) {
			line(l)
		}
	case modeSplash:
		for _, l := range m.header("", "", "") {
			line(l)
		}
		for _, l := range splash(taglines[m.tagline], m.w, rows) {
			line(l)
		}
	case modeChat:
		for _, l := range m.chatHeader() {
			line(l)
		}
		for _, l := range m.renderLog(rows) {
			line(l)
		}
	case modeSessions:
		for _, l := range m.header(fgStyle.Bold(true).Render("Sessions"), tildify(m.workdir), "") {
			line(l)
		}
		for _, l := range m.renderSessions(rows) {
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
	if m.mode == modePick && m.picker != nil && m.picker.compose != nil {
		m.omni.Placeholder = m.picker.compose.hint
	}
	if m.showChoices() {
		m.omni.Placeholder = "tab adds a note for the agent"
		if m.noting {
			m.omni.Placeholder = "a note for the agent · enter sends it with the answer · esc drops it"
		}
	}
	line(pad(onThemeBg(m.omni.View()), bg))
	line(sep)
	finish := func() string {
		b.WriteString(m.renderStatusLine())
		return m.overlayMenus(b.String())
	}
	if m.menu != nil {
		for _, l := range m.renderMenu(m.w) {
			line(l)
		}
		return finish()
	}
	if m.showChoices() {
		b.WriteString(strings.Join(m.renderChoices(), "\n"))
		b.WriteByte('\n')
	}
	return finish()
}

func main() {
	dir, file, argErr := startupPaths(os.Args[1:], os.Stderr)
	if errors.Is(argErr, flag.ErrHelp) {
		return
	}
	if argErr != nil {
		fmt.Fprintln(os.Stderr, "v:", argErr)
		os.Exit(2)
	}
	themeErr := loadThemes(filepath.Join(configDir(), "themes"))
	s, err := loadSettings()
	t := findTheme(s.Theme)
	if t == nil {
		t = findTheme(defaultTheme)
	}
	transparentBG = s.Background == "terminal"
	applyTheme(t)
	applyDiffSettings(s)
	applyPRSettings(s)
	m := newModel()
	m.workdir = dir
	m.settings = s
	switch {
	case err != nil:
		m.setErr("settings: " + err.Error())
	case findTheme(s.Theme) == nil:
		m.setErr("settings: unknown theme " + s.Theme)
	case themeErr != nil:
		m.setErr(strings.ReplaceAll(themeErr.Error(), "\n", "; "))
	}
	if file != "" {
		m.cmdOpen(file)
	}
	// Connect to Claude Code right away; it signs in with the user's login.
	m.listen = m.connect()
	if s.Background == "sync" {
		syncTerminalBackground()
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, err := p.Run()
	// Whatever the setting was at the end, the terminal gets its own background back.
	resetTerminalBackground()
	if fm, ok := final.(model); ok {
		fm.stopAgent()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// wantsHelp reports whether a "?" should open the help: nothing is being typed.
func (m *model) wantsHelp() bool {
	switch {
	case m.omni.Value() != "" || m.noting:
		return false
	case m.mode == modePick && m.picker != nil && m.picker.compose != nil:
		return false
	case m.fileMode() && m.doc != nil && m.doc.Ed.On:
		return false
	}
	return true
}
