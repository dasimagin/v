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
	preview bool                        // show the code preview while picking
	values  func() []string             // display names
	get     func(*model) int            // index of the current value
	set     func(m *model, i int) error // apply value i; save happens on Enter
	apply   func(m *model)              // after the choice is saved; may be nil
}

var options = []option{
	{
		label:   "Color theme",
		preview: true,
		values: func() []string {
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
		label: "Model",
		values: func() []string {
			names := make([]string, len(models))
			for i, x := range models {
				names[i] = x.Name
			}
			return names
		},
		get: func(m *model) int {
			for i, x := range models {
				if x.ID == m.settings.Model {
					return i
				}
			}
			return 0
		},
		set: func(m *model, i int) error {
			m.settings.Model = models[i].ID
			return nil
		},
		apply: (*model).agentSettingsChanged,
	},
	{
		label:  "Effort",
		values: func() []string { return efforts },
		get: func(m *model) int {
			for i, e := range efforts {
				if e == m.settings.Effort {
					return i
				}
			}
			return 0
		},
		set: func(m *model, i int) error {
			m.settings.Effort = efforts[i]
			return nil
		},
		apply: (*model).agentSettingsChanged,
	},
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
		n := len(o.values())
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
			if err := m.settings.save(); err != nil {
				m.setErr("save settings: " + err.Error())
			}
			if o.apply != nil && c.pick != c.orig {
				o.apply(m)
			}
		case "esc":
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
	case "esc":
		m.pop()
	default:
		return false
	}
	return true
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
	add("")
	for i, o := range options {
		mark, st := "  ", fgStyle
		if i == c.row {
			mark, st = "› ", accentStyle.Bold(true)
		}
		value := o.values()[o.get(m)] + " ▾"
		add(bg.Render("  ") + st.Render(mark+o.label+strings.Repeat(" ", labelW-lipgloss.Width(o.label)+3)) + selStyle.Render(" "+value+" "))
		if i == c.row && c.open {
			indent := bg.Render(strings.Repeat(" ", 4+labelW+3))
			for j, v := range o.values() {
				if j == c.pick {
					add(indent + accentStyle.Bold(true).Render("› "+v))
				} else {
					add(indent + fgStyle.Render("  "+v))
				}
			}
		}
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
