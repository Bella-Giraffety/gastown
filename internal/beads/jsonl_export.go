package beads

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ExportJSONL writes the resolved beads database to issues.jsonl without
// re-enabling Beads' automatic backup, git, or push side effects.
func ExportJSONL(dir, fallbackBeadsDir string) error {
	beadsDir := ""
	if fallbackBeadsDir != "" {
		beadsDir = ResolveBeadsDir(fallbackBeadsDir)
	}
	if beadsDir == "" && dir != "" {
		beadsDir = ResolveBeadsDir(dir)
	}
	if beadsDir == "" {
		return fmt.Errorf("could not resolve .beads directory")
	}

	workDir := dir
	if workDir == "" {
		workDir = beadsDir
	}
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	cmd := Command(workDir, beadsDir, ReadOnlyPinned, "export", "-o", issuesPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("exporting beads JSONL to %s: %w: %s", issuesPath, err, msg)
		}
		return fmt.Errorf("exporting beads JSONL to %s: %w", issuesPath, err)
	}
	return nil
}
