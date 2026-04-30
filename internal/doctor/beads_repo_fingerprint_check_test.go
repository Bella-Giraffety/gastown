package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func installMockBdRepoFingerprint(t *testing.T, output string) {
	t.Helper()

	binDir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Skip("shell-based bd shim not reliable on Windows CI")
	}

	script := "#!/bin/sh\n" +
		"printf '%b' \"" + strings.ReplaceAll(output, "\n", "\\n") + "\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(script), 0755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestBeadsRepoFingerprintCheck_Mismatch(t *testing.T) {
	installMockBdRepoFingerprint(t, "Dry run mode - no changes will be made\nWould update repository ID:\n  Old: dff473c6\n  New: 838a3cb3\n")

	townRoot := t.TempDir()
	rigName := "testrig"
	beadsDir := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Join(townRoot, rigName, "mayor", "rig", ".git")
	if err := os.WriteFile(gitDir, []byte("gitdir: /tmp/mock\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRepoFingerprintCheck()
	result := check.Run(&CheckContext{TownRoot: townRoot, RigName: rigName})

	if result.Status != StatusError {
		t.Fatalf("expected StatusError, got %v: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "fingerprint drift") {
		t.Fatalf("expected drift message, got %q", result.Message)
	}
	if len(result.Details) < 2 || !strings.Contains(result.Details[0], "dff473c6") || !strings.Contains(result.Details[1], "838a3cb3") {
		t.Fatalf("expected old/new repo IDs in details, got %v", result.Details)
	}
}

func TestBeadsRepoFingerprintCheck_Match(t *testing.T) {
	installMockBdRepoFingerprint(t, "Dry run mode - no changes will be made\nRepository ID already matches current clone\n")

	townRoot := t.TempDir()
	rigName := "testrig"
	beadsDir := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Join(townRoot, rigName, "mayor", "rig", ".git")
	if err := os.WriteFile(gitDir, []byte("gitdir: /tmp/mock\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRepoFingerprintCheck()
	result := check.Run(&CheckContext{TownRoot: townRoot, RigName: rigName})

	if result.Status != StatusOK {
		t.Fatalf("expected StatusOK, got %v: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "matches current clone") {
		t.Fatalf("unexpected message: %q", result.Message)
	}
}
