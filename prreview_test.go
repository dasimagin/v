package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestParseReview(t *testing.T) {
	findings := `{"summary":"s","findings":[{"path":"f.go","line":2,"level":"critical","body":"b"}]}`
	for name, out := range map[string]string{
		"structured": `{"is_error":false,"result":"","structured_output":` + findings + `}`,
		"fenced":     `{"is_error":false,"result":"` + "```json\\n" + strings.ReplaceAll(findings, `"`, `\"`) + "\\n```" + `"}`,
	} {
		res, err := parseReview([]byte(out))
		if err != nil || len(res.Findings) != 1 || res.Findings[0].Level != "critical" || res.Summary != "s" {
			t.Errorf("%s: %+v %v", name, res, err)
		}
	}
	if _, err := parseReview([]byte(`{"is_error":true,"result":"boom"}`)); err == nil {
		t.Error("an error result must fail")
	}
}

func TestApplyReviewAndPayload(t *testing.T) {
	diff := "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,3 +1,3 @@\n keep\n-old\n+new\n tail\n"
	d := &prData{Repo: "o/r", Number: 7, Files: splitDiff(diff)}
	cards := d.applyReview(reviewResult{Summary: "sum", Findings: []reviewFinding{
		{"f.go", 2, "critical", "bug here"},
		{"f.go", 3, "recommended", "design"},
		{"f.go", 1, "note", "optional"},
		{"f.go", 99, "critical", "not in the diff"},
		{"other.go", 1, "recommended", "unknown file"},
	}})
	if len(cards) != 5 || len(d.Files[0].Comments) != 3 || len(d.Notes) != 2 {
		t.Fatalf("cards = %d, inline = %d, notes = %v", len(cards), len(d.Files[0].Comments), d.Notes)
	}
	var bodies []string
	for _, c := range d.Files[0].Comments {
		bodies = append(bodies, c.Body)
	}
	if strings.Join(bodies, "|") != "!! bug here|! design|optional" {
		t.Errorf("bodies = %q", bodies)
	}
	// The card for line 2 shows the diff around it, ending on the commented line's neighbors.
	if c := cards[0]; !c.Anchored || len(c.Hunk) == 0 || !strings.HasPrefix(c.Hunk[c.Target], "+new") {
		t.Errorf("hunk = %q target %d", c.Hunk, c.Target)
	}
	for _, c := range cards[:4] {
		c.Decision = "y"
	}
	cards[4].Decision = "n"
	check := func(pending bool) {
		payload, n := reviewPayload(d.Summary, cards, pending)
		var got struct {
			Event    string
			Body     string
			Comments []struct {
				Path string
				Line int
				Side string
			}
		}
		if err := json.Unmarshal(payload, &got); err != nil || n != 4 || len(got.Comments) != 3 ||
			got.Comments[0].Side != "RIGHT" || !strings.Contains(got.Body, "sum") || !strings.Contains(got.Body, "not in the diff") ||
			strings.Contains(got.Body, "unknown file") {
			t.Errorf("payload = %s (%v, n=%d)", payload, err, n)
		}
		if pending == (got.Event != "") {
			t.Errorf("pending=%v but event=%q", pending, got.Event)
		}
	}
	check(false)
	check(true)
	d.clearDrafts()
	if len(d.Files[0].Comments) != 0 || len(d.Notes) != 0 || d.Summary != "" {
		t.Error("clearDrafts left drafts")
	}
}

func TestFindingCardShowsCommentUnderItsLine(t *testing.T) {
	m := &model{w: 80, h: 30}
	m.log = []message{{
		kind: msgFinding, file: "f.go:2", text: "!! bug here",
		hunk: []string{" keep", "+new", " tail"}, target: 1,
	}}
	var out []string
	for _, l := range m.renderLog(20) {
		out = append(out, ansi.Strip(l))
	}
	text := strings.Join(out, "\n")
	at := func(s string) int { return strings.Index(text, s) }
	if !(at("f.go:2") >= 0 && at("+new") > at("f.go:2") && at("bug here") > at("+new") && at("tail") > at("bug here")) {
		t.Errorf("card layout:\n%s", text)
	}
}
