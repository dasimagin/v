package main

import (
	"regexp"
	"strings"
)

// forcePush matches git push with --force, --force-with-lease, -f or a "+" refspec.
var forcePush = regexp.MustCompile(`\bgit\s+push\b[^\n]*(--force\b|--force-with-lease\b|\s-[a-zA-Z]*f[a-zA-Z]*\b|\s\+\S)`)

// autoDeny is the one rule that still holds when /apply runs in auto mode and
// nobody is watching: the agent must not rewrite the PR branch's history.
// It returns the reason a tool request is refused, or "" if it may go on.
func autoDeny(tool string, input map[string]any) string {
	if tool != "Bash" {
		return ""
	}
	cmd, _ := input["command"].(string)
	if forcePush.MatchString(cmd) {
		return "force-pushing is not allowed here; push normally, and if that is rejected, stop and tell the user"
	}
	return ""
}

// ownTool reports whether a tool is one of v's own (served by its MCP server).
// v gates those itself, so the permission prompt is skipped for them.
func ownTool(tool string) bool { return strings.HasPrefix(tool, "mcp__v__") }
