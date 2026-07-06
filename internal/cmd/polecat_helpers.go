package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
)

// polecatTarget represents a polecat to operate on.
type polecatTarget struct {
	rigName     string
	polecatName string
	mgr         *polecat.Manager
	r           *rig.Rig
}

// resolvePolecatTargets builds a list of polecats from command args.
// If useAll is true, the first arg is treated as a rig name and all polecats in it are returned.
// Otherwise, args are parsed as rig/polecat addresses.
func resolvePolecatTargets(args []string, useAll bool) ([]polecatTarget, error) {
	var targets []polecatTarget

	if useAll {
		// --all flag: first arg is just the rig name
		rigName := args[0]
		// Check if it looks like rig/polecat format
		if _, _, err := parseAddress(rigName); err == nil {
			return nil, fmt.Errorf("with --all, provide just the rig name (e.g., 'gt polecat <cmd> %s --all')", strings.Split(rigName, "/")[0])
		}

		mgr, r, err := getPolecatManager(rigName)
		if err != nil {
			return nil, err
		}

		polecats, err := mgr.List()
		if err != nil {
			return nil, fmt.Errorf("listing polecats: %w", err)
		}

		for _, p := range polecats {
			targets = append(targets, polecatTarget{
				rigName:     rigName,
				polecatName: p.Name,
				mgr:         mgr,
				r:           r,
			})
		}
	} else {
		// Multiple rig/polecat arguments - require explicit rig/polecat format
		for _, arg := range args {
			// Validate format: must contain "/" to avoid misinterpreting rig names as polecat names
			if !strings.Contains(arg, "/") {
				return nil, fmt.Errorf("invalid address '%s': must be in 'rig/polecat' format (e.g., 'gastown/Toast')", arg)
			}

			rigName, polecatName, err := parseAddress(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid address '%s': %w", arg, err)
			}

			mgr, r, err := getPolecatManager(rigName)
			if err != nil {
				return nil, err
			}

			targets = append(targets, polecatTarget{
				rigName:     rigName,
				polecatName: polecatName,
				mgr:         mgr,
				r:           r,
			})
		}
	}

	return targets, nil
}

// SafetyCheckResult holds the result of safety checks for a polecat.
type SafetyCheckResult struct {
	Polecat       string
	Blocked       bool
	Reasons       []string
	CleanupStatus polecat.CleanupStatus
	HookBead      string
	HookStale     bool // true if hooked bead is closed
	ActiveMR      string
	OpenMR        string
	GitState      *GitState
}

// checkPolecatSafety performs safety checks before destructive operations.
// Returns nil if the polecat is safe to operate on, or a SafetyCheckResult with reasons if blocked.
func checkPolecatSafety(target polecatTarget) *SafetyCheckResult {
	result := &SafetyCheckResult{
		Polecat: fmt.Sprintf("%s/%s", target.rigName, target.polecatName),
	}

	polecatInfo, infoErr := target.mgr.Get(target.polecatName)
	bd := beads.New(target.r.Path)
	agentBeadID := polecatBeadIDForRig(target.r, target.rigName, target.polecatName)
	agentIssue, fields, err := bd.GetAgentBead(agentBeadID)
	currentIssue := ""
	activeMR := ""
	branch := ""
	clonePath := ""
	if infoErr == nil && polecatInfo != nil {
		currentIssue = polecatInfo.Issue
		branch = polecatInfo.Branch
		clonePath = polecatInfo.ClonePath
	}
	if fields != nil {
		activeMR = fields.ActiveMR
	}
	sourceHint := currentIssue
	if fields != nil {
		sourceHint = agentSourceIssueHint(currentIssue, fields)
	}
	targetRefs := recoveryTargetRefs(bd, sourceHint, activeMR, branch)

	var gitState *GitState
	var gitErr error
	gitStateLoaded := false
	loadGitState := func() {
		if gitStateLoaded {
			return
		}
		gitStateLoaded = true
		if infoErr != nil || polecatInfo == nil || clonePath == "" {
			gitErr = fmt.Errorf("polecat info unavailable")
			return
		}
		gitState, gitErr = getGitStateWithTargets(clonePath, targetRefs)
		result.GitState = gitState
	}
	appendGitBlocker := func() {
		loadGitState()
		if blocker := recoveryGitStateBlocker(clonePath, gitState, gitErr); blocker != "" {
			result.Reasons = append(result.Reasons, blocker)
		}
	}

	if err != nil || fields == nil {
		// No agent bead - fall back to direct git state and fail closed on lookup errors.
		appendGitBlocker()
	} else {
		hookBead := agentHookBead(agentIssue, fields)
		activeMRAssessment := polecat.ActiveMRAssessment{}
		if fields.ActiveMR != "" {
			gitSafe := false
			if polecatInfo != nil {
				gitSafe = activeMRGitSafeForWorktree(polecatInfo.ClonePath)
			}
			activeMRAssessment = polecat.AssessActiveMR(bd, polecat.ActiveMRInput{ActiveMR: fields.ActiveMR, SourceIssueHint: sourceHint, RequireGitSafe: true, GitSafe: gitSafe})
		}
		beadTerminal := isAssignedBeadTerminal(bd, sourceHint)
		if activeMRAssessment.SourceTerminal {
			beadTerminal = true
		}

		// Check cleanup_status from agent bead
		result.CleanupStatus = polecat.CleanupStatus(fields.CleanupStatus)
		switch result.CleanupStatus {
		case polecat.CleanupClean:
			// OK
		default:
			if result.CleanupStatus == polecat.CleanupUnpushed {
				loadGitState()
			}
			gitSafe := false
			if polecatInfo != nil {
				gitSafe = activeMRGitSafeForWorktree(polecatInfo.ClonePath)
			}
			hookSafe, hookTerminal, _ := hookBeadSafeForCleanup(bd, hookBead)
			activeMRSafe := !activeMRAssessment.Pending
			if polecat.CanIgnoreStaleCleanupStatus(result.CleanupStatus, beadTerminal || hookTerminal, hookSafe, activeMRSafe, gitSafe) {
				// OK: stale self-report after terminal source and direct clean git.
			} else {
				result.Reasons = append(result.Reasons, cleanupStatusBlocker(result.CleanupStatus))
			}
		}
		if fields.PushFailed {
			result.Reasons = append(result.Reasons, "push_failed=true")
		}
		if fields.MRFailed {
			result.Reasons = append(result.Reasons, "mr_failed=true")
		}

		if hookBead != "" {
			result.HookBead = hookBead
			hookSafe, hookTerminal, blocker := hookBeadSafeForCleanup(bd, hookBead)
			if hookTerminal {
				result.HookStale = true
			} else if !hookSafe {
				result.Reasons = append(result.Reasons, blocker)
			}
		}

		if fields.ActiveMR != "" {
			result.ActiveMR = fields.ActiveMR
			if blocker := activeMRAssessment.Reason; activeMRAssessment.Pending && blocker != "" {
				result.Reasons = append(result.Reasons, blocker)
			}
		}

		appendGitBlocker()
	}

	// Check 2: Open MR beads for this branch
	if branch != "" {
		mr, mrErr := bd.FindMRForBranch(branch)
		if mrErr != nil {
			result.Reasons = append(result.Reasons, fmt.Sprintf("open_mr_lookup_error: %v", mrErr))
		} else if mr != nil {
			result.OpenMR = mr.ID
			result.Reasons = append(result.Reasons, fmt.Sprintf("has open MR (%s)", mr.ID))
		}
	}

	result.Blocked = len(result.Reasons) > 0
	return result
}

func rigPrefix(r *rig.Rig) string {
	townRoot := filepath.Dir(r.Path)
	return beads.GetPrefixForRig(townRoot, r.Name)
}

func polecatBeadIDForRig(r *rig.Rig, rigName, polecatName string) string {
	return beads.PolecatBeadIDWithPrefix(rigPrefix(r), rigName, polecatName)
}

// displaySafetyCheckBlocked prints blocked polecats and guidance.
func displaySafetyCheckBlocked(blocked []*SafetyCheckResult) {
	displaySafetyCheckBlockedTo(os.Stderr, blocked)
}

func displaySafetyCheckBlockedTo(w io.Writer, blocked []*SafetyCheckResult) {
	fmt.Fprintf(w, "%s Cannot nuke the following polecats:\n\n", style.Error.Render("Error:"))
	var polecatList []string
	for _, b := range blocked {
		fmt.Fprintf(w, "  %s:\n", style.Bold.Render(b.Polecat))
		for _, r := range b.Reasons {
			fmt.Fprintf(w, "    - %s\n", r)
		}
		polecatList = append(polecatList, b.Polecat)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Safety decision: REFUSE (non-force). Resolve blockers before nuking, or use --force to bypass them (LOSES WORK).")
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  1. Complete work: gt done (from polecat session)")
	fmt.Fprintln(w, "  2. Push changes: git push (from polecat worktree)")
	fmt.Fprintln(w, "  3. Escalate: gt mail send mayor/ -s \"RECOVERY_NEEDED\" -m \"...\"")
	fmt.Fprintf(w, "  4. Force nuke (LOSES WORK): gt polecat nuke --force %s\n", strings.Join(polecatList, " "))
	fmt.Fprintln(w)
}

func formatSafetyCheckBlockers(blocked []*SafetyCheckResult) string {
	parts := make([]string, 0, len(blocked))
	for _, b := range blocked {
		parts = append(parts, fmt.Sprintf("%s: %s", b.Polecat, strings.Join(b.Reasons, "; ")))
	}
	return strings.Join(parts, " | ")
}

// displayDryRunSafetyCheck shows the exact safety decision already computed for dry-run mode.
func displayDryRunSafetyCheck(result *SafetyCheckResult, force bool) {
	fmt.Printf("\n  Safety checks:\n")
	if result == nil {
		fmt.Printf("    - Decision: %s\n", style.Warning.Render("unknown"))
		return
	}
	if result.CleanupStatus != "" {
		if result.CleanupStatus.IsSafe() {
			fmt.Printf("    - Cleanup status: %s\n", style.Success.Render(string(result.CleanupStatus)))
		} else {
			fmt.Printf("    - Cleanup status: %s\n", style.Error.Render(string(result.CleanupStatus)))
		}
	}
	if result.GitState != nil {
		if result.GitState.Clean {
			fmt.Printf("    - Git state: %s\n", style.Success.Render("clean"))
		} else {
			fmt.Printf("    - Git state: %s\n", style.Error.Render("dirty"))
			if result.GitState.ComparisonBase != "" {
				fmt.Printf("      comparison: %s (%d unpreserved patch(es))\n", result.GitState.ComparisonBase, result.GitState.UnpreservedPatchCount)
			}
		}
	} else if containsSafetyReasonPrefix(result.Reasons, "git_state=unknown") {
		fmt.Printf("    - Git state: %s\n", style.Warning.Render("unknown"))
	} else {
		fmt.Printf("    - Git state: %s\n", style.Dim.Render("not checked"))
	}
	if result.HookBead != "" {
		if result.HookStale {
			fmt.Printf("    - Hook: %s (%s, closed - stale)\n", style.Warning.Render("stale"), result.HookBead)
		} else {
			fmt.Printf("    - Hook: %s (%s)\n", style.Error.Render("has work"), result.HookBead)
		}
	} else {
		fmt.Printf("    - Hook: %s\n", style.Success.Render("empty"))
	}
	if result.ActiveMR != "" {
		fmt.Printf("    - Active MR: %s\n", result.ActiveMR)
	}
	if result.OpenMR != "" {
		fmt.Printf("    - Open MR: %s (%s)\n", style.Error.Render("yes"), result.OpenMR)
	} else {
		fmt.Printf("    - Open MR: %s\n", style.Success.Render("none"))
	}
	if len(result.Reasons) > 0 {
		if force {
			fmt.Printf("    - Decision: %s\n", style.Warning.Render("BYPASS (--force; LOSES WORK)"))
			fmt.Printf("    - Bypassed blockers:\n")
		} else {
			fmt.Printf("    - Decision: %s\n", style.Error.Render("REFUSE"))
			fmt.Printf("    - Blockers:\n")
		}
		for _, reason := range result.Reasons {
			fmt.Printf("      - %s\n", reason)
		}
	} else {
		fmt.Printf("    - Decision: %s\n", style.Success.Render("PASS"))
	}
}

func containsSafetyReasonPrefix(reasons []string, prefix string) bool {
	for _, reason := range reasons {
		if strings.HasPrefix(reason, prefix) {
			return true
		}
	}
	return false
}
