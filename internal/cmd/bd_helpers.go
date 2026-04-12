package cmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// bdCmd is a builder for constructing bd exec.Command calls.
// It provides a fluent API for configuring environment variables,
// working directory, and I/O settings common to bd CLI invocations.
type bdCmd struct {
	args       []string
	dir        string
	env        []string
	stderr     io.Writer
	autoCommit bool
	gtRoot     string
	beadsDir   string
}

// BdCmd creates a new bd command builder with the given arguments.
// The command will execute "bd" with the provided arguments.
//
// Example:
//
//	err := cmd.BdCmd("show", beadID, "--json").
//	    Dir(workDir).
//	    Run()
func BdCmd(args ...string) *bdCmd {
	return &bdCmd{
		args:   args,
		env:    os.Environ(),
		stderr: os.Stderr,
	}
}

// WithAutoCommit sets BD_DOLT_AUTO_COMMIT=on in the environment.
// This is used for sequential dependent bd calls where each call
// needs to see the changes from previous calls.
func (b *bdCmd) WithAutoCommit() *bdCmd {
	b.autoCommit = true
	return b
}

// WithGTRoot adds GT_ROOT=root to the environment.
// This is required for bd to find town-level formulas and configuration.
func (b *bdCmd) WithGTRoot(root string) *bdCmd {
	b.gtRoot = root
	return b
}

// WithBeadsDir sets BEADS_DIR explicitly in the environment.
// This prevents inherited BEADS_DIR from the parent process from causing
// bd to write to the wrong database. The dir should be the resolved
// .beads directory path (e.g., from beads.ResolveBeadsDir).
func (b *bdCmd) WithBeadsDir(dir string) *bdCmd {
	b.beadsDir = dir
	return b
}

// Dir sets the working directory for the command.
func (b *bdCmd) Dir(dir string) *bdCmd {
	b.dir = dir
	return b
}

// RouteForBead resolves the correct rig database directory for the given bead
// ID and strips any inherited BEADS_DIR that would override that routing.
func (b *bdCmd) RouteForBead(beadID string) *bdCmd {
	if dir := resolveBeadDir(beadID); dir != "" && dir != "." {
		b.dir = dir
		b.beadsDir = beads.ResolveBeadsDir(dir)
	}
	return b
}

// StripBeadsDir removes inherited beads routing env from the environment.
// Use this when the command relies on Dir() for routing and an inherited
// BEADS_* binding would incorrectly override the working-directory-based
// database discovery.
func (b *bdCmd) StripBeadsDir() *bdCmd {
	b.env = beads.BoundEnv(b.env, "")
	return b
}

// Stderr sets the stderr writer for the command.
// Defaults to os.Stderr if not set.
func (b *bdCmd) Stderr(w io.Writer) *bdCmd {
	b.stderr = w
	return b
}

// filterEnvKey removes all entries matching the given key from the env slice.
// This ensures appended values aren't shadowed by existing entries, since
// glibc getenv() returns the first match in the environment array.
func filterEnvKey(env []string, key string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}

// buildEnv constructs the final environment slice based on configured options.
func (b *bdCmd) buildEnv() []string {
	env := b.env

	// Add BD_DOLT_AUTO_COMMIT=on for sequential dependent calls.
	// Filter existing entries first — glibc getenv() returns the first match,
	// so an existing "off" entry would shadow the appended "on".
	if b.autoCommit {
		env = filterEnvKey(env, "BD_DOLT_AUTO_COMMIT")
		env = append(env, "BD_DOLT_AUTO_COMMIT=on")
	}

	// Add GT_ROOT if specified.
	// Filter existing entries first for the same reason as above.
	if b.gtRoot != "" {
		env = filterEnvKey(env, "GT_TOWN_ROOT")
		env = filterEnvKey(env, "GT_ROOT")
		env = filterEnvKey(env, "GT_DOLT_DATA")
		env = filterEnvKey(env, "BEADS_DOLT_DATA_DIR")
		env = append(env, "GT_TOWN_ROOT="+b.gtRoot)
		env = append(env, "GT_ROOT="+b.gtRoot)
		doltData := filepath.Join(b.gtRoot, ".dolt-data")
		env = append(env, "GT_DOLT_DATA="+doltData)
		env = append(env, "BEADS_DOLT_DATA_DIR="+doltData)
	}

	// Add BEADS_DIR if specified.
	// This prevents inherited BEADS_DIR from causing bd to target the wrong
	// database (e.g., HQ instead of rig). See gt-ctir.
	if b.beadsDir != "" {
		env = beads.BoundEnv(env, b.beadsDir)
	}

	return env
}

// Build returns the configured exec.Cmd.
// This allows callers to further customize the command before execution.
func (b *bdCmd) Build() *exec.Cmd {
	args := b.resolvedArgs()
	cmd := exec.Command("bd", args...)
	cmd.Dir = b.dir
	cmd.Env = b.buildEnv()
	cmd.Stderr = b.stderr
	return cmd
}

// resolvedArgs returns the final args, stripping --allow-stale if bd doesn't support it.
func (b *bdCmd) resolvedArgs() []string {
	if beads.BdSupportsAllowStale() {
		return b.args
	}
	filtered := make([]string, 0, len(b.args))
	for _, a := range b.args {
		if a != "--allow-stale" {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

// Run builds and runs the command, returning any error.
// This is a convenience method equivalent to Build().Run().
func (b *bdCmd) Run() error {
	return b.Build().Run()
}

// Output builds and runs the command, returning stdout and any error.
// This is a convenience method equivalent to Build().Output().
// Note: Output() captures stdout but Stderr must still be configured
// separately if you want to capture stderr instead of it going to os.Stderr.
func (b *bdCmd) Output() ([]byte, error) {
	return b.Build().Output()
}

// CombinedOutput builds and runs the command, returning combined stdout+stderr.
// This overrides the configured Stderr writer to capture both streams.
// Useful for including command output in error messages.
func (b *bdCmd) CombinedOutput() ([]byte, error) {
	args := b.resolvedArgs()
	cmd := exec.Command("bd", args...)
	cmd.Dir = b.dir
	cmd.Env = b.buildEnv()
	return cmd.CombinedOutput()
}
