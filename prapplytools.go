package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// applyHub points v's MCP tools at the /apply run in progress. The tools are
// attached to the chat agent for the whole session; outside a run they refuse.
type applyHub struct {
	mu sync.Mutex
	rv *reviewFlow
}

func (h *applyHub) set(rv *reviewFlow) {
	h.mu.Lock()
	h.rv = rv
	h.mu.Unlock()
}

func (h *applyHub) get() *reviewFlow {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rv
}

// applyReadTools are the pr_* tools that only read.
var applyReadTools = map[string]bool{"description": true, "threads": true, "checks": true}

// applyTools are the hands of the chat agent while it applies review
// feedback: the pr_* readers, and pr_reply and pr_resolve, which ask the user
// first in manual mode. Git and the shell are the agent's own.
func applyTools(h *applyHub) []mcpTool {
	noRun := fmt.Errorf("no review is being applied right now; the user starts one with /apply")
	delegate := func(name string) func(context.Context, json.RawMessage) (string, error) {
		return func(ctx context.Context, raw json.RawMessage) (string, error) {
			rv := h.get()
			if rv == nil {
				return "", noRun
			}
			for _, t := range prTools(rv) {
				if t.Name == name {
					return t.Handler(ctx, raw)
				}
			}
			return "", fmt.Errorf("unknown tool %s", name)
		}
	}
	var tools []mcpTool
	for _, t := range prTools(&reviewFlow{data: &prData{}}) { // the definitions only
		if applyReadTools[strings.TrimPrefix(t.Name, toolPrefix)] {
			t.Handler = delegate(t.Name)
			tools = append(tools, t)
		}
	}
	return append(tools,
		mcpTool{
			Name:        toolPrefix + "reply",
			Description: "Reply in a review thread of the PR; thread is the id from pr_threads. In manual mode the user confirms it first and may decline.",
			Schema:      `{"type":"object","properties":{"thread":{"type":"string"},"body":{"type":"string"}},"required":["thread","body"]}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				rv := h.get()
				if rv == nil {
					return "", noRun
				}
				var a struct{ Thread, Body string }
				if err := json.Unmarshal(raw, &a); err != nil || a.Thread == "" || strings.TrimSpace(a.Body) == "" {
					return "", fmt.Errorf("invalid: a thread id and a body are required")
				}
				card := &reviewCard{Thread: a.Thread, Body: strings.TrimSpace(a.Body), label: "reply · " + rv.threadLabel(a.Thread), Question: "Post this reply to GitHub?"}
				if out, done := rv.gate(ctx, card, "reply"); done {
					return out, nil
				}
				if err := postThreadReply(rv.dir, rv.repo, rv.number, a.Thread, card.Body); err != nil {
					return "", err
				}
				return "posted", nil
			},
		},
		mcpTool{
			Name:        toolPrefix + "resolve",
			Description: "Mark a review thread as resolved; thread is the id from pr_threads. Resolve only threads you have actually addressed. In manual mode the user confirms it first and may decline.",
			Schema:      `{"type":"object","properties":{"thread":{"type":"string"}},"required":["thread"]}`,
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				rv := h.get()
				if rv == nil {
					return "", noRun
				}
				var a struct{ Thread string }
				if err := json.Unmarshal(raw, &a); err != nil || a.Thread == "" {
					return "", fmt.Errorf("invalid: a thread id is required")
				}
				card := &reviewCard{Thread: a.Thread, Body: "Mark the thread as resolved", label: "resolve · " + rv.threadLabel(a.Thread), Question: "Resolve this thread?"}
				if out, done := rv.gate(ctx, card, "resolution"); done {
					return out, nil
				}
				_, err := ghGraphQL(rv.dir, `mutation($id:ID!){resolveReviewThread(input:{threadId:$id}){thread{id}}}`, map[string]any{"id": a.Thread})
				if err != nil {
					return "", err
				}
				return "resolved", nil
			},
		})
}

// gate asks the user about a write (or lets it through in auto mode). done
// reports that the tool must return out instead of acting.
func (rv *reviewFlow) gate(ctx context.Context, c *reviewCard, what string) (out string, done bool) {
	answer, ended := rv.decide(ctx, c)
	switch {
	case ended:
		return "run_ended: the user stopped the run; finish your answer now", true
	case answer.key != "y":
		out = "declined: the user does not want this " + what + ". Do not retry it."
		if answer.note != "" {
			out += " The user said: " + answer.note
		}
		return out, true
	}
	return "", false
}

// postThreadReply answers in a thread: GitHub replies hang off a comment, so
// it finds the thread's first one.
func postThreadReply(dir, repo string, number int, thread, body string) error {
	out, err := ghGraphQL(dir, `query($id:ID!){node(id:$id){... on PullRequestReviewThread{comments(first:1){nodes{databaseId}}}}}`, map[string]any{"id": thread})
	if err != nil {
		return err
	}
	var res struct {
		Data struct {
			Node struct {
				Comments struct{ Nodes []struct{ DatabaseID int64 } }
			}
		}
	}
	if err := json.Unmarshal(out, &res); err != nil || len(res.Data.Node.Comments.Nodes) == 0 {
		return fmt.Errorf("thread %s was not found", thread)
	}
	_, err = ghOutput(dir, "api", "--method", "POST", replyEndpoint(repo, number, res.Data.Node.Comments.Nodes[0].DatabaseID), "-f", "body="+body)
	return err
}

// replyEndpoint is where a reply to a review comment is posted. The pull
// request number is part of the path; without it GitHub answers 404.
func replyEndpoint(repo string, number int, comment int64) string {
	return fmt.Sprintf("repos/%s/pulls/%d/comments/%d/replies", repo, number, comment)
}

// threadLabel says where a thread is, for the cards the user answers.
func (rv *reviewFlow) threadLabel(id string) string {
	if rv.data != nil {
		for _, f := range rv.data.Files {
			for _, t := range f.Threads {
				if t.ID != id {
					continue
				}
				who := ""
				if len(t.Comments) > 0 {
					who = " (@" + t.Comments[0].Author + ")"
				}
				if t.Line > 0 {
					return fmt.Sprintf("%s:%d%s", f.Path, t.Line, who)
				}
				return f.Path + who
			}
		}
	}
	return "thread " + id
}
