package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/git"
)

func TestPreservePolecatBranchBeforeNukeFailsClosedOnPushFailure(t *testing.T) {
	repo := setupNukePreserveRepo(t)
	installRejectingPrePushHook(t, repo)

	err := preservePolecatBranchBeforeNuke(git.NewGit(repo), "polecat/nuke", nil, true, false)
	if err == nil {
		t.Fatal("preservePolecatBranchBeforeNuke returned nil, want push failure")
	}
	if !strings.Contains(err.Error(), "push failed") {
		t.Fatalf("error = %v, want push failed", err)
	}
	assertRemoteBranchMissing(t, repo, "polecat/nuke")
}

func TestPreservePolecatBranchBeforeNukeForceBypassesPushFailure(t *testing.T) {
	repo := setupNukePreserveRepo(t)
	installRejectingPrePushHook(t, repo)

	if err := preservePolecatBranchBeforeNuke(git.NewGit(repo), "polecat/nuke", nil, true, true); err != nil {
		t.Fatalf("force preserve returned error: %v", err)
	}
	assertRemoteBranchMissing(t, repo, "polecat/nuke")
}

func TestPreservePolecatBranchBeforeNukeSkipsDetachedHead(t *testing.T) {
	repo := setupNukePreserveRepo(t)
	runGit(t, repo, "checkout", "--detach", "HEAD")

	if err := preservePolecatBranchBeforeNuke(git.NewGit(repo), "HEAD", nil, true, false); err != nil {
		t.Fatalf("detached HEAD preserve returned error: %v", err)
	}
	assertRemoteBranchMissing(t, repo, "HEAD")
}

func TestPreservePolecatBranchBeforeNukeDoesNotRecreateLandedBranch(t *testing.T) {
	repo := setupNukePreserveRepo(t)
	runGit(t, repo, "switch", "main")
	runGit(t, repo, "merge", "--ff-only", "polecat/nuke")
	runGit(t, repo, "push", "origin", "main")
	runGit(t, repo, "switch", "polecat/nuke")

	if err := preservePolecatBranchBeforeNuke(git.NewGit(repo), "polecat/nuke", []string{"origin/main"}, true, false); err != nil {
		t.Fatalf("preserve landed branch returned error: %v", err)
	}
	assertRemoteBranchMissing(t, repo, "polecat/nuke")
}

func TestPreservePolecatBranchBeforeNukeBareFallbackFailsClosedWhenRemoteMissing(t *testing.T) {
	repo := setupNukePreserveRepo(t)

	err := preservePolecatBranchBeforeNuke(git.NewGit(repo), "polecat/nuke", nil, false, false)
	if err == nil {
		t.Fatal("bare fallback preserve returned nil, want fail-closed error")
	}
	if !strings.Contains(err.Error(), "worktree is unavailable") {
		t.Fatalf("error = %v, want worktree unavailable", err)
	}
	assertRemoteBranchMissing(t, repo, "polecat/nuke")
}

func setupNukePreserveRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	repo := filepath.Join(root, "repo")
	runCmd(t, root, "git", "init", "--bare", remote)
	runCmd(t, root, "git", "init", repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	writeRecoveryFile(t, filepath.Join(repo, "README.md"), "base\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "base")
	runGit(t, repo, "branch", "-M", "main")
	runGit(t, repo, "remote", "add", "origin", remote)
	runGit(t, repo, "push", "-u", "origin", "main")
	runGit(t, repo, "switch", "-c", "polecat/nuke")
	writeRecoveryFile(t, filepath.Join(repo, "feature.txt"), "feature\n")
	runGit(t, repo, "add", "feature.txt")
	runGit(t, repo, "commit", "-m", "feature")
	return repo
}

func installRejectingPrePushHook(t *testing.T, repo string) {
	t.Helper()
	hook := filepath.Join(repo, ".git", "hooks", "pre-push")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatalf("write pre-push hook: %v", err)
	}
}

func assertRemoteBranchMissing(t *testing.T, repo, branch string) {
	t.Helper()
	exists, err := git.NewGit(repo).RemoteBranchExists("origin", branch)
	if err != nil {
		t.Fatalf("RemoteBranchExists(%s): %v", branch, err)
	}
	if exists {
		t.Fatalf("remote branch %s exists, want missing", branch)
	}
}
