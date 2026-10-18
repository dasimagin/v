package main

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestMarkdown(t *testing.T) {
	lines := markdown("## Defer\n\n- **bold** and `code`\n\n```go\nx := 1\n```\n", 60)
	plain := xansi.Strip(strings.Join(lines, "\n"))
	if strings.Contains(plain, "##") {
		t.Errorf("heading marks left in:\n%s", plain)
	}
	for _, want := range []string{"Defer", "• bold and code", "x := 1"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q in:\n%s", want, plain)
		}
	}
	if strings.Contains(strings.Join(lines, ""), "38;5;") {
		t.Error("code block should use 24-bit colors")
	}
	for _, l := range lines {
		if w := xansi.StringWidth(l); w > 60 {
			t.Errorf("line wider than 60: %d", w)
		}
	}
}
