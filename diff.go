package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// viewMode is how the file viewer shows a file.
type viewMode int

const (
	viewFlat viewMode = iota
	viewUnified
	viewSplit
)

var viewModeNames = []string{"Flat", "Unified", "Side-by-Side"}

// Base is the version of a file in git HEAD that diffs compare against.
type Base struct {
	Lines [][]seg
	Text  []string
	New   bool // not in HEAD yet: everything is added
}

// loadBase reads path from HEAD of its git repository. It returns nil when
// the file is not inside a repository.
func loadBase(path string) *Base {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	dir := filepath.Dir(abs)
	top, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil
	}
	root, err := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	if err != nil {
		return nil
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return nil
	}
	out, err := exec.Command("git", "-C", root, "show", "HEAD:"+filepath.ToSlash(rel)).Output()
	if err != nil {
		return &Base{New: true} // untracked, or no commits yet
	}
	_, lines, err := highlight(path, string(out))
	if err != nil {
		return &Base{New: true}
	}
	return &Base{Lines: lines, Text: texts(lines)}
}

// texts returns the plain text of highlighted lines.
func texts(lines [][]seg) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		var b strings.Builder
		for _, s := range l {
			b.WriteString(s.text)
		}
		out[i] = b.String()
	}
	return out
}

type rowKind int

const (
	rowCtx rowKind = iota
	rowAdd
	rowDel
	rowHunk
	rowNone // filler on one side of a side-by-side row
)

// side is one line of a diff: its number in its version (1-based) and tokens.
type side struct {
	kind rowKind
	no   int
	segs []seg
}

// diffRow is one screen row. Unified rows use only Left; side-by-side rows
// put HEAD on the left and the working copy on the right. Hunk rows carry the
// "@@ -a,b +c,d @@" header in head.
type diffRow struct {
	left, right side
	oldNo       int // unified: line number in HEAD, 0 if none
	head        string
}

const diffContext = 3

// hunks groups the changes between the base and the working copy, git
// style. While editing, the whole file is one group, so every line can be
// reached; such a group has no hunk header.
func hunks(d *Doc) (groups [][]difflib.OpCode, headers bool) {
	m := difflib.NewMatcher(d.Base.Text, d.Text)
	if d.Ed.On {
		return [][]difflib.OpCode{m.GetOpCodes()}, false
	}
	return m.GetGroupedOpCodes(diffContext), true
}

func hunkHeader(g []difflib.OpCode) string {
	first, last := g[0], g[len(g)-1]
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@", first.I1+1, last.I2-first.I1, first.J1+1, last.J2-first.J1)
}

// rowIndex maps each working-copy line to its row; -1 for lines not shown.
func rowIndex(n int) []int {
	at := make([]int, n)
	for i := range at {
		at[i] = -1
	}
	return at
}

// unifiedRows lays out the diff like `git diff`.
func unifiedRows(d *Doc) ([]diffRow, []int) {
	b := d.Base
	var rows []diffRow
	at := rowIndex(len(d.Text))
	groups, headers := hunks(d)
	for _, g := range groups {
		if headers {
			rows = append(rows, diffRow{left: side{kind: rowHunk}, head: hunkHeader(g)})
		}
		for _, op := range g {
			if op.Tag == 'e' {
				for k := 0; k < op.J2-op.J1; k++ {
					at[op.J1+k] = len(rows)
					rows = append(rows, diffRow{left: side{rowCtx, op.J1 + k + 1, d.Lines[op.J1+k]}, oldNo: op.I1 + k + 1})
				}
				continue
			}
			if op.Tag == 'd' || op.Tag == 'r' {
				for i := op.I1; i < op.I2; i++ {
					rows = append(rows, diffRow{left: side{rowDel, 0, b.Lines[i]}, oldNo: i + 1})
				}
			}
			if op.Tag == 'i' || op.Tag == 'r' {
				for j := op.J1; j < op.J2; j++ {
					at[j] = len(rows)
					rows = append(rows, diffRow{left: side{rowAdd, j + 1, d.Lines[j]}})
				}
			}
		}
	}
	return rows, at
}

// splitRows lays out the diff side by side: base left, working copy right.
func splitRows(d *Doc) ([]diffRow, []int) {
	b := d.Base
	var rows []diffRow
	at := rowIndex(len(d.Text))
	groups, headers := hunks(d)
	for _, g := range groups {
		if headers {
			rows = append(rows, diffRow{left: side{kind: rowHunk}, right: side{kind: rowHunk}, head: hunkHeader(g)})
		}
		for _, op := range g {
			n := max(op.I2-op.I1, op.J2-op.J1)
			for k := 0; k < n; k++ {
				var r diffRow
				r.left, r.right = side{kind: rowNone}, side{kind: rowNone}
				if i := op.I1 + k; i < op.I2 {
					kind := rowDel
					if op.Tag == 'e' {
						kind = rowCtx
					}
					r.left = side{kind, i + 1, b.Lines[i]}
				}
				if j := op.J1 + k; j < op.J2 {
					kind := rowAdd
					if op.Tag == 'e' {
						kind = rowCtx
					}
					r.right = side{kind, j + 1, d.Lines[j]}
					at[j] = len(rows)
				}
				rows = append(rows, r)
			}
		}
	}
	return rows, at
}
