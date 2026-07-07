package cmd

import (
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/formula"
)

func loadFormulaByName(name, townRoot, rigName string) (*formula.Formula, error) {
	content, err := formula.ResolveFormulaContentFromDirs(name, formulaContentSearchDirs(townRoot, rigName))
	if err != nil {
		return nil, err
	}
	return formula.Parse(content)
}

func formulaContentSearchDirs(townRoot, rigName string) []string {
	var dirs []string

	if townRoot != "" && rigName != "" {
		if rigDir := beads.GetRigDirForName(townRoot, rigName); rigDir != "" {
			dirs = appendFormulaDir(dirs, filepath.Join(beads.ResolveBeadsDir(rigDir), "formulas"))
		} else {
			dirs = appendFormulaDir(dirs, filepath.Join(beads.ResolveBeadsDir(filepath.Join(townRoot, rigName)), "formulas"))
		}
	}

	if townRoot != "" {
		dirs = appendFormulaDir(dirs, filepath.Join(beads.ResolveBeadsDir(townRoot), "formulas"))
	}

	// Preserve the legacy user formula tier for commands that already accepted it.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = appendFormulaDir(dirs, filepath.Join(home, ".beads", "formulas"))
	}

	return dirs
}

func appendFormulaDir(dirs []string, dir string) []string {
	if dir == "" {
		return dirs
	}
	for _, existing := range dirs {
		if existing == dir {
			return dirs
		}
	}
	return append(dirs, dir)
}
