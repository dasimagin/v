package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
)

const (
	previewMaxBytes = 128 << 10
	previewMaxLines = 400
	previewTTL      = 10 * time.Second
)

// previewLines are tokens like a highlighted file, so every preview is drawn by
// renderLine in the active theme.
type previewLines [][]seg

type previewEntry struct {
	at    time.Time
	lines previewLines
}

// previews caches by key; Git keys expire, file keys carry mtime and size.
var previews = map[string]previewEntry{}

func cachedPreview(key string, ttl time.Duration, load func() previewLines) previewLines {
	if e, ok := previews[key]; ok && (ttl == 0 || time.Since(e.at) < ttl) {
		return e.lines
	}
	if len(previews) > 100 {
		previews = map[string]previewEntry{}
	}
	lines := load()
	previews[key] = previewEntry{time.Now(), lines}
	return lines
}

func note(text string) previewLines { return previewLines{{{text, chroma.Comment}}} }

// pathPreview shows a file highlighted, or a directory's entries.
func pathPreview(path string) (key string, load func() previewLines) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", func() previewLines { return note(err.Error()) }
	}
	key = fmt.Sprintf("path:%s:%d:%d", path, fi.ModTime().UnixNano(), fi.Size())
	if fi.IsDir() {
		return key, func() previewLines { return dirPreview(path) }
	}
	return key, func() previewLines { return filePreview(path) }
}

func dirPreview(path string) previewLines {
	des, err := os.ReadDir(path)
	if err != nil {
		return note(err.Error())
	}
	type entry struct {
		name  string
		isDir bool
	}
	var es []entry
	for _, de := range des {
		isDir := de.IsDir()
		if de.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(path + string(os.PathSeparator) + de.Name()); err == nil {
				isDir = fi.IsDir()
			}
		}
		es = append(es, entry{de.Name(), isDir})
	}
	sort.SliceStable(es, func(i, j int) bool {
		hi, hj := strings.HasPrefix(es[i].name, "."), strings.HasPrefix(es[j].name, ".")
		if hi != hj {
			return hj
		}
		if es[i].isDir != es[j].isDir {
			return es[i].isDir
		}
		return strings.ToLower(es[i].name) < strings.ToLower(es[j].name)
	})
	if len(es) == 0 {
		return note("empty")
	}
	var out previewLines
	for _, e := range es {
		switch {
		case e.isDir:
			out = append(out, []seg{{e.name + "/", chroma.NameClass}})
		case strings.HasPrefix(e.name, "."):
			out = append(out, []seg{{e.name, chroma.Comment}})
		default:
			out = append(out, []seg{{e.name, chroma.Text}})
		}
		if len(out) == previewMaxLines {
			break
		}
	}
	return out
}

func filePreview(path string) previewLines {
	f, err := os.Open(path)
	if err != nil {
		return note(err.Error())
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, previewMaxBytes))
	if err != nil {
		return note(err.Error())
	}
	if len(b) == 0 {
		return note("empty file")
	}
	if strings.IndexByte(string(b), 0) >= 0 {
		return note("binary file")
	}
	_, lines, err := highlight(path, string(b))
	if err != nil {
		return note(err.Error())
	}
	if len(lines) > previewMaxLines {
		lines = lines[:previewMaxLines]
	}
	return lines
}

// branchPreview is the recent history of a ref, one commit per line.
func branchPreview(dir, ref string) previewLines {
	out, err := gitOutput(dir, "log", "-n", "100", "--format=%h%x09%cr%x09%an%x09%s", ref, "--")
	if err != nil {
		return note(err.Error())
	}
	var lines previewLines
	for _, l := range strings.Split(out, "\n") {
		if f := strings.SplitN(l, "\t", 4); len(f) == 4 {
			lines = append(lines, []seg{{f[0], chroma.LiteralNumber}, {"  " + f[3], chroma.Text}, {"  " + f[1] + " · " + f[2], chroma.Comment}})
		}
	}
	if len(lines) == 0 {
		return note("no commits")
	}
	return lines
}

// commitPreview is one commit: message, stat and a colored patch.
func commitPreview(dir, hash string) previewLines {
	out, err := gitOutput(dir, "show", "--stat", "--patch", fmt.Sprintf("-U%d", max(diffBefore, diffAfter)), "--format=%h%nAuthor: %an <%ae>%nDate:   %cd (%cr)%n%n%B", hash, "--")
	if err != nil {
		return note(err.Error())
	}
	var lines previewLines
	patch := false
	for i, l := range reflowPatch(strings.Split(out, "\n"), diffBefore, diffAfter, nil) {
		l = strings.TrimRight(l, "\r")
		tok := chroma.Text
		switch {
		case strings.HasPrefix(l, "diff --git"):
			patch, tok = true, chroma.GenericHeading
		case patch && strings.HasPrefix(l, "@@"):
			tok = chroma.GenericSubheading
		case patch && strings.HasPrefix(l, "+++"), patch && strings.HasPrefix(l, "---"):
			tok = chroma.Comment
		case patch && strings.HasPrefix(l, "+"):
			tok = chroma.GenericInserted
		case patch && strings.HasPrefix(l, "-"):
			tok = chroma.GenericDeleted
		case patch:
			tok = chroma.Text
		case i == 0:
			tok = chroma.LiteralNumber
		case strings.HasPrefix(l, "Author:"), strings.HasPrefix(l, "Date:"), strings.Contains(l, " | "):
			tok = chroma.Comment
		}
		var segs []seg
		if l != "" {
			segs = []seg{{l, tok}}
		}
		lines = append(lines, segs)
		if len(lines) == previewMaxLines*2 {
			lines = append(lines, []seg{{"…", chroma.Comment}})
			break
		}
	}
	return lines
}

type commit struct{ Hash, Short, When, Author, Subject string }

func loadCommits(dir string) ([]commit, error) {
	out, err := gitOutput(dir, "log", "-n", "500", "--format=%H%x09%h%x09%cr%x09%an%x09%s")
	if err != nil {
		return nil, err
	}
	var cs []commit
	for _, l := range strings.Split(out, "\n") {
		if f := strings.SplitN(l, "\t", 5); len(f) == 5 {
			cs = append(cs, commit{f[0], f[1], f[2], f[3], f[4]})
		}
	}
	return cs, nil
}
