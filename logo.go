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
	"There are two hard things: naming, cache invalidation, and off-by-one errors...",
	"Weeks of coding save hours of planning...",
	"Hello, World. Now what...",
	"Move fast and fix it later...",
	"The cloud is just someone else's computer...",
	"Legacy code: it works, nobody knows why...",
	"Fixed one bug, found three...",
	"Tests are documentation that can fail...",
	"It worked yesterday...",
	"Ctrl+C, Ctrl+V, ship...",
	"No code is the best code...",
	"Rubber duck debugging in progress...",
	"Tabs or spaces? Yes...",
	"Premature optimization is the root of all evil...",
	"A watched build never finishes...",
	"Why is it slow? It is always the database...",
	"Ask forgiveness, not permission, in prod...",
	"Compiling. Time for a coffee...",
	"The tests passed. Suspicious...",
	"Just one more refactor...",
	"Merge conflicts build character...",
	"Estimated: two days. Actual: two weeks...",
	"Have you read the logs?",
	"It's always DNS...",
	"Debugging: being the detective in a crime movie where you are also the murderer...",
	"Coffee.exe has stopped responding...",
	"Make it work, make it right, make it fast...",
	"Trust, but verify the diff...",
	"Magic is just code you have not read yet...",
	"Reviewing with love, shipping with hope...",
	"Green build, sleepy reviewer...",
	"One does not simply merge on Friday...",
	"The best code is the code you deleted...",
	"Cache me if you can...",
	"Semicolons are a state of mind...",
	"It depends...",
	"Another day, another dependency...",
	"The PR was small, they said...",
	// Code stays as typed.
	"git push --force and pray",
	"// TODO: write tests",
	"import antigravity",
	"if err != nil { return err }",
	"Segmentation fault (core dumped)",
	"sudo make me a sandwich",
	"while (!asleep) { code(); }",
	"rm -rf node_modules && pray",
	"git commit -m \"fix\"",
	"404: motivation not found",
	"panic: not enough coffee",
	"git blame yourself",
	"0 errors, 0 warnings, 1 surprise",
	"[ OK ] Reached target Vibes",
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
