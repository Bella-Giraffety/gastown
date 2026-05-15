package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTapGuardPRWorkflowAllowsAgentWithForkRemote(t *testing.T) {
	dir := initTapGuardGitRepo(t, "https://github.com/gastownhall/gastown.git")
	runGit(t, dir, "remote", "add", "fork", "https://github.com/example/gastown.git")
	withTapGuardCwd(t, dir)
	t.Setenv("GT_POLECAT", "toast")

	if err := runTapGuardPRWorkflow(nil, nil); err != nil {
		t.Fatalf("runTapGuardPRWorkflow() = %v, want allowed", err)
	}
}

func TestTapGuardPRWorkflowBlocksAgentWithoutForkRemote(t *testing.T) {
	dir := initTapGuardGitRepo(t, "https://github.com/gastownhall/gastown.git")
	withTapGuardCwd(t, dir)
	t.Setenv("GT_POLECAT", "toast")

	if err := runTapGuardPRWorkflow(nil, nil); err == nil {
		t.Fatal("runTapGuardPRWorkflow() = nil, want block without fork/upstream workflow")
	}
}

func TestTapGuardPRWorkflowAllowsSplitOriginPushURL(t *testing.T) {
	dir := initTapGuardGitRepo(t, "https://github.com/gastownhall/gastown.git")
	runGit(t, dir, "remote", "set-url", "--push", "origin", "https://github.com/example/gastown.git")
	withTapGuardCwd(t, dir)
	t.Setenv("GT_POLECAT", "toast")

	if err := runTapGuardPRWorkflow(nil, nil); err != nil {
		t.Fatalf("runTapGuardPRWorkflow() = %v, want allowed with split pushurl", err)
	}
}

func initTapGuardGitRepo(t *testing.T, origin string) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "remote", "add", "origin", origin)
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func withTapGuardCwd(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(filepath.Clean(dir)); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})
}
