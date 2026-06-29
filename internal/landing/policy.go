package landing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	githubrepo "github.com/steveyegge/gastown/internal/github"
	"github.com/steveyegge/gastown/internal/rig"
)

// Policy describes how work for a rig should be based and landed.
// It deliberately separates the clean base ref used by git from the target
// branch name stored in MR metadata and PR bases.
type Policy struct {
	DefaultBranch   string
	TargetBranch    string
	CleanBaseRef    string
	CleanBaseRemote string

	ForkBacked bool
	PRMode     bool

	DirectDefaultPushAllowed bool
	Reason                   string

	BaseRepo  string
	HeadRepo  string
	HeadOwner string
}

// Resolve derives the landing policy for a target branch from rig config and git remotes.
func Resolve(g *git.Git, rigPath, targetBranch, defaultBranch string) Policy {
	if defaultBranch == "" {
		if cfg, err := rig.LoadRigConfig(rigPath); err == nil && cfg.DefaultBranch != "" {
			defaultBranch = cfg.DefaultBranch
		} else {
			defaultBranch = "main"
		}
	}

	target := BranchName(targetBranch)
	if target == "" {
		target = defaultBranch
	}

	policy := Policy{
		DefaultBranch:            defaultBranch,
		TargetBranch:             target,
		CleanBaseRef:             "origin/" + target,
		CleanBaseRemote:          "origin",
		DirectDefaultPushAllowed: true,
	}

	strategy, allowDirect := loadMergeQueuePolicy(rigPath)
	policy.PRMode = strategy == "pr"

	rigCfg, _ := rig.LoadRigConfig(rigPath)
	originFetch, originPush, upstreamURL := remoteTopology(g, rigCfg)
	canonicalURL := upstreamURL
	if canonicalURL == "" && rigCfg != nil {
		canonicalURL = rigCfg.GitURL
	}

	policy.BaseRepo, _, _ = repoIdentity(canonicalURL)
	policy.HeadRepo, policy.HeadOwner, _ = repoIdentity(originPush)

	if canonicalURL != "" && originPush != "" && !sameRemoteRepo(canonicalURL, originPush) {
		// A distinct push target plus an upstream/canonical repo means target-branch
		// pushes land in a fork, not the canonical base. Default branches must fail
		// closed unless explicitly allowed.
		if upstreamURL != "" || (originFetch != "" && !sameRemoteRepo(originFetch, originPush)) || (rigCfg != nil && (rigCfg.PushURL != "" || rigCfg.UpstreamURL != "")) {
			policy.ForkBacked = true
			policy.Reason = "fork-backed topology detected"
			if upstreamURL != "" && target == defaultBranch {
				policy.CleanBaseRef = "upstream/" + target
				policy.CleanBaseRemote = "upstream"
			}
		}
	}

	if policy.PRMode {
		policy.DirectDefaultPushAllowed = false
		policy.Reason = "merge_queue.merge_strategy=pr"
	} else if policy.ForkBacked && target == defaultBranch {
		policy.DirectDefaultPushAllowed = false
		if policy.Reason == "" {
			policy.Reason = "fork-backed topology detected"
		}
	}

	// allow_direct_default_push is an explicit escape hatch for direct-mode fork rigs.
	// It never overrides PR mode: PR mode means the target branch is landed by the VCS.
	if allowDirect != nil && !policy.PRMode {
		policy.DirectDefaultPushAllowed = *allowDirect
		if *allowDirect {
			policy.Reason = "merge_queue.allow_direct_default_push=true"
		}
	}

	return policy
}

// NormalizeBaseRef converts a requested base branch/ref into a git start ref.
func NormalizeBaseRef(g *git.Git, rigPath, requested, defaultBranch string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return Resolve(g, rigPath, "", defaultBranch).CleanBaseRef
	}
	if ref := RemoteQualifiedRef(requested); ref != "" {
		return ref
	}
	return Resolve(g, rigPath, requested, defaultBranch).CleanBaseRef
}

// BranchName returns the branch component for MR/PR targets.
func BranchName(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/remotes/")
	for _, remote := range []string{"origin/", "upstream/"} {
		if strings.HasPrefix(ref, remote) {
			return strings.TrimPrefix(ref, remote)
		}
	}
	return ref
}

// RemoteQualifiedRef returns a normalized remote-qualified ref when the input is one.
func RemoteQualifiedRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "refs/remotes/") {
		return strings.TrimPrefix(ref, "refs/remotes/")
	}
	if strings.HasPrefix(ref, "origin/") || strings.HasPrefix(ref, "upstream/") {
		return ref
	}
	return ""
}

// RemoteFromRef returns the remote name for refs like origin/main or upstream/main.
func RemoteFromRef(ref string) string {
	ref = RemoteQualifiedRef(ref)
	if ref == "" {
		return ""
	}
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

// CheckDefaultBranchDirectPush returns an error if a target-branch direct push is disallowed.
func (p Policy) CheckDefaultBranchDirectPush(target string) error {
	target = BranchName(target)
	if target == "" {
		target = p.TargetBranch
	}
	if target != p.DefaultBranch || p.DirectDefaultPushAllowed {
		return nil
	}
	reason := p.Reason
	if reason == "" {
		reason = "landing policy"
	}
	return fmt.Errorf("direct push to default branch %s is disabled by %s; push a source branch and use PR/MR landing", target, reason)
}

// PRHead returns the gh-compatible head spec for a source branch.
func (p Policy) PRHead(branch string) string {
	if p.HeadOwner != "" && p.BaseRepo != "" && p.HeadRepo != "" && !strings.EqualFold(p.BaseRepo, p.HeadRepo) {
		return p.HeadOwner + ":" + branch
	}
	return branch
}

func remoteTopology(g *git.Git, rigCfg *rig.RigConfig) (originFetch, originPush, upstreamURL string) {
	if g != nil {
		originFetch, _ = g.RemoteURL("origin")
		originPush, _ = g.GetPushURL("origin")
		upstreamURL, _ = g.GetUpstreamURL()
	}
	if rigCfg != nil {
		if originFetch == "" {
			originFetch = rigCfg.GitURL
		}
		if originPush == "" {
			originPush = rigCfg.PushURL
		}
		if upstreamURL == "" {
			upstreamURL = rigCfg.UpstreamURL
		}
	}
	if originPush == "" {
		originPush = originFetch
	}
	return originFetch, originPush, upstreamURL
}

func repoIdentity(remoteURL string) (repo, owner string, ok bool) {
	owner, name, err := githubrepo.ParseRemoteURL(remoteURL)
	if err != nil {
		return "", "", false
	}
	return owner + "/" + name, owner, true
}

func sameRemoteRepo(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if repoA, _, okA := repoIdentity(a); okA {
		if repoB, _, okB := repoIdentity(b); okB {
			return strings.EqualFold(repoA, repoB)
		}
	}
	return canonicalRemoteString(a) == canonicalRemoteString(b)
}

func canonicalRemoteString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimSuffix(s, "/")
	if abs, err := filepath.Abs(s); err == nil && !strings.Contains(s, "://") {
		s = abs
	}
	return strings.ToLower(s)
}

func loadMergeQueuePolicy(rigPath string) (strategy string, allowDirect *bool) {
	strategy, allowDirect = loadRootMergeQueuePolicy(filepath.Join(rigPath, "config.json"))
	settings, err := config.LoadRigSettings(filepath.Join(rigPath, "settings", "config.json"))
	if err == nil && settings != nil && settings.MergeQueue != nil {
		if settings.MergeQueue.MergeStrategy != "" {
			strategy = settings.MergeQueue.MergeStrategy
		}
		if settings.MergeQueue.AllowDirectDefaultPush != nil {
			allowDirect = settings.MergeQueue.AllowDirectDefaultPush
		}
	}
	return strategy, allowDirect
}

func loadRootMergeQueuePolicy(path string) (strategy string, allowDirect *bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil
	}
	var raw struct {
		MergeQueue *struct {
			MergeStrategy          string `json:"merge_strategy"`
			AllowDirectDefaultPush *bool  `json:"allow_direct_default_push"`
		} `json:"merge_queue"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || raw.MergeQueue == nil {
		return "", nil
	}
	return raw.MergeQueue.MergeStrategy, raw.MergeQueue.AllowDirectDefaultPush
}
