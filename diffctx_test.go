package main

import (
	"strings"
	"testing"
)

func withContext(t *testing.T, before, after int) {
	t.Helper()
	ob, oa := diffBefore, diffAfter
	diffBefore, diffAfter = before, after
	t.Cleanup(func() { diffBefore, diffAfter = ob, oa })
}

func TestDiffContextSizes(t *testing.T) {
	base := "1\n2\n3\n4\n5\n6\n7\n8\n9\n"
	cur := "1\n2\n3\n4\nX\n6\n7\n8\n9\n"
	for _, c := range []struct {
		before, after int
		want          string
	}{
		{3, 3, "h" + "ccc" + "x" + "a" + "ccc"},
		{1, 2, "h" + "c" + "x" + "a" + "cc"},
		{0, 0, "h" + "x" + "a"},
	} {
		withContext(t, c.before, c.after)
		rows, _ := unifiedRows(docFrom(base, cur))
		if got := kinds(rows, false); got != c.want {
			t.Errorf("context %d/%d: kinds = %q, want %q", c.before, c.after, got, c.want)
		}
	}
}

func TestReflowPatch(t *testing.T) {
	patch := strings.Split("@@ -1,7 +1,7 @@ func f\n a\n b\n c\n-d\n+D\n e\n f\n g", "\n")
	got := strings.Join(reflowPatch(patch, 1, 2, nil), "\n")
	want := "@@ -3,4 +3,4 @@ func f\n c\n-d\n+D\n e\n f"
	if got != want {
		t.Errorf("reflow =\n%s\nwant\n%s", got, want)
	}
	// A pinned context line stays visible with its own context.
	pinned := reflowPatch(patch, 0, 0, func(kind byte, no int) bool { return kind == 'R' && no == 7 })
	if j := strings.Join(pinned, "\n"); !strings.Contains(j, " g") || !strings.Contains(j, "+D") {
		t.Errorf("pinned line lost: %s", j)
	}
}
