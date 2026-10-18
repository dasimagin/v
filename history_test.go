package main

import "testing"

func TestHistoryPrefixSearch(t *testing.T) {
	h := history{pos: -1, items: []string{"fix tests", "/open main.go", "fix the build", "explain this"}}
	step := func(got string, ok bool, want string) {
		t.Helper()
		if !ok || got != want {
			t.Fatalf("got %q, %v; want %q", got, ok, want)
		}
	}
	s, ok := h.older("fix")
	step(s, ok, "fix the build")
	s, ok = h.older(s)
	step(s, ok, "fix tests")
	if _, ok := h.older("fix tests"); ok {
		t.Fatal("no older match expected")
	}
	s, ok = h.newer()
	step(s, ok, "fix the build")
	s, ok = h.newer()
	step(s, ok, "fix") // back to the draft
	if h.pos != -1 {
		t.Errorf("browsing should end, pos = %d", h.pos)
	}
	s, ok = h.older("")
	step(s, ok, "explain this")
}
