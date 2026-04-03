package rig

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
)

// ShowIdentityBead loads a rig identity bead while preserving the town-level
// routing context when routes.jsonl already knows where that prefix lives.
// This avoids re-entering a nested rig workspace and trusting stale local
// metadata after the town has an authoritative route for the rig.
func ShowIdentityBead(townRoot, rigName string) (*beads.Issue, error) {
	rigPath := filepath.Join(townRoot, rigName)

	var prefix string
	if rigCfg, err := LoadRigConfig(rigPath); err == nil && rigCfg.Beads != nil {
		prefix = rigCfg.Beads.Prefix
	} else {
		prefix = config.GetRigPrefix(townRoot, rigName)
	}
	if prefix == "" {
		return nil, fmt.Errorf("rig %s has no beads prefix", rigName)
	}

	rigBeadID := beads.RigBeadIDWithPrefix(prefix, rigName)
	townBeadsDir := beads.ResolveBeadsDir(townRoot)
	if beads.GetRigPathForPrefix(townRoot, beads.ExtractPrefix(rigBeadID)) != "" {
		return showIdentityBeadInContext(townRoot, townBeadsDir, rigBeadID)
	}

	rigBeadsDir := beads.ResolveBeadsDir(rigPath)
	return showIdentityBeadInContext(rigPath, rigBeadsDir, rigBeadID)
}

func showIdentityBeadInContext(workDir, beadsDir, rigBeadID string) (*beads.Issue, error) {
	bd := beads.NewWithBeadsDir(workDir, beadsDir)
	out, err := bd.Run("show", rigBeadID, "--json")
	if err != nil {
		return nil, err
	}

	var issues []*beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd show output: %w", err)
	}
	if len(issues) == 0 {
		return nil, beads.ErrNotFound
	}

	return issues[0], nil
}
