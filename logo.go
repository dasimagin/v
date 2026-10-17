package main

import (
	"fmt"
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

var tagStyle = bg.Foreground(lipgloss.Color(colMuted)).Italic(true)

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

// splash returns the logo and the tagline, centered in width×rows.
func splash(tagline string, width, rows int) []string {
	block := append(renderLogo(), "", tagStyle.Render(tagline))
	top := max((rows-len(block))/2, 0)
	out := make([]string, rows)
	for i := range out {
		line := ""
		if i >= top && i < top+len(block) {
			line = block[i-top]
		}
		out[i] = center(line, width)
	}
	return out
}

// center pads a rendered line to width on both sides.
func center(s string, width int) string {
	w := lipgloss.Width(s)
	left := max((width-w)/2, 0)
	return bg.Render(strings.Repeat(" ", left)) + s + bg.Render(strings.Repeat(" ", max(width-left-w, 0)))
}
