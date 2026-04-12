package beads

import (
	"path/filepath"
	"testing"

	"os"
)

func TestBoundEnv_BindsBeadsDirectoryAndDatabase(t *testing.T) {
	beadsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(`{"backend":"dolt","database":"dolt","dolt_database":"rig_db"}`), 0644); err != nil {
		t.Fatal(err)
	}

	env := BoundEnv([]string{
		"PATH=/usr/bin",
		"BEADS_DIR=/wrong/.beads",
		"BEADS_DB=wrong",
		"BEADS_DOLT_SERVER_DATABASE=wrongdb",
	}, beadsDir)

	envMap := make(map[string]string, len(env))
	for _, entry := range env {
		for i := 0; i < len(entry); i++ {
			if entry[i] == '=' {
				envMap[entry[:i]] = entry[i+1:]
				break
			}
		}
	}

	if got := envMap["BEADS_DIR"]; got != beadsDir {
		t.Fatalf("BEADS_DIR = %q, want %q", got, beadsDir)
	}
	if got := envMap["BEADS_DOLT_SERVER_DATABASE"]; got != "rig_db" {
		t.Fatalf("BEADS_DOLT_SERVER_DATABASE = %q, want %q", got, "rig_db")
	}
	if got := envMap["BEADS_DB"]; got != "" {
		t.Fatalf("BEADS_DB = %q, want empty", got)
	}
}
