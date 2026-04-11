package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureBeadsRedirectRepairsStaleMetadata(t *testing.T) {
	townRoot := t.TempDir()
	rigRoot := filepath.Join(townRoot, "testrig")
	mayorBeads := filepath.Join(rigRoot, "mayor", "rig", ".beads")
	worktree := filepath.Join(rigRoot, "polecats", "worker1", "testrig")
	worktreeBeads := filepath.Join(worktree, ".beads")

	if err := os.MkdirAll(mayorBeads, 0755); err != nil {
		t.Fatalf("mkdir mayor beads: %v", err)
	}
	if err := os.MkdirAll(worktreeBeads, 0755); err != nil {
		t.Fatalf("mkdir worktree beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeBeads, "redirect"), []byte("../../../mayor/rig/.beads\n"), 0644); err != nil {
		t.Fatalf("write redirect: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeBeads, "metadata.json"), []byte(`{"dolt_database":"gastown"}`), 0644); err != nil {
		t.Fatalf("write stale metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeBeads, "config.yaml"), []byte("prefix: gs\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ensureBeadsRedirect(RoleContext{
		Role:     RolePolecat,
		TownRoot: townRoot,
		WorkDir:  worktree,
	})

	if _, err := os.Stat(filepath.Join(worktreeBeads, "metadata.json")); !os.IsNotExist(err) {
		t.Fatalf("metadata.json should be removed during redirect repair, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(worktreeBeads, "config.yaml")); err != nil {
		t.Fatalf("config.yaml should be preserved, got err=%v", err)
	}
	redirectBytes, err := os.ReadFile(filepath.Join(worktreeBeads, "redirect"))
	if err != nil {
		t.Fatalf("read redirect: %v", err)
	}
	if got, want := string(redirectBytes), "../../../mayor/rig/.beads\n"; got != want {
		t.Fatalf("redirect = %q, want %q", got, want)
	}
}
