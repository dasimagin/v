package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
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
			Description: "The PR's CI checks on GitHub, with a count of passed, failed and pending ones. status is all (default), failing or pending.",
			Schema:      `{"type":"object","properties":{"status":{"type":"string","enum":["all","failing","pending"]}}}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var a struct{ Status string }
				if err := decode(raw, &a); err != nil {
					return "", err
				}
				return rv.checks(a.Status)
			},
		},
		{
			Name: toolPrefix + "check_log",
			Description: "The end of the log of a failing CI check (the first failing one when name is empty; a part of a check's name is enough). " +
				"tail is how many lines to give back (default 150, at most 1000). Only GitHub Actions jobs have logs that can be read.",
			Schema: `{"type":"object","properties":{"name":{"type":"string"},"tail":{"type":"integer"}}}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var a struct {
					Name string
					Tail int
				}
				if err := decode(raw, &a); err != nil {
					return "", err
				}
				return rv.checkLog(a.Name, a.Tail)
			},
		},
		{
			Name: toolPrefix + "checks_wait",
			Description: "Wait until the PR's CI checks have finished (none pending), then say how they ended. CI is slow, so this can take many minutes: " +
				"use it after a push when you want to know the outcome, not as a habit. timeout_seconds defaults to 1200 and is at most 3600.",
			Schema: `{"type":"object","properties":{"timeout_seconds":{"type":"integer"}}}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				var a struct {
					TimeoutSeconds int `json:"timeout_seconds"`
				}
				if err := decode(raw, &a); err != nil {
					return "", err
				}
				timeout := time.Duration(min(max(a.TimeoutSeconds, 0), 3600)) * time.Second
				if timeout == 0 {
					timeout = 20 * time.Minute
				}
				checks, done, err := waitForChecks(ctx, func() ([]ciCheck, error) { return fetchChecks(rv.dir, rv.url) }, 20*time.Second, timeout)
				if err != nil {
					return "", err
				}
				if !done {
					return "still pending after " + timeout.String() + "\n" + describeChecks(checks, "pending"), nil
				}
				return "finished\n" + describeChecks(checks, "all"), nil
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

// ciCheck is one CI check of the PR, as GitHub reports it.
type ciCheck struct {
	Name, Bucket, Workflow, Link, Description string
}

// bucket values of `gh pr checks`: pass, fail, pending, skipping, cancel.
func (c ciCheck) label() string {
	if c.Workflow != "" {
		return c.Workflow + " / " + c.Name
	}
	return c.Name
}

func fetchChecks(dir, url string) ([]ciCheck, error) {
	out, err := ghLoose(dir, "pr", "checks", url, "--json", "name,bucket,workflow,link,description")
	if err != nil {
		return nil, err
	}
	var checks []ciCheck
	if err := json.Unmarshal(out, &checks); err != nil {
		return nil, err
	}
	return checks, nil
}

// describeChecks lists the checks that match status: all, failing or pending.
func describeChecks(checks []ciCheck, status string) string {
	var lines []string
	counts := map[string]int{}
	for _, c := range checks {
		counts[c.Bucket]++
		switch {
		case status == "failing" && c.Bucket != "fail":
			continue
		case status == "pending" && c.Bucket != "pending":
			continue
		}
		line := fmt.Sprintf("[%s] %s", c.Bucket, c.label())
		if c.Link != "" {
			line += " " + c.Link
		}
		lines = append(lines, line)
	}
	if len(checks) == 0 {
		return "no checks"
	}
	head := fmt.Sprintf("%d checks: %d passed, %d failed, %d pending", len(checks), counts["pass"], counts["fail"], counts["pending"])
	if len(lines) == 0 {
		return head + "\nnone with status " + status
	}
	return head + "\n" + strings.Join(lines, "\n")
}

// checks lists the PR's CI checks, optionally only the failing or pending ones.
func (rv *reviewFlow) checks(status string) (string, error) {
	checks, err := fetchChecks(rv.dir, rv.url)
	if err != nil {
		return "", err
	}
	return describeChecks(checks, status), nil
}

var jobLink = regexp.MustCompile(`/actions/runs/\d+/job/(\d+)`)

// checkJobID is the GitHub Actions job behind a check's link.
func checkJobID(link string) (string, bool) {
	m := jobLink.FindStringSubmatch(link)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// tailLines keeps the last n lines of text.
func tailLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return fmt.Sprintf("… %d lines left out …\n", len(lines)-n) + strings.Join(lines[len(lines)-n:], "\n")
}

// checkLog is the end of the log of a failing check; name picks the check
// (a part of its name is enough). Checks that are not GitHub Actions jobs only have a link.
func (rv *reviewFlow) checkLog(name string, tail int) (string, error) {
	checks, err := fetchChecks(rv.dir, rv.url)
	if err != nil {
		return "", err
	}
	var picked *ciCheck
	for i, c := range checks {
		if name == "" && c.Bucket == "fail" || name != "" && strings.Contains(strings.ToLower(c.label()), strings.ToLower(name)) {
			picked = &checks[i]
			break
		}
	}
	if picked == nil {
		return "", fmt.Errorf("no check matches %q; pr_checks lists them", name)
	}
	job, ok := checkJobID(picked.Link)
	if !ok {
		return fmt.Sprintf("%s has no log that v can read; see %s", picked.label(), picked.Link), nil
	}
	out, err := ghOutput(rv.dir, "api", fmt.Sprintf("repos/%s/actions/jobs/%s/logs", rv.repo, job))
	if err != nil {
		return "", fmt.Errorf("the log of %s is not available yet or cannot be read: %w", picked.label(), err)
	}
	if tail <= 0 {
		tail = 150
	}
	return fmt.Sprintf("log of %s (%s):\n%s", picked.label(), picked.Bucket, tailLines(string(out), min(tail, 1000))), nil
}

// waitForChecks polls until no check is pending, the context ends or the timeout passes.
func waitForChecks(ctx context.Context, fetch func() ([]ciCheck, error), every, timeout time.Duration) ([]ciCheck, bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		checks, err := fetch()
		if err != nil {
			return nil, false, err
		}
		pending := 0
		for _, c := range checks {
			if c.Bucket == "pending" {
				pending++
			}
		}
		if pending == 0 {
			return checks, true, nil
		}
		if time.Now().Add(every).After(deadline) {
			return checks, false, nil
		}
		select {
		case <-ctx.Done():
			return checks, false, ctx.Err()
		case <-time.After(every):
		}
	}
}
