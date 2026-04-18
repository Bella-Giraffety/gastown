package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/workspace"
)

// findMailWorkDir returns the town root for all mail operations.
//
// Two-level beads architecture:
// - Town beads (~/gt/.beads/): ALL mail and coordination
// - Clone beads (<rig>/crew/*/.beads/): Project issues only
//
// Mail ALWAYS uses town beads, regardless of sender or recipient address.
// This ensures messages are visible to all agents in the town.
//
// GT_TOWN_ROOT is preferred over workspace detection because workspace.Find
// stops at the first mayor/town.json when not in a worktree path. Rigs that
// have their own mayor/town.json (e.g., gastown/) would be misidentified as
// the town root when running from the rig directory.
func findMailWorkDir() (string, error) {
	for _, envName := range []string{"GT_TOWN_ROOT", "GT_ROOT"} {
		if townRoot := os.Getenv(envName); townRoot != "" {
			if ok, _ := workspace.IsWorkspace(townRoot); ok {
				return townRoot, nil
			}
		}
	}
	return workspace.FindFromCwdOrError()
}

// findLocalBeadsDir finds the nearest .beads directory by walking up from CWD.
// Used for project work (molecules, issue creation) that uses clone beads.
//
// Priority:
//  1. BEADS_DIR environment variable (set by session manager for polecats)
//  2. Walk up from CWD looking for .beads directory
//
// Polecats use redirect-based beads access, so their worktree doesn't have a full
// .beads directory. The session manager sets BEADS_DIR to the correct location.
func findLocalBeadsDir() (string, error) {
	// Check BEADS_DIR environment variable first (set by session manager for polecats).
	// This is important for polecats that use redirect-based beads access.
	if beadsDir := os.Getenv("BEADS_DIR"); beadsDir != "" {
		// BEADS_DIR points directly to the .beads directory, return its parent
		if _, err := os.Stat(beadsDir); err == nil {
			return filepath.Dir(beadsDir), nil
		}
	}

	// Fallback: walk up from CWD
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	path := cwd
	for {
		if _, err := os.Stat(filepath.Join(path, ".beads")); err == nil {
			return path, nil
		}

		parent := filepath.Dir(path)
		if parent == path {
			break // Reached root
		}
		path = parent
	}

	return "", fmt.Errorf("no .beads directory found")
}

// detectSender determines the current context's address.
// Priority:
//  1. GT_ROLE env var → use the role-based identity (agent session)
//  2. .gt-agent metadata → use explicit agent identity for supported debug flows
//  3. No match → return "overseer" (human at terminal)
func detectSender() string {
	// Check GT_ROLE first (authoritative for agent sessions)
	role := os.Getenv("GT_ROLE")
	if role != "" {
		// Agent session - build address from role and context
		return detectSenderFromRole(role)
	}

	if fromFile := detectSenderFromCurrentAgentFile(); fromFile != "" {
		return fromFile
	}

	return "overseer"
}

// detectSenderFromRole builds an address from the GT_ROLE and related env vars.
// GT_ROLE can be either a simple role name ("crew", "polecat") or a full address
// ("greenplace/crew/joe") depending on how the session was started.
//
// If GT_ROLE is a simple name but required env vars (GT_RIG, GT_POLECAT, etc.)
// are missing, fall back only to explicit .gt-agent metadata.
func detectSenderFromRole(role string) string {
	rig := os.Getenv("GT_RIG")

	// Check if role is already a full address (contains /)
	if strings.Contains(role, "/") {
		// GT_ROLE is already a full address, use it directly
		return role
	}

	// GT_ROLE is a simple role name, build the full address
	switch role {
	case constants.RoleMayor:
		return "mayor/"
	case constants.RoleDeacon:
		return "deacon/"
	case constants.RolePolecat:
		polecat := os.Getenv("GT_POLECAT")
		if rig != "" && polecat != "" {
			return fmt.Sprintf("%s/%s", rig, polecat)
		}
		return detectSenderFromCurrentAgentFileOrOverseer()
	case constants.RoleCrew:
		crew := os.Getenv("GT_CREW")
		if rig != "" && crew != "" {
			return fmt.Sprintf("%s/crew/%s", rig, crew)
		}
		return detectSenderFromCurrentAgentFileOrOverseer()
	case constants.RoleWitness:
		if rig != "" {
			return fmt.Sprintf("%s/witness", rig)
		}
		return detectSenderFromCurrentAgentFileOrOverseer()
	case constants.RoleRefinery:
		if rig != "" {
			return fmt.Sprintf("%s/refinery", rig)
		}
		return detectSenderFromCurrentAgentFileOrOverseer()
	case "dog":
		dogName := os.Getenv("GT_DOG_NAME")
		if dogName != "" {
			return fmt.Sprintf("deacon/dogs/%s", dogName)
		}
		return detectSenderFromCurrentAgentFileOrOverseer()
	default:
		return detectSenderFromCurrentAgentFileOrOverseer()
	}
}

func detectSenderFromCurrentAgentFile() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	return detectSenderFromAgentFile(cwd)
}

func detectSenderFromCurrentAgentFileOrOverseer() string {
	if fromFile := detectSenderFromCurrentAgentFile(); fromFile != "" {
		return fromFile
	}
	return "overseer"
}

type agentIdentityFile struct {
	Role string `json:"role"`
	Rig  string `json:"rig"`
	Name string `json:"name"`
}

func detectSenderFromAgentFile(startDir string) string {
	path := startDir
	for {
		agentPath := filepath.Join(path, ".gt-agent")
		data, err := os.ReadFile(agentPath)
		if err == nil {
			var parsed agentIdentityFile
			if json.Unmarshal(data, &parsed) == nil {
				if id := identityFromAgentFile(parsed); id != "" {
					return id
				}
			}
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return ""
}

func identityFromAgentFile(parsed agentIdentityFile) string {
	role := strings.TrimSpace(strings.ToLower(parsed.Role))
	rig := strings.TrimSpace(parsed.Rig)
	name := strings.TrimSpace(parsed.Name)

	switch role {
	case constants.RoleMayor:
		return "mayor/"
	case constants.RoleDeacon:
		return "deacon/"
	case constants.RoleWitness:
		if rig != "" {
			return fmt.Sprintf("%s/witness", rig)
		}
	case constants.RoleRefinery:
		if rig != "" {
			return fmt.Sprintf("%s/refinery", rig)
		}
	case constants.RoleCrew:
		if rig != "" && name != "" {
			return fmt.Sprintf("%s/crew/%s", rig, name)
		}
	case constants.RolePolecat:
		if rig != "" && name != "" {
			return fmt.Sprintf("%s/polecats/%s", rig, name)
		}
	case "dog":
		if name != "" {
			return fmt.Sprintf("deacon/dogs/%s", name)
		}
	}

	return ""
}
