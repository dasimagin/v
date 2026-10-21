package main

import (
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// headerRows is an empty row, the screen title and the rule below it.
const headerRows = 3

const toolbarRow = 1

// header draws a screen header. title is already styled; info is shown
// muted after it; button (styled, may be empty) sits at the right edge.
func (m *model) header(title, info, button string) []string {
	if button == "" {
		st := buttonStyle
		if m.viewMenu != nil {
			st = buttonOpen
		}
		button = st.Render(m.modeLabel())
	}
	if m.mode == modePick && m.picker != nil && m.picker.kind == "prdiff" {
		button = m.submitButton() + bg.Render(" ") + m.listButton() + bg.Render(" ") + button
	}
	left := bg.Render(" ") + brand() + bg.Render("   ") + title
	if info != "" {
		left += hintStyle.Render("   " + info)
	}
	right := button
	if right != "" {
		right += bg.Render(" ")
	}
	rw := lipgloss.Width(right)
	left = ansi.Truncate(left, max(m.w-rw-1, 0), "…")
	gap := max(m.w-lipgloss.Width(left)-rw, 0)
	return []string{
		blank(m.w),
		left + blank(gap) + right,
		sepStyle.Render(strings.Repeat("─", m.w)),
	}
}

// brandPhase moves the shimmer of the brand; the animation tick advances it.
var brandPhase float64

// triangle folds x into [0, 1] going up and down, so a color can slide to and fro.
func triangle(x float64) float64 {
	x -= 2 * float64(int(x/2))
	if x < 0 {
		x += 2
	}
	if x > 1 {
		return 2 - x
	}
	return x
}

// brand is "Vibing…" in bold, each letter a step along the logo gradient; the
// gradient slides along the word as the phase moves, so the colors shimmer.
func brand() string { return brandAt(brandPhase) }

func brandAt(phase float64) string {
	const word = "Vibing…"
	runes := []rune(word)
	var b strings.Builder
	for i, r := range runes {
		t := triangle(phase + float64(i)/float64(len(runes)-1)*0.9)
		b.WriteString(bg.Foreground(gradient(cur.Accent2, cur.Accent, t)).Bold(true).Render(string(r)))
	}
	return b.String()
}

// brandTickMsg advances the shimmer.
type brandTickMsg struct{}

const (
	brandFrame = 90 * time.Millisecond // the shimmer's frame rate
	brandStep  = 0.018                 // phase per frame: a full slide takes about 10 s
)

// brandTick schedules the next frame, or a slow check of the setting when the animation is off.
func brandTick(animate bool) tea.Cmd {
	d := brandFrame
	if !animate {
		d = time.Second
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return brandTickMsg{} })
}

// pathTitle shows a path with the directory muted and the name in bold.
// Files are relative to the working directory; directories are shown in
// full (with ~), since "." says little.
func (m *model) pathTitle(p string, dir bool) string {
	if dir {
		p = strings.TrimSuffix(tildify(p), "/")
		d, name := filepath.Split(p)
		return hintStyle.Render(d) + fgStyle.Bold(true).Render(name+"/")
	}
	d, name := filepath.Split(shortPath(p, m.workdir))
	return hintStyle.Render(d) + fgStyle.Bold(true).Render(name)
}
