package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolecatStopPendingWork(t *testing.T) {
	t.Run("clean feature branch", func(t *testing.T) {
		repo := setupPolecatStopRepo(t)

		got, ok, err := polecatStopPendingWork(repo)
		if err != nil {
			t.Fatalf("polecatStopPendingWork: %v", err)
		}
		if ok {
			t.Fatalf("polecatStopPendingWork() hasWork = true, summary = %q", got.summary())
		}
	})

	t.Run("dirty non-runtime file", func(t *testing.T) {
		repo := setupPolecatStopRepo(t)
		writePolecatStopFile(t, repo, "implementation.go", "package main\n")

		got, ok, err := polecatStopPendingWork(repo)
		if err != nil {
			t.Fatalf("polecatStopPendingWork: %v", err)
		}
		if !ok || !strings.Contains(got.summary(), "uncommitted work") {
			t.Fatalf("polecatStopPendingWork() = (%v, %v), want uncommitted work", got.summary(), ok)
		}
	})

	t.Run("runtime-only dirt ignored", func(t *testing.T) {
		repo := setupPolecatStopRepo(t)
		writePolecatStopFile(t, repo, filepath.Join(".opencode", "plugins", "gastown.js"), "module.exports = {}\n")

		got, ok, err := polecatStopPendingWork(repo)
		if err != nil {
			t.Fatalf("polecatStopPendingWork: %v", err)
		}
		if ok {
			t.Fatalf("polecatStopPendingWork() hasWork = true, summary = %q", got.summary())
		}
	})

	t.Run("committed ahead branch", func(t *testing.T) {
		repo := setupPolecatStopRepo(t)
		writePolecatStopFile(t, repo, "implementation.go", "package main\n")
		runPolecatStopGit(t, repo, "add", "implementation.go")
		runPolecatStopGit(t, repo, "commit", "-m", "implementation")

		got, ok, err := polecatStopPendingWork(repo)
		if err != nil {
			t.Fatalf("polecatStopPendingWork: %v", err)
		}
		if !ok || !strings.Contains(got.summary(), "unsubmitted commit") {
			t.Fatalf("polecatStopPendingWork() = (%v, %v), want unsubmitted commit", got.summary(), ok)
		}
	})

	t.Run("pushed source branch still pending target", func(t *testing.T) {
		repo := setupPolecatStopRepo(t)
		writePolecatStopFile(t, repo, "implementation.go", "package main\n")
		runPolecatStopGit(t, repo, "add", "implementation.go")
		runPolecatStopGit(t, repo, "commit", "-m", "implementation")
		runPolecatStopGit(t, repo, "push", "-u", "origin", "HEAD")

		got, ok, err := polecatStopPendingWork(repo)
		if err != nil {
			t.Fatalf("polecatStopPendingWork: %v", err)
		}
		if !ok || !strings.Contains(got.summary(), "unsubmitted commit") {
			t.Fatalf("polecatStopPendingWork() = (%v, %v), want pushed-but-unsubmitted commit", got.summary(), ok)
		}
	})

	t.Run("base branch ignored", func(t *testing.T) {
		repo := setupPolecatStopRepo(t)
		runPolecatStopGit(t, repo, "checkout", "main")
		writePolecatStopFile(t, repo, "implementation.go", "package main\n")

		got, ok, err := polecatStopPendingWork(repo)
		if err != nil {
			t.Fatalf("polecatStopPendingWork: %v", err)
		}
		if ok {
			t.Fatalf("polecatStopPendingWork() hasWork = true, summary = %q", got.summary())
		}
	})

	t.Run("non repo fails without submit", func(t *testing.T) {
		_, ok, err := polecatStopPendingWork(t.TempDir())
		if err == nil {
			t.Fatal("polecatStopPendingWork() err = nil, want git inspection error")
		}
		if ok {
			t.Fatal("polecatStopPendingWork() hasWork = true, want false")
		}
	})
}

func setupPolecatStopRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	remote := t.TempDir()
	runPolecatStopGit(t, remote, "init", "--bare")
	runPolecatStopGit(t, repo, "init")
	runPolecatStopGit(t, repo, "config", "user.email", "test@example.com")
	runPolecatStopGit(t, repo, "config", "user.name", "Test User")
	runPolecatStopGit(t, repo, "remote", "add", "origin", remote)

	writePolecatStopFile(t, repo, "README.md", "main\n")
	runPolecatStopGit(t, repo, "add", "README.md")
	runPolecatStopGit(t, repo, "commit", "-m", "main")
	runPolecatStopGit(t, repo, "branch", "-M", "main")
	runPolecatStopGit(t, repo, "push", "-u", "origin", "main")
	runPolecatStopGit(t, repo, "remote", "set-head", "origin", "main")
	runPolecatStopGit(t, repo, "checkout", "-b", "polecat/synth/gt-test", "origin/main")

	return repo
}

func runPolecatStopGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writePolecatStopFile(t *testing.T, dir, name, contents string) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
