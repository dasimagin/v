package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// applyHunks applies the hunks of a unified diff ("@@ -a,b +c,d @@" and
// " ", "-", "+" lines; file headers are skipped) to lines. Context and
// removed lines must match.
func applyHunks(lines []string, diff string) ([]string, error) {
	var out []string
	at := 0 // next unconsumed line of the input
	in := false
	for _, l := range strings.Split(strings.TrimSuffix(diff, "\n"), "\n") {
		switch {
		case strings.HasPrefix(l, "@@"):
			start, err := hunkStart(l)
			if err != nil {
				return nil, err
			}
			if start < at {
				return nil, errors.New("hunks overlap")
			}
			if start > len(lines) {
				return nil, fmt.Errorf("hunk starts past the end: line %d", start+1)
			}
			out = append(out, lines[at:start]...)
			at, in = start, true
		case !in, strings.HasPrefix(l, `\`):
			// File headers before the first hunk; "\ No newline at end of file".
		case strings.HasPrefix(l, "+"):
			out = append(out, l[1:])
		case strings.HasPrefix(l, "-"), strings.HasPrefix(l, " "), l == "":
			want := ""
			if l != "" {
				want = l[1:]
			}
			if at >= len(lines) || lines[at] != want {
				return nil, fmt.Errorf("the diff does not match line %d", at+1)
			}
			if !strings.HasPrefix(l, "-") {
				out = append(out, lines[at])
			}
			at++
		}
	}
	return append(out, lines[at:]...), nil
}

// hunkStart is the 0-based first old line of a hunk header "@@ -a,b +c,d @@".
func hunkStart(header string) (int, error) {
	f := strings.Fields(header)
	if len(f) < 3 || !strings.HasPrefix(f[1], "-") {
		return 0, fmt.Errorf("bad hunk header %q", header)
	}
	old := strings.SplitN(f[1][1:], ",", 2)
	n, err := strconv.Atoi(old[0])
	if err != nil {
		return 0, fmt.Errorf("bad hunk header %q", header)
	}
	count := 1
	if len(old) == 2 {
		if count, err = strconv.Atoi(old[1]); err != nil {
			return 0, fmt.Errorf("bad hunk header %q", header)
		}
	}
	if count == 0 {
		return n, nil // an insertion after line n
	}
	return max(n-1, 0), nil
}
