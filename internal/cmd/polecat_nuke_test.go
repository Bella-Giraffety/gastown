package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/polecat"
)

func TestNukeLocalBranchName(t *testing.T) {
	tests := []struct {
		branch string
		wantOK bool
	}{
		{branch: "polecat/raider/gt-123@abc", wantOK: true},
		{branch: ""},
		{branch: "HEAD"},
		{branch: "main"},
		{branch: "master"},
		{branch: "integration/test"},
		{branch: "origin/polecat/raider"},
		{branch: "upstream/main"},
		{branch: "feature/test"},
		{branch: "polecat/bad:ref"},
		{branch: "polecat/bad..ref"},
		{branch: "-polecat/bad"},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			got, ok := nukeLocalBranchName(tt.branch)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.branch {
				t.Fatalf("branch = %q, want %q", got, tt.branch)
			}
		})
	}
}

func TestNukePolecatTargetUsesCachedSafetyResult(t *testing.T) {
	err := nukePolecatTarget(polecatTarget{rigName: "gastown", polecatName: "raider"}, &SafetyCheckResult{
		Blocked: true,
		Reasons: []string{"cached safety blocker"},
	}, true, false)
	if err == nil {
		t.Fatal("nukePolecatTarget returned nil, want cached safety refusal")
	}
	if !strings.Contains(err.Error(), "cached safety blocker") {
		t.Fatalf("error = %v, want cached safety blocker", err)
	}
}

func TestPreservePolecatBranchBeforeNukeFailsClosedOnPushFailure(t *testing.T) {
	repo := setupNukeBranchRepo(t)
	installRejectingPrePushHook(t, repo)

	branch, err := preservePolecatBranchBeforeNuke(&polecat.Polecat{ClonePath: repo}, nil, false)
	if err == nil {
		t.Fatal("preservePolecatBranchBeforeNuke returned nil, want push failure")
	}
	if branch != "" {
		t.Fatalf("branch = %q, want empty on fail-closed error", branch)
	}
	if !strings.Contains(err.Error(), "refusing to nuke: push failed") {
		t.Fatalf("error = %v, want refusing push failure", err)
	}
}

func TestPreservePolecatBranchBeforeNukeForceBypassesPushFailure(t *testing.T) {
	repo := setupNukeBranchRepo(t)
	installRejectingPrePushHook(t, repo)

	branch, err := preservePolecatBranchBeforeNuke(&polecat.Polecat{ClonePath: repo}, nil, true)
	if err != nil {
		t.Fatalf("preservePolecatBranchBeforeNuke(force): %v", err)
	}
	if branch != "polecat/nuke-test" {
		t.Fatalf("branch = %q, want polecat/nuke-test", branch)
	}
}

func TestPreservePolecatBranchBeforeNukeSkipsDetachedHead(t *testing.T) {
	repo := setupNukeBranchRepo(t)
	runGitCmd(t, repo, "checkout", "--detach", "HEAD")

	branch, err := preservePolecatBranchBeforeNuke(&polecat.Polecat{ClonePath: repo}, nil, false)
	if err != nil {
		t.Fatalf("preservePolecatBranchBeforeNuke(detached): %v", err)
	}
	if branch != "" {
		t.Fatalf("branch = %q, want empty for detached HEAD", branch)
	}
}

func setupNukeBranchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.git")
	repo := filepath.Join(dir, "repo")
	runGitCmd(t, "", "init", "--bare", remote)
	runGitCmd(t, "", "init", repo)
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	writeTestFile(t, filepath.Join(repo, "README.md"), "base\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "base")
	runGitCmd(t, repo, "branch", "-M", "main")
	runGitCmd(t, repo, "remote", "add", "origin", remote)
	runGitCmd(t, repo, "push", "-u", "origin", "main")
	runGitCmd(t, repo, "switch", "-c", "polecat/nuke-test")
	writeTestFile(t, filepath.Join(repo, "work.txt"), "work\n")
	runGitCmd(t, repo, "add", "work.txt")
	runGitCmd(t, repo, "commit", "-m", "work")
	return repo
}

func installRejectingPrePushHook(t *testing.T, repo string) {
	t.Helper()
	hook := filepath.Join(repo, ".git", "hooks", "pre-push")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatalf("write pre-push hook: %v", err)
	}
}
