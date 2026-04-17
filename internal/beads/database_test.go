package beads

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoundEnvRebindsBeadsContext(t *testing.T) {
	townRoot := t.TempDir()
	beadsDir := filepath.Join(townRoot, "rig", ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", beadsDir, err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(`{"dolt_database":"gastown"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(metadata.json): %v", err)
	}

	env := []string{
		"PATH=/usr/bin",
		"BEADS_DIR=/wrong/.beads",
		"BEADS_DB=/wrong.db",
		"BEADS_DOLT_SERVER_DATABASE=hq",
	}

	got := BoundEnv(env, beadsDir)
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "BEADS_DIR=/wrong/.beads") {
		t.Fatalf("BoundEnv() kept inherited BEADS_DIR: %s", joined)
	}
	if strings.Contains(joined, "BEADS_DB=/wrong.db") {
		t.Fatalf("BoundEnv() kept inherited BEADS_DB: %s", joined)
	}
	if strings.Contains(joined, "BEADS_DOLT_SERVER_DATABASE=hq") {
		t.Fatalf("BoundEnv() kept inherited BEADS_DOLT_SERVER_DATABASE: %s", joined)
	}
	if !strings.Contains(joined, "BEADS_DIR="+beadsDir) {
		t.Fatalf("BoundEnv() missing rebound BEADS_DIR: %s", joined)
	}
	if !strings.Contains(joined, "BEADS_DOLT_SERVER_DATABASE=gastown") {
		t.Fatalf("BoundEnv() missing rebound database env: %s", joined)
	}
}
