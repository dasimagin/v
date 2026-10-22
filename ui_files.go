package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
)

// uiSeg is a piece of text with a token class; the page colors classes from
// the theme's palette.
type uiSeg struct {
	T string `json:"t"`
	C string `json:"c,omitempty"`
}

type uiLines [][]uiSeg

// tokenClass names the kind of a syntax token: the page has a color per kind.
func tokenClass(t chroma.TokenType) string {
	switch t {
	case chroma.KeywordConstant, chroma.NameConstant:
		return "constant"
	case chroma.KeywordType, chroma.NameClass, chroma.NameNamespace:
		return "type"
	case chroma.NameFunction, chroma.NameDecorator, chroma.GenericSubheading, chroma.GenericHeading:
		return "func"
	case chroma.NameBuiltin:
		return "builtin"
	case chroma.NameTag:
		return "tag"
	case chroma.NameAttribute:
		return "attr"
	case chroma.NameVariable, chroma.NameProperty:
		return "variable"
	case chroma.LiteralStringEscape:
		return "escape"
	case chroma.GenericInserted:
		return "added"
	case chroma.GenericDeleted:
		return "deleted"
	}
	switch {
	case t.InCategory(chroma.Comment):
		return "comment"
	case t.InCategory(chroma.Keyword):
		return "keyword"
	case t.InSubCategory(chroma.LiteralString):
		return "string"
	case t.InSubCategory(chroma.LiteralNumber):
		return "number"
	case t.InCategory(chroma.Operator):
		return "operator"
	}
	return ""
}

func uiLinesOf(lines [][]seg) uiLines {
	out := make(uiLines, len(lines))
	for i, l := range lines {
		out[i] = make([]uiSeg, len(l))
		for j, s := range l {
			out[i][j] = uiSeg{s.text, tokenClass(s.tok)}
		}
	}
	return out
}

// uiItem is a row of a list: a file or folder for /open, a commit for /log.
type uiItem struct {
	Value string   `json:"value"` // path or hash
	Label string   `json:"label"`
	Meta  string   `json:"meta"`
	Dir   bool     `json:"dir"`
	Query string   `json:"query"`           // /open: what to type to go into the folder
	Tag   string   `json:"tag"`             // /branch: "current", "local" or "remote"
	Sub   string   `json:"sub"`             // /branch: the branch's latest commit
	When  string   `json:"when"`            // /branch: how long ago
	Refs  []logRef `json:"refs,omitempty"`  // /log: the tags and the other branches on the commit
	Below bool     `json:"below,omitempty"` // /open: a match from the folders below, not from this folder
}

// uiCrumb is one folder of the path above the list: its name, and what to type to list it.
type uiCrumb struct {
	Name  string `json:"name"`
	Query string `json:"query"`
}

type uiPick struct {
	Crumbs []uiCrumb `json:"crumbs,omitempty"` // /open: the folders from home (or the root) down to the listed one
	Dir    string    `json:"dir"`              // /open: the folder that is listed
	Up     string    `json:"up"`               // /open: what to type to go to its parent
	Items  []uiItem  `json:"items"`
	Err    string    `json:"err"`
}

const uiPickMax = 300

// queryFor is what to type in the omnibox to list a folder: a path relative to
// workdir when it is inside it, otherwise the absolute one with "~" for home.
func queryFor(workdir, dir string) string {
	if rel, err := filepath.Rel(workdir, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
		if rel == "." {
			return ""
		}
		return rel + "/"
	}
	return strings.TrimSuffix(tildify(dir), "/") + "/"
}

// uiCrumbs are the folders from home (or the root, outside home) down to dir.
func uiCrumbs(workdir, dir string) []uiCrumb {
	home, _ := os.UserHomeDir()
	var chain []string
	for d := dir; ; d = filepath.Dir(d) {
		chain = append(chain, d)
		if d == filepath.Dir(d) || (home != "" && d == home) {
			break
		}
	}
	out := make([]uiCrumb, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		d := chain[i]
		name := filepath.Base(d)
		switch {
		case home != "" && d == home:
			name = "~"
		case d == filepath.Dir(d):
			name = string(filepath.Separator)
		}
		out = append(out, uiCrumb{name, queryFor(workdir, d)})
	}
	return out
}

// kind is "open", "workdir", "log", "branch" or "resume".
func uiPickList(workdir, kind, query string) uiPick {
	switch kind {
	case "log":
		return uiLogList(workdir, query)
	case "branch":
		return uiSwitchList(workdir, query)
	case "resume":
		return uiResumeList(workdir, query)
	}
	if query == "~" {
		query = "~/"
	}
	parent, needle := filepath.Split(query)
	dir := workPath(workdir, parent)
	pick := uiPick{Dir: tildify(dir), Crumbs: uiCrumbs(workdir, dir), Items: []uiItem{}}
	if up := filepath.Dir(dir); up != dir {
		pick.Up = queryFor(workdir, up)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		pick.Err = err.Error()
		return pick
	}
	type scored struct {
		uiItem
		score int
	}
	var found []scored
	for _, de := range entries {
		name := de.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(needle, ".") {
			continue
		}
		score, ok := fuzzyScore(needle, name)
		if !ok {
			continue
		}
		path := filepath.Join(dir, name)
		isDir := de.IsDir()
		if de.Type()&os.ModeSymlink != 0 {
			fi, err := os.Stat(path)
			isDir = err == nil && fi.IsDir()
		}
		if kind == "workdir" && !isDir {
			continue
		}
		it := uiItem{Value: path, Label: name, Dir: isDir}
		if isDir {
			it.Label += "/"
			it.Query = queryFor(workdir, path)
		}
		found = append(found, scored{it, score})
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].score != found[j].score {
			return found[i].score > found[j].score
		}
		return found[i].Dir && !found[j].Dir
	})
	// Something is typed: under the matches of this folder, set off by a line, what matches in the folders
	// below (their folders and files, fuzzy over the whole relative path).
	var below []scored
	if strings.TrimSpace(needle) != "" {
		for _, c := range uiProjectEntries(dir) {
			if !strings.Contains(strings.TrimSuffix(c.rel, "/"), "/") {
				continue // the entries of this folder are above
			}
			if kind == "workdir" && !c.dir {
				continue
			}
			score, ok := fuzzyScore(needle, strings.TrimSuffix(c.rel, "/"))
			if !ok {
				continue
			}
			path := filepath.Join(dir, c.rel)
			name := filepath.Base(path)
			it := uiItem{Value: path, Label: name, Meta: filepath.Dir(strings.TrimSuffix(c.rel, "/")) + "/", Dir: c.dir, Below: true}
			if c.dir {
				it.Label += "/"
				it.Query = queryFor(workdir, path)
			}
			below = append(below, scored{it, score})
		}
		sort.SliceStable(below, func(i, j int) bool { return below[i].score > below[j].score })
		if len(below) > uiPickMax {
			below = below[:uiPickMax]
		}
	}
	if kind == "workdir" && needle == "" {
		pick.Items = append(pick.Items, uiItem{Value: dir, Label: "· this folder", Meta: tildify(dir)})
	}
	for _, f := range found {
		if len(pick.Items) == uiPickMax {
			break
		}
		pick.Items = append(pick.Items, f.uiItem)
	}
	for _, f := range below {
		pick.Items = append(pick.Items, f.uiItem)
	}
	return pick
}

// uiLogList lists the commits of the current branch with the tags and other branches on them.
func uiLogList(workdir, query string) uiPick {
	pick := uiPick{Items: []uiItem{}}
	commits, err := loadLog(workdir)
	if err != nil {
		pick.Err = "No history here · " + err.Error()
		return pick
	}
	type scored struct {
		uiItem
		score int
	}
	var found []scored
	for _, c := range commits {
		score, ok := fuzzyScore(query, c.Short+" "+c.Subject+" "+c.Author)
		if ok {
			it := uiItem{Value: c.Hash, Label: c.Subject, Meta: c.Short + " · " + c.Author + " · " + c.When, Refs: c.Refs}
			found = append(found, scored{it, score})
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].score > found[j].score })
	for _, f := range found {
		if len(pick.Items) == uiPickMax {
			break
		}
		pick.Items = append(pick.Items, f.uiItem)
	}
	return pick
}

// uiSwitchList lists the branches; an item's Query is the name to switch to.
func uiSwitchList(workdir, query string) uiPick {
	pick := uiPick{Items: []uiItem{}}
	r := readRepository(workdir)
	if r == nil {
		pick.Err = "Workdir is not in a Git repository"
		return pick
	}
	type scored struct {
		uiItem
		score int
	}
	tips := map[string][2]string{}
	if out, err := gitOutput(workdir, "for-each-ref", "--format=%(refname:short)%1f%(committerdate:relative)%1f%(subject)", "refs/heads", "refs/remotes"); err == nil {
		for _, l := range strings.Split(out, "\n") {
			if f := strings.SplitN(l, "\x1f", 3); len(f) == 3 {
				tips[f[0]] = [2]string{f[1], f[2]}
			}
		}
	}
	var found []scored
	for _, b := range r.Branches {
		score, ok := fuzzyScore(query, b.Ref)
		if !ok {
			continue
		}
		it := uiItem{Value: b.Ref, Label: b.Ref, Query: b.Name, Tag: "local", When: tips[b.Ref][0], Sub: tips[b.Ref][1]}
		switch {
		case !b.Remote && b.Name == r.Branch:
			it.Meta, it.Tag = "current", "current"
			if query == "" {
				score++
			}
		case b.Remote:
			it.Meta, it.Tag = "remote", "remote"
		}
		found = append(found, scored{it, score})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].score > found[j].score })
	for _, f := range found {
		pick.Items = append(pick.Items, f.uiItem)
	}
	return pick
}

// uiResumeList lists the past sessions of the directory; Value is the file.
func uiResumeList(workdir, query string) uiPick {
	pick := uiPick{Items: []uiItem{}}
	type scored struct {
		uiItem
		score int
	}
	var found []scored
	for _, s := range listSessions(workdir) {
		score, ok := fuzzyScore(query, s.Title)
		if !ok {
			continue
		}
		found = append(found, scored{uiItem{Value: s.path, Label: s.Title, Meta: fmt.Sprintf("%d prompts · %s", s.Prompts, ago(s.Modified))}, score})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].score > found[j].score })
	for _, f := range found {
		if len(pick.Items) == uiPickMax {
			break
		}
		pick.Items = append(pick.Items, f.uiItem)
	}
	return pick
}

// uiSessionPreview is the conversation of a session, roughly as it went.
func uiSessionPreview(workdir, path string) uiLines {
	out := uiLines{}
	add := func(class, text string, maxLines int) {
		lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
		for i, l := range lines {
			if i == maxLines {
				out = append(out, []uiSeg{{"…", "comment"}})
				break
			}
			out = append(out, []uiSeg{{l, class}})
		}
	}
	for _, m := range sessionLog(path, workdir) {
		switch m.kind {
		case msgUser:
			add("func", "▌ "+m.text, 12)
			out = append(out, nil)
		case msgAgent:
			add("", m.text, 12)
			out = append(out, nil)
		case msgTool:
			add("comment", "• "+m.text, 1)
		}
		if len(out) > 600 {
			out = append(out, []uiSeg{{"…", "comment"}})
			break
		}
	}
	if len(out) == 0 {
		out = append(out, []uiSeg{{"empty session", "comment"}})
	}
	return out
}

// uiPickPreview is the pane next to the list: a folder's names, a file's
// first lines, or a commit's message and patch.
func uiPickPreview(workdir, kind, value string) uiLines {
	if value == "" {
		return uiLines{}
	}
	switch kind {
	case "log":
		return uiLinesOf(commitPreview(workdir, value))
	case "branch":
		return uiLinesOf(branchPreview(workdir, value))
	case "resume":
		return uiSessionPreview(workdir, value)
	}
	_, load := pathPreview(value)
	return uiLinesOf(load())
}

type uiFile struct {
	Path  string  `json:"path"` // as shown: with ~ for home
	Abs   string  `json:"abs"`  // the real path, to act on
	Lang  string  `json:"lang"`
	Lines uiLines `json:"lines"`
	Err   string  `json:"err"`

	// The file as it is, for editing; Edit is false when the page only has a part of it.
	Text  string `json:"text"`
	CRLF  bool   `json:"crlf"`  // it uses Windows line ends: they are put back on save
	Mtime int64  `json:"mtime"` // when it was last written, in milliseconds (nanoseconds do not fit a JavaScript number), to notice a change made behind the editor
	Edit  bool   `json:"edit"`
}

const (
	uiFileMaxBytes = 1 << 20
	uiFileMaxLines = 5000
)

// uiReadFile reads a file with syntax highlighting, for the viewer.
func uiReadFile(path string) uiFile {
	abs, _ := filepath.Abs(path)
	f := uiFile{Path: tildify(path), Abs: abs, Lines: uiLines{}}
	fi, err := os.Stat(path)
	switch {
	case err != nil:
		f.Err = err.Error()
		return f
	case fi.IsDir():
		f.Err = "this is a folder"
		return f
	case fi.Size() > uiFileMaxBytes:
		f.Err = "the file is too big to show here"
		return f
	}
	b, err := os.ReadFile(path)
	if err != nil {
		f.Err = err.Error()
		return f
	}
	if strings.IndexByte(string(b), 0) >= 0 {
		f.Err = "binary file"
		return f
	}
	lang, lines, err := highlight(path, string(b))
	if err != nil {
		f.Err = err.Error()
		return f
	}
	f.Lang = lang
	f.Mtime = fi.ModTime().UnixMilli()
	if len(lines) > uiFileMaxLines {
		lines = append(lines[:uiFileMaxLines], []seg{{"…", chroma.Comment}})
	} else if utf8.Valid(b) {
		f.Edit = true
		f.CRLF = bytes.Contains(b, []byte("\r\n"))
		f.Text = strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	f.Lines = uiLinesOf(lines)
	return f
}

// uiSaveFile writes the editor's text over the file. It refuses if the file was changed since it was read
// (mtime), and keeps the file's mode and, if it had them, its Windows line ends.
func uiSaveFile(path, text string, crlf bool, mtime int64) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return fmt.Errorf("this is a folder")
	}
	if mtime != 0 && fi.ModTime().UnixMilli() != mtime {
		return fmt.Errorf("the file changed on disk after it was opened: reopen it to see the new version")
	}
	if crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".v-save-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), fi.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// uiFileTime is the file's modification time as uiFile.Mtime has it, after a save.
func uiFileTime(path string) int64 {
	if fi, err := os.Stat(path); err == nil {
		return fi.ModTime().UnixMilli()
	}
	return 0
}

// uiHighlight colors text the way the file at path would be, for the editor while you type.
func uiHighlight(path, text string) uiLines {
	_, lines, err := highlight(path, text)
	if err != nil {
		return uiLines{}
	}
	return uiLinesOf(lines)
}

// projectEntry is a file or a folder below the listed folder: rel is its path from it, with a slash at
// the end for a folder.
type projectEntry struct {
	rel string
	dir bool
}

// uiProjectEntries lists the files below dir and the folders that hold them: the files Git knows (and the
// untracked ones it does not ignore) in a repository, else a walk that skips hidden folders and the usual
// heaps. The answer is kept for a few seconds, as it is asked for on every key typed.
func uiProjectEntries(dir string) []projectEntry {
	projectFiles.Lock()
	defer projectFiles.Unlock()
	if projectFiles.dir == dir && time.Since(projectFiles.at) < 20*time.Second {
		return projectFiles.entries
	}
	var files []string
	if out, err := gitOutput(dir, "ls-files", "--cached", "--others", "--exclude-standard"); err == nil && out != "" {
		for _, l := range strings.Split(out, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				files = append(files, l)
			}
		}
	} else {
		skip := map[string]bool{"node_modules": true, "vendor": true, "target": true, "dist": true, "build": true, "__pycache__": true}
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			name := d.Name()
			if d.IsDir() && p != dir && (strings.HasPrefix(name, ".") || skip[name]) {
				return filepath.SkipDir
			}
			if !d.IsDir() {
				if rel, err := filepath.Rel(dir, p); err == nil {
					files = append(files, filepath.ToSlash(rel))
				}
			}
			if len(files) >= 50000 {
				return filepath.SkipAll
			}
			return nil
		})
	}
	var entries []projectEntry
	seen := map[string]bool{}
	for _, f := range files {
		for i := 0; i < len(f); i++ {
			if f[i] == '/' && !seen[f[:i+1]] {
				seen[f[:i+1]] = true
				entries = append(entries, projectEntry{f[:i+1], true})
			}
		}
		entries = append(entries, projectEntry{f, false})
	}
	projectFiles.dir, projectFiles.at, projectFiles.entries = dir, time.Now(), entries
	return entries
}

var projectFiles struct {
	sync.Mutex
	dir     string
	at      time.Time
	entries []projectEntry
}
