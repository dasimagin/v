// Command v is a terminal tool for coding together with an AI agent.
package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// One Dark Pro Darker palette.
const (
	colBg     = "#23272e"
	colFg     = "#abb2bf"
	colMuted  = "#7f848e"
	colGutter = "#495162"
	colRed    = "#e06c75"
	colBlue   = "#61afef"
	colPurple = "#c678dd"
)

// logo is a pseudo-3D "V": blocks are the face, box-drawing lines the shadow.
var logo = []string{
	"██╗          ██╗",
	"╚██╗        ██╔╝",
	" ╚██╗      ██╔╝ ",
	"  ╚██╗    ██╔╝  ",
	"   ╚██╗  ██╔╝   ",
	"    ╚██╗██╔╝    ",
	"     ╚███╔╝     ",
	"      ╚══╝      ",
}

var taglines = []string{
	"Vibing time...",
	"Code & coffee...",
	"Let's deploy this shit...",
	"Works on my machine...",
	"It compiles. Ship it...",
	"Deploy on Friday, what could go wrong...",
	"It's not a bug, it's a feature...",
	"Senior Stack Overflow engineer...",
	"Have you tried turning it off and on again?",
	"Vibes > specs...",
	"If it works, don't touch it...",
	"A hack, but a reliable one...",
	"Works in my venv...",
	"One typo, 400 lines of template errors...",
	"Undefined behavior, defined vibes...",
	// Code stays as typed.
	"git push --force and pray",
	"// TODO: write tests",
	"import antigravity",
	"if err != nil { return err }",
	"Segmentation fault (core dumped)",
}

type model struct {
	w, h    int
	omni    textinput.Model
	tagline int
	status  string // transient error; replaces the hint until the next Enter
	where   string // "branch · ~/path" shown on the right of the status line
}

var (
	bg        = lipgloss.NewStyle().Background(lipgloss.Color(colBg))
	tagStyle  = bg.Foreground(lipgloss.Color(colMuted)).Italic(true)
	sepStyle  = bg.Foreground(lipgloss.Color(colGutter))
	hintStyle = bg.Foreground(lipgloss.Color(colMuted))
	errStyle  = bg.Foreground(lipgloss.Color(colRed))
)

func newModel() model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.PromptStyle = bg.Foreground(lipgloss.Color(colBlue))
	ti.TextStyle = bg.Foreground(lipgloss.Color(colFg))
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(colFg))
	ti.Placeholder = "/q to quit"
	ti.PlaceholderStyle = hintStyle
	ti.Focus()
	return model{omni: ti, tagline: rand.IntN(len(taglines)), where: where()}
}

// where describes the working directory: git branch (if any) and path with ~.
func where() string {
	dir, _ := os.Getwd()
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(dir, home) {
		dir = "~" + dir[len(home):]
	}
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if branch := strings.TrimSpace(string(out)); err == nil && branch != "" {
		return branch + " · " + dir
	}
	return dir
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.omni.Width = m.w - 4
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			v := strings.TrimSpace(m.omni.Value())
			m.omni.SetValue("")
			switch v {
			case "/q", "/quit":
				return m, tea.Quit
			case "":
				m.status = ""
			default:
				m.status = "only /q works for now"
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.omni, cmd = m.omni.Update(msg)
	return m, cmd
}

// gradient blends a into b; colors are "#rrggbb", t is in [0, 1].
func gradient(a, b string, t float64) lipgloss.Color {
	x, _ := strconv.ParseUint(a[1:], 16, 32)
	y, _ := strconv.ParseUint(b[1:], 16, 32)
	var out uint64
	for shift := 16; shift >= 0; shift -= 8 {
		ca, cb := float64(x>>shift&0xff), float64(y>>shift&0xff)
		out |= uint64(ca+(cb-ca)*t) << shift
	}
	return lipgloss.Color(fmt.Sprintf("#%06x", out))
}

// renderLogo colors the face blocks with a blue→purple gradient and the shadow darker.
func renderLogo() []string {
	out := make([]string, len(logo))
	for i, line := range logo {
		t := float64(i) / float64(len(logo)-1)
		face := bg.Foreground(gradient(colBlue, colPurple, t)).Bold(true)
		shadow := bg.Foreground(gradient("#3b5a7a", "#5a3d6b", t))
		var b strings.Builder
		for _, r := range line {
			s := string(r)
			switch r {
			case '█':
				b.WriteString(face.Render(s))
			case ' ':
				b.WriteString(bg.Render(s))
			default:
				b.WriteString(shadow.Render(s))
			}
		}
		out[i] = b.String()
	}
	return out
}

// center pads a rendered line to the screen width.
func (m model) center(s string) string {
	w := lipgloss.Width(s)
	left := max((m.w-w)/2, 0)
	return bg.Render(strings.Repeat(" ", left)) + s + bg.Render(strings.Repeat(" ", max(m.w-left-w, 0)))
}

func (m model) View() string {
	if m.w == 0 {
		return ""
	}
	tag := tagStyle.Render(taglines[m.tagline])
	block := append(renderLogo(), bg.Render(""), tag)

	// Bottom: line, omnibox, line, status. Same frame as Claude Code's input.
	rows := max(m.h-4, len(block))
	top := (rows - len(block)) / 2
	var b strings.Builder
	for i := 0; i < rows; i++ {
		line := ""
		if i >= top && i < top+len(block) {
			line = block[i-top]
		}
		b.WriteString(m.center(line))
		b.WriteByte('\n')
	}

	sep := sepStyle.Render(strings.Repeat("─", m.w))
	omni := m.omni.View()
	b.WriteString(sep + "\n")
	b.WriteString(omni + bg.Render(strings.Repeat(" ", max(m.w-lipgloss.Width(omni), 0))) + "\n")
	b.WriteString(sep + "\n")

	left, style := " "+m.status, errStyle
	if m.status == "" {
		left, style = " v is for vibing · /q is for exit", hintStyle
	}
	right := m.where + " "
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		right, gap = "", max(m.w-lipgloss.Width(left), 0)
	}
	b.WriteString(style.Render(left) + bg.Render(strings.Repeat(" ", gap)) + hintStyle.Render(right))
	return b.String()
}

func main() {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
