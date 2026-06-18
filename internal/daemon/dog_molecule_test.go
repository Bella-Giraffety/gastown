package daemon

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseWispID(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantID string
	}{
		{
			name:   "standard wisp output",
			input:  "✓ Spawned wisp: gt-wisp-abc123 — Reap stale wisps",
			wantID: "gt-wisp-abc123",
		},
		{
			name:   "wisp ID with ANSI codes",
			input:  "\033[32m✓\033[0m Spawned wisp: \033[1mgt-wisp-xyz789\033[0m — Title",
			wantID: "gt-wisp-xyz789",
		},
		{
			name:   "empty output",
			input:  "",
			wantID: "",
		},
		{
			name:   "no wisp ID in output",
			input:  "Error: something went wrong",
			wantID: "",
		},
		{
			name:   "wisp ID at end of line",
			input:  "Created gt-wisp-def456",
			wantID: "gt-wisp-def456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWispID(tt.input)
			if got != tt.wantID {
				t.Errorf("parseWispID(%q) = %q, want %q", tt.input, got, tt.wantID)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no ANSI", "hello", "hello"},
		{"color code", "\033[32mgreen\033[0m", "green"},
		{"bold", "\033[1mbold\033[0m", "bold"},
		{"multiple codes", "\033[32m✓\033[0m \033[1mtext\033[0m", "✓ text"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI(tt.input)
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// newTestDogMol returns a dogMol with a discarding logger for unit tests.
func newTestDogMol() *dogMol {
	return &dogMol{
		stepIDs: make(map[string]string),
		logger:  log.New(os.Stderr, "", 0),
	}
}

func TestParsePour(t *testing.T) {
	t.Run("captures root and step IDs from bd mol wisp --json", func(t *testing.T) {
		// Exactly the shape `bd mol wisp mol-dog-doctor --json` emits.
		raw := `{
			"created": 4,
			"id_mapping": {
				"mol-dog-doctor": "hq-wisp-root1",
				"mol-dog-doctor.probe": "hq-wisp-probe1",
				"mol-dog-doctor.inspect": "hq-wisp-inspect1",
				"mol-dog-doctor.report": "hq-wisp-report1"
			},
			"new_epic_id": "hq-wisp-root1",
			"phase": "vapor",
			"schema_version": 1
		}`
		dm := newTestDogMol()
		dm.parsePour(raw, "mol-dog-doctor")

		if dm.rootID != "hq-wisp-root1" {
			t.Fatalf("rootID = %q, want hq-wisp-root1", dm.rootID)
		}
		want := map[string]string{
			"probe":   "hq-wisp-probe1",
			"inspect": "hq-wisp-inspect1",
			"report":  "hq-wisp-report1",
		}
		if len(dm.stepIDs) != len(want) {
			t.Fatalf("stepIDs = %v, want %v", dm.stepIDs, want)
		}
		for slug, id := range want {
			if dm.stepIDs[slug] != id {
				t.Errorf("stepIDs[%q] = %q, want %q", slug, dm.stepIDs[slug], id)
			}
		}
		// The root must not be recorded as a step.
		if _, ok := dm.stepIDs["mol-dog-doctor"]; ok {
			t.Error("root formula key must not be recorded as a step")
		}
	})

	t.Run("falls back to text root ID on non-JSON output", func(t *testing.T) {
		dm := newTestDogMol()
		dm.parsePour("✓ Created wisp: hq-wisp-fallback — mol-dog-doctor", "mol-dog-doctor")
		if dm.rootID != "hq-wisp-fallback" {
			t.Fatalf("rootID = %q, want hq-wisp-fallback (text fallback)", dm.rootID)
		}
		if len(dm.stepIDs) != 0 {
			t.Errorf("stepIDs should be empty on fallback, got %v", dm.stepIDs)
		}
	})

	t.Run("uses id_mapping root when new_epic_id is absent", func(t *testing.T) {
		raw := `{"id_mapping":{"mol-dog-reaper":"hq-wisp-r","mol-dog-reaper.scan":"hq-wisp-s"}}`
		dm := newTestDogMol()
		dm.parsePour(raw, "mol-dog-reaper")
		if dm.rootID != "hq-wisp-r" {
			t.Fatalf("rootID = %q, want hq-wisp-r", dm.rootID)
		}
		if dm.stepIDs["scan"] != "hq-wisp-s" {
			t.Errorf("stepIDs[scan] = %q, want hq-wisp-s", dm.stepIDs["scan"])
		}
	})

	t.Run("uses alternate root fields", func(t *testing.T) {
		for _, field := range []string{"root_id", "result_id"} {
			t.Run(field, func(t *testing.T) {
				raw := `{"` + field + `":"hq-wisp-root","id_mapping":{"mol-dog-json.scan":"hq-wisp-scan"}}`
				dm := newTestDogMol()
				dm.parsePour(raw, "mol-dog-json")
				if dm.rootID != "hq-wisp-root" {
					t.Fatalf("rootID = %q, want hq-wisp-root", dm.rootID)
				}
				if dm.stepIDs["scan"] != "hq-wisp-scan" {
					t.Errorf("stepIDs[scan] = %q, want hq-wisp-scan", dm.stepIDs["scan"])
				}
			})
		}
	})

	t.Run("prefers mapped root over mismatched fallback", func(t *testing.T) {
		raw := `{"new_epic_id":"hq-wisp-root-a","id_mapping":{"mol-dog-json":"hq-wisp-root-b","mol-dog-json.scan":"hq-wisp-scan"}}`
		dm := newTestDogMol()
		dm.parsePour(raw, "mol-dog-json")
		if dm.rootID != "hq-wisp-root-b" {
			t.Fatalf("rootID = %q, want mapped hq-wisp-root-b", dm.rootID)
		}
		if dm.stepIDs["scan"] != "hq-wisp-scan" {
			t.Fatalf("stepIDs[scan] = %q, want hq-wisp-scan", dm.stepIDs["scan"])
		}
	})

	t.Run("captures steps even when root is missing", func(t *testing.T) {
		raw := `{"id_mapping":{"mol-dog-json.scan":"hq-wisp-scan"}}`
		dm := newTestDogMol()
		dm.parsePour(raw, "mol-dog-json")
		if dm.rootID != "" {
			t.Fatalf("rootID = %q, want empty", dm.rootID)
		}
		if dm.stepIDs["scan"] != "hq-wisp-scan" {
			t.Fatalf("stepIDs[scan] = %q, want hq-wisp-scan", dm.stepIDs["scan"])
		}
	})
}

func TestPourDogMoleculeUsesJSONMappingNoChildrenRead(t *testing.T) {
	raw := `{
		"id_mapping": {
			"mol-dog-doctor": "hq-wisp-root1",
			"mol-dog-doctor.probe": "hq-wisp-probe1",
			"mol-dog-doctor.inspect": "hq-wisp-inspect1"
		},
		"new_epic_id": "hq-wisp-root1"
	}`
	bdPath, logPath := writeDogMolFakeBD(t, raw)
	d := &Daemon{
		config: &Config{TownRoot: t.TempDir()},
		bdPath: bdPath,
		logger: log.New(os.Stderr, "", 0),
	}

	dm := d.pourDogMolecule("mol-dog-doctor", map[string]string{"run": "abc"})
	if dm.rootID != "hq-wisp-root1" {
		t.Fatalf("rootID = %q, want hq-wisp-root1", dm.rootID)
	}
	if dm.stepIDs["probe"] != "hq-wisp-probe1" || dm.stepIDs["inspect"] != "hq-wisp-inspect1" {
		t.Fatalf("stepIDs = %v, want captured probe and inspect", dm.stepIDs)
	}

	log := readDogMolBDLog(t, logPath)
	if strings.Contains(log, "show ") || strings.Contains(log, " --children") {
		t.Fatalf("pour called children discovery unexpectedly; log:\n%s", log)
	}
	if !strings.Contains(log, "mol wisp mol-dog-doctor --json") {
		t.Fatalf("pour did not call bd mol wisp --json; log:\n%s", log)
	}
}

func TestDogMolCloseForceClosesCapturedStepsAndRoot(t *testing.T) {
	bdPath, logPath := writeDogMolFakeBD(t, `{}`)
	dm := &dogMol{
		rootID: "hq-wisp-root1",
		stepIDs: map[string]string{
			"probe":   "hq-wisp-probe1",
			"inspect": "hq-wisp-inspect1",
		},
		bdPath:   bdPath,
		townRoot: t.TempDir(),
		logger:   log.New(os.Stderr, "", 0),
	}

	dm.close()
	log := readDogMolBDLog(t, logPath)
	for _, want := range []string{
		"close hq-wisp-probe1 --force",
		"close hq-wisp-inspect1 --force",
		"close hq-wisp-root1 --force",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("missing %q in fake bd log:\n%s", want, log)
		}
	}
	if strings.Contains(log, "show ") || strings.Contains(log, " --children") {
		t.Fatalf("close called children discovery unexpectedly; log:\n%s", log)
	}
}

func TestDogMolCloseStepDoesNotRecloseStep(t *testing.T) {
	bdPath, logPath := writeDogMolFakeBD(t, `{}`)
	dm := &dogMol{
		rootID:  "hq-wisp-root1",
		stepIDs: map[string]string{"probe": "hq-wisp-probe1"},
		bdPath:  bdPath,
		logger:  log.New(os.Stderr, "", 0),
	}

	dm.closeStep("probe")
	dm.close()
	log := readDogMolBDLog(t, logPath)
	if got := strings.Count(log, "close hq-wisp-probe1"); got != 1 {
		t.Fatalf("close hq-wisp-probe1 called %d times, want 1; log:\n%s", got, log)
	}
	if !strings.Contains(log, "close hq-wisp-root1 --force") {
		t.Fatalf("root was not force-closed; log:\n%s", log)
	}
}

func TestDogMolCloseClosesCapturedStepsWithoutRoot(t *testing.T) {
	bdPath, logPath := writeDogMolFakeBD(t, `{}`)
	dm := &dogMol{
		stepIDs: map[string]string{"probe": "hq-wisp-probe1"},
		bdPath:  bdPath,
		logger:  log.New(os.Stderr, "", 0),
	}

	dm.close()
	log := readDogMolBDLog(t, logPath)
	if !strings.Contains(log, "close hq-wisp-probe1 --force") {
		t.Fatalf("captured step was not force-closed without root; log:\n%s", log)
	}
	if strings.Contains(log, "close  --force") {
		t.Fatalf("empty root was closed unexpectedly; log:\n%s", log)
	}
	if strings.Contains(log, "show ") || strings.Contains(log, " --children") {
		t.Fatalf("close called children discovery unexpectedly; log:\n%s", log)
	}
}

func TestDogMolGracefulDegradation(t *testing.T) {
	// A dogMol with empty rootID should be a no-op for all operations.
	dm := &dogMol{
		rootID:  "",
		stepIDs: make(map[string]string),
	}

	// These should not panic or error — graceful degradation.
	dm.closeStep("scan")
	dm.failStep("scan", "test failure")
	dm.close()
}

func writeDogMolFakeBD(t *testing.T, output string) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake bd shell script requires bash")
	}
	dir := t.TempDir()
	bdPath := filepath.Join(dir, "bd")
	logPath := filepath.Join(dir, "bd.log")
	outputPath := filepath.Join(dir, "bd-output.json")
	if err := os.WriteFile(outputPath, []byte(output), 0o644); err != nil {
		t.Fatalf("write fake bd output: %v", err)
	}
	script := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$DOG_MOL_BD_LOG"
case "${1:-}" in
  show)
    echo "unexpected children read" >&2
    exit 42
    ;;
  mol)
    cat "$DOG_MOL_BD_OUTPUT"
    ;;
esac
`
	if err := os.WriteFile(bdPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake bd: %v", err)
	}
	t.Setenv("DOG_MOL_BD_LOG", logPath)
	t.Setenv("DOG_MOL_BD_OUTPUT", outputPath)
	return bdPath, logPath
}

func readDogMolBDLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fake bd log: %v", err)
	}
	return string(data)
}
