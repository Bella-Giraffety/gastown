package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestExecuteSling_AcquiresBeadLock verifies that executeSling acquires the
// per-bead flock before reading bead status. This prevents TOCTOU races where
// multiple batch/queue dispatch calls read status=open concurrently.
func TestExecuteSling_AcquiresBeadLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("advisory flock is a no-op on Windows")
	}

	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0o755); err != nil {
		t.Fatalf("failed to create .beads: %v", err)
	}

	beadID := "gt-locktest1"

	// Hold the flock from outside executeSling — this simulates a concurrent dispatch.
	release, err := tryAcquireSlingBeadLock(townRoot, beadID)
	if err != nil {
		t.Fatalf("pre-acquire lock: %v", err)
	}
	defer release()

	// Create a bd stub (won't be reached since lock should block first)
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	bdScript := `#!/bin/sh
echo '[{"title":"Test","status":"open","assignee":"","description":""}]'
exit 0
`
	writeBDStub(t, binDir, bdScript, "")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	params := SlingParams{
		BeadID:   beadID,
		RigName:  "testrig",
		TownRoot: townRoot,
	}

	_, err = executeSling(params)
	if err == nil {
		t.Fatal("expected executeSling to fail when lock is held, got nil error")
	}
	if !strings.Contains(err.Error(), "already being slung") {
		t.Fatalf("expected lock contention error, got: %v", err)
	}
}

// TestExecuteSling_LockReleasedAfterReturn verifies that the flock is released
// when executeSling returns (even on error), allowing a subsequent call to proceed.
func TestExecuteSling_LockReleasedAfterReturn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("advisory flock is a no-op on Windows")
	}

	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0o755); err != nil {
		t.Fatalf("failed to create .beads: %v", err)
	}

	beadID := "gt-lockrel1"

	// Create a bd stub that returns closed status (causes executeSling to error out)
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	bdScript := `#!/bin/sh
echo '[{"title":"Done","status":"closed","assignee":"","description":""}]'
exit 0
`
	writeBDStub(t, binDir, bdScript, "")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	params := SlingParams{
		BeadID:   beadID,
		RigName:  "testrig",
		TownRoot: townRoot,
	}

	// First call — acquires lock, fails on closed guard, releases lock
	_, err := executeSling(params)
	if err == nil {
		t.Fatal("expected closed guard error")
	}

	// Second call — should acquire the lock (not contention error)
	_, err = executeSling(params)
	if err == nil {
		t.Fatal("expected closed guard error on second call")
	}
	if strings.Contains(err.Error(), "already being slung") {
		t.Fatal("lock was not released after first executeSling returned")
	}
}

func TestExecuteSling_HookFailureRollsBackFormulaArtifacts(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0o755); err != nil {
		t.Fatalf("failed to create .beads: %v", err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	bdScript := `#!/bin/sh
case "$1" in
  show)
    echo '[{"title":"Test","status":"open","assignee":"","description":""}]'
    ;;
esac
exit 0
`
	writeBDStub(t, binDir, bdScript, "")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	prevSpawn := spawnPolecatForSling
	prevInstantiate := instantiateFormulaOnBeadFn
	prevHook := hookBeadWithRetryFn
	prevRollback := rollbackSlingArtifactsFn
	prevAcquire := acquireSlingAssigneeLockFn
	t.Cleanup(func() {
		spawnPolecatForSling = prevSpawn
		instantiateFormulaOnBeadFn = prevInstantiate
		hookBeadWithRetryFn = prevHook
		rollbackSlingArtifactsFn = prevRollback
		acquireSlingAssigneeLockFn = prevAcquire
	})

	spawnPolecatForSling = func(rigName string, opts SlingSpawnOptions) (*SpawnedPolecatInfo, error) {
		return &SpawnedPolecatInfo{
			RigName:     rigName,
			PolecatName: "Toast",
			ClonePath:   filepath.Join(townRoot, "polecats", "Toast", "gastown"),
		}, nil
	}
	instantiateFormulaOnBeadFn = func(ctx context.Context, formulaName, beadID, title, hookWorkDir, townRoot string, skipCook bool, extraVars []string) (*FormulaOnBeadResult, error) {
		return &FormulaOnBeadResult{WispRootID: "gt-wisp-xyz", BeadToHook: beadID}, nil
	}
	hookBeadWithRetryFn = func(beadID, targetAgent, hookDir string) error {
		return errors.New("synthetic hook failure")
	}
	acquireSlingAssigneeLockFn = func(townRoot, targetAgent string) (func(), error) {
		return func() {}, nil
	}

	rollbackCalled := false
	rollbackSlingArtifactsFn = func(spawnInfo *SpawnedPolecatInfo, beadID, hookWorkDir, convoyID string) {
		rollbackCalled = true
		if spawnInfo == nil || spawnInfo.PolecatName != "Toast" {
			t.Fatalf("unexpected spawn info in rollback: %+v", spawnInfo)
		}
		if beadID != "gt-test123" {
			t.Fatalf("rollback beadID = %q, want %q", beadID, "gt-test123")
		}
		if convoyID != "" {
			t.Fatalf("rollback convoyID = %q, want empty", convoyID)
		}
	}

	params := SlingParams{
		BeadID:           "gt-test123",
		FormulaName:      "mol-polecat-work",
		RigName:          "gastown",
		TownRoot:         townRoot,
		NoConvoy:         true,
		SkipCook:         true,
		FormulaFailFatal: true,
	}

	_, err := executeSling(params)
	if err == nil {
		t.Fatal("expected hook failure")
	}
	if !strings.Contains(err.Error(), "failed to hook bead") {
		t.Fatalf("expected hook failure error, got: %v", err)
	}
	if !rollbackCalled {
		t.Fatal("expected full rollback on hook failure")
	}
}
