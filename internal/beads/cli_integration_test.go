package beads_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/testutil"
)

// TestIntegration runs the real bd CLI against a per-test disposable server.
func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skipf("bd CLI unavailable: %v", err)
	}

	port := testutil.StartIsolatedDoltContainer(t)
	workDir := t.TempDir()
	gitInit := exec.Command("git", "init", "--quiet", "--initial-branch=main")
	gitInit.Dir = workDir
	if output, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("initialize disposable git repository: %v\n%s", err, output)
	}

	initCmd := exec.Command("bd", "init", "--prefix", fmt.Sprintf("bi%d", os.Getpid()),
		"--quiet", "--server", "--server-port", port)
	initCmd.Dir = workDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("initialize disposable beads database: %v\n%s", err, output)
	}

	b := beads.New(workDir)
	if _, err := b.Create(beads.CreateOptions{Title: "integration fixture", Type: "task"}); err != nil {
		t.Fatalf("create disposable integration fixture: %v", err)
	}

	t.Run("List", func(t *testing.T) {
		issues, err := b.List(beads.ListOptions{Status: "open"})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		t.Logf("Found %d open issues", len(issues))
	})

	t.Run("Ready", func(t *testing.T) {
		issues, err := b.Ready()
		if err != nil {
			t.Fatalf("Ready failed: %v", err)
		}
		t.Logf("Found %d ready issues", len(issues))
	})

	t.Run("Blocked", func(t *testing.T) {
		issues, err := b.Blocked()
		if err != nil {
			t.Fatalf("Blocked failed: %v", err)
		}
		t.Logf("Found %d blocked issues", len(issues))
	})

	t.Run("Show", func(t *testing.T) {
		issues, err := b.List(beads.ListOptions{})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(issues) == 0 {
			t.Skip("no issues to show")
		}

		issue, err := b.Show(issues[0].ID)
		if err != nil {
			t.Fatalf("Show(%s) failed: %v", issues[0].ID, err)
		}
		t.Logf("Showed issue: %s - %s", issue.ID, issue.Title)
	})
}
