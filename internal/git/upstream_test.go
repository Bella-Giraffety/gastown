package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGit_UpstreamRemote(t *testing.T) {
	tmp := t.TempDir()
	g := NewGit(tmp)
	runGit(t, tmp, "init", "--initial-branch", "main")

	t.Run("initially absent", func(t *testing.T) {
		has, err := g.HasUpstreamRemote()
		if err != nil {
			t.Fatalf("HasUpstreamRemote: %v", err)
		}
		if has {
			t.Fatal("expected no upstream remote initially")
		}

		url, err := g.GetUpstreamURL()
		if err != nil {
			t.Fatalf("GetUpstreamURL: %v", err)
		}
		if url != "" {
			t.Errorf("expected empty URL, got %q", url)
		}
	})

	upstream1 := "https://example.com/upstream1.git"

	t.Run("add", func(t *testing.T) {
		if err := g.AddUpstreamRemote(upstream1); err != nil {
			t.Fatalf("AddUpstreamRemote: %v", err)
		}

		has, err := g.HasUpstreamRemote()
		if err != nil {
			t.Fatalf("HasUpstreamRemote: %v", err)
		}
		if !has {
			t.Fatal("expected upstream remote to exist")
		}

		url, err := g.GetUpstreamURL()
		if err != nil {
			t.Fatalf("GetUpstreamURL: %v", err)
		}
		if url != upstream1 {
			t.Errorf("URL = %q, want %q", url, upstream1)
		}
	})

	t.Run("idempotent same URL is true no-op", func(t *testing.T) {
		if err := g.AddUpstreamRemote(upstream1); err != nil {
			t.Fatalf("AddUpstreamRemote: %v", err)
		}

		url, err := g.GetUpstreamURL()
		if err != nil {
			t.Fatalf("GetUpstreamURL: %v", err)
		}
		if url != upstream1 {
			t.Errorf("URL = %q, want %q", url, upstream1)
		}
	})

	upstream2 := "https://example.com/upstream2.git"

	t.Run("update different URL", func(t *testing.T) {
		if err := g.AddUpstreamRemote(upstream2); err != nil {
			t.Fatalf("AddUpstreamRemote: %v", err)
		}

		url, err := g.GetUpstreamURL()
		if err != nil {
			t.Fatalf("GetUpstreamURL: %v", err)
		}
		if url != upstream2 {
			t.Errorf("URL = %q, want %q", url, upstream2)
		}
	})
}

func commitGitTestFile(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	g := NewGit(dir)
	if err := g.Add(name); err != nil {
		t.Fatalf("git add %s: %v", name, err)
	}
	if err := g.Commit("Update " + name); err != nil {
		t.Fatalf("git commit %s: %v", name, err)
	}
	sha, err := g.Rev("HEAD")
	if err != nil {
		t.Fatalf("rev HEAD: %v", err)
	}
	return strings.TrimSpace(sha)
}

func TestGit_FetchRemoteTrackingBranch(t *testing.T) {
	remote := t.TempDir()
	runGit(t, remote, "init", "--initial-branch", "main")
	runGit(t, remote, "config", "user.email", "test@test.com")
	runGit(t, remote, "config", "user.name", "Test User")
	commitGitTestFile(t, remote, "README.md", "initial\n")

	remoteGit := NewGit(remote)
	if err := remoteGit.CheckoutNewBranch("integration/foo", "main"); err != nil {
		t.Fatalf("checkout integration/foo: %v", err)
	}
	integrationSHA := commitGitTestFile(t, remote, "integration.txt", "integration\n")

	local := filepath.Join(t.TempDir(), "local")
	runGit(t, t.TempDir(), "clone", remote, local)
	localGit := NewGit(local)
	if err := localGit.FetchRemoteTrackingBranch("origin", "integration/foo"); err != nil {
		t.Fatalf("FetchRemoteTrackingBranch: %v", err)
	}
	got, err := localGit.Rev(RemoteTrackingRef("origin", "integration/foo"))
	if err != nil {
		t.Fatalf("rev tracking ref: %v", err)
	}
	if got != integrationSHA {
		t.Fatalf("tracking ref = %s, want %s", got, integrationSHA)
	}
}

func TestGit_CountRefDivergence(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "--initial-branch", "main")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test User")
	commitGitTestFile(t, dir, "README.md", "initial\n")
	g := NewGit(dir)

	if err := g.CheckoutNewBranch("left", "main"); err != nil {
		t.Fatalf("checkout left: %v", err)
	}
	commitGitTestFile(t, dir, "left.txt", "left\n")
	if err := g.CheckoutNewBranch("right", "main"); err != nil {
		t.Fatalf("checkout right: %v", err)
	}
	commitGitTestFile(t, dir, "right.txt", "right\n")

	tests := []struct {
		name      string
		leftRef   string
		rightRef  string
		leftOnly  int
		rightOnly int
	}{
		{name: "identical", leftRef: "main", rightRef: "main", leftOnly: 0, rightOnly: 0},
		{name: "right ahead", leftRef: "main", rightRef: "left", leftOnly: 0, rightOnly: 1},
		{name: "left ahead", leftRef: "left", rightRef: "main", leftOnly: 1, rightOnly: 0},
		{name: "two way", leftRef: "left", rightRef: "right", leftOnly: 1, rightOnly: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			div, err := g.CountRefDivergence(tt.leftRef, tt.rightRef)
			if err != nil {
				t.Fatalf("CountRefDivergence: %v", err)
			}
			if div.LeftOnly != tt.leftOnly || div.RightOnly != tt.rightOnly {
				t.Fatalf("divergence = %+v, want left=%d right=%d", div, tt.leftOnly, tt.rightOnly)
			}
		})
	}

	if _, err := g.CountRefDivergence("main", "missing-ref"); err == nil {
		t.Fatal("CountRefDivergence should fail for missing ref")
	}
}
