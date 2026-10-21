package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
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
