package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHealthDatabaseNamesUsesWorkspaceDatabases(t *testing.T) {
	townRoot := t.TempDir()
	dataDir := filepath.Join(townRoot, ".dolt-data")

	for _, name := range []string{"gastown", "hq", "beads"} {
		if err := os.MkdirAll(filepath.Join(dataDir, name, ".dolt", "noms"), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		manifest := filepath.Join(dataDir, name, ".dolt", "noms", "manifest")
		if err := os.WriteFile(manifest, []byte("stub"), 0644); err != nil {
			t.Fatalf("write manifest for %s: %v", name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dataDir, ".internal"), 0755); err != nil {
		t.Fatalf("mkdir hidden dir: %v", err)
	}

	got := healthDatabaseNames(townRoot)
	want := []string{"beads", "gastown", "hq"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("healthDatabaseNames() = %v, want %v", got, want)
	}
}

func TestHealthDatabaseNamesMissingDataDir(t *testing.T) {
	if got := healthDatabaseNames(t.TempDir()); got != nil {
		t.Fatalf("healthDatabaseNames() = %v, want nil", got)
	}
}
