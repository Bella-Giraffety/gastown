package polecat

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
)

type polecatBaseSelection struct {
	StartPoint    string
	LogicalBranch string
	Remote        string
	Branch        string
}

func (m *Manager) ResolveStartPoint(baseBranch string) (polecatBaseSelection, error) {
	repoGit, err := m.repoBase()
	if err != nil {
		return polecatBaseSelection{}, fmt.Errorf("finding repo base: %w", err)
	}
	return resolvePolecatStartPoint(m.rig, repoGit, baseBranch)
}

func resolvePolecatStartPoint(r *rig.Rig, repoGit *git.Git, baseBranch string) (polecatBaseSelection, error) {
	defaultBranch := polecatDefaultBranch(r, repoGit)
	selection, err := normalizePolecatBase(repoGit, strings.TrimSpace(baseBranch), defaultBranch)
	if err != nil {
		return polecatBaseSelection{}, err
	}

	if selection.Branch == defaultBranch && (selection.Remote == "origin" || selection.Remote == "upstream") {
		if err := guardForkDefaultBranchMirror(r, repoGit, defaultBranch); err != nil {
			return polecatBaseSelection{}, err
		}
		// Keep formulas/refinery on the logical default branch. They use origin/<branch>,
		// so default-branch work is allowed only after origin is proven upstream-equivalent.
		selection.StartPoint = "origin/" + defaultBranch
		selection.Remote = "origin"
		selection.Branch = defaultBranch
		selection.LogicalBranch = defaultBranch
	}

	return selection, nil
}

func polecatDefaultBranch(r *rig.Rig, repoGit *git.Git) string {
	if r != nil && r.Path != "" {
		if cfg, err := rig.LoadRigConfig(r.Path); err == nil && strings.TrimSpace(cfg.DefaultBranch) != "" {
			return strings.TrimSpace(cfg.DefaultBranch)
		}
	}
	if repoGit != nil {
		if branch := strings.TrimSpace(repoGit.RemoteDefaultBranch()); branch != "" {
			return branch
		}
	}
	return "main"
}

func normalizePolecatBase(repoGit *git.Git, baseBranch, defaultBranch string) (polecatBaseSelection, error) {
	if baseBranch == "" {
		baseBranch = defaultBranch
	}
	if strings.HasPrefix(baseBranch, "refs/remotes/") {
		remoteBranch := strings.TrimPrefix(baseBranch, "refs/remotes/")
		remote, branch, ok := strings.Cut(remoteBranch, "/")
		if !ok || remote == "" || branch == "" {
			return polecatBaseSelection{}, fmt.Errorf("invalid remote base ref %q", baseBranch)
		}
		return polecatBaseSelection{StartPoint: baseBranch, LogicalBranch: branch, Remote: remote, Branch: branch}, nil
	}
	if strings.HasPrefix(baseBranch, "refs/") {
		return polecatBaseSelection{StartPoint: baseBranch, LogicalBranch: baseBranch}, nil
	}

	remote, branch, ok := remoteQualifiedBase(repoGit, baseBranch)
	if !ok {
		remote, branch = "origin", baseBranch
	}
	return polecatBaseSelection{StartPoint: remote + "/" + branch, LogicalBranch: branch, Remote: remote, Branch: branch}, nil
}

func remoteQualifiedBase(repoGit *git.Git, value string) (string, string, bool) {
	remote, branch, ok := strings.Cut(value, "/")
	if !ok || remote == "" || branch == "" {
		return "", "", false
	}
	if repoGit != nil {
		if remotes, err := repoGit.Remotes(); err == nil {
			for _, candidate := range remotes {
				if candidate == remote {
					return remote, branch, true
				}
			}
		}
	}
	return "", "", false
}

func guardForkDefaultBranchMirror(r *rig.Rig, repoGit *git.Git, branch string) error {
	hasUpstream, err := ensurePolecatUpstreamRemote(r, repoGit)
	if err != nil {
		return err
	}
	if !hasUpstream {
		return nil
	}

	if err := repoGit.FetchRemoteTrackingBranch("origin", branch); err != nil {
		return forkBaseError(r, branch, "cannot verify fork main: fetching origin/%s failed: %v", branch, err)
	}
	if err := repoGit.FetchRemoteTrackingBranch("upstream", branch); err != nil {
		return forkBaseError(r, branch, "cannot verify fork main: fetching upstream/%s failed: %v", branch, err)
	}

	originRef := git.RemoteTrackingRef("origin", branch)
	upstreamRef := git.RemoteTrackingRef("upstream", branch)
	originSHA, originErr := repoGit.Rev(originRef + "^{commit}")
	if originErr != nil {
		return forkBaseError(r, branch, "cannot verify fork main: resolving %s failed: %v", originRef, originErr)
	}
	upstreamSHA, upstreamErr := repoGit.Rev(upstreamRef + "^{commit}")
	if upstreamErr != nil {
		return forkBaseError(r, branch, "cannot verify fork main: resolving %s failed: %v", upstreamRef, upstreamErr)
	}
	if originSHA == upstreamSHA {
		return nil
	}

	div, divErr := repoGit.CountRefDivergence(originRef, upstreamRef)
	if divErr != nil {
		return forkBaseError(r, branch, "origin/%s is not identical to upstream/%s (origin %s, upstream %s); could not compute divergence: %v", branch, branch, shortSHA(originSHA), shortSHA(upstreamSHA), divErr)
	}
	return forkBaseError(r, branch, "origin/%s is a divergent fork main (%d ahead / %d behind upstream/%s)", branch, div.LeftOnly, div.RightOnly, branch)
}

func ensurePolecatUpstreamRemote(r *rig.Rig, repoGit *git.Git) (bool, error) {
	hasUpstream, err := repoGit.HasUpstreamRemote()
	if err != nil {
		return false, fmt.Errorf("checking upstream remote: %w", err)
	}
	if hasUpstream {
		return true, nil
	}
	upstreamURL := ""
	if r != nil {
		upstreamURL = strings.TrimSpace(r.UpstreamURL)
		if upstreamURL == "" && r.Path != "" {
			if cfg, err := rig.LoadRigConfig(r.Path); err == nil {
				upstreamURL = strings.TrimSpace(cfg.UpstreamURL)
			}
		}
	}
	if upstreamURL == "" {
		return false, nil
	}
	if err := repoGit.AddUpstreamRemote(upstreamURL); err != nil {
		return false, fmt.Errorf("configuring upstream remote from upstream_url: %w", err)
	}
	return true, nil
}

func forkBaseError(r *rig.Rig, branch, format string, args ...any) error {
	rigName := "<rig>"
	rigPath := "<town>/<rig>"
	if r != nil {
		if r.Name != "" {
			rigName = r.Name
		}
		if r.Path != "" {
			rigPath = r.Path
		}
	}
	detail := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s\n\nPolecats for upstream PR work must start from origin/%s only when it exactly mirrors upstream/%s.\n\nInspect:\n  git -C %s/mayor/rig fetch origin %s\n  git -C %s/mayor/rig fetch upstream %s\n  git -C %s/mayor/rig log --oneline --graph origin/%s...upstream/%s\n\nSafe remediation, after confirming fork-only commits are disposable:\n  git -C %s/mayor/rig push --force-with-lease origin refs/remotes/upstream/%s:refs/heads/%s\n\nThen retry the sling for rig %s.", detail, branch, branch, rigPath, branch, rigPath, branch, rigPath, branch, branch, rigPath, branch, branch, rigName)
}

func shortSHA(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}
