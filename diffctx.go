package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// Lines of context shown before and after each change, in every diff:
// file views, agent proposals, commits and PRs. Set from the settings.
var diffBefore, diffAfter = 3, 3

var diffContextChoices = []int{0, 1, 2, 3, 5, 8, 10, 15, 20, 30, 50}

func applyDiffSettings(s Settings) {
	diffBefore, diffAfter = max(s.DiffBefore, 0), max(s.DiffAfter, 0)
	previews = map[string]previewEntry{}
}

// groupOpCodes is difflib's GetGroupedOpCodes with separate context sizes:
// after lines follow a change, before lines precede the next one.
func groupOpCodes(m *difflib.SequenceMatcher, before, after int) [][]difflib.OpCode {
	codes := m.GetOpCodes()
	if len(codes) == 0 {
		codes = []difflib.OpCode{{Tag: 'e', I1: 0, I2: 1, J1: 0, J2: 1}}
	}
	if c := codes[0]; c.Tag == 'e' {
		codes[0] = difflib.OpCode{Tag: c.Tag, I1: max(c.I1, c.I2-before), I2: c.I2, J1: max(c.J1, c.J2-before), J2: c.J2}
	}
	if last := len(codes) - 1; codes[last].Tag == 'e' {
		c := codes[last]
		codes[last] = difflib.OpCode{Tag: c.Tag, I1: c.I1, I2: min(c.I2, c.I1+after), J1: c.J1, J2: min(c.J2, c.J1+after)}
	}
	var groups [][]difflib.OpCode
	var group []difflib.OpCode
	for _, c := range codes {
		i1, i2, j1, j2 := c.I1, c.I2, c.J1, c.J2
		if c.Tag == 'e' && i2-i1 > before+after {
			group = append(group, difflib.OpCode{Tag: c.Tag, I1: i1, I2: min(i2, i1+after), J1: j1, J2: min(j2, j1+after)})
			groups = append(groups, group)
			group = nil
			i1, j1 = max(i1, i2-before), max(j1, j2-before)
		}
		group = append(group, difflib.OpCode{Tag: c.Tag, I1: i1, I2: i2, J1: j1, J2: j2})
	}
	if len(group) > 0 && !(len(group) == 1 && group[0].Tag == 'e') {
		groups = append(groups, group)
	}
	return groups
}

type patchLine struct {
	text     string
	kind     byte // ' ', '+', '-' or '\\' for "No newline at end of file"
	old, new int  // line numbers; 0 where the line is absent
	hunk     int
}

// reflowPatch re-trims the context of unified-diff text to the configured
// before/after sizes. It can only drop context the patch already has, so ask
// git for at least that much. Lines for which pin reports true count as
// changes: review comments stay visible. Text without hunks passes through.
func reflowPatch(lines []string, before, after int, pin func(kind byte, no int) bool) []string {
	var out []string
	for i := 0; i < len(lines); {
		if !strings.HasPrefix(lines[i], "@@") {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i
		for j < len(lines) && !strings.HasPrefix(lines[j], "diff --git") {
			j++
		}
		out = append(out, reflowHunks(lines[i:j], before, after, pin)...)
		i = j
	}
	return out
}

func reflowHunks(lines []string, before, after int, pin func(kind byte, no int) bool) []string {
	var flat []patchLine
	var heads []string // the text after the second @@ of each hunk header
	hunk := -1
	oldN, newN := 0, 0
	for _, l := range lines {
		if m := hunkPattern.FindStringSubmatch(l); m != nil {
			hunk++
			head := ""
			if k := strings.Index(l[2:], "@@"); k >= 0 {
				head = l[k+4:]
			}
			heads = append(heads, head)
			oldN, _ = strconv.Atoi(m[1])
			newN, _ = strconv.Atoi(m[2])
			continue
		}
		pl := patchLine{text: l, kind: ' ', hunk: hunk}
		if l != "" {
			pl.kind = l[0]
		}
		switch pl.kind {
		case '+':
			pl.new = newN
			newN++
		case '-':
			pl.old = oldN
			oldN++
		case '\\':
		default:
			pl.kind, pl.old, pl.new = ' ', oldN, newN
			oldN++
			newN++
		}
		flat = append(flat, pl)
	}
	isChange := func(p patchLine) bool {
		return p.kind == '+' || p.kind == '-' || (pin != nil && p.kind == ' ' && pin('R', p.new))
	}
	keep := make([]bool, len(flat))
	for i, p := range flat {
		if !isChange(p) {
			continue
		}
		keep[i] = true
		for k, n := i-1, 0; k >= 0 && n < before && flat[k].hunk == p.hunk; k-- {
			if flat[k].kind != '\\' {
				n++
			}
			keep[k] = true
		}
		for k, n := i+1, 0; k < len(flat) && n < after && flat[k].hunk == p.hunk; k++ {
			if flat[k].kind != '\\' {
				n++
			}
			keep[k] = true
		}
	}
	for i, p := range flat {
		if p.kind == '\\' && i > 0 {
			keep[i] = keep[i-1]
		}
	}
	var out []string
	for i := 0; i < len(flat); {
		if !keep[i] {
			i++
			continue
		}
		j := i
		for j < len(flat) && keep[j] && flat[j].hunk == flat[i].hunk {
			j++
		}
		oldStart, newStart, oldCount, newCount := 0, 0, 0, 0
		for _, p := range flat[i:j] {
			if p.old > 0 && oldStart == 0 {
				oldStart = p.old
			}
			if p.new > 0 && newStart == 0 {
				newStart = p.new
			}
			if p.kind != '+' && p.kind != '\\' {
				oldCount++
			}
			if p.kind != '-' && p.kind != '\\' {
				newCount++
			}
		}
		if oldStart == 0 { // only additions: git anchors the range at the line before
			oldStart = anchor(flat[:i], flat[i:j], true)
		}
		if newStart == 0 {
			newStart = anchor(flat[:i], flat[i:j], false)
		}
		out = append(out, fmt.Sprintf("@@ -%s +%s @@%s", rng(oldStart, oldCount), rng(newStart, newCount), heads[flat[i].hunk]))
		for _, p := range flat[i:j] {
			out = append(out, p.text)
		}
		i = j
	}
	return out
}

// anchor is the number git would put in a header whose side has no lines: the line before the range.
func anchor(before, run []patchLine, old bool) int {
	for _, p := range run {
		n := p.new
		if old {
			n = p.old
		}
		if n > 0 {
			return n
		}
	}
	for k := len(before) - 1; k >= 0; k-- {
		n := before[k].new
		if old {
			n = before[k].old
		}
		if n > 0 {
			return n
		}
	}
	return 0
}

func rng(start, count int) string {
	if count == 1 {
		return strconv.Itoa(start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}
