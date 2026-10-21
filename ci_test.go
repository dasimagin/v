package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

var sampleChecks = []ciCheck{
	{Name: "build", Workflow: "CI", Bucket: "pass", Link: "https://github.com/o/r/actions/runs/11/job/101"},
	{Name: "test", Workflow: "CI", Bucket: "fail", Link: "https://github.com/o/r/actions/runs/11/job/102"},
	{Name: "lint", Workflow: "CI", Bucket: "pending", Link: "https://github.com/o/r/actions/runs/11/job/103"},
	{Name: "external", Bucket: "fail", Link: "https://ci.example.com/build/9"},
}

func TestDescribeChecksFiltersByStatus(t *testing.T) {
	all := describeChecks(sampleChecks, "all")
	if !strings.Contains(all, "4 checks: 1 passed, 2 failed, 1 pending") || !strings.Contains(all, "[pass] CI / build") {
		t.Errorf("all = %q", all)
	}
	failing := describeChecks(sampleChecks, "failing")
	if !strings.Contains(failing, "CI / test") || !strings.Contains(failing, "external") || strings.Contains(failing, "CI / build") || strings.Contains(failing, "CI / lint") {
		t.Errorf("failing = %q", failing)
	}
	pending := describeChecks(sampleChecks, "pending")
	if !strings.Contains(pending, "CI / lint") || strings.Contains(pending, "CI / test") {
		t.Errorf("pending = %q", pending)
	}
	if got := describeChecks(nil, "all"); got != "no checks" {
		t.Errorf("no checks = %q", got)
	}
	if got := describeChecks(sampleChecks[:1], "failing"); !strings.Contains(got, "none with status failing") {
		t.Errorf("nothing failing = %q", got)
	}
}

func TestCheckJobIDAndTail(t *testing.T) {
	if id, ok := checkJobID("https://github.com/o/r/actions/runs/11/job/102"); !ok || id != "102" {
		t.Errorf("job id = %q %v", id, ok)
	}
	if _, ok := checkJobID("https://ci.example.com/build/9"); ok {
		t.Error("an external check has no Actions job")
	}
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		b.WriteString("line")
		b.WriteString(string(rune('0'+i%10)) + "\n")
	}
	got := tailLines(b.String(), 3)
	kept := 0
	for _, l := range strings.Split(got, "\n") {
		if strings.HasPrefix(l, "line") {
			kept++
		}
	}
	if kept != 3 || !strings.Contains(got, "7 lines left out") {
		t.Errorf("tail = %q", got)
	}
	if short := tailLines("a\nb\n", 5); short != "a\nb" {
		t.Errorf("a short log stays whole: %q", short)
	}
}

func TestWaitForChecks(t *testing.T) {
	calls := 0
	fetch := func() ([]ciCheck, error) {
		calls++
		if calls < 3 {
			return []ciCheck{{Name: "a", Bucket: "pending"}}, nil
		}
		return []ciCheck{{Name: "a", Bucket: "pass"}}, nil
	}
	checks, done, err := waitForChecks(context.Background(), fetch, 5*time.Millisecond, time.Second)
	if err != nil || !done || calls != 3 || checks[0].Bucket != "pass" {
		t.Errorf("wait until finished: done=%v calls=%d err=%v", done, calls, err)
	}
	// A check that never finishes ends at the timeout, reporting what is pending.
	checks, done, err = waitForChecks(context.Background(), func() ([]ciCheck, error) {
		return []ciCheck{{Name: "slow", Bucket: "pending"}}, nil
	}, 5*time.Millisecond, 30*time.Millisecond)
	if err != nil || done || checks[0].Name != "slow" {
		t.Errorf("timeout: done=%v err=%v", done, err)
	}
	// Stopping the run stops the wait; errors come through.
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	if _, _, err := waitForChecks(ctx, func() ([]ciCheck, error) { return []ciCheck{{Bucket: "pending"}}, nil }, time.Second, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("cancel: %v", err)
	}
	if _, _, err := waitForChecks(context.Background(), func() ([]ciCheck, error) { return nil, errors.New("gh failed") }, time.Millisecond, time.Second); err == nil {
		t.Error("an error from gh must come through")
	}
}

func TestCITools(t *testing.T) {
	names := map[string]bool{}
	for _, tool := range prTools(&reviewFlow{data: &prData{}}) {
		names[tool.Name] = true
	}
	for _, want := range []string{"pr_checks", "pr_check_log", "pr_checks_wait"} {
		if !names[want] {
			t.Errorf("the reviewer lacks %s", want)
		}
	}
	apply := map[string]bool{}
	for _, tool := range applyTools(&applyHub{}) {
		apply[tool.Name] = true
	}
	for _, want := range []string{"pr_checks", "pr_check_log", "pr_checks_wait", "pr_reply", "pr_resolve"} {
		if !apply[want] {
			t.Errorf("the fixer lacks %s", want)
		}
	}
}
