package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// splash draws the start screen in width×rows: the logo with a joke under it,
// both centered on one axis. A long joke wraps.
func splash(tagline string, width, rows int) []string {
	const jokeW = 40
	block := renderLogo()
	block = append(block, "")
	for _, l := range strings.Split(ansi.Wrap(tagline, min(jokeW, max(width-4, 10)), ""), "\n") {
		block = append(block, hintStyle.Italic(true).Render(strings.TrimSpace(l)))
	}
	return centered(block, width, rows)
}

// centered puts a block of lines in the middle of width×rows.
func centered(block []string, width, rows int) []string {
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
