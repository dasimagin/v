package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// The tools the reviewer gets, served by v's MCP server. All of them are
// about the one PR under review; the writes only propose comments to v.
const toolPrefix = "pr_"

const (
	listDefault = 500 // lines per page of a diff or file
	fileMaxSize = 2 << 20
)

// ghInput runs gh with stdin and returns stdout.
func ghInput(dir string, stdin []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %s", args[0], firstOutputLine(stderr.String(), err))
	}
	return out, nil
}

// ghLoose is ghOutput for commands that exit non-zero to report a state
// (gh pr checks does when something failed) yet still print their result.
func ghLoose(dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && len(bytes.TrimSpace(out)) == 0 {
		return nil, fmt.Errorf("gh %s: %s", args[0], firstOutputLine(stderr.String(), err))
	}
	return out, nil
}

func ghGraphQL(dir, query string, vars map[string]any) ([]byte, error) {
	args := []string{"api", "graphql", "-f", "query=" + query}
	for k, v := range vars {
		if n, ok := v.(int); ok {
			args = append(args, "-F", fmt.Sprintf("%s=%d", k, n))
		} else {
			args = append(args, "-f", fmt.Sprintf("%s=%v", k, v))
		}
	}
	return ghOutput(dir, args...)
}

// numberedPatch is a file's patch with the new-file line number in front of
// every line that exists there, which is what a comment anchors to.
func numberedPatch(f prFile, context int) []string {
	var out []string
	newN := 0
	for _, l := range reflowPatch(f.Patch, context, context, nil) {
		switch {
		case hunkPattern.MatchString(l):
			newN, _ = strconv.Atoi(hunkPattern.FindStringSubmatch(l)[2])
			out = append(out, "      "+l)
		case newN == 0 || strings.HasPrefix(l, "diff --git"):
			out = append(out, "      "+l)
		case strings.HasPrefix(l, "-"), strings.HasPrefix(l, "\\"):
			out = append(out, "      |"+l)
		default:
			out = append(out, fmt.Sprintf("%5d |%s", newN, l))
			newN++
		}
	}
	return out
}

// page returns lines[offset:offset+limit] and a note about the rest.
func page(lines []string, offset, limit int) string {
	if limit <= 0 {
		limit = listDefault
	}
	offset = max(offset, 0)
	if offset >= len(lines) {
		return fmt.Sprintf("(no more lines; there are %d)", len(lines))
	}
	end := min(offset+limit, len(lines))
	out := strings.Join(lines[offset:end], "\n")
	if end < len(lines) {
		out += fmt.Sprintf("\n[%d more lines; call again with offset=%d]", len(lines)-end, end)
	}
	return out
}

type fileArgs struct {
	Path   string `json:"path"`
	Ref    string `json:"ref"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

func prTools(rv *reviewFlow) []mcpTool {
	file := func(path string) (prFile, error) {
		for _, f := range rv.data.Files {
			if f.Path == path {
				return f, nil
			}
		}
		return prFile{}, fmt.Errorf("%s is not changed in this PR; call %slist for the changed files", path, toolPrefix)
	}
	decode := func(raw json.RawMessage, v any) error {
		if len(raw) == 0 {
			return nil
		}
		if err := json.Unmarshal(raw, v); err != nil {
			return fmt.Errorf("bad arguments: %w", err)
		}
		return nil
	}
	return []mcpTool{
		{
			Name:        toolPrefix + "description",
			Description: "The PR's title, author, branches, labels and description.",
			Schema:      `{"type":"object","properties":{}}`,
			Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
				d := rv.data
				state := strings.ToLower(d.State)
				if d.Draft {
					state += ", draft"
				}
				return fmt.Sprintf("#%d %s\nby @%s · %s · %s ← %s · %d files\nlabels: %s\n\n%s", d.Number, d.Title, d.Author, state,
					d.Base, d.Head, len(d.Files), strings.Join(d.Labels, ", "), strings.TrimSpace(d.Body)), nil
			},
		},
		{
			Name:        toolPrefix + "list",
			Description: "The files this PR changes, with added and removed line counts and the number of existing review comments. Optional filter: a substring of the path.",
			Schema:      `{"type":"object","properties":{"filter":{"type":"string"}}}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var a struct{ Filter string }
				if err := decode(raw, &a); err != nil {
					return "", err
				}
				var lines []string
				for _, f := range rv.data.Files {
					if a.Filter == "" || strings.Contains(f.Path, a.Filter) {
						lines = append(lines, fmt.Sprintf("%s  +%d -%d  %d threads", f.Path, f.Adds, f.Dels, len(f.Threads)))
					}
				}
				if len(lines) == 0 {
					return "no matching files", nil
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name: toolPrefix + "diff",
			Description: "The diff of one changed file. Every line that exists in the new file is prefixed with its line number there; pr_comment anchors to those numbers. " +
				"context sets the unchanged lines around changes (default 3). Long diffs come in pages: use offset and limit.",
			Schema: `{"type":"object","properties":{"path":{"type":"string"},"context":{"type":"integer"},"offset":{"type":"integer"},"limit":{"type":"integer"}},"required":["path"]}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var a struct {
					Path    string
					Context *int
					Offset  int
					Limit   int
				}
				if err := decode(raw, &a); err != nil {
					return "", err
				}
				f, err := file(a.Path)
				if err != nil {
					return "", err
				}
				n := 3
				if a.Context != nil {
					n = max(*a.Context, 0)
				}
				return page(numberedPatch(f, n), a.Offset, a.Limit), nil
			},
		},
		{
			Name: toolPrefix + "file",
			Description: "A whole file as of the PR head (ref=head, default) or the base branch tip (ref=base), with line numbers. " +
				"Use it for code the diff does not show. Long files come in pages: use offset and limit.",
			Schema: `{"type":"object","properties":{"path":{"type":"string"},"ref":{"type":"string","enum":["head","base"]},"offset":{"type":"integer"},"limit":{"type":"integer"}},"required":["path"]}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var a fileArgs
				if err := decode(raw, &a); err != nil {
					return "", err
				}
				sha := rv.data.HeadSHA
				if a.Ref == "base" {
					sha = rv.data.BaseSHA
				}
				segs := strings.Split(strings.Trim(a.Path, "/"), "/")
				for i := range segs {
					segs[i] = url.PathEscape(segs[i])
				}
				out, err := ghOutput(rv.dir, "api", "-H", "Accept: application/vnd.github.raw",
					fmt.Sprintf("repos/%s/contents/%s?ref=%s", rv.repo, strings.Join(segs, "/"), sha))
				if err != nil {
					return "", fmt.Errorf("cannot read %s at %s: %w", a.Path, a.Ref, err)
				}
				if len(out) > fileMaxSize {
					return "", fmt.Errorf("%s is too large to read", a.Path)
				}
				lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
				for i := range lines {
					lines[i] = fmt.Sprintf("%5d |%s", i+1, lines[i])
				}
				return page(lines, a.Offset, a.Limit), nil
			},
		},
		{
			Name: toolPrefix + "threads",
			Description: "The review threads on this PR, with their ids, and your own proposals in this review. status is all (default), unresolved, resolved, " +
				"pending (comments that are not published yet, including yours accepted in this review) or rejected (proposals the user turned down).",
			Schema: `{"type":"object","properties":{"status":{"type":"string","enum":["all","unresolved","resolved","pending","rejected"]}}}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var a struct{ Status string }
				if err := decode(raw, &a); err != nil {
					return "", err
				}
				return rv.threads(a.Status)
			},
		},
		{
			Name:        toolPrefix + "checks",
			Description: "The PR's CI checks. status is all (default) or failing.",
			Schema:      `{"type":"object","properties":{"status":{"type":"string","enum":["all","failing"]}}}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var a struct{ Status string }
				if err := decode(raw, &a); err != nil {
					return "", err
				}
				return rv.checks(a.Status == "failing")
			},
		},
		{
			Name: toolPrefix + "comment",
			Description: "Propose an inline review comment on a line of the new file that appears in the diff (use the numbers from pr_diff). " +
				"The user may reject it; the result says so. level is critical, recommended or note.",
			Schema: `{"type":"object","properties":{"path":{"type":"string"},"line":{"type":"integer"},"level":{"type":"string","enum":["critical","recommended","note"]},"body":{"type":"string"}},"required":["path","line","level","body"]}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var a struct {
					Path  string
					Line  int
					Level string
					Body  string
				}
				if err := decode(raw, &a); err != nil {
					return "", err
				}
				f, err := file(a.Path)
				if err != nil {
					return "", err
				}
				if !f.newLines()[a.Line] {
					return "", fmt.Errorf("invalid: line %d of %s is not in the diff; comments can only attach to lines shown by %sdiff", a.Line, a.Path, toolPrefix)
				}
				if strings.TrimSpace(a.Body) == "" {
					return "", fmt.Errorf("invalid: the comment is empty")
				}
				c := &reviewCard{Path: a.Path, Line: a.Line, Body: levelPrefix(a.Level) + strings.TrimSpace(a.Body)}
				c.Hunk, c.Target = f.excerpt(a.Line)
				c.label = fmt.Sprintf("%s:%d", a.Path, a.Line)
				return rv.propose(ctx, c), nil
			},
		},
		{
			Name:        toolPrefix + "reply",
			Description: "Propose a reply in an existing review thread; thread is the id from pr_threads. The user may reject it.",
			Schema:      `{"type":"object","properties":{"thread":{"type":"string"},"body":{"type":"string"}},"required":["thread","body"]}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var a struct {
					Thread string
					Body   string
				}
				if err := decode(raw, &a); err != nil {
					return "", err
				}
				if a.Thread == "" || strings.TrimSpace(a.Body) == "" {
					return "", fmt.Errorf("invalid: a thread id and a body are required")
				}
				c := &reviewCard{Thread: a.Thread, Body: strings.TrimSpace(a.Body), label: "reply in thread " + a.Thread}
				return rv.propose(ctx, c), nil
			},
		},
	}
}

// threads lists the PR's review threads and this review's own proposals.
func (rv *reviewFlow) threads(status string) (string, error) {
	if status == "" {
		status = "all"
	}
	var lines []string
	show := func(s string) bool { return status == "all" || status == s }
	if show("resolved") || show("unresolved") || show("pending") {
		owner, name, _ := strings.Cut(rv.repo, "/")
		out, err := ghGraphQL(rv.dir, `query($owner:String!,$name:String!,$n:Int!){repository(owner:$owner,name:$name){pullRequest(number:$n){reviewThreads(first:100){nodes{id isResolved isOutdated path line comments(first:30){nodes{author{login} body createdAt state}}}}}}}`,
			map[string]any{"owner": owner, "name": name, "n": rv.number})
		if err != nil {
			lines = append(lines, "(could not read the threads on GitHub: "+err.Error()+")")
			out = []byte("{}")
		}
		var res struct {
			Data struct {
				Repository struct {
					PullRequest struct {
						ReviewThreads struct{ Nodes []threadNode }
					}
				}
			}
		}
		if err := json.Unmarshal(out, &res); err != nil {
			return "", err
		}
		for _, t := range res.Data.Repository.PullRequest.ReviewThreads.Nodes {
			state := "unresolved"
			switch {
			case len(t.Comments.Nodes) > 0 && t.Comments.Nodes[0].State == "PENDING":
				state = "pending"
			case t.IsResolved:
				state = "resolved"
			}
			if !show(state) {
				continue
			}
			head := fmt.Sprintf("thread %s [%s] %s:%d", t.ID, state, t.Path, t.Line)
			if t.IsOutdated {
				head += " (outdated)"
			}
			lines = append(lines, head)
			for _, c := range t.Comments.Nodes {
				lines = append(lines, fmt.Sprintf("  @%s: %s", c.Author.Login, strings.ReplaceAll(strings.TrimSpace(c.Body), "\n", "\n    ")))
			}
		}
	}
	rv.mu.Lock()
	for _, c := range rv.cards {
		state := "pending"
		if c.Decision != "y" {
			state = "rejected"
		}
		if !show(state) {
			continue
		}
		where := c.label
		lines = append(lines, fmt.Sprintf("yours [%s] %s\n  %s", state, where, strings.ReplaceAll(c.Body, "\n", "\n  ")))
	}
	rv.mu.Unlock()
	if len(lines) == 0 {
		return "no threads", nil
	}
	return strings.Join(lines, "\n"), nil
}

// checks lists the PR's CI checks, optionally only the failing ones.
func (rv *reviewFlow) checks(failingOnly bool) (string, error) {
	out, err := ghLoose(rv.dir, "pr", "checks", rv.url, "--json", "name,bucket,workflow,link,description")
	if err != nil {
		return "", err
	}
	var checks []struct {
		Name, Bucket, Workflow, Link, Description string
	}
	if err := json.Unmarshal(out, &checks); err != nil {
		return "", err
	}
	var lines []string
	for _, c := range checks {
		if failingOnly && c.Bucket != "fail" {
			continue
		}
		line := fmt.Sprintf("[%s] %s", c.Bucket, c.Name)
		if c.Workflow != "" {
			line = fmt.Sprintf("[%s] %s / %s", c.Bucket, c.Workflow, c.Name)
		}
		if c.Link != "" {
			line += " " + c.Link
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "no checks", nil
	}
	return strings.Join(lines, "\n"), nil
}
