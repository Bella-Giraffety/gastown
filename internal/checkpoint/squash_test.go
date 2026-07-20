package checkpoint

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a fresh git repo with an initial commit and returns its path.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args[1:], err, out)
		}
	}

	// Create initial commit on main
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "initial commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args[1:], err, out)
		}
	}

	return dir
}

// createBranch creates a branch from current HEAD and switches to it.
func createBranch(t *testing.T, dir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout -b %s failed: %v\n%s", branch, err, out)
	}
}

// addCommit adds a file and commits with the given message.
func addCommit(t *testing.T, dir, filename, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", filename},
		{"git", "commit", "-m", msg},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args[1:], err, out)
		}
	}
}

func addCommitWithBody(t *testing.T, dir, filename, content, subject, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", filename},
		{"git", "commit", "-m", subject, "-m", body},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args[1:], err, out)
		}
	}
}

// getCommitSubjects returns the commit subjects on the branch since main.
func getCommitSubjects(t *testing.T, dir string) []string {
	t.Helper()
	cmd := exec.Command("git", "log", "--format=%s", "main..HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

func getHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func getStatus(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func gitSucceeds(dir string, args ...string) bool {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run() == nil
}

func TestCountWIPCommits_NoWIP(t *testing.T) {
	dir := initTestRepo(t)
	createBranch(t, dir, "feature")
	addCommit(t, dir, "a.go", "package a", "add feature A")
	addCommit(t, dir, "b.go", "package b", "add feature B")

	count, err := CountWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0 WIP commits, got %d", count)
	}
}

func TestCountWIPCommits_AllWIP(t *testing.T) {
	dir := initTestRepo(t)
	createBranch(t, dir, "feature")
	addCommit(t, dir, "a.go", "package a", WIPCommitPrefix)
	addCommit(t, dir, "b.go", "package b", WIPCommitPrefix)

	count, err := CountWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 WIP commits, got %d", count)
	}
}

func TestCountWIPCommits_Mixed(t *testing.T) {
	dir := initTestRepo(t)
	createBranch(t, dir, "feature")
	addCommit(t, dir, "a.go", "package a", "real work")
	addCommit(t, dir, "b.go", "package b", WIPCommitPrefix)
	addCommit(t, dir, "c.go", "package c", "more real work")

	count, err := CountWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 WIP commit, got %d", count)
	}
}

func TestSquashWIPCommits_NoWIP(t *testing.T) {
	dir := initTestRepo(t)
	createBranch(t, dir, "feature")
	addCommit(t, dir, "a.go", "package a", "real work")
	before := getHead(t, dir)

	wipCount, err := SquashWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if wipCount != 0 {
		t.Errorf("expected 0, got %d", wipCount)
	}

	// Verify commit is untouched
	subjects := getCommitSubjects(t, dir)
	if len(subjects) != 1 || subjects[0] != "real work" {
		t.Errorf("expected [real work], got %v", subjects)
	}
	if after := getHead(t, dir); after != before {
		t.Errorf("HEAD changed for no-WIP branch: got %s, want %s", after, before)
	}
}

func TestSquashWIPCommits_AllWIP(t *testing.T) {
	dir := initTestRepo(t)
	createBranch(t, dir, "feature")
	addCommit(t, dir, "a.go", "package a", WIPCommitPrefix)
	addCommit(t, dir, "b.go", "package b", WIPCommitPrefix)

	wipCount, err := SquashWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if wipCount != 2 {
		t.Errorf("expected 2, got %d", wipCount)
	}

	// Verify squashed into single commit with generic message
	subjects := getCommitSubjects(t, dir)
	if len(subjects) != 1 {
		t.Errorf("expected 1 commit after squash, got %d: %v", len(subjects), subjects)
	}
	if len(subjects) > 0 && subjects[0] != "squashed WIP checkpoint commits" {
		t.Errorf("expected generic message, got %q", subjects[0])
	}

	// Verify files exist
	for _, f := range []string{"a.go", "b.go"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected %s to exist after squash", f)
		}
	}
}

func TestSquashWIPCommits_Mixed(t *testing.T) {
	dir := initTestRepo(t)
	createBranch(t, dir, "feature")
	addCommit(t, dir, "a.go", "package a", "implement auth handler")
	addCommit(t, dir, "b.go", "package b", WIPCommitPrefix)
	addCommit(t, dir, "c.go", "package c", "add auth tests")
	addCommit(t, dir, "d.go", "package d", WIPCommitPrefix)

	wipCount, err := SquashWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if wipCount != 2 {
		t.Errorf("expected 2, got %d", wipCount)
	}

	// Verify squashed into single commit with non-WIP subjects preserved
	subjects := getCommitSubjects(t, dir)
	if len(subjects) != 1 {
		t.Errorf("expected 1 commit after squash, got %d: %v", len(subjects), subjects)
	}

	// Verify all files exist
	for _, f := range []string{"a.go", "b.go", "c.go", "d.go"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected %s to exist after squash", f)
		}
	}
}

func TestSquashWIPCommits_PreservesNonWIPMessageBody(t *testing.T) {
	dir := initTestRepo(t)
	createBranch(t, dir, "feature")
	addCommitWithBody(t, dir, "a.go", "package a", "real work", "Co-authored-by: furiosa <blair@humcapital.com>")
	addCommit(t, dir, "b.go", "package b", WIPCommitPrefix)

	wipCount, err := SquashWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if wipCount != 1 {
		t.Fatalf("expected 1, got %d", wipCount)
	}

	cmd := exec.Command("git", "log", "-1", "--format=%B")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	if !strings.Contains(string(out), "Co-authored-by: furiosa <blair@humcapital.com>") {
		t.Fatalf("squashed commit message dropped attribution trailer:\n%s", out)
	}
}

func TestSquashWIPCommits_IgnoresCurrentIndex(t *testing.T) {
	dir := initTestRepo(t)
	createBranch(t, dir, "feature")
	addCommit(t, dir, "a.go", "package a", "real work")
	addCommit(t, dir, "b.go", "package b", WIPCommitPrefix)

	if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("not committed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "staged.txt")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add staged.txt failed: %v\n%s", err, out)
	}

	wipCount, err := SquashWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if wipCount != 1 {
		t.Fatalf("expected 1, got %d", wipCount)
	}
	if gitSucceeds(dir, "cat-file", "-e", "HEAD:staged.txt") {
		t.Fatal("staged.txt was swept into the squashed commit")
	}
	if status := getStatus(t, dir); !strings.Contains(status, "A  staged.txt") {
		t.Fatalf("staged change was not preserved in the index, status %q", status)
	}
}

func TestSquashWIPCommits_NoCommits(t *testing.T) {
	dir := initTestRepo(t)
	createBranch(t, dir, "feature")

	wipCount, err := SquashWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if wipCount != 0 {
		t.Errorf("expected 0 for no commits, got %d", wipCount)
	}
}

func TestSquashWIPCommits_ReturnsErrorOnCommitTreeFailure(t *testing.T) {
	dir := initTestRepo(t)
	createBranch(t, dir, "feature")
	addCommit(t, dir, "a.go", "package a", "real work")
	addCommit(t, dir, "b.go", "package b", WIPCommitPrefix)

	before := getHead(t, dir)
	beforeSubjects := getCommitSubjects(t, dir)
	if status := getStatus(t, dir); status != "" {
		t.Fatalf("repo should start clean, got status %q", status)
	}

	t.Setenv("GIT_AUTHOR_NAME", "")
	t.Setenv("GIT_AUTHOR_EMAIL", "")
	t.Setenv("GIT_COMMITTER_NAME", "")
	t.Setenv("GIT_COMMITTER_EMAIL", "")

	if _, err := SquashWIPCommits(dir, "main"); err == nil {
		t.Fatal("expected commit-tree failure, got nil")
	}
	if after := getHead(t, dir); after != before {
		t.Fatalf("HEAD = %s after failed squash, want original %s", after, before)
	}
	if status := getStatus(t, dir); status != "" {
		t.Fatalf("repo status after rollback = %q, want clean", status)
	}
	afterSubjects := getCommitSubjects(t, dir)
	if strings.Join(afterSubjects, "\n") != strings.Join(beforeSubjects, "\n") {
		t.Fatalf("subjects changed after rollback: got %v, want %v", afterSubjects, beforeSubjects)
	}
}
