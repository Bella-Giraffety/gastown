package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	gitpkg "github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/refinery"
)

func TestResolveMQSubmitCommitSHAUsesSubmittedBranch(t *testing.T) {
	repo := t.TempDir()
	runGitForMQSubmitTest(t, repo, "init")
	runGitForMQSubmitTest(t, repo, "config", "user.email", "test@example.com")
	runGitForMQSubmitTest(t, repo, "config", "user.name", "Test User")

	writeMQSubmitTestFile(t, repo, "file.txt", "main\n")
	runGitForMQSubmitTest(t, repo, "add", "file.txt")
	runGitForMQSubmitTest(t, repo, "commit", "-m", "main")
	runGitForMQSubmitTest(t, repo, "branch", "-M", "main")
	mainSHA := runGitForMQSubmitTest(t, repo, "rev-parse", "HEAD")

	runGitForMQSubmitTest(t, repo, "checkout", "-b", "feature/pr-target")
	writeMQSubmitTestFile(t, repo, "file.txt", "feature\n")
	runGitForMQSubmitTest(t, repo, "commit", "-am", "feature")
	featureSHA := runGitForMQSubmitTest(t, repo, "rev-parse", "HEAD")
	runGitForMQSubmitTest(t, repo, "tag", "feature/pr-target", mainSHA)

	runGitForMQSubmitTest(t, repo, "checkout", "main")
	g := gitpkg.NewGit(repo)
	got, err := resolveMQSubmitCommitSHA(g, "feature/pr-target")
	if err != nil {
		t.Fatalf("resolveMQSubmitCommitSHA: %v", err)
	}
	if got != featureSHA {
		t.Fatalf("resolveMQSubmitCommitSHA() = %s, want submitted branch tip %s", got, featureSHA)
	}
	if got == mainSHA {
		t.Fatalf("resolveMQSubmitCommitSHA() used HEAD %s instead of submitted branch tip", mainSHA)
	}
}

func TestVerifyMQSubmitPushedBranchRequiresRemoteBranch(t *testing.T) {
	repo := t.TempDir()
	remote := t.TempDir()
	runGitForMQSubmitTest(t, remote, "init", "--bare")

	runGitForMQSubmitTest(t, repo, "init")
	runGitForMQSubmitTest(t, repo, "config", "user.email", "test@example.com")
	runGitForMQSubmitTest(t, repo, "config", "user.name", "Test User")
	runGitForMQSubmitTest(t, repo, "remote", "add", "origin", remote)

	writeMQSubmitTestFile(t, repo, "file.txt", "main\n")
	runGitForMQSubmitTest(t, repo, "add", "file.txt")
	runGitForMQSubmitTest(t, repo, "commit", "-m", "main")
	runGitForMQSubmitTest(t, repo, "branch", "-M", "main")
	runGitForMQSubmitTest(t, repo, "push", "-u", "origin", "main")

	runGitForMQSubmitTest(t, repo, "checkout", "-b", "feature/pr-target")
	writeMQSubmitTestFile(t, repo, "file.txt", "feature\n")
	runGitForMQSubmitTest(t, repo, "commit", "-am", "feature")
	featureSHA := runGitForMQSubmitTest(t, repo, "rev-parse", "HEAD")

	g := gitpkg.NewGit(repo)
	err := verifyMQSubmitPushedBranch(g, "feature/pr-target", featureSHA)
	if err == nil {
		t.Fatal("verifyMQSubmitPushedBranch() = nil, want missing remote branch error")
	}
	for _, want := range []string{"git push origin feature/pr-target", "gt done"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("verifyMQSubmitPushedBranch() error missing %q: %v", want, err)
		}
	}

	runGitForMQSubmitTest(t, repo, "push", "origin", "feature/pr-target")
	if err := verifyMQSubmitPushedBranch(g, "feature/pr-target", featureSHA); err != nil {
		t.Fatalf("verifyMQSubmitPushedBranch() after push: %v", err)
	}
}

func TestDeletePostMergeRemoteBranchUsesSubmittedSHA(t *testing.T) {
	installNoPRGHForMQSubmitTest(t)
	repo := t.TempDir()
	remote := filepath.Join(t.TempDir(), "origin.git")
	runGitForMQSubmitTest(t, filepath.Dir(remote), "init", "--bare", "--initial-branch=main", remote)

	runGitForMQSubmitTest(t, repo, "init")
	runGitForMQSubmitTest(t, repo, "config", "user.email", "test@example.com")
	runGitForMQSubmitTest(t, repo, "config", "user.name", "Test User")
	runGitForMQSubmitTest(t, repo, "remote", "add", "origin", remote)
	runGitForMQSubmitTest(t, repo, "remote", "add", "upstream", "https://github.com/upstream/repo.git")

	writeMQSubmitTestFile(t, repo, "file.txt", "main\n")
	runGitForMQSubmitTest(t, repo, "add", "file.txt")
	runGitForMQSubmitTest(t, repo, "commit", "-m", "main")
	runGitForMQSubmitTest(t, repo, "branch", "-M", "main")
	runGitForMQSubmitTest(t, repo, "push", "-u", "origin", "main")

	branch := "polecat/test/gt-abc"
	runGitForMQSubmitTest(t, repo, "checkout", "-b", branch)
	writeMQSubmitTestFile(t, repo, "work.txt", "old\n")
	runGitForMQSubmitTest(t, repo, "add", "work.txt")
	runGitForMQSubmitTest(t, repo, "commit", "-m", "old")
	oldSHA := runGitForMQSubmitTest(t, repo, "rev-parse", "HEAD")
	runGitForMQSubmitTest(t, repo, "push", "origin", branch)
	writeMQSubmitTestFile(t, repo, "work.txt", "new\n")
	runGitForMQSubmitTest(t, repo, "commit", "-am", "new")
	newSHA := runGitForMQSubmitTest(t, repo, "rev-parse", "HEAD")
	runGitForMQSubmitTest(t, repo, "push", "origin", branch)

	g := gitpkg.NewGit(repo)
	err := deletePostMergeRemoteBranch(g, &refinery.MergeRequest{Branch: branch, CommitSHA: oldSHA})
	if err == nil {
		t.Fatal("expected leased delete to reject advanced remote branch")
	}
	remoteLine := runGitForMQSubmitTest(t, repo, "ls-remote", "origin", "refs/heads/"+branch)
	if !strings.HasPrefix(remoteLine, newSHA) {
		t.Fatalf("remote branch changed after stale leased delete: got %q want prefix %s", remoteLine, newSHA)
	}

	if err := deletePostMergeRemoteBranch(g, &refinery.MergeRequest{Branch: branch, CommitSHA: newSHA}); err != nil {
		t.Fatalf("matching leased delete: %v", err)
	}
	if remoteLine := runGitForMQSubmitTest(t, repo, "ls-remote", "origin", "refs/heads/"+branch); remoteLine != "" {
		t.Fatalf("remote branch still exists after matching leased delete: %q", remoteLine)
	}
}

func installNoPRGHForMQSubmitTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte(`#!/bin/sh
if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  printf '[]\n'
  exit 0
fi
printf 'unexpected gh args: %s\n' "$*" >&2
exit 1
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func runGitForMQSubmitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeMQSubmitTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidateMoleculePrereqs(t *testing.T) {
	tests := []struct {
		name      string
		children  []*beads.Issue
		wantErr   bool
		wantInErr []string // Substrings expected in error message
	}{
		{
			name:     "nil children",
			children: nil,
			wantErr:  false,
		},
		{
			name:     "empty children",
			children: []*beads.Issue{},
			wantErr:  false,
		},
		{
			name: "all prereqs closed",
			children: []*beads.Issue{
				{ID: "gt-mol.1", Title: "Load context", Status: "closed"},
				{ID: "gt-mol.2", Title: "Set up branch", Status: "closed"},
				{ID: "gt-mol.3", Title: "Implement", Status: "closed"},
				{ID: "gt-mol.4", Title: "Self-review", Status: "closed"},
				{ID: "gt-mol.5", Title: "Build check", Status: "closed"},
				{ID: "gt-mol.6", Title: "Commit changes", Status: "closed"},
				{ID: "gt-mol.7", Title: "Rebase verify", Status: "closed"},
				{ID: "gt-mol.8", Title: "Submit MR", Status: "open"},
				{ID: "gt-mol.9", Title: "Wait for verdict", Status: "open"},
				{ID: "gt-mol.10", Title: "Self-clean", Status: "open"},
			},
			wantErr: false,
		},
		{
			name: "missing self-review step",
			children: []*beads.Issue{
				{ID: "gt-mol.1", Title: "Load context", Status: "closed"},
				{ID: "gt-mol.2", Title: "Set up branch", Status: "closed"},
				{ID: "gt-mol.3", Title: "Implement", Status: "closed"},
				{ID: "gt-mol.4", Title: "Self-review", Status: "open"},
				{ID: "gt-mol.5", Title: "Build check", Status: "closed"},
				{ID: "gt-mol.6", Title: "Commit changes", Status: "closed"},
				{ID: "gt-mol.7", Title: "Rebase verify", Status: "closed"},
				{ID: "gt-mol.8", Title: "Submit MR", Status: "open"},
			},
			wantErr:   true,
			wantInErr: []string{"gt-mol.4", "Self-review", "--skip-deps"},
		},
		{
			name: "multiple incomplete steps",
			children: []*beads.Issue{
				{ID: "gt-mol.1", Title: "Load context", Status: "closed"},
				{ID: "gt-mol.2", Title: "Set up branch", Status: "open"},
				{ID: "gt-mol.3", Title: "Implement", Status: "in_progress"},
				{ID: "gt-mol.4", Title: "Self-review", Status: "open"},
				{ID: "gt-mol.5", Title: "Submit MR", Status: "open"},
			},
			wantErr:   true,
			wantInErr: []string{"gt-mol.2", "gt-mol.3", "gt-mol.4"},
		},
		{
			name: "no submit step found — checks all steps",
			children: []*beads.Issue{
				{ID: "gt-mol.1", Title: "Load context", Status: "closed"},
				{ID: "gt-mol.2", Title: "Implement", Status: "open"},
				{ID: "gt-mol.3", Title: "Build check", Status: "open"},
			},
			wantErr:   true,
			wantInErr: []string{"gt-mol.2", "gt-mol.3"},
		},
		{
			name: "post-submit steps open is OK",
			children: []*beads.Issue{
				{ID: "gt-mol.1", Title: "Load context", Status: "closed"},
				{ID: "gt-mol.2", Title: "Submit MR", Status: "open"},
				{ID: "gt-mol.3", Title: "Wait for verdict", Status: "open"},
			},
			wantErr: false,
		},
		{
			name: "case insensitive submit detection",
			children: []*beads.Issue{
				{ID: "gt-mol.1", Title: "Implement", Status: "closed"},
				{ID: "gt-mol.2", Title: "SUBMIT MR and enter awaiting_verdict", Status: "open"},
				{ID: "gt-mol.3", Title: "Self-clean", Status: "open"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMoleculePrereqs(tt.children)
			if tt.wantErr && err == nil {
				t.Errorf("validateMoleculePrereqs() = nil, want error")
				return
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateMoleculePrereqs() = %v, want nil", err)
				return
			}
			if err != nil {
				errMsg := err.Error()
				for _, want := range tt.wantInErr {
					if !strings.Contains(errMsg, want) {
						t.Errorf("error message missing %q, got: %s", want, errMsg)
					}
				}
			}
		})
	}
}
