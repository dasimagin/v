package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

// taglines are the jokes under the logo: taglines.txt, one per line, or the
// user's own ~/.v/taglines.txt in its place.
var taglines = loadTaglines()

//go:embed taglines.txt
var builtinTaglinesText string

func splitTaglines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}

func builtinTaglines() []string { return splitTaglines(builtinTaglinesText) }

func loadTaglines() []string {
	if b, err := os.ReadFile(filepath.Join(configDir(), "taglines.txt")); err == nil {
		if t := splitTaglines(string(b)); len(t) > 1 {
			return t
		}
	}
	return builtinTaglines()
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

// renderLogo colors the face blocks with the theme accent gradient and the shadow darker.
func renderLogo() []string {
	out := make([]string, len(logo))
	for i, line := range logo {
		t := float64(i) / float64(len(logo)-1)
		face := bg.Foreground(gradient(cur.Accent, cur.Accent2, t)).Bold(true)
		// The shadow is the face color sunk halfway into the background.
		shadow := bg.Foreground(gradient(string(gradient(cur.Accent, cur.Bg, 0.5)), string(gradient(cur.Accent2, cur.Bg, 0.5)), t))
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

// center pads a rendered line to width on both sides.
func center(s string, width int) string {
	w := lipgloss.Width(s)
	left := max((width-w)/2, 0)
	return bg.Render(strings.Repeat(" ", left)) + s + bg.Render(strings.Repeat(" ", max(width-left-w, 0)))
}
