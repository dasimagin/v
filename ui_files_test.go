package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestToolDetail(t *testing.T) {
	if got := toolDetail("Bash", map[string]any{"command": "go test ./..."}); got != "go test ./..." {
		t.Errorf("Bash: %q", got)
	}
	if got := toolDetail("Edit", map[string]any{"old_string": "a", "new_string": "b\nc"}); got != "- a\n+ b\n+ c" {
		t.Errorf("Edit: %q", got)
	}
	if got := toolDetail("Grep", map[string]any{"pattern": "x", "head_limit": 5.0}); got != "head_limit: 5\npattern: x" {
		t.Errorf("Grep: %q", got)
	}
	if got := resultText([]byte(`[{"type":"text","text":"one"},{"type":"text","text":"two"}]`)); got != "one\ntwo" {
		t.Errorf("resultText: %q", got)
	}
	if got := clipText(strings.Repeat("é", 10), 5); !strings.HasPrefix(got, "éé") || !strings.HasSuffix(got, "… cut") {
		t.Errorf("clipText: %q", got)
	}
}

func TestDiffContextOptions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ob, oa := diffBefore, diffAfter
	t.Cleanup(func() { diffBefore, diffAfter = ob, oa })
	s := Settings{DiffBefore: 3, DiffAfter: 3}
	if err := setUIOption(&s, "diff_before", "8"); err != nil {
		t.Fatal(err)
	}
	if err := setUIOption(&s, "diff_after", "1"); err != nil {
		t.Fatal(err)
	}
	if s.DiffBefore != 8 || s.DiffAfter != 1 || diffBefore != 8 || diffAfter != 1 {
		t.Errorf("settings %d/%d, in use %d/%d", s.DiffBefore, s.DiffAfter, diffBefore, diffAfter)
	}
	if err := setUIOption(&s, "diff_before", "7"); err == nil {
		t.Error("7 is not one of the choices")
	}
}

func TestUIImage(t *testing.T) {
	dir := t.TempDir()
	svg := filepath.Join(dir, "a.svg")
	os.WriteFile(svg, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 192 96"><rect/></svg>`), 0o644)
	i := uiImageOf(svg)
	if i.Err != "" || i.Mime != "image/svg+xml" || i.Width != 192 || i.Height != 96 || !strings.HasPrefix(i.Data, "data:image/svg+xml;base64,") {
		t.Errorf("svg: %+v", i)
	}
	// A 1x1 PNG.
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89, 0, 0, 0, 0x0d, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0xf8, 0xff, 0xff, 0x3f, 0, 5, 0xfe, 2, 0xfe, 0xa7, 0x35, 0x81, 0x84, 0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82}
	p := filepath.Join(dir, "b.png")
	os.WriteFile(p, png, 0o644)
	if i := uiImageOf(p); i.Width != 1 || i.Height != 1 || i.Mime != "image/png" {
		t.Errorf("png: %+v", i)
	}
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("x"), 0o644)
	if i := uiImageOf(filepath.Join(dir, "c.txt")); i.Err == "" {
		t.Error("a text file is not a picture")
	}
}

func TestUICrumbs(t *testing.T) {
	home, _ := os.UserHomeDir()
	work := filepath.Join(home, "a", "b")
	cs := uiCrumbs(work, filepath.Join(work, "c", "d"))
	if len(cs) < 5 || cs[0].Name != "~" || cs[len(cs)-1].Name != "d" || cs[len(cs)-1].Query != "c/d/" {
		t.Errorf("crumbs below the workdir: %+v", cs)
	}
	if cs[len(cs)-3].Name != "b" || cs[len(cs)-3].Query != "" {
		t.Errorf("the workdir itself is typed as nothing: %+v", cs)
	}
	if up := uiCrumbs(work, filepath.Join(home, "a")); up[len(up)-1].Query != "~/a/" {
		t.Errorf("a folder above the workdir is typed with ~: %+v", up)
	}
}

func TestUISaveFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.go")
	os.WriteFile(p, []byte("package a\r\n\r\nfunc A() {}\r\n"), 0o640)
	f := uiReadFile(p)
	if !f.Edit || !f.CRLF || f.Text != "package a\n\nfunc A() {}\n" || f.Abs != p {
		t.Fatalf("read: %+v", f)
	}
	if err := uiSaveFile(p, f.Text+"// more\n", f.CRLF, f.Mtime); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "package a\r\n\r\nfunc A() {}\r\n// more\r\n" {
		t.Errorf("saved with Windows line ends: %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o640 {
		t.Errorf("the mode is kept: %v", fi.Mode())
	}
	// The file changed after f was read: the save must refuse.
	os.WriteFile(p, []byte("other"), 0o640)
	os.Chtimes(p, time.Now().Add(time.Hour), time.Now().Add(time.Hour))
	if err := uiSaveFile(p, "mine", false, f.Mtime); err == nil {
		t.Error("a save over a changed file must be refused")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("no temp files are left: %v", entries)
	}
}
