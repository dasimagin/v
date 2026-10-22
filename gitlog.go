package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// logRef is a branch or a tag that points at a commit.
type logRef struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // head (where you are), branch, remote or tag
}

// logCommit is a commit of /log with where it sits in the history.
type logCommit struct {
	Hash, Short, When, Author, Subject string
	Parents                            []string
	Refs                               []logRef
}

const logMax = 300

// loadLog reads the history of the current branch, newest first. Every commit carries the tags
// and the other branches that point at it, and the parents that the graph needs; only what the
// current branch reaches is listed, so a repository with many branches stays small.
func loadLog(dir string) ([]logCommit, error) {
	head, _ := gitOutput(dir, "rev-parse", "HEAD")
	logCache.Lock()
	defer logCache.Unlock()
	if head != "" && logCache.dir == dir && logCache.head == head && time.Since(logCache.at) < 20*time.Second {
		return logCache.commits, nil
	}
	cs, err := readLog(dir)
	if err == nil {
		logCache.dir, logCache.head, logCache.at, logCache.commits = dir, head, time.Now(), cs
	}
	return cs, err
}

// logCache keeps the last log for a few seconds: the list asks for it on every key typed,
// and the answer only changes when HEAD moves.
var logCache struct {
	sync.Mutex
	dir, head string
	at        time.Time
	commits   []logCommit
}

func readLog(dir string) ([]logCommit, error) {
	const sep = "\x1f"
	// The default order, not --topo-order: git stops after logMax commits instead of walking the
	// whole history to sort it, which is what makes a huge repository slow.
	args := []string{"log", "-n", fmt.Sprint(logMax), "--decorate=short",
		"--format=%H" + sep + "%h" + sep + "%cr" + sep + "%an" + sep + "%P" + sep + "%D" + sep + "%s"}
	out, err := gitOutput(dir, args...)
	if err != nil {
		return nil, err
	}
	remotes := gitRemotes(dir)
	var cs []logCommit
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, sep, 7)
		if len(f) != 7 {
			continue
		}
		cs = append(cs, logCommit{Hash: f[0], Short: f[1], When: f[2], Author: f[3], Parents: strings.Fields(f[4]), Refs: parseRefs(f[5], remotes), Subject: f[6]})
	}
	return cs, nil
}

// gitRemotes are the names of the repository's remotes, to tell origin/main from feature/x.
func gitRemotes(dir string) map[string]bool {
	remotes := map[string]bool{}
	if r, err := gitOutput(dir, "remote"); err == nil {
		for _, name := range strings.Fields(r) {
			remotes[name] = true
		}
	}
	return remotes
}

// parseRefs reads git's decoration: "HEAD -> main, tag: v1, origin/main".
func parseRefs(d string, remotes map[string]bool) []logRef {
	var refs []logRef
	for _, part := range strings.Split(d, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case part == "HEAD":
			refs = append(refs, logRef{"HEAD", "head"})
		case strings.HasPrefix(part, "HEAD -> "):
			refs = append(refs, logRef{strings.TrimPrefix(part, "HEAD -> "), "head"})
		case strings.HasPrefix(part, "tag: "):
			refs = append(refs, logRef{strings.TrimPrefix(part, "tag: "), "tag"})
		default:
			remote, _, ok := strings.Cut(part, "/")
			if ok && remotes[remote] {
				if strings.HasSuffix(part, "/HEAD") {
					continue // origin/HEAD says nothing a reader needs
				}
				refs = append(refs, logRef{part, "remote"})
			} else {
				refs = append(refs, logRef{part, "branch"})
			}
		}
	}
	return refs
}

// graphRow is one line of the history graph: where the commit's dot is and which lanes
// run past it, end in it from above, or start from it towards its parents.
type graphRow struct {
	Node  int   `json:"node"`
	Lanes int   `json:"lanes"`
	Pass  []int `json:"pass"`
	In    []int `json:"in"`
	Out   []int `json:"out"`
}

const graphMaxLanes = 8 // wider histories are drawn with the extra lanes folded into the last

// buildGraph lays the commits (children before parents) out in lanes.
func buildGraph(cs []logCommit) []graphRow {
	var lanes []string // the commit each lane is waiting for; "" is free
	free := func() int {
		for i, h := range lanes {
			if h == "" {
				return i
			}
		}
		lanes = append(lanes, "")
		return len(lanes) - 1
	}
	find := func(h string) int {
		for i, x := range lanes {
			if x == h {
				return i
			}
		}
		return -1
	}
	rows := make([]graphRow, len(cs))
	for i, c := range cs {
		var in []int
		for l, h := range lanes {
			if h == c.Hash {
				in = append(in, l)
			}
		}
		node := free()
		if len(in) > 0 {
			node = in[0]
		}
		var pass []int
		for l, h := range lanes {
			if h != "" && h != c.Hash {
				pass = append(pass, l)
			}
		}
		for _, l := range in {
			lanes[l] = ""
		}
		var out []int
		for pi, p := range c.Parents {
			l := find(p)
			if l < 0 {
				if pi == 0 {
					l = node
				} else {
					l = free()
				}
				lanes[l] = p
			}
			out = append(out, l)
		}
		n := node + 1
		for _, set := range [][]int{pass, in, out} {
			for _, l := range set {
				n = max(n, l+1)
			}
		}
		rows[i] = graphRow{Node: node, Lanes: n, Pass: pass, In: in, Out: out}
		for len(lanes) > 0 && lanes[len(lanes)-1] == "" {
			lanes = lanes[:len(lanes)-1]
		}
	}
	return rows
}

// text draws the row with box characters, two columns per lane, for the terminal.
func (g graphRow) text(width int) string {
	cells := []rune(strings.Repeat(" ", max(width, g.Lanes)*2))
	col := func(l int) int { return min(l, graphMaxLanes-1) * 2 }
	line := func(a, b int) {
		for x := min(a, b) + 1; x < max(a, b); x++ {
			cells[x] = '─'
		}
	}
	for _, l := range g.Pass {
		cells[col(l)] = '│'
	}
	for _, l := range g.In {
		if l != g.Node {
			cells[col(l)] = map[bool]rune{true: '╯', false: '╰'}[col(l) > col(g.Node)]
			line(col(l), col(g.Node))
		}
	}
	for _, l := range g.Out {
		if l != g.Node {
			cells[col(l)] = map[bool]rune{true: '╮', false: '╭'}[col(l) > col(g.Node)]
			line(col(l), col(g.Node))
		}
	}
	cells[col(g.Node)] = '●'
	return strings.TrimRight(string(cells), " ")
}

const logRefsShown = 3 // more refs than this on a commit are folded into "+N"

// refsText is "(main, tag: v1)" for the terminal; empty if there are no refs.
func refsText(refs []logRef) string {
	if len(refs) == 0 {
		return ""
	}
	var names []string
	for i, r := range refs {
		if i == logRefsShown {
			names = append(names, fmt.Sprintf("+%d", len(refs)-logRefsShown))
			break
		}
		switch r.Kind {
		case "tag":
			names = append(names, "tag: "+r.Name)
		case "head":
			names = append(names, "● "+r.Name)
		default:
			names = append(names, r.Name)
		}
	}
	return "(" + strings.Join(names, ", ") + ") "
}
