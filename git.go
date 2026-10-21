package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type gitBranch struct {
	Name, Ref string
	Remote    bool
}

// repository is local Git state. PR discovery is optional enrichment.
type repository struct {
	Root, Branch, Head string
	HeadBranch         string // upstream branch name, when it differs from the local name
	Branches           []gitBranch
	Source, Target     string // remote URLs, from Git
	Link               string // branch.<name>.v-pr in local Git config
	PRs                []pullRequest
	PRError            string
	PRChecked          time.Time
}

func (r *repository) key() string {
	if r == nil {
		return ""
	}
	return strings.Join([]string{r.Root, r.Branch, r.HeadBranch, r.Source, r.Target, r.Link}, "\n")
}

func gitOutput(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("git %s: %s", args[0], firstOutputLine(string(out), err))
	}
	return strings.TrimSpace(string(out)), nil
}

func firstOutputLine(out string, err error) string {
	if s := strings.TrimSpace(out); s != "" {
		return strings.SplitN(s, "\n", 2)[0]
	}
	return err.Error()
}

func readRepository(dir string) *repository {
	root, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil
	}
	r := &repository{Root: root}
	r.Branch, _ = gitOutput(dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	r.Head, _ = gitOutput(dir, "rev-parse", "--short", "HEAD")
	refs, _ := gitOutput(dir, "for-each-ref", "--format=%(refname)\t%(symref)", "refs/heads", "refs/remotes")
	locals := map[string]bool{}
	for _, line := range strings.Split(refs, "\n") {
		ref, _, _ := strings.Cut(line, "\t")
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			r.Branches = append(r.Branches, gitBranch{Name: name, Ref: name})
			locals[name] = true
		}
	}
	// A branch on several remotes (origin and upstream in a fork) is listed once, preferring origin.
	remotes := map[string]int{}
	for _, line := range strings.Split(refs, "\n") {
		ref, sym, _ := strings.Cut(line, "\t")
		if remote, ok := strings.CutPrefix(ref, "refs/remotes/"); ok && sym == "" {
			owner, name, _ := strings.Cut(remote, "/")
			if name == "" || name == "HEAD" || locals[name] {
				continue
			}
			b := gitBranch{Name: name, Ref: remote, Remote: true}
			if i, seen := remotes[name]; !seen {
				remotes[name] = len(r.Branches)
				r.Branches = append(r.Branches, b)
			} else if owner == "origin" {
				r.Branches[i] = b
			}
		}
	}
	remote := ""
	r.HeadBranch = r.Branch
	if r.Branch != "" {
		remote, _ = gitOutput(dir, "config", "--get", "branch."+r.Branch+".remote")
		if remote != "" && remote != "." {
			if merge, err := gitOutput(dir, "config", "--get", "branch."+r.Branch+".merge"); err == nil {
				if name, ok := strings.CutPrefix(merge, "refs/heads/"); ok {
					r.HeadBranch = name
				}
			}
		}
		r.Link, _ = gitOutput(dir, "config", "--local", "--get", "branch."+r.Branch+".v-pr")
	}
	if remote == "" || remote == "." {
		remote = "origin"
	}
	r.Source, _ = gitOutput(dir, "remote", "get-url", remote)
	if r.Source == "" {
		if remotes, err := gitOutput(dir, "remote"); err == nil && remotes != "" {
			r.Source, _ = gitOutput(dir, "remote", "get-url", strings.Split(remotes, "\n")[0])
		}
	}
	r.Target = r.Source
	// Fork checkouts commonly track origin and name the base remote upstream.
	if upstream, err := gitOutput(dir, "remote", "get-url", "upstream"); err == nil {
		r.Target = upstream
	}
	return r
}

type repositoryMsg struct {
	seq   int
	repo  *repository
	force bool
}
type pullRequestsMsg struct {
	seq int
	key string
	prs []pullRequest
	err error
}
type branchSwitchedMsg struct {
	seq int
	err error
}
type prLinkedMsg struct {
	seq int
	err error
}
type repositoryTickMsg struct{}

func repositoryTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return repositoryTickMsg{} })
}

func repositoryCommand(dir string, seq int, force bool) tea.Cmd {
	return func() tea.Msg { return repositoryMsg{seq, readRepository(dir), force} }
}

func (m *model) refreshRepository(force bool) tea.Cmd {
	if m.repoLoading || m.repoChanging {
		return nil
	}
	m.repoLoading = true
	return repositoryCommand(m.workdir, m.repoSeq, force)
}

func (m *model) onRepository(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case repositoryTickMsg:
		return tea.Batch(repositoryTick(), m.refreshRepository(false))
	case repositoryMsg:
		if msg.seq != m.repoSeq {
			return nil
		}
		m.repoLoading = false
		old, r := m.repo, msg.repo
		if r != nil && r.key() == old.key() {
			r.PRs, r.PRError, r.PRChecked = old.PRs, old.PRError, old.PRChecked
		} else {
			m.prLoading, m.repoMenu = false, nil
		}
		m.repo = r
		if m.picker != nil && m.picker.kind == "switch" {
			m.rebuildPicker()
		}
		if r == nil || r.Branch == "" || m.prLoading {
			return nil
		}
		if !msg.force && time.Since(r.PRChecked) < time.Minute {
			return nil
		}
		m.prLoading = true
		seq, key, snapshot := m.repoSeq, r.key(), *r
		return func() tea.Msg {
			prs, err := discoverPullRequests(snapshot)
			return pullRequestsMsg{seq, key, prs, err}
		}
	case pullRequestsMsg:
		if msg.seq != m.repoSeq || msg.key != m.repo.key() {
			return nil
		}
		m.prLoading = false
		m.repo.PRs, m.repo.PRChecked, m.repo.PRError = msg.prs, time.Now(), ""
		if msg.err != nil {
			m.repo.PRError = msg.err.Error()
		}
		if m.repoMenu != nil && m.repoMenu.kind == "pr" {
			m.repoMenu = nil
			return m.cmdPR("")
		}
	case branchSwitchedMsg:
		if msg.seq != m.repoSeq {
			return nil
		}
		m.repoChanging = false
		if msg.err != nil {
			m.setErr(msg.err.Error())
		} else {
			m.clearWorkspaceViews()
			m.workspaceNotice = "The user switched the Git branch in " + m.workdir + ". Read the current branch and files before continuing."
			m.flash("branch switched")
		}
		return m.refreshRepository(true)
	case prLinkedMsg:
		if msg.seq != m.repoSeq {
			return nil
		}
		m.repoChanging = false
		if msg.err != nil {
			m.setErr(msg.err.Error())
		} else {
			m.flash("PR link saved for this branch")
		}
		return m.refreshRepository(true)
	}
	return nil
}

func (m *model) switchBranch(b gitBranch) tea.Cmd {
	if m.repo == nil || (!b.Remote && b.Name == m.repo.Branch) || !m.canSwitchWorkspace() {
		return nil
	}
	m.repoSeq++
	m.repoChanging, m.repoLoading, m.prLoading = true, false, false
	m.repoMenu = nil
	m.flash("switching to " + b.Name + "…")
	dir, seq := m.workdir, m.repoSeq
	return func() tea.Msg {
		args := []string{"switch", "--", b.Ref}
		if b.Remote {
			args = []string{"switch", "--track", "-c", b.Name, "--", b.Ref}
		}
		_, err := gitOutput(dir, args...)
		return branchSwitchedMsg{seq, err}
	}
}

func (m *model) linkPR(link string) tea.Cmd {
	if m.repo == nil || m.repo.Branch == "" || m.repoChanging {
		return nil
	}
	m.repoSeq++
	m.repoChanging, m.repoLoading, m.prLoading = true, false, false
	m.repoMenu = nil
	dir, branch, seq := m.workdir, m.repo.Branch, m.repoSeq
	return func() tea.Msg {
		args := []string{"config", "--local", "--replace-all", "branch." + branch + ".v-pr", link}
		if link == "" {
			args = []string{"config", "--local", "--unset-all", "branch." + branch + ".v-pr"}
		}
		_, err := gitOutput(dir, args...)
		return prLinkedMsg{seq, err}
	}
}
