package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// session is a past Claude Code conversation in the working directory.
type session struct {
	ID       string
	Title    string // the first prompt, on one line
	Prompts  int
	Modified time.Time
	path     string
}

// sessionsScreen is the state of /resume.
type sessionsScreen struct {
	list     []session
	sel, top int
}

// projectDirs are where Claude Code keeps the sessions of dir: every
// character other than a letter or digit in the path becomes "-".
func projectDirs(dir string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	nonWord := regexp.MustCompile(`[^A-Za-z0-9]`)
	paths := []string{dir}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil && resolved != dir {
		paths = append(paths, resolved)
	}
	var out []string
	for _, p := range paths {
		out = append(out, filepath.Join(home, ".claude", "projects", nonWord.ReplaceAllString(p, "-")))
	}
	return out
}

// sessionRecord is the part of a session log line that v reads.
type sessionRecord struct {
	Type        string          `json:"type"`
	IsMeta      bool            `json:"isMeta"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
}

// readSession calls fn for each main-thread user or assistant record.
func readSession(path string, fn func(r sessionRecord)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var r sessionRecord
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.IsMeta || r.IsSidechain {
			continue
		}
		if r.Type == "user" || r.Type == "assistant" {
			fn(r)
		}
	}
	return sc.Err()
}

// promptText is the text the user typed in a user record, "" for tool
// results and commands.
func promptText(raw json.RawMessage) string {
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &msg) != nil {
		return ""
	}
	var s string
	if json.Unmarshal(msg.Content, &s) != nil {
		for _, b := range blocks(raw) {
			if b.Type == "text" {
				s += b.Text
			}
		}
	}
	if strings.HasPrefix(strings.TrimSpace(s), "<") {
		return "" // command output, reminders and other injected text
	}
	return s
}

// listSessions returns the sessions of dir, newest first.
func listSessions(dir string) []session {
	var out []session
	seen := map[string]bool{}
	for _, pd := range projectDirs(dir) {
		files, _ := filepath.Glob(filepath.Join(pd, "*.jsonl"))
		for _, f := range files {
			id := strings.TrimSuffix(filepath.Base(f), ".jsonl")
			fi, err := os.Stat(f)
			if err != nil || seen[id] {
				continue
			}
			seen[id] = true
			s := session{ID: id, Modified: fi.ModTime(), path: f}
			_ = readSession(f, func(r sessionRecord) {
				if r.Type != "user" {
					return
				}
				if p := strings.TrimSpace(promptText(r.Message)); p != "" {
					if s.Title == "" {
						s.Title = strings.Join(strings.Fields(p), " ")
					}
					s.Prompts++
				}
			})
			if s.Prompts > 0 {
				out = append(out, s)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	return out
}

// sessionLog rebuilds the chat log of a session from its file.
func sessionLog(path string, dirs ...string) []message {
	var log []message
	_ = readSession(path, func(r sessionRecord) {
		if r.Type == "user" {
			if p := promptText(r.Message); strings.TrimSpace(p) != "" {
				log = append(log, message{kind: msgUser, text: p})
			}
			return
		}
		for _, b := range blocks(r.Message) {
			switch b.Type {
			case "text":
				if strings.TrimSpace(b.Text) != "" {
					log = append(log, message{kind: msgAgent, text: b.Text})
				}
			case "tool_use":
				log = append(log, message{kind: msgTool, text: toolSummary(b.Name, b.Input, dirs...)})
			case "thinking":
				log = append(log, message{kind: msgThinking, text: b.Thinking})
			}
		}
	})
	return log
}

func (m *model) cmdResume(string) tea.Cmd {
	if m.settings.Agent == agentCodex {
		m.setErr("/resume lists Claude Code sessions; Codex threads are not there yet")
		return nil
	}
	dir := m.workdir
	list := listSessions(dir)
	if len(list) == 0 {
		m.setErr("no past sessions in " + tildify(dir))
		return nil
	}
	m.push()
	m.mode, m.sessions = modeSessions, &sessionsScreen{list: list}
	return nil
}

// resume switches to a past session: its chat comes back, and the next
// prompt continues it in Claude Code with --resume.
func (m *model) resume(s session) {
	m.stopAgent()
	m.log = sessionLog(s.path, m.workdir)
	m.note(fmt.Sprintf("resumed the session from %s", ago(s.Modified)))
	m.sessionID, m.resumeNext = s.ID, true
	m.stack = nil
	m.mode = modeChat
}

func (m *model) sessionsKey(k string) bool {
	ss, rows := m.sessions, m.bodyRows()
	switch k {
	case "up":
		ss.sel = max(ss.sel-1, 0)
	case "down":
		ss.sel = min(ss.sel+1, len(ss.list)-1)
	case "pgup":
		ss.sel = max(ss.sel-rows, 0)
	case "pgdown":
		ss.sel = min(ss.sel+rows, len(ss.list)-1)
	case "enter":
		m.resume(ss.list[ss.sel])
	case "esc", "left":
		m.pop()
	default:
		return false
	}
	return true
}

// ago is a short relative time: "5m ago", "3h ago", "2d ago", or a date.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Format("Jan 2")
}

func (m *model) renderSessions(rows int) []string {
	ss := m.sessions
	if ss.sel < ss.top {
		ss.top = ss.sel
	}
	if ss.sel >= ss.top+rows {
		ss.top = ss.sel - rows + 1
	}
	out := make([]string, 0, rows)
	for i := ss.top; i < len(ss.list) && len(out) < rows; i++ {
		s := ss.list[i]
		when := fmt.Sprintf("%-9s", ago(s.Modified))
		count := fmt.Sprintf("%d prompts", s.Prompts)
		if s.Prompts == 1 {
			count = "1 prompt"
		}
		mark, st := "    ", fgStyle
		if i == ss.sel {
			mark, st = "  › ", accentStyle.Bold(true)
		}
		titleW := max(m.w-lipgloss.Width(mark+when)-lipgloss.Width(count)-6, 10)
		title := truncate(s.Title, titleW)
		title += strings.Repeat(" ", max(titleW-lipgloss.Width(title), 0)) // counts line up on the right
		line := st.Render(mark) + hintStyle.Render(when+"  ") + st.Render(title) + hintStyle.Render("   "+count)
		out = append(out, line+blank(m.w-lipgloss.Width(line)))
	}
	for len(out) < rows {
		out = append(out, blank(m.w))
	}
	return out
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
