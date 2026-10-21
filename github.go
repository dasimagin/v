package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type pullRequest struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	State          string `json:"state"`
	BaseRefName    string `json:"baseRefName"`
	IsDraft        bool   `json:"isDraft"`
	HeadRepository struct {
		Name string `json:"name"`
	} `json:"headRepository"`
	HeadRepositoryOwner struct {
		Login string `json:"login"`
	} `json:"headRepositoryOwner"`
}

// githubRepository only recognizes GitHub remotes; Git itself works with any host.
func githubRepository(remote string) string {
	if strings.HasPrefix(remote, "git@github.com:") {
		remote = "https://github.com/" + strings.TrimPrefix(remote, "git@github.com:")
	}
	u, err := url.Parse(remote)
	if err != nil || !strings.EqualFold(u.Hostname(), "github.com") {
		return ""
	}
	path := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	if strings.Count(path, "/") != 1 {
		return ""
	}
	return path
}

// discoverPullRequests is the only GitHub-specific operation: optional, read-only,
// and off the UI thread. Branches and explicit PR links never require gh.
func discoverPullRequests(r repository) ([]pullRequest, error) {
	repo := githubRepository(r.Target)
	if repo == "" || r.Branch == "" {
		return nil, nil
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("install gh and run gh auth login to discover PRs, or use /pr <url>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "list", "--repo", repo, "--head", r.HeadBranch,
		"--state", "open", "--limit", "100", "--json", "number,title,url,state,isDraft,baseRefName,headRepository,headRepositoryOwner")
	cmd.Dir = r.Root
	out, err := cmd.Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("PR lookup: %s", firstOutputLine(string(e.Stderr), err))
		}
		return nil, fmt.Errorf("PR lookup: %w", err)
	}
	var prs []pullRequest
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, err
	}
	// Equal branch names in other forks do not establish a link to this branch.
	source := githubRepository(r.Source)
	var matches []pullRequest
	for _, p := range prs {
		head := p.HeadRepositoryOwner.Login + "/" + p.HeadRepository.Name
		if source == "" || strings.EqualFold(source, head) {
			matches = append(matches, p)
		}
	}
	return matches, nil
}

func linkedPullRequest(link string) pullRequest {
	p := pullRequest{URL: link}
	if u, err := url.Parse(link); err == nil {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) > 0 {
			p.Number, _ = strconv.Atoi(parts[len(parts)-1])
		}
	}
	return p
}

func (r *repository) pullRequests() []pullRequest {
	if r == nil {
		return nil
	}
	prs := append([]pullRequest(nil), r.PRs...)
	if r.Link != "" {
		for _, p := range prs {
			if p.URL == r.Link {
				return prs
			}
		}
		prs = append([]pullRequest{linkedPullRequest(r.Link)}, prs...)
	}
	return prs
}

func (r *repository) selectedPR() *pullRequest {
	prs := r.pullRequests()
	for _, p := range prs {
		if p.URL == r.Link {
			return &p
		}
	}
	if len(prs) > 0 {
		return &prs[0]
	}
	return nil
}
