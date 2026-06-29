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
printf '%s|%s|%s|%s|%s|%s\n' "$*" "$(pwd)" "${BEADS_DIR:-}" "${BD_READONLY:-}" "${BD_EXPORT_AUTO:-}" "${BD_NO_GIT_OPS:-}" > "` + logPath + `"
printf 'summary should be captured\n' >&2
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BEADS_DIR", filepath.Join(townRoot, "wrong", ".beads"))
	t.Setenv("BD_EXPORT_AUTO", "true")
	t.Setenv("BD_NO_GIT_OPS", "false")

	if err := ExportJSONL(townRoot, ""); err != nil {
		t.Fatalf("ExportJSONL() error: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	fields := strings.Split(strings.TrimSpace(string(data)), "|")
	if len(fields) != 6 {
		t.Fatalf("log fields = %v, want 6", fields)
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
	if fields[3] != "true" {
		t.Fatalf("BD_READONLY = %q, want true", fields[3])
	}
	if fields[4] != "false" || fields[5] != "true" {
		t.Fatalf("side-effect env not suppressed: BD_EXPORT_AUTO=%q BD_NO_GIT_OPS=%q", fields[4], fields[5])
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
