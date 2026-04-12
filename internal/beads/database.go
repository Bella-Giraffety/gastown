package beads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// DatabaseNameFromMetadata reads the dolt_database field from .beads/metadata.json.
// Returns empty string if metadata doesn't exist or has no database configured.
func DatabaseNameFromMetadata(beadsDir string) string {
	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		return ""
	}
	var meta struct {
		DoltDatabase string `json:"dolt_database"`
	}
	if json.Unmarshal(data, &meta) != nil {
		return ""
	}
	return meta.DoltDatabase
}

// DatabaseEnv returns the BEADS_DOLT_SERVER_DATABASE=<name> env var string
// for the given beadsDir, or empty string if no database is configured.
func DatabaseEnv(beadsDir string) string {
	db := DatabaseNameFromMetadata(beadsDir)
	if db == "" {
		return ""
	}
	return "BEADS_DOLT_SERVER_DATABASE=" + db
}

// BoundEnv returns env with beads routing pinned to beadsDir.
//
// It removes inherited BEADS_DIR, BEADS_DB, and
// BEADS_DOLT_SERVER_DATABASE entries before appending the target values so a
// parent process cannot shadow the intended database.
func BoundEnv(env []string, beadsDir string) []string {
	filtered := make([]string, 0, len(env)+2)
	for _, entry := range env {
		if strings.HasPrefix(entry, "BEADS_DIR=") ||
			strings.HasPrefix(entry, "BEADS_DB=") ||
			strings.HasPrefix(entry, "BEADS_DOLT_SERVER_DATABASE=") {
			continue
		}
		filtered = append(filtered, entry)
	}
	if beadsDir == "" {
		return filtered
	}
	filtered = append(filtered, "BEADS_DIR="+beadsDir)
	if dbEnv := DatabaseEnv(beadsDir); dbEnv != "" {
		filtered = append(filtered, dbEnv)
	}
	return filtered
}
