package polecat

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
)

// BaseSelection separates the physical git start point from the logical branch
// name used by formulas and refinery targets.
type BaseSelection struct {
	StartPoint    string
	LogicalBranch string
	Remote        string
	Branch        string
}

// ResolveStartPoint returns the authoritative base for a new polecat branch.
func (m *Manager) ResolveStartPoint(baseBranch string) (BaseSelection, error) {
	repoGit, err := m.repoBase()
	if err != nil {
		return BaseSelection{}, fmt.Errorf("finding repo base: %w", err)
	}
	return resolvePolecatStartPoint(m.rig, repoGit, baseBranch)
}

// ValidateResumeBranch guards pathological resumes of the default branch while
// leaving normal feature/PR branch resumes untouched.
func (m *Manager) ValidateResumeBranch(resumeBranch string) error {
	repoGit, err := m.repoBase()
	if err != nil {
		return fmt.Errorf("finding repo base: %w", err)
	}
	return validateDefaultResumeBranch(m.rig, repoGit, resumeBranch)
}

func resolvePolecatStartPoint(r *rig.Rig, repoGit *git.Git, baseBranch string) (BaseSelection, error) {
	defaultBranch := polecatDefaultBranch(r, repoGit)
	selection, err := normalizePolecatBase(repoGit, strings.TrimSpace(baseBranch), defaultBranch)
	if err != nil {
		return BaseSelection{}, err
	}

	if selection.Branch == defaultBranch && (selection.Remote == "origin" || selection.Remote == "upstream") {
		if err := guardForkDefaultBranchMirror(r, repoGit, defaultBranch); err != nil {
			return BaseSelection{}, err
		}
		// Formulas and refinery still target origin/<branch>. Default-branch work is
		// therefore allowed only after origin is proven upstream-equivalent.
		selection.StartPoint = "origin/" + defaultBranch
		selection.Remote = "origin"
		selection.Branch = defaultBranch
		selection.LogicalBranch = defaultBranch
	}

	return selection, nil
}

func validateDefaultResumeBranch(r *rig.Rig, repoGit *git.Git, resumeBranch string) error {
	defaultBranch := polecatDefaultBranch(r, repoGit)
	selection, err := normalizePolecatBase(repoGit, strings.TrimSpace(resumeBranch), defaultBranch)
	if err != nil {
		return err
	}
	if selection.Branch != defaultBranch || (selection.Remote != "origin" && selection.Remote != "upstream") {
		return nil
	}
	_, err = resolvePolecatStartPoint(r, repoGit, defaultBranch)
	return err
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

func normalizePolecatBase(repoGit *git.Git, baseBranch, defaultBranch string) (BaseSelection, error) {
	if baseBranch == "" {
		baseBranch = defaultBranch
	}
	if strings.HasPrefix(baseBranch, "refs/remotes/") {
		remoteBranch := strings.TrimPrefix(baseBranch, "refs/remotes/")
		remote, branch, ok := strings.Cut(remoteBranch, "/")
		if !ok || remote == "" || branch == "" {
			return BaseSelection{}, fmt.Errorf("invalid remote base ref %q", baseBranch)
		}
		if remote == "origin" {
			return BaseSelection{StartPoint: baseBranch, LogicalBranch: branch, Remote: remote, Branch: branch}, nil
		}
		if remote == "upstream" && branch == defaultBranch {
			return BaseSelection{StartPoint: baseBranch, LogicalBranch: branch, Remote: remote, Branch: branch}, nil
		}
		return BaseSelection{}, fmt.Errorf("unsupported remote base ref %q", baseBranch)
	}
	if strings.HasPrefix(baseBranch, "refs/heads/") {
		branch := strings.TrimPrefix(baseBranch, "refs/heads/")
		if branch == "" {
			return BaseSelection{}, fmt.Errorf("invalid local base ref %q", baseBranch)
		}
		return BaseSelection{StartPoint: "origin/" + branch, LogicalBranch: branch, Remote: "origin", Branch: branch}, nil
	}
	if strings.HasPrefix(baseBranch, "refs/") {
		return BaseSelection{}, fmt.Errorf("unsupported base ref %q", baseBranch)
	}

	if strings.HasPrefix(baseBranch, "origin/") {
		branch := strings.TrimPrefix(baseBranch, "origin/")
		if branch == "" {
			return BaseSelection{}, fmt.Errorf("invalid origin base %q", baseBranch)
		}
		return BaseSelection{StartPoint: "origin/" + branch, LogicalBranch: branch, Remote: "origin", Branch: branch}, nil
	}
	if baseBranch == "upstream/"+defaultBranch {
		return BaseSelection{StartPoint: "upstream/" + defaultBranch, LogicalBranch: defaultBranch, Remote: "upstream", Branch: defaultBranch}, nil
	}
	return BaseSelection{StartPoint: "origin/" + baseBranch, LogicalBranch: baseBranch, Remote: "origin", Branch: baseBranch}, nil
}

func guardForkDefaultBranchMirror(r *rig.Rig, repoGit *git.Git, branch string) error {
	guard, err := shouldGuardForkDefaultBranch(r, repoGit)
	if err != nil {
		return err
	}
	if !guard {
		return nil
	}

	if err := repoGit.FetchRemoteTrackingBranch("origin", branch); err != nil {
		return forkBaseError(r, branch, "cannot verify fork main: fetching origin/%s failed: %v", branch, err)
	}
	originRef := git.RemoteTrackingRef("origin", branch)
	comparisonRef := originRef
	comparisonLabel := "origin/" + branch
	if hasUpstream, _ := repoGit.HasUpstreamRemote(); hasUpstream {
		if err := repoGit.FetchRemoteTrackingBranch("upstream", branch); err != nil {
			return forkBaseError(r, branch, "cannot verify fork main: fetching upstream/%s failed: %v", branch, err)
		}
		upstreamRef := git.RemoteTrackingRef("upstream", branch)
		if err := requireMirrorRef(r, repoGit, branch, "origin/"+branch, originRef, upstreamRef, "upstream/"+branch); err != nil {
			return err
		}
		comparisonRef = upstreamRef
		comparisonLabel = "upstream/" + branch
	}

	forkURL := configuredPushURL(r)
	if forkURL == "" {
		if pushURL, err := repoGit.GetPushURL("origin"); err == nil {
			forkURL = strings.TrimSpace(pushURL)
		}
	}
	originURL := ""
	if url, err := repoGit.RemoteURL("origin"); err == nil {
		originURL = strings.TrimSpace(url)
	}
	if forkURL == "" || originURL == "" || forkURL == originURL {
		return nil
	}
	if err := repoGit.FetchRemoteTrackingBranchFrom(forkURL, "fork", branch); err != nil {
		return forkBaseError(r, branch, "cannot verify fork main: fetching fork main from origin push URL failed: %v", err)
	}
	return requireMirrorRef(r, repoGit, branch, "fork main", git.RemoteTrackingRef("fork", branch), comparisonRef, comparisonLabel)
}

func requireMirrorRef(r *rig.Rig, repoGit *git.Git, branch, label, ref, comparisonRef, comparisonLabel string) error {
	refSHA, refErr := repoGit.Rev(ref + "^{commit}")
	if refErr != nil {
		return forkBaseError(r, branch, "cannot verify fork main: resolving %s failed: %v", ref, refErr)
	}
	comparisonSHA, comparisonErr := repoGit.Rev(comparisonRef + "^{commit}")
	if comparisonErr != nil {
		return forkBaseError(r, branch, "cannot verify fork main: resolving %s failed: %v", comparisonRef, comparisonErr)
	}
	if refSHA == comparisonSHA {
		return nil
	}

	div, divErr := repoGit.CountRefDivergence(ref, comparisonRef)

	if divErr != nil {
		return forkBaseError(r, branch, "%s differs from %s (%s %s, %s %s); could not compute divergence: %v", label, comparisonLabel, label, shortSHA(refSHA), comparisonLabel, shortSHA(comparisonSHA), divErr)
	}
	return forkBaseError(r, branch, "%s is a divergent fork main (%d ahead / %d behind %s)", label, div.LeftOnly, div.RightOnly, comparisonLabel)
}

func shouldGuardForkDefaultBranch(r *rig.Rig, repoGit *git.Git) (bool, error) {
	if repoGit == nil {
		return false, nil
	}
	if hasDistinctOriginPushURL(r, repoGit) {
		return true, nil
	}
	if configuredUpstreamURL(r) != "" {
		hasUpstream, err := repoGit.HasUpstreamRemote()
		if err != nil {
			return false, fmt.Errorf("checking upstream remote: %w", err)
		}
		if !hasUpstream {
			return false, forkBaseError(r, polecatDefaultBranch(r, repoGit), "upstream_url is configured but no upstream remote exists")
		}
		return true, nil
	}

	hasUpstream, err := repoGit.HasUpstreamRemote()
	if err != nil {
		return false, fmt.Errorf("checking upstream remote: %w", err)
	}
	if !hasUpstream {
		return false, nil
	}
	originURL, originErr := repoGit.RemoteURL("origin")
	upstreamURL, upstreamErr := repoGit.GetUpstreamURL()
	if originErr != nil || upstreamErr != nil || strings.TrimSpace(originURL) == "" || strings.TrimSpace(upstreamURL) == "" {
		return false, nil
	}
	return strings.TrimSpace(originURL) != strings.TrimSpace(upstreamURL), nil
}

func hasDistinctOriginPushURL(r *rig.Rig, repoGit *git.Git) bool {
	originURL, originErr := repoGit.RemoteURL("origin")
	if originErr != nil || strings.TrimSpace(originURL) == "" {
		return false
	}
	pushURL := configuredPushURL(r)
	if pushURL == "" {
		if got, err := repoGit.GetPushURL("origin"); err == nil {
			pushURL = got
		}
	}
	return strings.TrimSpace(pushURL) != "" && strings.TrimSpace(pushURL) != strings.TrimSpace(originURL)
}

func configuredUpstreamURL(r *rig.Rig) string {
	if r == nil {
		return ""
	}
	if upstreamURL := strings.TrimSpace(r.UpstreamURL); upstreamURL != "" {
		return upstreamURL
	}
	if r.Path != "" {
		if cfg, err := rig.LoadRigConfig(r.Path); err == nil {
			return strings.TrimSpace(cfg.UpstreamURL)
		}
	}
	return ""
}

func configuredPushURL(r *rig.Rig) string {
	if r == nil {
		return ""
	}
	if pushURL := strings.TrimSpace(r.PushURL); pushURL != "" {
		return pushURL
	}
	if r.Path != "" {
		if cfg, err := rig.LoadRigConfig(r.Path); err == nil {
			return strings.TrimSpace(cfg.PushURL)
		}
	}
	return ""
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
	return fmt.Errorf("%s\n\nPolecats for upstream PR work must start from origin/%s only when it exactly mirrors upstream/%s, and fork main must also mirror upstream/%s when origin pushes to a fork.\n\nInspect:\n  git -C %s/mayor/rig remote -v  # origin (push) must be your fork; never push upstream\n  FORK_URL=$(git -C %s/mayor/rig remote get-url --push origin)\n  git -C %s/mayor/rig fetch origin +refs/heads/%s:refs/remotes/origin/%s\n  git -C %s/mayor/rig fetch upstream +refs/heads/%s:refs/remotes/upstream/%s\n  git -C %s/mayor/rig fetch \"$FORK_URL\" +refs/heads/%s:refs/remotes/fork/%s\n  git -C %s/mayor/rig log --oneline --graph refs/remotes/upstream/%s...refs/remotes/fork/%s\n\nSafe remediation, after confirming origin (push) is your fork and fork-only commits are disposable:\n  # Never push upstream. This pushes to origin's push URL (your fork).\n  git -C %s/mayor/rig push --force-with-lease origin refs/remotes/upstream/%s:refs/heads/%s\n\nSee docs/guides/fork-rig-setup.md#safe-fork-main-verificationsync, then retry the sling for rig %s.", detail, branch, branch, branch, rigPath, rigPath, rigPath, branch, branch, rigPath, branch, branch, rigPath, branch, branch, rigPath, branch, branch, rigPath, branch, branch, rigName)
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}
