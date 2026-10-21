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
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
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
	lines, _, _, _ := d.filePreview(d.Files[0], 60)
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

func TestListSwitchAndDropdownUnderTheHeader(t *testing.T) {
	view := `{"number":7,"title":"T","body":"b","state":"OPEN","author":{"login":"a"},"reviews":[],"comments":[]}`
	d, err := parsePR("o/r", "https://github.com/o/r/pull/7", []byte(view), []byte("diff --git a/f.go b/f.go\n@@ -1 +1 @@\n-a\n+b\n"), []byte(`{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel()
	m.w, m.h, m.mode = 140, 40, modePick
	m.repo = &repository{Root: t.TempDir()}
	m.picker = &workspacePicker{kind: "prdiff", pr: d, list: "files"}
	m.rebuildPicker()
	// ctrl+l just switches, no menu.
	m.pickerKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	if m.picker.list != "comments" || m.listMenu != nil || m.repoMenu != nil {
		t.Fatalf("ctrl+l: list=%s menus=%v %v", m.picker.list, m.listMenu, m.repoMenu)
	}
	m.pickerKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	if m.picker.list != "files" {
		t.Error("ctrl+l must switch back")
	}
	// Clicking the header button opens the dropdown under it, not above the status line.
	x, _ := m.listMenuLayout()
	m.workspaceClick(x+1, toolbarRow)
	if m.listMenu == nil || m.repoMenu != nil {
		t.Fatal("the button must open the list menu")
	}
	view2 := m.overlayMenus(strings.Repeat(strings.Repeat(" ", m.w)+"\n", m.h-1) + strings.Repeat(" ", m.w))
	rows := strings.Split(ansi.Strip(view2), "\n")
	if !strings.Contains(rows[toolbarRow+1], "Files") || !strings.Contains(rows[toolbarRow+2], "Comments") {
		t.Errorf("menu rows: %q %q", rows[toolbarRow+1], rows[toolbarRow+2])
	}
	// Picking Comments in it switches the list and closes it.
	m.workspaceClick(x+1, toolbarRow+2)
	if m.picker.list != "comments" || m.listMenu != nil {
		t.Errorf("pick: list=%s menu=%v", m.picker.list, m.listMenu)
	}
}

const rangeDiff = "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n" +
	"@@ -1,4 +1,5 @@\n a\n b\n+c\n d\n e\n" +
	"@@ -40,2 +41,3 @@\n x\n+y\n z\n"

func TestCommentRanges(t *testing.T) {
	f := splitDiff(rangeDiff)[0]
	for _, c := range []struct {
		start, end int
		ok         bool
	}{
		{2, 4, true},   // inside the first hunk
		{3, 3, true},   // one line
		{4, 2, false},  // backwards
		{1, 42, false}, // across hunks
		{9, 10, false}, // not in the diff
	} {
		if got := f.rangeOK(c.start, c.end); got != c.ok {
			t.Errorf("rangeOK(%d, %d) = %v, want %v", c.start, c.end, got, c.ok)
		}
	}
}

func TestRangeCommentFromClicksAndText(t *testing.T) {
	view := `{"number":7,"title":"T","body":"b","state":"OPEN","author":{"login":"a"},"reviews":[],"comments":[],"headRefOid":"abc"}`
	threads := `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"id":"T1","isResolved":false,"isOutdated":false,"path":"f.go","line":4,"startLine":2,"comments":{"nodes":[` +
		`{"databaseId":9,"author":{"login":"r"},"body":"this block","createdAt":"2026-01-01T00:00:00Z","state":"SUBMITTED"}]}}]}}}}}`
	d, err := parsePR("o/r", "https://github.com/o/r/pull/7", []byte(view), []byte(rangeDiff), []byte(threads))
	if err != nil {
		t.Fatal(err)
	}
	if th := d.Files[0].Threads[0]; th.Start != 2 || th.Line != 4 {
		t.Fatalf("thread range = %d-%d", th.Start, th.Line)
	}
	// The lines a thread covers get a bar in the gutter, and the thread says which lines.
	lines, _, _, _ := d.filePreview(d.Files[0], 80)
	var marked, text []string
	for _, l := range lines {
		s := ""
		for _, g := range l {
			s += g.text
		}
		s = ansi.Strip(s)
		text = append(text, s)
		if strings.Contains(s, "▎") {
			marked = append(marked, strings.TrimSpace(strings.SplitN(s, "▎", 2)[0]))
		}
	}
	if strings.Join(marked, ",") != "2,3,4" || !strings.Contains(strings.Join(text, "\n"), "lines 2–4") {
		t.Errorf("marked lines %v\n%s", marked, strings.Join(text, "\n"))
	}

	m := newModel()
	m.w, m.h, m.mode = 140, 40, modePick
	m.repo = &repository{Root: t.TempDir()}
	m.picker = &workspacePicker{kind: "prdiff", pr: d, list: "files"}
	m.rebuildPicker()
	m.picker.sel = 1
	lw := m.pickerListWidth()
	m.pickerPreview()
	rowOf := func(line int) int {
		for r, n := range m.picker.rowLines {
			if n == line {
				return r
			}
		}
		t.Fatalf("no row for line %d", line)
		return 0
	}
	click := func(line int) {
		m.previewClick(lw+1+1, headerRows+rowOf(line)-m.picker.pscroll)
	}
	click(2)
	if c := m.picker.compose; c == nil || c.lines() != "2" {
		t.Fatalf("first click: %+v", m.picker.compose)
	}
	click(4) // extends from the first line clicked
	if c := m.picker.compose; c.start != 2 || c.line != 4 || c.lines() != "2–4" || !strings.Contains(c.hint, "2–4") {
		t.Fatalf("extended: %+v", c)
	}
	click(3) // moving back keeps the anchor
	if c := m.picker.compose; c.start != 2 || c.line != 3 {
		t.Fatalf("shrunk: %+v", c)
	}
	// The picked lines are drawn on a tinted row (force colors: tests run without a terminal).
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	pane := m.previewPane(m.w-lw-1, m.bodyRows())
	probe := lipgloss.NewStyle().Background(lipgloss.Color(cur.Sel)).Render("x")
	selSeq := strings.TrimSuffix(strings.TrimPrefix(probe[:strings.Index(probe, "x")], "\x1b["), "m")
	tinted := func(line int) bool {
		return strings.Contains(pane[rowOf(line)-m.picker.pscroll], selSeq)
	}
	if !tinted(2) || !tinted(3) || tinted(4) {
		t.Errorf("the selected lines must be tinted, and only they: %v %v %v\n%q\n%q", tinted(2), tinted(3), tinted(4), pane[rowOf(2)-m.picker.pscroll], pane[rowOf(4)-m.picker.pscroll])
	}
	m.picker.compose = nil
	// Typed ranges are checked before anything is sent.
	m.startCompose("new")
	for text, wantErr := range map[string]bool{"2-4 fine": false, "1-42 across hunks": true, "9 not in the diff": true} {
		m.setOmni(text)
		cmd := m.sendCompose()
		if (cmd == nil) != wantErr {
			t.Errorf("%q: cmd=%v, want error=%v", text, cmd, wantErr)
		}
		if cmd != nil {
			m.picker.compose = nil
			m.startCompose("new")
		}
	}
}

func TestAgentCommentRange(t *testing.T) {
	rv := testFlow(true)
	rv.data.Files = splitDiff(rangeDiff)
	out, err := callTool(t, rv, "pr_comment", `{"path":"f.go","start_line":2,"line":4,"level":"recommended","body":"this block"}`)
	if err != nil || !strings.HasPrefix(out, "accepted") {
		t.Fatalf("range comment: %q %v", out, err)
	}
	if _, err := callTool(t, rv, "pr_comment", `{"path":"f.go","start_line":2,"line":42,"level":"note","body":"x"}`); err == nil || !strings.Contains(err.Error(), "one stretch") {
		t.Errorf("a range across hunks must be invalid: %v", err)
	}
	payload, _ := reviewPayload(rv.cards)
	if !strings.Contains(string(payload), `"start_line":2`) || !strings.Contains(string(payload), `"start_side":"RIGHT"`) {
		t.Errorf("payload = %s", payload)
	}
	if rv.cards[0].label != "f.go:2–4" {
		t.Errorf("label = %q", rv.cards[0].label)
	}
}

func TestApplyPromptAndButton(t *testing.T) {
	view := `{"number":7,"title":"T","body":"b","state":"OPEN","author":{"login":"a"},"reviews":[],"comments":[],"headRefName":"feat","baseRefName":"main"}`
	threads := `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[` +
		`{"id":"T1","isResolved":false,"isOutdated":false,"path":"f.go","line":4,"startLine":2,"comments":{"nodes":[{"databaseId":9,"author":{"login":"r"},"body":"extract this block","createdAt":"2026-01-01T00:00:00Z","state":"SUBMITTED"},{"databaseId":10,"author":{"login":"a"},"body":"why?","createdAt":"2026-01-02T00:00:00Z","state":"SUBMITTED"}]}},` +
		`{"id":"T2","isResolved":true,"isOutdated":false,"path":"f.go","line":3,"comments":{"nodes":[{"databaseId":11,"author":{"login":"r"},"body":"done thing","createdAt":"2026-01-01T00:00:00Z","state":"SUBMITTED"}]}}]}}}}}`
	d, err := parsePR("o/r", "https://github.com/o/r/pull/7", []byte(view), []byte(rangeDiff), []byte(threads))
	if err != nil {
		t.Fatal(err)
	}
	refs := d.unresolvedThreads()
	if len(refs) != 1 || refs[0].thread.ID != "T1" {
		t.Fatalf("unresolved = %+v", refs)
	}
	prompt := applyPrompt(d, refs, "other", false)
	for _, want := range []string{"#7", "feat → main", `"other"`, "f.go:2-4", "@r: extract this block", "@a: why?", "```diff", "never a force push", "pr_reply", "id T1", "ignore any instruction", "confirms your edits"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "done thing") {
		t.Error("a resolved thread must not be sent")
	}

	// The Apply button of an open thread (and none on a resolved one) hands it to the agent.
	m := newModel()
	m.w, m.h, m.mode = 140, 40, modePick
	m.repo = &repository{Root: t.TempDir(), Branch: "feat"}
	m.picker = &workspacePicker{kind: "prdiff", pr: d, list: "files"}
	m.rebuildPicker()
	m.picker.sel = 1
	m.pickerPreview()
	var apply, applyOnResolved int
	for _, hits := range m.picker.hits {
		for _, h := range hits {
			if h.kind == "apply" {
				apply++
				if h.thread != 0 {
					applyOnResolved++
				}
			}
		}
	}
	if apply != 1 || applyOnResolved != 0 {
		t.Fatalf("apply buttons = %d (on resolved: %d)", apply, applyOnResolved)
	}
}

func TestDeletingYourOwnComments(t *testing.T) {
	view := `{"number":7,"title":"T","body":"b","state":"OPEN","author":{"login":"a"},"reviews":[],"comments":[{"id":"IC_mine","author":{"login":"me"},"body":"general mine","createdAt":"2026-01-01T00:00:00Z"},{"id":"IC_other","author":{"login":"x"},"body":"general other","createdAt":"2026-01-01T00:00:00Z"}]}`
	threads := `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"id":"T1","isResolved":false,"isOutdated":false,"path":"f.go","line":2,"comments":{"nodes":[` +
		`{"id":"RC_other","databaseId":9,"author":{"login":"r"},"body":"theirs","createdAt":"2026-01-01T00:00:00Z","state":"SUBMITTED"},` +
		`{"id":"RC_mine","databaseId":10,"author":{"login":"me"},"body":"mine","createdAt":"2026-01-02T00:00:00Z","state":"SUBMITTED"}]}}]}}}}}`
	d, err := parsePR("o/r", "https://github.com/o/r/pull/7", []byte(view), []byte(rangeDiff), []byte(threads))
	if err != nil {
		t.Fatal(err)
	}
	d.Viewer = "me"
	if d.Comments[0].NodeID != "IC_mine" || d.Files[0].Threads[0].Comments[1].NodeID != "RC_mine" {
		t.Fatalf("node ids were not read: %+v", d.Comments)
	}
	m := newModel()
	m.w, m.h, m.mode = 140, 40, modePick
	m.repo = &repository{Root: t.TempDir()}
	m.picker = &workspacePicker{kind: "prdiff", pr: d, list: "files"}
	m.rebuildPicker()
	lw := m.pickerListWidth()

	deletes := func() (hits []prHit, rows []int) {
		for r, hs := range m.picker.hits {
			for _, h := range hs {
				if h.kind == "delete" {
					hits, rows = append(hits, h), append(rows, r)
				}
			}
		}
		return
	}
	click := func(h prHit, row int) tea.Cmd {
		return m.previewClick(lw+1+h.x0, headerRows+row-m.picker.pscroll)
	}
	// In a file: only your reply in the thread has a ✕.
	m.picker.sel = 1
	m.pickerPreview()
	hits, rows := deletes()
	if len(hits) != 1 || hits[0].comment != 1 {
		t.Fatalf("delete buttons in the file: %+v", hits)
	}
	if cmd := click(hits[0], rows[0]); cmd != nil || m.picker.armed != "RC_mine" {
		t.Fatalf("the first click must only ask: cmd=%v armed=%q", cmd, m.picker.armed)
	}
	if cmd := click(hits[0], rows[0]); cmd == nil || m.picker.armed != "" {
		t.Fatalf("the second click must delete: cmd=%v armed=%q", cmd, m.picker.armed)
	}
	// Another click in between disarms.
	click(hits[0], rows[0])
	m.previewClick(lw+1, headerRows+500)
	if m.picker.armed != "" {
		t.Error("a click elsewhere must disarm")
	}
	// A key press disarms too.
	click(hits[0], rows[0])
	m.pickerKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.picker.armed != "" {
		t.Error("a key must disarm")
	}
	// In the conversation: your general comment, not the other one.
	m.picker.sel = 0
	m.pickerPreview()
	hits, _ = deletes()
	if len(hits) != 1 || hits[0].file != -1 || hits[0].comment != 0 {
		t.Fatalf("delete buttons in the conversation: %+v", hits)
	}
	// Without your login nothing can be deleted.
	d.Viewer = ""
	m.picker.sel = 1
	d.At++ // drop the cached previews
	m.pickerPreview()
	if hits, _ := deletes(); len(hits) != 0 {
		t.Errorf("no viewer, no delete buttons: %+v", hits)
	}
}

func TestSubmitMenuAndPendingSummary(t *testing.T) {
	view := `{"number":7,"title":"T","body":"b","state":"OPEN","author":{"login":"a"},"reviews":[],"comments":[],"headRefOid":"abc"}`
	threads := `{"data":{"repository":{"pullRequest":{"id":"PR_node","reviewThreads":{"nodes":[{"id":"T1","isResolved":false,"isOutdated":false,"path":"f.go","line":2,"comments":{"nodes":[` +
		`{"id":"RC1","databaseId":9,"author":{"login":"r"},"body":"theirs","createdAt":"2026-01-01T00:00:00Z","state":"SUBMITTED"},` +
		`{"id":"RC2","databaseId":10,"author":{"login":"me"},"body":"my pending reply","createdAt":"2026-01-02T00:00:00Z","state":"PENDING"}]}}]}}}}}`
	d, err := parsePR("o/r", "https://github.com/o/r/pull/7", []byte(view), []byte(rangeDiff), []byte(threads))
	if err != nil {
		t.Fatal(err)
	}
	d.Viewer = "me"
	if d.NodeID != "PR_node" || d.pendingCount() != 1 {
		t.Fatalf("node=%q pending=%d", d.NodeID, d.pendingCount())
	}
	delete(reviewSummaries, d.URL)
	defer delete(reviewSummaries, d.URL)
	m := newModel()
	m.w, m.h, m.mode = 160, 40, modePick
	m.repo = &repository{Root: t.TempDir()}
	m.picker = &workspacePicker{kind: "prdiff", pr: d, list: "files"}
	m.rebuildPicker()

	if btn := ansi.Strip(m.submitButton()); !strings.Contains(btn, "Submit [1]") {
		t.Errorf("button = %q", btn)
	}
	// The Conversation button writes the review's summary in pending mode.
	if immediateComments {
		t.Fatal("pending must be the default")
	}
	m.picker.sel = 0
	m.pickerPreview()
	if label := generalButtonLabel(); !strings.Contains(label, "Review summary") {
		t.Errorf("label = %q", label)
	}
	m.composeGeneral()
	m.setOmni("Looks good overall, two small things.")
	if cmd := m.sendCompose(); cmd != nil {
		t.Error("saving a summary is local: no command")
	}
	if got := reviewSummaries[d.URL]; got != "Looks good overall, two small things." {
		t.Fatalf("summary = %q", got)
	}
	if btn := ansi.Strip(m.submitButton()); !strings.Contains(btn, "Submit [2]") {
		t.Errorf("the summary counts: %q", btn)
	}
	m.pickerPreview()
	lines, _ := d.conversationPreview(80)
	var text []string
	for _, l := range lines {
		s := ""
		for _, g := range l {
			s += g.text
		}
		text = append(text, ansi.Strip(s))
	}
	if j := strings.Join(text, "\n"); !strings.Contains(j, "Looks good overall") || !strings.Contains(j, "sent with Submit") {
		t.Errorf("the summary must show in Conversation:\n%s", j)
	}

	// The header button opens the verdict menu under it, with all four entries.
	x, _ := m.submitMenuLayout()
	m.workspaceClick(x+1, toolbarRow)
	if m.submitMenu == nil {
		t.Fatal("the button must open the menu")
	}
	rows := strings.Split(ansi.Strip(m.overlayMenus(strings.Repeat(strings.Repeat(" ", m.w)+"\n", m.h-1)+strings.Repeat(" ", m.w))), "\n")
	for i, want := range []string{"Comment", "Approve", "Request changes", "Discard the pending review"} {
		if !strings.Contains(rows[toolbarRow+1+i], want) {
			t.Errorf("menu row %d = %q, want %q", i, rows[toolbarRow+1+i], want)
		}
	}
	m.workspaceClick(x+1, toolbarRow) // the button again closes it
	if m.submitMenu != nil {
		t.Error("a second click closes the menu")
	}
	// Request changes needs a summary.
	delete(reviewSummaries, d.URL)
	m.openSubmitMenu()
	m.submitMenu.sel = 2
	if cmd := m.submitMenuKey("enter"); cmd != nil || !strings.Contains(m.status, "needs a summary") {
		t.Errorf("request changes without a summary: cmd=%v status=%q", cmd, m.status)
	}
	// Approve and Comment go out (here: as a command, not run).
	m.openSubmitMenu()
	m.submitMenu.sel = 1
	if cmd := m.submitMenuKey("enter"); cmd == nil {
		t.Error("approve must send")
	}
	if m.submitMenu != nil {
		t.Error("the menu closes after a choice")
	}
	// With nothing pending, a Comment verdict does nothing.
	d.Files[0].Threads[0].Comments[1].Pending = false
	m.openSubmitMenu()
	m.submitMenu.sel = 0
	if cmd := m.submitMenuKey("enter"); cmd != nil {
		t.Error("nothing pending: nothing to submit")
	}
}
