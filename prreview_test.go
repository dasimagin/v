package main

import (
	"encoding/json"
	"strings"
	"testing"
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
	d.applyReview(reviewResult{Summary: "sum", Findings: []reviewFinding{
		{"f.go", 2, "critical", "bug here"},
		{"f.go", 3, "recommended", "design"},
		{"f.go", 1, "note", "optional"},
		{"f.go", 99, "critical", "not in the diff"},
		{"other.go", 1, "recommended", "unknown file"},
	}})
	if got := len(d.Files[0].Comments); got != 3 {
		t.Fatalf("inline drafts = %d", got)
	}
	if len(d.Notes) != 2 || d.draftCount() != 5 {
		t.Errorf("notes = %v, count = %d", d.Notes, d.draftCount())
	}
	var bodies []string
	for _, c := range d.Files[0].Comments {
		bodies = append(bodies, c.Body)
	}
	if strings.Join(bodies, "|") != "!! bug here|! design|optional" {
		t.Errorf("bodies = %q", bodies)
	}
	payload, n := d.reviewPayload()
	var got struct {
		Event    string
		Body     string
		Comments []struct {
			Path string
			Line int
			Side string
		}
	}
	if err := json.Unmarshal(payload, &got); err != nil || n != 5 || got.Event != "COMMENT" || len(got.Comments) != 3 ||
		got.Comments[0].Side != "RIGHT" || !strings.Contains(got.Body, "sum") || !strings.Contains(got.Body, "not in the diff") {
		t.Errorf("payload = %s (%v, n=%d)", payload, err, n)
	}
	d.clearDrafts()
	if d.draftCount() != 0 || d.Summary != "" {
		t.Error("clearDrafts left drafts")
	}
}
