package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// BeadsRepoFingerprintCheck detects when a rig's shared Beads database was
// stamped with a different repository fingerprint than the current clone.
// This blocks hook/mail flows even when Dolt itself is healthy.
type BeadsRepoFingerprintCheck struct {
	BaseCheck
}

// NewBeadsRepoFingerprintCheck creates a new repo fingerprint drift check.
func NewBeadsRepoFingerprintCheck() *BeadsRepoFingerprintCheck {
	return &BeadsRepoFingerprintCheck{
		BaseCheck: BaseCheck{
			CheckName:        "beads-repo-fingerprint",
			CheckDescription: "Check for shared Beads repo fingerprint drift",
			CheckCategory:    CategoryRig,
		},
	}
}

func (c *BeadsRepoFingerprintCheck) Run(ctx *CheckContext) *CheckResult {
	rigPath := ctx.RigPath()
	if rigPath == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "No rig specified",
		}
	}

	mayorRigPath := filepath.Join(rigPath, "mayor", "rig")
	if _, err := os.Stat(filepath.Join(mayorRigPath, ".git")); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "No mayor/rig clone found",
			FixHint: "Run rig-is-git-repo check first",
		}
	}

	beadsDir := beads.ResolveBeadsDir(mayorRigPath)
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No beads database (skipped)",
		}
	}

	cmd := exec.Command("bd", "migrate", "--update-repo-id", "--dry-run", "--yes")
	cmd.Dir = mayorRigPath
	cmd.Env = beads.BoundEnv(os.Environ(), beadsDir)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not inspect Beads repo fingerprint drift",
			Details: []string{strings.TrimSpace(text), err.Error()},
			FixHint: "Run 'bd migrate --update-repo-id --dry-run --yes' in mayor/rig to inspect manually",
		}
	}

	if strings.Contains(text, "Would update repository ID:") {
		oldID := extractRepoIDLine(text, "Old:")
		newID := extractRepoIDLine(text, "New:")
		details := []string{
			fmt.Sprintf("Stored repo ID: %s", oldID),
			fmt.Sprintf("Current repo ID: %s", newID),
			"Changing the shared Beads repo ID can break other active clones if they still expect the old fingerprint.",
		}
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "Beads repo fingerprint drift detected",
			Details: details,
			FixHint: "Audit active clones, then run 'bd migrate --update-repo-id --yes' in mayor/rig only when it is safe to change the shared database fingerprint",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "Beads repo fingerprint matches current clone",
	}
}

func extractRepoIDLine(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return "unknown"
}
