package main

import (
	"strings"
	"testing"
)

func TestApplyHunks(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	got, err := applyHunks(lines, "@@ -2,2 +2,3 @@\n b\n-c\n+C\n+C2\n@@ -5 +6 @@\n-e\n+E\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "a,b,C,C2,d,E" {
		t.Errorf("got %v", got)
	}
	if _, err := applyHunks(lines, "@@ -1 +1 @@\n-x\n+y\n"); err == nil {
		t.Error("a mismatching diff must fail")
	}
	got, err = applyHunks(nil, "@@ -0,0 +1,2 @@\n+new\n+file\n")
	if err != nil || strings.Join(got, ",") != "new,file" {
		t.Errorf("new file: %v, %v", got, err)
	}
	got, err = applyHunks([]string{"x"}, "--- a/f\n+++ b/f\n@@ -1 +1,2 @@\n x\n+y\n\\ No newline at end of file\n")
	if err != nil || strings.Join(got, ",") != "x,y" {
		t.Errorf("headers: %v, %v", got, err)
	}
}

func TestShellCommand(t *testing.T) {
	if got := shellCommand(`/bin/zsh -lc "rg -n foo a.py"`); got != "rg -n foo a.py" {
		t.Errorf("got %q", got)
	}
}
