package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractBeadIDFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"simple", []string{"myproject-abc"}, "myproject-abc"},
		{"with flags after", []string{"gt-abc123", "--json"}, "gt-abc123"},
		{"with flags before", []string{"--json", "hq-xyz"}, "hq-xyz"},
		{"flags only", []string{"--json", "-v"}, ""},
		{"empty", []string{}, ""},
		{"mixed", []string{"-v", "bd-def456", "--json"}, "bd-def456"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractBeadIDFromArgs(tc.args)
			if got != tc.want {
				t.Errorf("extractBeadIDFromArgs(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestStripEnvKey(t *testing.T) {
	env := []string{"PATH=/usr/bin", "BEADS_DIR=/town/.beads", "HOME=/home/user", "BEADS_DIR=/other"}
	got := stripEnvKey(env, "BEADS_DIR")

	for _, e := range got {
		if e == "BEADS_DIR=/town/.beads" || e == "BEADS_DIR=/other" {
			t.Errorf("BEADS_DIR should be stripped, found: %s", e)
		}
	}
	if len(got) != 2 {
		t.Errorf("expected 2 entries after stripping, got %d", len(got))
	}
}

func TestStripEnvKey_NoMatch(t *testing.T) {
	env := []string{"PATH=/usr/bin", "HOME=/home/user"}
	got := stripEnvKey(env, "BEADS_DIR")
	if len(got) != 2 {
		t.Errorf("expected 2 entries (no change), got %d", len(got))
	}
}

func TestResolveBeadDir_UsesRoutesForNonHQPrefix(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatal(err)
	}
	routes := []byte("{\"prefix\":\"gs-\",\"path\":\"gastown/mayor/rig\"}\n{\"prefix\":\"hq-\",\"path\":\".\"}\n")
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), routes, 0644); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(townRoot); err != nil {
		t.Fatal(err)
	}

	if got := resolveBeadDir("gs-60j"); got != filepath.Join(townRoot, "gastown", "mayor", "rig") {
		t.Fatalf("resolveBeadDir(gs-60j) = %q, want %q", got, filepath.Join(townRoot, "gastown", "mayor", "rig"))
	}
	if got := resolveBeadDir("hq-123"); got != townRoot {
		t.Fatalf("resolveBeadDir(hq-123) = %q, want %q", got, townRoot)
	}
}

func TestGetIssueDetailsBatch_GroupsMixedPrefixesByResolvedDir(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatal(err)
	}
	rigDir := filepath.Join(townRoot, "gastown", "mayor", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	routes := []byte("{\"prefix\":\"gs-\",\"path\":\"gastown/mayor/rig\"}\n{\"prefix\":\"hq-\",\"path\":\".\"}\n")
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), routes, 0644); err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(townRoot, "bd.log")
	bdScript := `#!/bin/sh
set -e
echo "$(pwd)|$*" >> "${BD_LOG}"
if [ "$1" = "show" ]; then
  printf '['
  first=1
  shift
  for arg in "$@"; do
    [ "$arg" = "--json" ] && continue
    if [ $first -eq 0 ]; then printf ','; fi
    first=0
    printf '{"id":"%s","title":"T","status":"open"}' "$arg"
  done
  printf ']'
  exit 0
fi
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo %CD%^|%*>>"%BD_LOG%"
if "%1"=="show" (
  echo [{"id":"%2","title":"T","status":"open"}]
  exit /b 0
)
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)
	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(townRoot); err != nil {
		t.Fatal(err)
	}

	got := getIssueDetailsBatch([]string{"gs-60j", "hq-123"})
	if _, ok := got["gs-60j"]; !ok {
		t.Fatalf("expected gs-60j in result, got %#v", got)
	}
	if _, ok := got["hq-123"]; !ok {
		t.Fatalf("expected hq-123 in result, got %#v", got)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logLines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
	if len(logLines) != 2 {
		t.Fatalf("expected two grouped bd show invocations, got %d lines:\n%s", len(logLines), string(logBytes))
	}
	if !strings.Contains(logLines[0], rigDir+"|show gs-60j --json") && !strings.Contains(logLines[1], rigDir+"|show gs-60j --json") {
		t.Fatalf("expected one invocation from rig dir %q, log:\n%s", rigDir, string(logBytes))
	}
	if !strings.Contains(logLines[0], townRoot+"|show hq-123 --json") && !strings.Contains(logLines[1], townRoot+"|show hq-123 --json") {
		t.Fatalf("expected one invocation from town root %q, log:\n%s", townRoot, string(logBytes))
	}
	_ = bytes.Buffer{}
	_ = json.RawMessage{}
	_ = exec.ErrNotFound
}
