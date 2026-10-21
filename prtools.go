package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// The tools the reviewer gets, served by v's MCP server. All of them are
// about the one PR under review; the writes only propose comments to v.
const toolPrefix = "pr_"

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

func prTools(rv *reviewFlow) []mcpTool {
	file := func(path string) (prFile, error) {
		for _, f := range rv.data.Files {
			if f.Path == path {
				return f, nil
			}
		}
		return prFile{}, fmt.Errorf("%s is not changed in this PR; look at the diff of the working tree for the changed files", path)
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
			Description: "Propose an inline review comment on a line of the new file that the PR diff shows (the line number in the file as of the PR head). " +
				"For a comment about several lines, also give start_line: the comment then covers start_line to line, and both must be in the same hunk. " +
				"The user may reject it; the result says so. level is critical, recommended or note.",
			Schema: `{"type":"object","properties":{"path":{"type":"string"},"line":{"type":"integer"},"start_line":{"type":"integer"},"level":{"type":"string","enum":["critical","recommended","note"]},"body":{"type":"string"}},"required":["path","line","level","body"]}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var a struct {
					Path      string
					Line      int
					StartLine int `json:"start_line"`
					Level     string
					Body      string
				}
				if err := decode(raw, &a); err != nil {
					return "", err
				}
				f, err := file(a.Path)
				if err != nil {
					return "", err
				}
				start := a.StartLine
				if start == 0 || start == a.Line {
					start = a.Line
				}
				if !f.newLines()[a.Line] {
					return "", fmt.Errorf("invalid: line %d of %s is not in the diff; comments can only attach to lines that the PR changes or shows as context", a.Line, a.Path)
				}
				if !f.rangeOK(start, a.Line) {
					return "", fmt.Errorf("invalid: lines %d to %d of %s are not one stretch of the diff; a range must start before it ends and stay inside one hunk", start, a.Line, a.Path)
				}
				if strings.TrimSpace(a.Body) == "" {
					return "", fmt.Errorf("invalid: the comment is empty")
				}
				c := &reviewCard{Path: a.Path, Line: a.Line, Body: levelPrefix(a.Level) + strings.TrimSpace(a.Body)}
				c.label = fmt.Sprintf("%s:%d", a.Path, a.Line)
				if start < a.Line {
					c.Start = start
					c.label = fmt.Sprintf("%s:%d–%d", a.Path, start, a.Line)
				}
				c.View = f.viewRange(start, a.Line)
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
