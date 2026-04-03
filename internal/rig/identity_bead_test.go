package rig

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestShowIdentityBead_PreservesTownRoutingContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a bash bd stub")
	}

	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "recovered")
	townBeadsDir := filepath.Join(townRoot, ".beads")
	localRigBeadsDir := filepath.Join(rigPath, ".beads")

	for _, dir := range []string{
		townBeadsDir,
		filepath.Join(townRoot, "mayor"),
		filepath.Join(rigPath, "mayor"),
		localRigBeadsDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigPath, "mayor", "town.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigPath, "config.json"), []byte(`{"beads":{"prefix":"rc"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townBeadsDir, "routes.jsonl"), []byte(`{"prefix":"rc-","path":"recovered/mayor/rig"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localRigBeadsDir, "metadata.json"), []byte(`{"dolt_database":"wrongdb"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bdStub := filepath.Join(binDir, "bd")
	script := `#!/usr/bin/env bash
set -euo pipefail

if [[ "${BEADS_DIR:-}" == "` + townBeadsDir + `" && "${1:-}" == "show" && "${2:-}" == "rc-rig-recovered" && "${3:-}" == "--json" ]]; then
  printf '%s\n' '[{"id":"rc-rig-recovered","labels":["gt:rig","status:docked"]}]'
  exit 0
fi

printf 'unexpected bd invocation: BEADS_DIR=%s args=%s\n' "${BEADS_DIR:-}" "$*" >&2
exit 1
`
	if err := os.WriteFile(bdStub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	issue, err := ShowIdentityBead(townRoot, "recovered")
	if err != nil {
		t.Fatalf("ShowIdentityBead: %v", err)
	}
	if issue.ID != "rc-rig-recovered" {
		t.Fatalf("issue.ID = %q, want %q", issue.ID, "rc-rig-recovered")
	}
	if len(issue.Labels) != 2 || issue.Labels[1] != "status:docked" {
		t.Fatalf("issue.Labels = %v, want docked rig labels", issue.Labels)
	}
}
