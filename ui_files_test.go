package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUIPickListOpen(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0o644)

	p := uiPickList(dir, "open", "")
	if len(p.Items) != 2 || p.Items[0].Label != "src/" || !p.Items[0].Dir || p.Items[0].Query != "src/" {
		t.Fatalf("items = %+v", p.Items)
	}
	if got := uiPickList(dir, "open", "mg").Items; len(got) != 1 || got[0].Label != "main.go" {
		t.Errorf("fuzzy = %+v", got)
	}
	if got := uiPickList(dir, "open", "src/").Dir; got == "" {
		t.Error("no dir for src/")
	}
	if up := uiPickList(dir, "open", "src/").Up; up != "" {
		t.Errorf("up from src/ = %q, want the workdir itself (empty query)", up)
	}
}

func TestUIReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	os.WriteFile(path, []byte("package a\n\nfunc F() {}\n"), 0o644)
	f := uiReadFile(path)
	if f.Err != "" || f.Lang != "Go" || len(f.Lines) != 3 {
		t.Fatalf("file = %+v", f)
	}
	if f.Lines[0][0].C != "keyword" {
		t.Errorf("first token class = %q", f.Lines[0][0].C)
	}
	if uiReadFile(dir).Err == "" {
		t.Error("a folder should not open as a file")
	}
}

func TestUIPickListWorkdirAndSwitch(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a"), 0o755)
	os.WriteFile(filepath.Join(dir, "f.txt"), nil, 0o644)

	items := uiPickList(dir, "workdir", "").Items
	if len(items) != 2 || items[0].Label != "· this folder" || items[0].Value != dir || items[1].Label != "a/" {
		t.Fatalf("workdir items = %+v", items)
	}
	if got := uiPickList(dir, "workdir", "a").Items; len(got) != 1 || got[0].Label != "a/" {
		t.Errorf("workdir fuzzy = %+v", got)
	}
	if p := uiPickList(dir, "switch", ""); p.Err == "" {
		t.Errorf("switch outside a repository should say so, got %+v", p)
	}

	wd := &uiWorkdir{dir: dir}
	if got, err := wd.set("a"); err != nil || got != filepath.Join(dir, "a") || wd.get() != got {
		t.Errorf("set = %q, %v", got, err)
	}
	if _, err := wd.set("f.txt"); err == nil {
		t.Error("a file is not a workdir")
	}
}

func TestUIPRView(t *testing.T) {
	d := &prData{Number: 7, Title: "T", Body: "body", Files: []prFile{{
		Path:  "a.go",
		Patch: []string{"diff --git a/a.go b/a.go", "--- a/a.go", "+++ b/a.go", "@@ -1,2 +1,3 @@", " keep", "-gone", "+new", "+more"},
		Adds:  2, Dels: 1,
		Threads: []prThread{{Line: 3, Comments: []prComment{{Author: "r", Body: "hm"}}}, {Line: 0, Outdated: true}},
	}}}
	p := &uiPRs{data: d}

	v := p.View("a.go")
	kinds := ""
	for _, l := range v.Lines {
		kinds += l.K[:1]
	}
	if kinds != "hmmhcda"+"a" {
		t.Errorf("line kinds = %q", kinds)
	}
	if v.Lines[6].No != 2 || len(v.Lines[6].Threads) != 0 || v.Lines[7].No != 3 || len(v.Lines[7].Threads) != 1 {
		t.Errorf("numbers and threads = %+v / %+v", v.Lines[6], v.Lines[7])
	}
	if len(v.Outdated) != 1 {
		t.Errorf("outdated = %+v", v.Outdated)
	}
	if c := p.View(""); c.Body != "body" {
		t.Errorf("conversation = %+v", c)
	}
	if got := (&uiPRs{}).View("a.go"); len(got.Lines) != 0 {
		t.Error("no PR open should give an empty view")
	}
}

func TestUIDetail(t *testing.T) {
	c := uiCommitOf(".", "HEAD")
	if c.Err != "" || c.Subject == "" || len(c.Files) == 0 {
		t.Fatalf("commit: %+v", c)
	}
	for _, f := range c.Files {
		if f.Path == "" || (len(f.Rows) == 0 && !f.Binary) {
			t.Errorf("file %q has no rows", f.Path)
		}
	}
	b := uiBranchOf(".", "HEAD")
	if b.Err != "" || len(b.Commits) == 0 {
		t.Fatalf("branch: %+v", b)
	}
	files, _ := parsePatch("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n a\n-b\n+c\n")
	if len(files) != 1 || files[0].Add != 1 || files[0].Del != 1 || files[0].Rows[2].Old != 2 || files[0].Rows[3].New != 2 {
		t.Errorf("parsePatch: %+v", files)
	}
}
