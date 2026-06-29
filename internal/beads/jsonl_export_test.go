package beads

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExportJSONLUsesResolvedBeadsDirAndSuppressedEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot := t.TempDir()
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "bd.log")
	bdScript := `#!/bin/sh
printf '%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s\n' "$*" "$(pwd)" "${BEADS_DIR:-}" "${BEADS_DOLT_SERVER_DATABASE:-}" "${BEADS_DB:-}" "${BD_DB:-}" "${BEADS_DOLT_DATA_DIR:-}" "${BD_DOLT_AUTO_COMMIT:-}" "${BD_READONLY:-}" "${BD_EXPORT_AUTO:-}" "${BD_BACKUP_ENABLED:-}" "${BD_DOLT_AUTO_PUSH:-}" "${BD_NO_PUSH:-}" "${BD_EXPORT_GIT_ADD:-}" "${BD_NO_GIT_OPS:-}" "${BEADS_NO_AUTO_IMPORT:-}" > "` + logPath + `"
printf 'summary should be captured\n' >&2
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GT_DOLT_DATA", "")
	t.Setenv("GT_DOLT_HOST", "")
	t.Setenv("GT_DOLT_PORT", "")
	t.Setenv("BEADS_DIR", filepath.Join(townRoot, "wrong", ".beads"))
	t.Setenv("BEADS_DOLT_SERVER_DATABASE", "wrong-db")
	t.Setenv("BEADS_DB", "wrong-beads-db")
	t.Setenv("BD_DB", "wrong-bd-db")
	t.Setenv("BEADS_DOLT_DATA_DIR", filepath.Join(townRoot, "wrong-dolt"))
	t.Setenv("BD_DOLT_AUTO_COMMIT", "on")
	t.Setenv("BD_READONLY", "false")
	t.Setenv("BD_EXPORT_AUTO", "true")
	t.Setenv("BD_BACKUP_ENABLED", "true")
	t.Setenv("BD_DOLT_AUTO_PUSH", "true")
	t.Setenv("BD_NO_PUSH", "false")
	t.Setenv("BD_EXPORT_GIT_ADD", "true")
	t.Setenv("BD_NO_GIT_OPS", "false")
	t.Setenv("BEADS_NO_AUTO_IMPORT", "0")

	if err := ExportJSONL(townRoot, ""); err != nil {
		t.Fatalf("ExportJSONL() error: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	fields := strings.Split(strings.TrimSpace(string(data)), "|")
	if len(fields) != 16 {
		t.Fatalf("log fields = %v, want 16", fields)
	}
	if want := "export -o " + filepath.Join(beadsDir, "issues.jsonl"); fields[0] != want {
		t.Fatalf("args = %q, want %q", fields[0], want)
	}
	if fields[1] != townRoot {
		t.Fatalf("cwd = %q, want %q", fields[1], townRoot)
	}
	if fields[2] != beadsDir {
		t.Fatalf("BEADS_DIR = %q, want %q", fields[2], beadsDir)
	}
	if fields[3] != "" || fields[4] != "" || fields[5] != "" || fields[6] != "" {
		t.Fatalf("stale target env leaked: DB=%q BEADS_DB=%q BD_DB=%q DATA_DIR=%q", fields[3], fields[4], fields[5], fields[6])
	}
	if fields[7] != "off" || fields[8] != "true" {
		t.Fatalf("export should be read-only, got BD_DOLT_AUTO_COMMIT=%q BD_READONLY=%q", fields[7], fields[8])
	}
	if fields[9] != "false" || fields[10] != "false" || fields[11] != "false" || fields[12] != "true" || fields[13] != "false" || fields[14] != "true" || fields[15] != "1" {
		t.Fatalf("side-effect env not suppressed: BD_EXPORT_AUTO=%q BD_BACKUP_ENABLED=%q BD_DOLT_AUTO_PUSH=%q BD_NO_PUSH=%q BD_EXPORT_GIT_ADD=%q BD_NO_GIT_OPS=%q BEADS_NO_AUTO_IMPORT=%q", fields[9], fields[10], fields[11], fields[12], fields[13], fields[14], fields[15])
	}
}

func TestExportJSONLIncludesExportOutputOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	binDir := t.TempDir()
	bdScript := `#!/bin/sh
printf 'export failed loudly\n' >&2
exit 1
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := ExportJSONL(townRoot, "")
	if err == nil {
		t.Fatal("ExportJSONL() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "export failed loudly") {
		t.Fatalf("error %q did not include export output", err)
	}
}

func TestExportJSONLTimesOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	binDir := t.TempDir()
	bdScript := `#!/bin/sh
sleep 2
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GT_BD_TIMEOUT_SEC", "1")

	err := ExportJSONL(townRoot, "")
	if err == nil {
		t.Fatal("ExportJSONL() succeeded, want timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error %q did not report timeout", err)
	}
}
