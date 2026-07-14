package refinery

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/bitbucket"
	"github.com/steveyegge/gastown/internal/git"
)

// bitbucketPRProvider implements PRProvider using the Bitbucket Cloud REST API.
type bitbucketPRProvider struct {
	git       *git.Git
	workspace string
	repoSlug  string
}

func newBitbucketPRProvider(g *git.Git) (PRProvider, error) {
	remoteURL, err := g.RemoteURL("origin")
	if err != nil {
		return nil, fmt.Errorf("bitbucket provider: failed to get origin remote URL: %w", err)
	}
	workspace, repoSlug, err := bitbucket.ParseBitbucketRemote(remoteURL)
	if err != nil {
		return nil, fmt.Errorf("bitbucket provider: %w", err)
	}
	return &bitbucketPRProvider{
		git:       g,
		workspace: workspace,
		repoSlug:  repoSlug,
	}, nil
}

func (p *bitbucketPRProvider) FindPullRequest(branch, _ string, prNumber int, headSHA string) (*git.PullRequestInfo, error) {
	if prNumber > 0 {
		return p.git.GetBitbucketPullRequest(p.workspace, p.repoSlug, prNumber, headSHA)
	}
	return p.git.FindBitbucketPullRequest(p.workspace, p.repoSlug, branch, headSHA)
}

func (p *bitbucketPRProvider) IsPRApproved(pr *git.PullRequestInfo) (bool, error) {
	return p.git.IsBitbucketPRApproved(p.workspace, p.repoSlug, pr.Number)
}

func (p *bitbucketPRProvider) MergePR(pr *git.PullRequestInfo, method string) (string, error) {
	return "", fmt.Errorf("bitbucket PR merge cannot enforce exact submitted-head matching at merge time; holding MR")
}
