package cmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/cli"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/style"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// PatrolConfig holds role-specific patrol configuration.
type PatrolConfig struct {
	RoleName      string       // "deacon", "witness", "refinery"
	PatrolMolName string       // "mol-deacon-patrol", etc.
	BeadsDir      string       // where to look for beads
	Assignee      string       // agent identity for pinning
	HeaderEmoji   string       // display emoji
	HeaderTitle   string       // "Patrol Status", etc.
	WorkLoopSteps []string     // role-specific instructions
	ExtraVars     []string     // additional --var key=value args for wisp creation
	Beads         *beads.Beads // optional injected beads instance (for test isolation)
}

func buildPatrolConfig(roleInfo RoleContext, role Role) (PatrolConfig, error) {
	switch role {
	case RoleDeacon:
		return PatrolConfig{
			RoleName:      "deacon",
			PatrolMolName: constants.MolDeaconPatrol,
			BeadsDir:      roleInfo.TownRoot,
			Assignee:      getAgentAssigneeIdentity(RoleContext{Role: RoleDeacon}),
		}, nil
	case RoleWitness:
		return PatrolConfig{
			RoleName:      "witness",
			PatrolMolName: constants.MolWitnessPatrol,
			BeadsDir:      roleInfo.TownRoot,
			Assignee:      roleInfo.Rig + "/witness",
		}, nil
	case RoleRefinery:
		return PatrolConfig{
			RoleName:      "refinery",
			PatrolMolName: constants.MolRefineryPatrol,
			BeadsDir:      roleInfo.TownRoot,
			Assignee:      roleInfo.Rig + "/refinery",
			ExtraVars:     buildRefineryPatrolVars(roleInfo),
		}, nil
	default:
		return PatrolConfig{}, fmt.Errorf("unsupported role for patrol: %q", role)
	}
}

// findActivePatrol finds an active patrol molecule for the role.
// Returns the patrol ID, display line, and whether one was found.
// Returns an error if discovery fails (e.g. transient bd failure),
// so callers can distinguish "no patrol" from "discovery failed"
// and avoid auto-spawning duplicates.
//
// Patrol molecules are intentionally hooked to the agent (hooked status).
// A hooked patrol root is reportable until gt patrol report closes it; child
// state is not the authority because completed children are exactly what report
// needs to consume.
func findActivePatrol(cfg PatrolConfig) (patrolID, patrolLine string, found bool, err error) {
	b := cfg.Beads
	if b == nil {
		b = beads.New(cfg.BeadsDir)
	}

	// Find hooked patrol beads for this agent
	hookedBeads, listErr := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: cfg.Assignee,
		Priority: -1,
	})
	if listErr != nil {
		return "", "", false, fmt.Errorf("listing hooked beads: %w", listErr)
	}

	// Identify the newest reportable patrol. Discovery is intentionally read-only;
	// replacement cleanup runs only after report closes the selected patrol.
	var activeBead *beads.Issue
	for _, bead := range hookedBeads {
		if !isPatrolWispTitle(bead.Title, cfg.PatrolMolName) {
			continue
		}
		if activeBead == nil || bead.CreatedAt > activeBead.CreatedAt || (bead.CreatedAt == activeBead.CreatedAt && bead.ID > activeBead.ID) {
			activeBead = bead
		}
	}

	if activeBead != nil {
		return activeBead.ID, formatBeadLine(activeBead), true, nil
	}
	return "", "", false, nil
}

func isPatrolWispTitle(title, molName string) bool {
	return title == molName || title == molName+" (wisp)"
}

// formatBeadLine formats a bead issue into a display line similar to bd list output.
func formatBeadLine(issue *beads.Issue) string {
	return fmt.Sprintf("%s  %s [%s]", issue.ID, issue.Title, issue.Status)
}

// burnPreviousPatrolWisps finds and burns all existing patrol wisps for a role.
// This prevents orphaned root wisp accumulation when a new patrol cycle starts
// without the previous one being properly closed (gt-92jh).
// Errors are logged as warnings but don't block new patrol creation.
func burnPreviousPatrolWisps(cfg PatrolConfig) {
	b := cfg.Beads
	if b == nil {
		b = beads.New(cfg.BeadsDir)
	}

	var burned int
	for _, assignee := range patrolCleanupAssignees(cfg) {
		// Find all hooked patrol beads for this agent identity.
		hookedBeads, err := b.List(beads.ListOptions{
			Status:   beads.StatusHooked,
			Assignee: assignee,
			Priority: -1,
		})
		if err != nil {
			style.PrintWarning("burn: could not list hooked beads for %s: %v", assignee, err)
			continue
		}

		for _, bead := range hookedBeads {
			if !isPatrolWispTitle(bead.Title, cfg.PatrolMolName) {
				continue
			}

			// Close all descendant wisps, then the root.
			closeDescendants(b, bead.ID)
			if err := b.ForceCloseWithReason("burned: replaced by new patrol cycle", bead.ID); err != nil {
				style.PrintWarning("burn: could not close patrol %s: %v", bead.ID, err)
				continue
			}
			burned++
		}
	}

	if burned > 0 {
		fmt.Printf("%s Burned %d previous patrol wisp(s)\n", style.Dim.Render("🔥"), burned)
	}
}

func patrolCleanupAssignees(cfg PatrolConfig) []string {
	assignees := []string{cfg.Assignee}
	if cfg.RoleName == "deacon" && cfg.Assignee == "deacon/" {
		assignees = append(assignees, "deacon")
	}
	return assignees
}

// autoSpawnPatrol creates and pins a new patrol wisp.
// Before creating, it burns any existing patrol wisps for this role to prevent
// orphaned root wisp accumulation (gt-92jh). This makes the function
// self-cleaning regardless of the caller.
// Returns the patrol ID or an error.
func autoSpawnPatrol(cfg PatrolConfig) (string, error) {
	// Resolve the beads directory following redirects.
	// This ensures bd targets the correct database (e.g., rig database
	// instead of HQ) regardless of inherited BEADS_DIR. See gt-ctir.
	resolvedBeadsDir := beads.ResolveBeadsDir(cfg.BeadsDir)

	// Burn any existing patrol wisps for this role before creating a new one.
	// Without this, each patrol cycle leaks a root wisp into the DB, producing
	// ~500-700 orphans/day across all patrol formulas (gt-92jh).
	burnPreviousPatrolWisps(cfg)

	// Find the proto ID for the patrol molecule
	cmdCatalog := exec.Command("gt", "formula", "list")
	cmdCatalog.Dir = cfg.BeadsDir
	var stdoutCatalog, stderrCatalog bytes.Buffer
	cmdCatalog.Stdout = &stdoutCatalog
	cmdCatalog.Stderr = &stderrCatalog

	if err := cmdCatalog.Run(); err != nil {
		errMsg := strings.TrimSpace(stderrCatalog.String())
		if errMsg != "" {
			return "", fmt.Errorf("failed to list formulas: %s", errMsg)
		}
		return "", fmt.Errorf("failed to list formulas: %w", err)
	}

	// Find patrol molecule in formula list
	// Format: "formula-name         description"
	var protoID string
	catalogLines := strings.Split(stdoutCatalog.String(), "\n")
	for _, line := range catalogLines {
		if strings.Contains(line, cfg.PatrolMolName) {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				protoID = parts[0]
				break
			}
		}
	}

	if protoID == "" {
		return "", fmt.Errorf("proto %s not found in catalog", cfg.PatrolMolName)
	}

	// Create the patrol wisp (root only — steps are read inline at prime time,
	// not tracked as individual DB rows). Child wisps are reserved for pour=true
	// formulas like releases where checkpoint recovery matters.
	spawnArgs := []string{"mol", "wisp", "create", protoID, "--root-only", "--actor", cfg.RoleName}
	for _, v := range cfg.ExtraVars {
		spawnArgs = append(spawnArgs, "--var", v)
	}
	cmdSpawn := BdCmd(spawnArgs...).
		WithAutoCommit().
		WithBeadsDir(resolvedBeadsDir).
		Dir(cfg.BeadsDir).
		Build()
	var stdoutSpawn, stderrSpawn bytes.Buffer
	cmdSpawn.Stdout = &stdoutSpawn
	cmdSpawn.Stderr = &stderrSpawn

	if err := cmdSpawn.Run(); err != nil {
		return "", fmt.Errorf("failed to create patrol wisp: %s", stderrSpawn.String())
	}

	// Parse the created molecule ID from output
	// Format: "Root issue: <rig>-wisp-<hash>" where rig prefix varies
	var patrolID string
	spawnOutput := stdoutSpawn.String()
	for _, line := range strings.Split(spawnOutput, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Root issue:") {
			patrolID = strings.TrimSpace(strings.TrimPrefix(line, "Root issue:"))
			break
		}
	}
	// Fallback: look for any token containing "-wisp-"
	if patrolID == "" {
		for _, line := range strings.Split(spawnOutput, "\n") {
			for _, p := range strings.Fields(line) {
				if strings.Contains(p, "-wisp-") {
					patrolID = p
					break
				}
			}
			if patrolID != "" {
				break
			}
		}
	}

	if patrolID == "" {
		return "", fmt.Errorf("created wisp but could not parse ID from output")
	}

	// Hook the wisp to the agent so gt mol status sees it
	if err := BdCmd("update", patrolID, "--status=hooked", "--assignee="+cfg.Assignee).
		WithAutoCommit().
		WithBeadsDir(resolvedBeadsDir).
		Dir(cfg.BeadsDir).
		Run(); err != nil {
		return patrolID, fmt.Errorf("created wisp %s but failed to hook", patrolID)
	}

	return patrolID, nil
}

// outputPatrolContext is the main function that handles patrol display logic.
// It finds or creates a patrol and outputs the status and work loop.
func outputPatrolContext(cfg PatrolConfig) {
	fmt.Println()
	fmt.Printf("%s\n\n", style.Bold.Render(fmt.Sprintf("## %s %s", cfg.HeaderEmoji, cfg.HeaderTitle)))

	// Try to find an active patrol
	patrolID, patrolLine, hasPatrol, findErr := findActivePatrol(cfg)

	if findErr != nil {
		// Discovery failed — do NOT auto-spawn to avoid creating duplicates
		style.PrintWarning("patrol discovery failed: %v", findErr)
		fmt.Println("Status: **Discovery failed** — cannot determine patrol state")
		fmt.Println(style.Dim.Render("Check bd connectivity and retry. Not spawning new patrol to avoid duplicates."))
		return
	}

	if !hasPatrol {
		// No active patrol - auto-spawn one
		fmt.Printf("Status: **No active patrol** - creating %s...\n", cfg.PatrolMolName)
		fmt.Println()

		var err error
		patrolID, err = autoSpawnPatrol(cfg)
		if err != nil {
			if patrolID != "" {
				fmt.Printf("⚠ %s\n", err.Error())
			} else {
				fmt.Println(style.Dim.Render(err.Error()))
				fmt.Println(style.Dim.Render("Run `" + cli.Name() + " formula list` to troubleshoot."))
				return
			}
		} else {
			fmt.Printf("✓ Created and hooked patrol wisp: %s\n", patrolID)
		}
	} else {
		// Has active patrol - show status
		fmt.Println("Status: **Patrol Active**")
		fmt.Printf("Patrol: %s\n\n", strings.TrimSpace(patrolLine))
	}

	// Show patrol work loop instructions
	fmt.Printf("**%s Patrol Work Loop:**\n", cases.Title(language.English).String(cfg.RoleName))
	for i, step := range cfg.WorkLoopSteps {
		fmt.Printf("%d. %s\n", i+1, step)
	}

	if patrolID != "" {
		fmt.Println()
		fmt.Printf("Current patrol ID: %s\n", patrolID)
	}
}
