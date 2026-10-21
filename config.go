package main

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
		label: "Model",
		desc:  "The model the agent runs on",
		values: func(*model) []string {
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
			if i < len(models) {
				m.settings.Model = models[i].ID
			}
			return nil
		},
		apply: (*model).agentSettingsChanged,
	},
	{
		label:  "Effort",
		desc:   "How hard the model thinks: more is slower and uses more of your plan",
		values: func(*model) []string { return efforts },
		get: func(m *model) int {
			for i, e := range efforts {
				if e == m.settings.Effort {
					return i
				}
			}
			return 0
		},
		set: func(m *model, i int) error {
			if i < len(efforts) {
				m.settings.Effort = efforts[i]
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
	{
		label:  "Review mode (experimental)",
		desc:   "/review: manual asks about each comment; auto sends them at once as a pending review you publish later",
		values: func(*model) []string { return []string{"Manual", "Auto"} },
		get: func(m *model) int {
			if m.settings.ReviewMode == reviewAuto {
				return 1
			}
			return 0
		},
		set: func(m *model, i int) error {
			m.settings.ReviewMode = []string{reviewManual, reviewAuto}[i]
			return nil
		},
	},
	{
		label:  "Apply mode (experimental)",
		desc:   "/apply: manual asks about every edit, commit, push and reply; auto lets the agent do all of it on its own",
		values: func(*model) []string { return []string{"Manual", "Auto"} },
		get: func(m *model) int {
			if m.settings.ApplyMode == reviewAuto {
				return 1
			}
			return 0
		},
		set: func(m *model, i int) error {
			m.settings.ApplyMode = []string{reviewManual, reviewAuto}[i]
			return nil
		},
	},
	{
		label:  "Comments on the PR screen",
		desc:   "Pending: replies and comments you write wait in your pending review until you submit it from the header. Immediate: each one is published at once",
		values: func(*model) []string { return []string{"Pending review", "Publish immediately"} },
		get: func(m *model) int {
			if m.settings.PRComments == "immediate" {
				return 1
			}
			return 0
		},
		set: func(m *model, i int) error {
			m.settings.PRComments = []string{"pending", "immediate"}[i]
			applyPRSettings(m.settings)
			return nil
		},
	},
	{
		label:  "Animations",
		desc:   "The brand in the header shimmers; turn it off for a still screen",
		values: func(*model) []string { return []string{"On", "Off"} },
		get: func(m *model) int {
			if m.settings.Animate {
				return 0
			}
			return 1
		},
		set: func(m *model, i int) error {
			m.settings.Animate = i == 0
			return nil
		},
	},
	{
		label: "Converge iterations (experimental)",
		desc:  "The most rounds of review and fix that /converge runs before it stops",
		values: func(*model) []string {
			names := make([]string, len(iterationChoices))
			for i, n := range iterationChoices {
				names[i] = fmt.Sprint(n)
			}
			return names
		},
		get: func(m *model) int {
			for i, n := range iterationChoices {
				if n == m.settings.Iterations {
					return i
				}
			}
			return 4 // 5, the default
		},
		set: func(m *model, i int) error {
			m.settings.Iterations = iterationChoices[i]
			return nil
		},
	},
	{
		label: "Background",
		desc:  "Theme: v paints its own background. Terminal: v leaves it to the terminal, so no rectangle shows when the two colors differ. Theme + terminal: also sets the terminal's own background to the theme's (kitty, iTerm2, WezTerm), and restores it on exit",
		values: func(*model) []string {
			return []string{"Theme color", "Terminal's own", "Theme color, set on the terminal too"}
		},
		get: func(m *model) int {
			for i, v := range backgroundModes {
				if v == m.settings.Background {
					return i
				}
			}
			return 0
		},
		set: func(m *model, i int) error {
			m.settings.Background = backgroundModes[i]
			m.applyBackground()
			return nil
		},
	},
}

var iterationChoices = []int{1, 2, 3, 4, 5, 6, 8, 10}

// backgroundModes are the values of the Background setting, in the order of its dropdown.
var backgroundModes = []string{"theme", "terminal", "sync"}

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

// configScreen is the state of /config: the focused row, the open dropdown
// and the scroll position. The omnibox is its filter: row indexes the list of
// options that match what is typed there.
type configScreen struct {
	row, top int
	open     bool
	pick     int    // highlighted value in the open dropdown
	orig     int    // value before the dropdown opened, restored on Esc
	query    string // the filter that row and top belong to
}

// configVisible lists the options that match the filter typed in the omnibox,
// best match first; with nothing typed, all of them in their own order.
func (m *model) configVisible() []int {
	q := strings.TrimSpace(m.omni.Value())
	var all []int
	if q == "" {
		for i := range options {
			all = append(all, i)
		}
		return all
	}
	type hit struct{ i, score int }
	var hits []hit
	for i, o := range options {
		if score, ok := fuzzyScore(q, o.label+" "+o.desc); ok {
			hits = append(hits, hit{i, score})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	for _, h := range hits {
		all = append(all, h.i)
	}
	return all
}

// syncConfig restarts the list at the top when the filter changed.
func (m *model) syncConfig() {
	if c := m.cfg; c != nil && c.query != m.omni.Value() {
		c.query, c.row, c.top, c.open = m.omni.Value(), 0, 0, false
	}
}

func (m *model) cmdConfig(string) tea.Cmd {
	m.push()
	m.mode, m.cfg = modeConfig, &configScreen{}
	return nil
}

func (m *model) setTheme(t *Theme) {
	applyTheme(t)
	if m.settings.Background == "sync" {
		syncTerminalBackground() // the terminal follows the theme
	}
	previews, prAnchors, prLineRows, prBoxHits, prConvHits = map[string]previewEntry{}, map[string][]int{}, map[string]map[int]int{}, map[string][]boxHits{}, map[string]map[int][]prHit{} // styled previews hold the old colors
	m.styleOmni()
}

// configKey handles navigation on the /config screen.
func (m *model) configKey(k string) bool {
	c := m.cfg
	vis := m.configVisible()
	if len(vis) == 0 {
		if k == "esc" || k == "left" {
			m.pop()
			return true
		}
		return false
	}
	c.row = min(c.row, len(vis)-1)
	o := options[vis[c.row]]
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
		c.row = min(c.row+1, len(vis)-1)
	case "pgup":
		c.row = max(c.row-8, 0)
	case "pgdown":
		c.row = min(c.row+8, len(vis)-1)
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

// cfgLine is one row of the settings list, with what a click on it means.
type cfgLine struct {
	text   string
	opt    int    // index into options, or -1
	kind   string // "blank", "value", "item", "desc", "preview", "note"
	item   int    // for "item": the value it picks
	x0, x1 int    // for "value": the columns of its button
}

// configLines builds the whole list for the options that match the filter.
func (m *model) configLines() []cfgLine {
	c := m.cfg
	vis := m.configVisible()
	var out []cfgLine
	if len(vis) == 0 {
		return []cfgLine{{opt: -1, kind: "note", text: hintStyle.Render("  Nothing matches · keep typing, or clear the filter with esc")}}
	}
	labelW := 0
	for _, o := range options {
		labelW = max(labelW, lipgloss.Width(o.label))
	}
	// Each option: label and value on one row, a description under it, and an
	// empty row before the next one. The open dropdown hangs under the value as a panel.
	const margin, gap = 4, 6
	valueX := margin + 2 + labelW + gap
	panel := lipgloss.NewStyle().Background(lipgloss.Color(cur.Panel))
	for p, i := range vis {
		o := options[i]
		out = append(out, cfgLine{opt: i, kind: "blank"})
		mark, st := "  ", fgStyle
		if p == c.row {
			mark, st = "› ", accentStyle.Bold(true)
		}
		value := " " + o.values(m)[o.get(m)] + " ▾ "
		valueStyle := selStyle
		if p == c.row && c.open {
			valueStyle = buttonOpen
		}
		out = append(out, cfgLine{opt: i, kind: "value", x0: valueX, x1: valueX + lipgloss.Width(value),
			text: blank(margin) + st.Render(mark+o.label) + blank(labelW-lipgloss.Width(o.label)+gap) + valueStyle.Render(value)})
		if p == c.row && c.open {
			w := 0
			for _, v := range o.values(m) {
				w = max(w, lipgloss.Width(v))
			}
			for j, v := range o.values(m) {
				item, ist := "  "+v, panel.Foreground(lipgloss.Color(cur.Fg))
				if j == c.pick {
					item, ist = "› "+v, panel.Foreground(lipgloss.Color(cur.Accent)).Bold(true)
				}
				out = append(out, cfgLine{opt: i, kind: "item", item: j, x0: valueX, x1: valueX + w + 5,
					text: blank(valueX) + ist.Render(" "+item+strings.Repeat(" ", w-lipgloss.Width(v)+1))})
			}
			continue
		}
		for _, l := range strings.Split(ansi.Wrap(o.desc, max(m.w-margin-4, 20), ""), "\n") { // a long description wraps
			out = append(out, cfgLine{opt: i, kind: "desc", text: blank(margin+2) + hintStyle.Render(strings.TrimSpace(l))})
		}
	}
	// The code preview shows only while picking an option that affects it.
	if _, lines, err := highlight("preview.go", previewSrc); err == nil && c.open && options[vis[c.row]].preview {
		out = append(out, cfgLine{opt: -1, kind: "preview"}, cfgLine{opt: -1, kind: "preview", text: bg.Render("  ") + hintStyle.Render("Preview")})
		for n, l := range lines {
			out = append(out, cfgLine{opt: -1, kind: "preview", text: gutterStyle.Render(fmt.Sprintf("%4d ", n+1)) + renderLine(l, 0, max(m.w-5, 1))})
		}
	}
	return out
}

// configKeepVisible scrolls so that the focused option, with its open dropdown, is on screen.
func (m *model) configKeepVisible(lines []cfgLine, rows int) {
	c := m.cfg
	vis := m.configVisible()
	if len(vis) == 0 {
		c.top = 0
		return
	}
	first, last := -1, -1
	for i, l := range lines {
		if l.opt == vis[min(c.row, len(vis)-1)] {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first >= 0 {
		if first < c.top {
			c.top = max(first-1, 0) // the blank row above belongs to it
		}
		if last >= c.top+rows {
			c.top = last - rows + 1
		}
	}
	c.top = min(max(c.top, 0), max(len(lines)-rows, 0))
}

// configClick handles a click in the body of the settings screen: a row takes
// the focus, its value button opens the dropdown, an entry of the open
// dropdown picks it, and a click anywhere else closes the dropdown.
func (m *model) configClick(x, y int) bool {
	c := m.cfg
	m.syncConfig()
	lines := m.configLines()
	idx := c.top + y - headerRows
	if idx < 0 || idx >= len(lines) {
		return false
	}
	l := lines[idx]
	vis := m.configVisible()
	pos := func(opt int) int {
		for p, i := range vis {
			if i == opt {
				return p
			}
		}
		return -1
	}
	if c.open {
		cur := vis[min(c.row, len(vis)-1)]
		if l.kind == "item" && l.opt == cur && x >= l.x0 && x < l.x1 {
			c.pick = l.item
			_ = options[cur].set(m, c.pick)
			m.configKey("enter")
			return true
		}
		m.configKey("esc") // anywhere else: close, keeping the old value
		if l.kind != "value" || l.opt == cur {
			return true
		}
	}
	p := pos(l.opt)
	if p < 0 {
		return true
	}
	c.row = p
	if l.kind == "value" && x >= l.x0 && x < l.x1 {
		m.configKey("enter") // open it
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
	m.syncConfig()
	lines := m.configLines()
	m.configKeepVisible(lines, rows)
	out := make([]string, 0, rows)
	for i := m.cfg.top; i < len(lines) && len(out) < rows; i++ {
		out = append(out, lines[i].text)
	}
	for len(out) < rows {
		out = append(out, "")
	}
	for i, l := range out {
		out[i] = l + bg.Render(strings.Repeat(" ", max(m.w-lipgloss.Width(l), 0)))
	}
	return out
}
