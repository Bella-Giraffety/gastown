package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doctor"
	"github.com/steveyegge/gastown/internal/workspace"
)

var repairCmd = &cobra.Command{
	Use:     "repair",
	GroupID: GroupDiag,
	Short:   "Repair database identity and configuration issues",
	Long: `Repair common database identity mismatches and configuration issues.

This is a focused version of 'gt doctor --fix' that targets the most common
failure mode: metadata.json pointing to the wrong Dolt database after a crash,
rig addition, or bd init conflict.

What it repairs:
  - metadata.json dolt_database pointing to wrong database
  - Missing config.json for registered rigs
  - Prefix mismatches between config.json and rigs.json
  - Missing Dolt databases
  - Missing rig identity beads
  - Stale Dolt port in metadata.json

For a full diagnostic, use 'gt doctor' instead.
For a full diagnostic with auto-fix, use 'gt doctor --fix'.`,
	RunE: runRepair,
}

func init() {
	rootCmd.AddCommand(repairCmd)
	repairCmd.AddCommand(repairBootstrapCmd)
}

func rigRepairChecks() []doctor.Check {
	return []doctor.Check{
		doctor.NewDoltMetadataCheck(),
		doctor.NewRigConfigSyncCheck(),
		doctor.NewRoutesCheck(),
		doctor.NewDatabasePrefixCheck(),
		doctor.NewStaleBeadsRedirectCheck(),
		doctor.NewBeadsRedirectTargetCheck(),
		doctor.NewRigBeadsCheck(),
		doctor.NewAgentBeadsCheck(),
		doctor.NewStaleDoltPortCheck(),
	}
}

func bootstrapRepairChecks() []doctor.Check {
	return []doctor.Check{
		doctor.NewDoltMetadataCheck(),
		doctor.NewTownConfigExistsCheck(),
		doctor.NewTownConfigValidCheck(),
		doctor.NewRigsRegistryExistsCheck(),
		doctor.NewMayorExistsCheck(),
		doctor.NewTownBeadsConfigCheck(),
		doctor.NewRoutesCheck(),
		doctor.NewTmuxGlobalEnvCheck(),
		doctor.NewStaleDoltPortCheck(),
		doctor.NewDatabasePrefixCheck(),
	}
}

func runRepairChecks(townRoot, rigName, title string, checks ...doctor.Check) error {
	ctx := &doctor.CheckContext{
		TownRoot: townRoot,
		RigName:  rigName,
		Verbose:  true,
	}

	d := doctor.NewDoctor()
	d.RegisterAll(checks...)

	if title != "" {
		fmt.Println(title)
		fmt.Println()
	}

	report := d.FixStreaming(ctx, os.Stdout, 0)
	if report.HasErrors() {
		return fmt.Errorf("repair left %d blocking issue(s)", report.Summary.Errors)
	}
	return nil
}

func runRepair(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	ctx := &doctor.CheckContext{
		TownRoot: townRoot,
		Verbose:  true,
	}

	// Run the identity/config repair checks
	checks := []doctor.Check{
		doctor.NewRigConfigSyncCheck(),
		doctor.NewStaleDoltPortCheck(),
	}

	fmt.Println("Repairing database identity and configuration...")
	fmt.Println()

	hasIssues := false
	for _, check := range checks {
		result := check.Run(ctx)
		if result.Status == doctor.StatusOK {
			fmt.Printf("  ✓ %s: %s\n", result.Name, result.Message)
			continue
		}

		hasIssues = true
		fmt.Printf("  ⚠ %s: %s\n", result.Name, result.Message)
		for _, d := range result.Details {
			fmt.Printf("    - %s\n", d)
		}

		// Auto-fix
		if check.CanFix() {
			fmt.Printf("    Fixing...\n")
			if err := check.Fix(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "    ✗ Fix failed: %v\n", err)
			} else {
				fmt.Printf("    ✓ Fixed\n")
			}
		}
	}

	if !hasIssues {
		fmt.Println("\nAll identity checks passed — no repairs needed.")
	} else {
		fmt.Println("\nRepair complete. Run 'gt doctor' for a full diagnostic.")
	}

	return nil
}
