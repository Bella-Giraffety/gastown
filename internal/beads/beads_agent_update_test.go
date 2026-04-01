package beads

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func installMockBDAgentStateUpdater(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "bd.log")

	if runtime.GOOS == "windows" {
		psPath := filepath.Join(binDir, "bd.ps1")
		psScript := `
$logFile = '` + strings.ReplaceAll(logPath, "'", "''") + `'
Add-Content -Path $logFile -Value ($args -join ' ')

$cmd = ''
foreach ($arg in $args) {
  if ($arg -like '--*') { continue }
  $cmd = $arg
  break
}

switch ($cmd) {
  'version' { exit 0 }
  'sql' {
    Write-Output 'OK, 1 rows affected'
    exit 0
  }
  'show' {
    Write-Output '[{"id":"gs-gastown-polecat-guzzle","title":"Polecat guzzle","issue_type":"agent","labels":["gt:agent"],"description":"Polecat guzzle\n\nrole_type: polecat\nrig: gastown\nagent_state: spawning\nhook_bead: gs-rt0\ncleanup_status: null\nactive_mr: null\nnotification_level: null"}]'
    exit 0
  }
  'update' {
    Write-Output '[]'
    exit 0
  }
  default { exit 0 }
}
`
		cmdScript := "@echo off\r\npwsh -NoProfile -NoLogo -File \"" + psPath + "\" %*\r\n"
		if err := os.WriteFile(psPath, []byte(psScript), 0644); err != nil {
			t.Fatalf("write mock bd ps1: %v", err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "bd.cmd"), []byte(cmdScript), 0644); err != nil {
			t.Fatalf("write mock bd cmd: %v", err)
		}
	} else {
		script := `#!/bin/sh
LOG_FILE='` + logPath + `'
printf '%s\n' "$*" >> "$LOG_FILE"

cmd=""
for arg in "$@"; do
  case "$arg" in
    --*) ;;
    *) cmd="$arg"; break ;;
  esac
done

case "$cmd" in
  version)
    exit 0
    ;;
  sql)
    printf 'OK, 1 rows affected\n'
    exit 0
    ;;
  show)
    printf '%s\n' '[{"id":"gs-gastown-polecat-guzzle","title":"Polecat guzzle","issue_type":"agent","labels":["gt:agent"],"description":"Polecat guzzle\n\nrole_type: polecat\nrig: gastown\nagent_state: spawning\nhook_bead: gs-rt0\ncleanup_status: null\nactive_mr: null\nnotification_level: null"}]'
    exit 0
    ;;
  update)
    printf '[]\n'
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
		if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(script), 0755); err != nil {
			t.Fatalf("write mock bd: %v", err)
		}
	}

	ResetBdAllowStaleCacheForTest()
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestUpdateAgentState_UsesDirectWispSQL(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	logPath := installMockBDAgentStateUpdater(t)

	bd := NewIsolated(tmpDir)
	if err := bd.UpdateAgentState("gs-gastown-polecat-guzzle", "working"); err != nil {
		t.Fatalf("UpdateAgentState: %v", err)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(logData)

	if strings.Contains(log, " set-state ") || strings.Contains(log, " agent state ") {
		t.Fatalf("UpdateAgentState used the wrong bd command:\n%s", log)
	}
	if !strings.Contains(log, "sql UPDATE wisps SET agent_state = 'working' WHERE id = 'gs-gastown-polecat-guzzle'") {
		t.Fatalf("UpdateAgentState did not update wisps.agent_state directly:\n%s", log)
	}
	if !strings.Contains(log, "show gs-gastown-polecat-guzzle --json") {
		t.Fatalf("UpdateAgentState did not reload the agent bead for description sync:\n%s", log)
	}
	if !strings.Contains(log, "update gs-gastown-polecat-guzzle") {
		t.Fatalf("UpdateAgentState did not sync the description field:\n%s", log)
	}
}

func TestEscapeSQLString(t *testing.T) {
	got := escapeSQLString("that's all")
	if got != "that''s all" {
		t.Fatalf("escapeSQLString = %q, want %q", got, "that''s all")
	}
}
