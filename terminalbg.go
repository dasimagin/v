package main

import (
	"fmt"
	"os"
)

// oscBackground is the sequence that sets the terminal's default background
// (OSC 11), or resets it when hex is empty (OSC 111). Inside tmux it is wrapped
// to pass through, which tmux allows only with allow-passthrough on.
func oscBackground(hex string, tmux bool) string {
	seq := "\x1b]111\x07"
	if hex != "" {
		seq = fmt.Sprintf("\x1b]11;%s\x07", hex)
	}
	if tmux {
		return "\x1bPtmux;" + replaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
	}
	return seq
}

func replaceAll(s, old, new string) string {
	out := ""
	for _, r := range s {
		if string(r) == old {
			out += new
		} else {
			out += string(r)
		}
	}
	return out
}

// writeTerminal writes a control sequence to the terminal, if stdout is one.
func writeTerminal(s string) {
	if fi, err := os.Stdout.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return
	}
	_, _ = os.Stdout.WriteString(s)
}

// syncTerminalBackground makes the terminal's own background the theme's, so
// its margins and padding match the screen.
func syncTerminalBackground() { writeTerminal(oscBackground(cur.Bg, os.Getenv("TMUX") != "")) }

// resetTerminalBackground gives the terminal its own background back.
func resetTerminalBackground() { writeTerminal(oscBackground("", os.Getenv("TMUX") != "")) }

// applyBackground puts the Background setting into effect: the styles are
// rebuilt, and the terminal's background is set or given back.
func (m *model) applyBackground() {
	transparentBG = m.settings.Background == "terminal"
	m.setTheme(cur)
	previews = map[string]previewEntry{}
	if m.settings.Background == "sync" {
		syncTerminalBackground()
	} else {
		resetTerminalBackground()
	}
}
