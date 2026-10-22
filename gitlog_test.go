package main

import (
	"slices"
	"testing"
)

func TestParseRefs(t *testing.T) {
	got := parseRefs("HEAD -> main, tag: v1.0, origin/main, origin/HEAD, feature/x", map[string]bool{"origin": true})
	want := []logRef{{"main", "head"}, {"v1.0", "tag"}, {"origin/main", "remote"}, {"feature/x", "branch"}}
	if !slices.Equal(got, want) {
		t.Errorf("refs = %v, want %v", got, want)
	}
}

// A merge: M has parents A (main line) and B (a side branch); both come from R.
func TestBuildGraph(t *testing.T) {
	cs := []logCommit{
		{Hash: "M", Parents: []string{"A", "B"}},
		{Hash: "A", Parents: []string{"R"}},
		{Hash: "B", Parents: []string{"R"}},
		{Hash: "R"},
	}
	rows := buildGraph(cs)
	if rows[0].Node != 0 || !slices.Equal(rows[0].Out, []int{0, 1}) {
		t.Errorf("merge row: %+v", rows[0])
	}
	if rows[1].Node != 0 || !slices.Equal(rows[1].Pass, []int{1}) {
		t.Errorf("A runs beside the side branch: %+v", rows[1])
	}
	if rows[2].Node != 1 || !slices.Equal(rows[2].Pass, []int{0}) || !slices.Equal(rows[2].Out, []int{0}) {
		t.Errorf("B joins the main line: %+v", rows[2])
	}
	if rows[3].Node != 0 || !slices.Equal(rows[3].In, []int{0}) || len(rows[3].Out) != 0 {
		t.Errorf("the root: %+v", rows[3])
	}
	if got := rows[0].text(2); got != "●─╮" {
		t.Errorf("text of the merge = %q", got)
	}
}
