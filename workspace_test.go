package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestGithubRepository(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:a/b.git":     "a/b",
		"https://github.com/a/b":     "a/b",
		"https://github.com/a/b.git": "a/b",
		"https://gitlab.com/a/b":     "",
		"https://github.com/a/b/c":   "",
		"":                           "",
	} {
		if got := githubRepository(in); got != want {
			t.Errorf("githubRepository(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLinkedPullRequest(t *testing.T) {
	if p := linkedPullRequest("https://github.com/a/b/pull/42"); p.Number != 42 {
		t.Errorf("number = %d", p.Number)
	}
	if p := linkedPullRequest("https://gitlab.com/a/b/-/merge_requests/7"); p.Number != 7 {
		t.Errorf("number = %d", p.Number)
	}
}

func TestFuzzyScore(t *testing.T) {
	if _, ok := fuzzyScore("xyz", "feature/abc"); ok {
		t.Error("xyz should not match")
	}
	prefix, _ := fuzzyScore("fea", "feature/abc")
	scattered, _ := fuzzyScore("fea", "f-x-e-x-a")
	if prefix <= scattered {
		t.Errorf("prefix %d should outrank scattered %d", prefix, scattered)
	}
	if _, ok := fuzzyScore("", "anything"); !ok {
		t.Error("empty query matches everything")
	}
}

func TestStartupPaths(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	os.WriteFile(file, nil, 0o644)
	for _, c := range []struct {
		args      []string
		dir, file string
	}{
		{[]string{dir}, dir, ""},
		{[]string{"-C", dir}, dir, ""},
		{[]string{"--workdir", dir, "f.txt"}, dir, "f.txt"},
	} {
		d, f, err := startupPaths(c.args, io.Discard)
		if err != nil || d != c.dir || f != c.file {
			t.Errorf("%v → %q %q %v", c.args, d, f, err)
		}
	}
	if _, _, err := startupPaths([]string{"a", "b"}, io.Discard); err == nil {
		t.Error("two positional args must fail")
	}
	if _, _, err := startupPaths([]string{"-C", file}, io.Discard); err == nil {
		t.Error("file as workdir must fail")
	}
}

func TestReadRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	if readRepository(dir) != nil {
		t.Fatal("non-repository must give nil")
	}
	run("init", "-b", "main")
	run("commit", "--allow-empty", "-m", "x")
	run("branch", "feature")
	run("remote", "add", "origin", "git@github.com:a/b.git")
	run("config", "branch.main.v-pr", "https://github.com/a/b/pull/5")
	r := readRepository(dir)
	if r == nil || r.Branch != "main" || len(r.Branches) != 2 || r.Link == "" {
		t.Fatalf("unexpected repository: %+v", r)
	}
	if githubRepository(r.Source) != "a/b" {
		t.Errorf("source = %q", r.Source)
	}
	if p := r.selectedPR(); p == nil || p.Number != 5 {
		t.Errorf("selectedPR = %+v", p)
	}
}

func TestOpenPickerListsFilesAndDirs(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "main.go"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0o644)
	m := &model{workdir: dir}
	m.picker = &workspacePicker{kind: "open"}
	m.directoryChoices()
	var got []string
	for _, it := range m.picker.items {
		got = append(got, it.label)
	}
	if len(got) != 2 || got[0] != "main.go" || got[1] != "sub/" {
		t.Errorf("items = %v", got)
	}
	m.picker = &workspacePicker{kind: "workdir"}
	m.directoryChoices()
	if len(m.picker.items) != 1 || m.picker.items[0].label != "sub/" {
		t.Errorf("workdir items = %+v", m.picker.items)
	}
}

func TestPathPreview(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "bin"), []byte{0, 1, 2}, 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	text := func(l previewLines) string {
		s := ""
		for _, line := range l {
			for _, g := range line {
				s += g.text
			}
			s += "\n"
		}
		return s
	}
	for path, want := range map[string]string{
		filepath.Join(dir, "a.go"): "package a",
		filepath.Join(dir, "bin"):  "binary file",
		dir:                        "sub/",
	} {
		_, load := pathPreview(path)
		if got := text(load()); !strings.Contains(got, want) {
			t.Errorf("%s: %q lacks %q", path, got, want)
		}
	}
}

func TestHistoryPreviews(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run("init", "-b", "main")
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("one\n"), 0o644)
	run("add", ".")
	run("commit", "-m", "first")
	cs, err := loadCommits(dir)
	if err != nil || len(cs) != 1 || cs[0].Subject != "first" {
		t.Fatalf("commits = %+v, %v", cs, err)
	}
	if l := branchPreview(dir, "main"); len(l) != 1 {
		t.Errorf("branch preview = %v", l)
	}
	added := false
	for _, line := range commitPreview(dir, cs[0].Hash) {
		for _, g := range line {
			added = added || (g.tok == chroma.GenericInserted && g.text == "+one")
		}
	}
	if !added {
		t.Error("commit preview lacks the added line")
	}
}

func TestPRPreviewPlacesComments(t *testing.T) {
	view := `{"number":7,"title":"T","body":"body","state":"OPEN","author":{"login":"a"},"reviews":[],"comments":[]}`
	diff := "diff --git a/f.go b/f.go\nindex 1..2 100644\n--- a/f.go\n+++ b/f.go\n@@ -1,2 +1,3 @@\n keep\n-old\n+new\n+more\n"
	threads := `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"id":"T1","isResolved":false,"isOutdated":false,"path":"f.go","line":2,"comments":{"nodes":[` +
		`{"databaseId":9,"author":{"login":"r"},"body":"why?","createdAt":"2026-01-01T00:00:00Z","state":"SUBMITTED"},` +
		`{"databaseId":10,"author":{"login":"a"},"body":"because","createdAt":"2026-01-02T00:00:00Z","state":"SUBMITTED"}]}}]}}}}}`
	d, err := parsePR("o/r", "https://github.com/o/r/pull/7", []byte(view), []byte(diff), []byte(threads))
	if err != nil || len(d.Files) != 1 || d.Files[0].Adds != 2 || d.Files[0].Dels != 1 {
		t.Fatalf("parse: %+v %v", d, err)
	}
	var texts []string
	lines, _, _ := d.filePreview(d.Files[0], 60)
	for _, line := range lines {
		s := ""
		for _, g := range line {
			s += g.text
		}
		texts = append(texts, ansi.Strip(s))
	}
	// new line 2 is "+new": the comment must follow it, before "+more".
	at := func(sub string) int {
		for i, s := range texts {
			if strings.Contains(s, sub) {
				return i
			}
		}
		return -1
	}
	if !(at("+new") < at("why?") && at("why?") < at("because") && at("because") < at("+more")) || at("+new") < 0 {
		t.Errorf("comment misplaced: %q", texts)
	}
	if th := d.Files[0].Threads; len(th) != 1 || th[0].ID != "T1" || len(th[0].Comments) != 2 || th[0].Comments[0].ID != 9 {
		t.Errorf("threads = %+v", th)
	}
	if repo, n := parsePRURL("https://github.com/o/r/pull/7/files"); repo != "o/r" || n != 7 {
		t.Errorf("parsePRURL = %q %d", repo, n)
	}
}

func TestComposeOnThePRScreen(t *testing.T) {
	view := `{"number":7,"title":"T","body":"b","state":"OPEN","author":{"login":"a"},"reviews":[],"comments":[],"headRefOid":"abc"}`
	diff := "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,2 +1,2 @@\n keep\n-old\n+new\n"
	threads := `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"id":"T1","isResolved":false,"isOutdated":false,"path":"f.go","line":2,"comments":{"nodes":[` +
		`{"databaseId":9,"author":{"login":"r"},"body":"why?","createdAt":"2026-01-01T00:00:00Z","state":"SUBMITTED"}]}}]}}}}}`
	d, err := parsePR("o/r", "https://github.com/o/r/pull/7", []byte(view), []byte(diff), []byte(threads))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel()
	m.w, m.h, m.mode = 120, 30, modePick
	m.repo = &repository{Root: t.TempDir()}
	m.picker = &workspacePicker{kind: "prdiff", pr: d, list: "files"}
	m.rebuildPicker()
	m.picker.sel = 1 // the Conversation entry comes first, then the files
	// On a file: a reply needs a thread, a new comment is on the file.
	if cmd := m.startCompose("reply"); cmd != nil || m.picker.compose != nil {
		t.Error("a reply must not start on a file item")
	}
	m.startCompose("new")
	if c := m.picker.compose; c == nil || c.kind != "line" || c.file.Path != "f.go" {
		t.Fatalf("compose = %+v", m.picker.compose)
	}
	// A line outside the diff is refused before anything is sent.
	m.omni.SetValue("99 nope")
	if cmd := m.sendCompose(); cmd != nil || m.picker.compose == nil {
		t.Error("a line outside the diff must not be sent")
	}
	m.omni.SetValue("no number")
	if cmd := m.sendCompose(); cmd != nil {
		t.Error("a missing line number must not be sent")
	}
	m.composeKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.picker.compose != nil {
		t.Error("esc must cancel")
	}
	// In the comments list a reply targets the thread.
	m.picker.list = "comments"
	m.rebuildPicker()
	m.picker.sel = 1 // Conversation comes first
	m.startCompose("reply")
	if c := m.picker.compose; c == nil || c.kind != "reply" || c.thread.ID != "T1" || !strings.Contains(c.hint, "@r") {
		t.Fatalf("reply compose = %+v", m.picker.compose)
	}
}

func TestClickingThreadButtonsAndDiffLines(t *testing.T) {
	view := `{"number":7,"title":"T","body":"b","state":"OPEN","author":{"login":"a"},"reviews":[],"comments":[],"headRefOid":"abc"}`
	diff := "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,2 +1,2 @@\n keep\n-old\n+new\n"
	threads := `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"id":"T1","isResolved":false,"isOutdated":false,"path":"f.go","line":2,"comments":{"nodes":[` +
		`{"databaseId":9,"author":{"login":"r"},"body":"why?","createdAt":"2026-01-01T00:00:00Z","state":"SUBMITTED"}]}}]}}}}}`
	d, err := parsePR("o/r", "https://github.com/o/r/pull/7", []byte(view), []byte(diff), []byte(threads))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel()
	m.w, m.h, m.mode = 140, 40, modePick
	m.repo = &repository{Root: t.TempDir()}
	m.picker = &workspacePicker{kind: "prdiff", pr: d, list: "files"}
	m.rebuildPicker()
	m.picker.sel = 1 // f.go
	lw := m.pickerListWidth()
	m.pickerPreview() // fills the click map for this frame

	find := func(kind string) (row int, h prHit) {
		for r, hits := range m.picker.hits {
			for _, x := range hits {
				if x.kind == kind {
					return r, x
				}
			}
		}
		t.Fatalf("no %s hit in %v", kind, m.picker.hits)
		return
	}
	click := func(kind string) tea.Cmd {
		row, h := find(kind)
		return m.previewClick(lw+1+h.x0, headerRows+row-m.picker.pscroll)
	}
	// A diff line starts a comment on that line.
	click("line")
	if c := m.picker.compose; c == nil || c.kind != "line" || c.line == 0 {
		t.Fatalf("line click: %+v", m.picker.compose)
	}
	m.picker.compose = nil
	// Reply opens the composer on the thread.
	click("reply")
	if c := m.picker.compose; c == nil || c.kind != "reply" || c.thread.ID != "T1" {
		t.Fatalf("reply click: %+v", m.picker.compose)
	}
	m.picker.compose = nil
	// Resolve flips the thread at once and asks GitHub in the background.
	if cmd := click("resolve"); cmd == nil || !d.Files[0].Threads[0].Resolved {
		t.Fatalf("resolve click: cmd=%v resolved=%v", cmd, d.Files[0].Threads[0].Resolved)
	}
	m.onPRResolved(prResolvedMsg{thread: "T1", resolved: true, err: fmt.Errorf("denied")})
	if d.Files[0].Threads[0].Resolved {
		t.Error("a refused resolve must flip back")
	}
	// Clicking elsewhere does nothing.
	if cmd := m.previewClick(lw+1, headerRows+500); cmd != nil || m.picker.compose != nil {
		t.Error("a click on nothing must do nothing")
	}
	// The conversation has its own New comment button.
	m.picker.sel = 0
	m.pickerPreview()
	row, h := find("general")
	m.previewClick(lw+1+h.x0, headerRows+row)
	if c := m.picker.compose; c == nil || c.kind != "general" {
		t.Fatalf("general click: %+v", m.picker.compose)
	}
}

func TestCommentBodyIsTinted(t *testing.T) {
	lines := markdownOn("hello `code`", 20)
	if len(lines) != 1 || !strings.Contains(lines[0], bgSeq(cur.Panel)) || strings.Contains(lines[0], bgSeq(cur.Bg)) {
		t.Errorf("not tinted: %q", lines)
	}
	if w := ansi.StringWidth(lines[0]); w != 20 {
		t.Errorf("width = %d, want a solid block of 20", w)
	}
}
