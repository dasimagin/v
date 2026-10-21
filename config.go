package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// option is one row on the /config screen: a value picked from a dropdown.
type option struct {
	label   string
	desc    string                      // one muted line under the label
	preview bool                        // show the code preview while picking
	values  func(*model) []string       // display names
	get     func(*model) int            // index of the current value
	set     func(m *model, i int) error // apply value i; save happens on Enter
	apply   func(m *model)              // after the choice is saved; may be nil
}

var options = []option{
	{
		label:   "Color theme",
		desc:    "Colors of v and of the code; drop VS Code themes into ~/.v/themes",
		preview: true,
		values: func(*model) []string {
			names := make([]string, len(themes))
			for i, t := range themes {
				names[i] = t.Name
			}
			return names
		},
		get: func(*model) int {
			for i, t := range themes {
				if t == cur {
					return i
				}
			}
			return 0
		},
		set: func(m *model, i int) error {
			m.setTheme(themes[i])
			m.settings.Theme = themes[i].ID
			return nil
		},
	},
	{
		label: "Agent",
		desc:  "Who does the work; each signs in with its own subscription",
		values: func(*model) []string {
			return []string{"Claude Code", "Codex"}
		},
		get: func(m *model) int {
			if m.settings.Agent == agentCodex {
				return 1
			}
			return 0
		},
		set: func(m *model, i int) error {
			m.settings.Agent = []string{"claude", agentCodex}[i]
			return nil
		},
		apply: (*model).agentChanged,
	},
	{
		label: "Model",
		desc:  "The model the agent runs on",
		values: func(m *model) []string {
			var names []string
			for _, x := range agentModels(m) {
				names = append(names, x.Name)
			}
			return names
		},
		get: func(m *model) int {
			cur := m.settings.Model
			if m.settings.Agent == agentCodex {
				cur = m.settings.CodexModel
				if cur == "" {
					cur = codexDefault().ID
				}
			}
			for i, x := range agentModels(m) {
				if x.ID == cur {
					return i
				}
			}
			return 0
		},
		set: func(m *model, i int) error {
			list := agentModels(m)
			if i >= len(list) {
				return nil
			}
			if m.settings.Agent == agentCodex {
				m.settings.CodexModel = list[i].ID
				m.settings.CodexEffort = "" // efforts differ between models
			} else {
				m.settings.Model = list[i].ID
			}
			return nil
		},
		apply: (*model).agentSettingsChanged,
	},
	{
		label:  "Effort",
		desc:   "How hard the model thinks: more is slower and uses more of your plan",
		values: agentEfforts,
		get: func(m *model) int {
			cur := m.settings.Effort
			if m.settings.Agent == agentCodex {
				cur = m.settings.CodexEffort
				if cur == "" {
					return 0 // "default"
				}
			}
			for i, e := range agentEfforts(m) {
				if e == cur {
					return i
				}
			}
			return 0
		},
		set: func(m *model, i int) error {
			list := agentEfforts(m)
			if i >= len(list) {
				return nil
			}
			if m.settings.Agent == agentCodex {
				m.settings.CodexEffort = list[i]
				if i == 0 {
					m.settings.CodexEffort = "" // "default"
				}
			} else {
				m.settings.Effort = list[i]
			}
			return nil
		},
		apply: (*model).agentSettingsChanged,
	},
	{
		label: "Diff context before",
		desc:  "Unchanged lines shown above each change, in every diff",
		values: func(*model) []string {
			names := make([]string, len(diffContextChoices))
			for i, n := range diffContextChoices {
				names[i] = fmt.Sprint(n, " lines")
			}
			return names
		},
		get: func(m *model) int { return contextIndex(m.settings.DiffBefore) },
		set: func(m *model, i int) error {
			m.settings.DiffBefore = diffContextChoices[i]
			applyDiffSettings(m.settings)
			m.refreshDiffs()
			return nil
		},
	},
	{
		label: "Diff context after",
		desc:  "Unchanged lines shown below each change, in every diff",
		values: func(*model) []string {
			names := make([]string, len(diffContextChoices))
			for i, n := range diffContextChoices {
				names[i] = fmt.Sprint(n, " lines")
			}
			return names
		},
		get: func(m *model) int { return contextIndex(m.settings.DiffAfter) },
		set: func(m *model, i int) error {
			m.settings.DiffAfter = diffContextChoices[i]
			applyDiffSettings(m.settings)
			m.refreshDiffs()
			return nil
		},
	},
}

// contextIndex is the dropdown entry closest to n lines of context.
func contextIndex(n int) int {
	best := 0
	for i, c := range diffContextChoices {
		if abs(c-n) < abs(diffContextChoices[best]-n) {
			best = i
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// refreshDiffs drops the diffs built with the old context sizes.
func (m *model) refreshDiffs() {
	reset := func(d *Doc) {
		if d != nil {
			d.unified, d.split = nil, nil
		}
	}
	reset(m.doc)
	for _, s := range m.stack {
		reset(s.doc)
	}
}

type modelChoice struct{ ID, Name string }

// agentModels are the models of the chosen agent. Codex's list comes from
// Codex itself, the first time it is needed.
func agentModels(m *model) []modelChoice {
	var out []modelChoice
	if m != nil && m.settings.Agent == agentCodex {
		list, err := loadCodexModels()
		if err != nil {
			m.setErr("codex: " + err.Error())
		}
		for _, x := range list {
			out = append(out, modelChoice{x.ID, x.Name})
		}
		return out
	}
	for _, x := range models {
		out = append(out, modelChoice{x.ID, x.Name})
	}
	return out
}

// agentEfforts are the efforts the chosen model supports. For Codex the
// first one, "default", leaves the choice to the model.
func agentEfforts(m *model) []string {
	if m == nil || m.settings.Agent != agentCodex {
		return efforts
	}
	id := m.settings.CodexModel
	if id == "" {
		id = codexDefault().ID
	}
	for _, x := range codexModels {
		if x.ID == id {
			return append([]string{"default"}, x.Efforts...)
		}
	}
	return []string{"default"}
}

// configScreen is the state of /config: the focused row and the open dropdown.
type configScreen struct {
	row  int
	open bool
	pick int // highlighted value in the open dropdown
	orig int // value before the dropdown opened, restored on Esc
}

func (m *model) cmdConfig(string) tea.Cmd {
	m.push()
	m.mode, m.cfg = modeConfig, &configScreen{}
	return nil
}

func (m *model) setTheme(t *Theme) {
	applyTheme(t)
	m.styleOmni()
}

// configKey handles navigation on the /config screen.
func (m *model) configKey(k string) bool {
	c, o := m.cfg, options[m.cfg.row]
	if c.open {
		n := len(o.values(m))
		switch k {
		case "up", "down":
			d := 1
			if k == "up" {
				d = -1
			}
			c.pick = ((c.pick+d)%n + n) % n
			_ = o.set(m, c.pick) // live preview
		case "enter", " ":
			c.open = false
			m.saveOption(o, c.pick != c.orig)
		case "esc", "left":
			c.open = false
			_ = o.set(m, c.orig)
		default:
			return false
		}
		return true
	}
	switch k {
	case "up":
		c.row = max(c.row-1, 0)
	case "down":
		c.row = min(c.row+1, len(options)-1)
	case "enter", " ", "right":
		c.open, c.pick = true, o.get(m)
		c.orig = c.pick
	case "esc", "left":
		m.pop()
	default:
		return false
	}
	return true
}

// saveOption stores a chosen value and, when it changed, applies it.
func (m *model) saveOption(o option, changed bool) {
	if err := m.settings.save(); err != nil {
		m.setErr("save settings: " + err.Error())
	}
	if o.apply != nil && changed {
		o.apply(m)
	}
}

const previewSrc = `// Preview of the syntax theme.
package main

import "fmt"

type Vibe struct {
	Name  string
	Level int // 0..11
}

func main() {
	v := Vibe{Name: "coding", Level: 11}
	if v.Level > 10 {
		fmt.Printf("%s goes to %d\n", v.Name, v.Level)
	}
}`

// renderConfig draws the settings rows, the open dropdown and a code preview.
func (m *model) renderConfig(rows int) []string {
	c := m.cfg
	var out []string
	add := func(s string) { out = append(out, s) }
	labelW := 0
	for _, o := range options {
		labelW = max(labelW, lipgloss.Width(o.label))
	}
	// Each option: label and value on one row, a description under it, and
	// an empty row before the next one. The open dropdown hangs under the
	// value as a panel.
	const margin, gap = 4, 6
	valueX := margin + 2 + labelW + gap
	panel := lipgloss.NewStyle().Background(lipgloss.Color(cur.Panel))
	for i, o := range options {
		add("")
		mark, st := "  ", fgStyle
		if i == c.row {
			mark, st = "› ", accentStyle.Bold(true)
		}
		value := " " + o.values(m)[o.get(m)] + " ▾ "
		valueStyle := selStyle
		if i == c.row && c.open {
			valueStyle = buttonOpen
		}
		add(blank(margin) + st.Render(mark+o.label) + blank(labelW-lipgloss.Width(o.label)+gap) + valueStyle.Render(value))
		if i == c.row && c.open {
			w := 0
			for _, v := range o.values(m) {
				w = max(w, lipgloss.Width(v))
			}
			for j, v := range o.values(m) {
				item, ist := "  "+v, panel.Foreground(lipgloss.Color(cur.Fg))
				if j == c.pick {
					item, ist = "› "+v, panel.Foreground(lipgloss.Color(cur.Accent)).Bold(true)
				}
				add(blank(valueX) + ist.Render(" "+item+strings.Repeat(" ", w-lipgloss.Width(v)+1)))
			}
			continue
		}
		add(blank(margin+2) + hintStyle.Render(o.desc))
	}
	// The code preview shows only while picking an option that affects it.
	if _, lines, err := highlight("preview.go", previewSrc); err == nil && c.open && options[c.row].preview {
		add("")
		add(bg.Render("  ") + hintStyle.Render("Preview"))
		for n, l := range lines {
			add(gutterStyle.Render(fmt.Sprintf("%4d ", n+1)) + renderLine(l, 0, max(m.w-5, 1)))
		}
	}
	for len(out) < rows {
		add("")
	}
	out = out[:rows]
	for i, l := range out {
		out[i] = l + bg.Render(strings.Repeat(" ", max(m.w-lipgloss.Width(l), 0)))
	}
	return out
}
