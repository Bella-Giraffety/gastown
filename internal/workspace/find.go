// Package workspace provides workspace detection and management.
package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
)

// ErrNotFound indicates no workspace was found.
var ErrNotFound = errors.New("not in a Gas Town workspace")

// Markers used to detect a Gas Town workspace.
const (
	// PrimaryMarker is the main config file that identifies a workspace.
	// The town.json file lives in mayor/ along with other mayor config.
	PrimaryMarker = "mayor/town.json"

	// SecondaryMarker is an alternative indicator at the town level.
	// Note: This can match rig-level mayors too, so we continue searching
	// upward after finding this to look for primary markers.
	SecondaryMarker = "mayor"
)

// Find locates the town root by walking up from the given directory.
// It prefers mayor/town.json over mayor/ directory as workspace marker.
// Always continues to the outermost workspace, correctly handling nested
// workspace structures (e.g., rig directories with their own mayor/town.json).
// Does not resolve symlinks to stay consistent with os.Getwd().
func Find(startDir string) (string, error) {
	absDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	var primaryMatch, secondaryMatch string

	current := absDir
	for {
		// Always keep updating primaryMatch and secondaryMatch to find the outermost
		// directory with the respective markers. This handles nested workspace
		// structures where inner workspaces (e.g., rig directories or worktrees)
		// have their own mayor/town.json, ensuring we return the actual town root.
		if _, err := os.Stat(filepath.Join(current, PrimaryMarker)); err == nil {
			primaryMatch = current
		}

		if info, err := os.Stat(filepath.Join(current, SecondaryMarker)); err == nil && info.IsDir() {
			secondaryMatch = current
		}

		parent := filepath.Dir(current)
		if parent == current {
			if primaryMatch != "" {
				return primaryMatch, nil
			}
			return secondaryMatch, nil
		}
		current = parent
	}
}

// FindOrError is like Find but returns a user-friendly error if not found.
func FindOrError(startDir string) (string, error) {
	root, err := Find(startDir)
	if err != nil {
		return "", err
	}
	if root == "" {
		return "", ErrNotFound
	}
	return root, nil
}

// FindFromCwd locates the town root from the current working directory.
func FindFromCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting current directory: %w", err)
	}
	return Find(cwd)
}

// FindFromEnv returns the explicit town root override from GT_TOWN_ROOT or
// GT_ROOT when it points at a valid Gas Town workspace.
func FindFromEnv() string {
	for _, envName := range []string{"GT_TOWN_ROOT", "GT_ROOT"} {
		if townRoot := os.Getenv(envName); townRoot != "" {
			if ok, _ := IsWorkspace(townRoot); ok {
				return townRoot
			}
		}
	}
	return ""
}

// FindFromStartDirOrEnv resolves the authoritative town root for a command.
// It uses GT_TOWN_ROOT / GT_ROOT when the start dir is inside that town (for
// worktrees, subprocesses, and tmux sessions) or when cwd detection cannot find
// any workspace at all.
func FindFromStartDirOrEnv(startDir string) (string, error) {
	envRoot := FindFromEnv()
	if envRoot != "" && isWithinTownRoot(startDir, envRoot) {
		return envRoot, nil
	}

	root, err := Find(startDir)
	if err == nil && root != "" {
		return root, nil
	}
	if envRoot != "" {
		return envRoot, nil
	}
	return root, err
}

func isWithinTownRoot(startDir, townRoot string) bool {
	if startDir == "" || townRoot == "" {
		return false
	}

	rel, err := filepath.Rel(townRoot, startDir)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// FindFromCwdOrError is like FindFromCwd but returns an error if not found.
// It prefers explicit GT_TOWN_ROOT / GT_ROOT when they describe the current
// town, or falls back to them when cwd detection cannot find any workspace.
func FindFromCwdOrError() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		if root := FindFromEnv(); root != "" {
			return root, nil
		}
		return "", fmt.Errorf("getting current directory: %w", err)
	}

	root, err := FindFromStartDirOrEnv(cwd)
	if err != nil {
		return "", err
	}
	if root == "" {
		return "", ErrNotFound
	}
	return root, nil
}

// FindFromCwdWithFallback is like FindFromCwdOrError but returns (townRoot, cwd, error).
// If getcwd fails, returns (townRoot, "", nil) using GT_TOWN_ROOT fallback.
// This is useful for commands like `gt done` that need to continue even if the
// working directory is deleted (e.g., polecat worktree nuked by Witness).
func FindFromCwdWithFallback() (townRoot string, cwd string, err error) {
	cwd, err = os.Getwd()
	if err != nil {
		if townRoot = FindFromEnv(); townRoot != "" {
			return townRoot, "", nil // cwd is gone but townRoot is valid
		}
		return "", "", fmt.Errorf("getting current directory: %w", err)
	}

	townRoot, err = FindFromStartDirOrEnv(cwd)
	if err != nil {
		return "", "", err
	}
	if townRoot == "" {
		return "", "", ErrNotFound
	}
	return townRoot, cwd, nil
}

// IsWorkspace checks if the given directory is a Gas Town workspace root.
// A directory is a workspace if it has a primary marker (mayor/town.json)
// or a secondary marker (mayor/ directory).
func IsWorkspace(dir string) (bool, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false, fmt.Errorf("resolving path: %w", err)
	}

	// Check for primary marker (mayor/town.json)
	primaryPath := filepath.Join(absDir, PrimaryMarker)
	if _, err := os.Stat(primaryPath); err == nil {
		return true, nil
	}

	// Check for secondary marker (mayor/ directory)
	secondaryPath := filepath.Join(absDir, SecondaryMarker)
	info, err := os.Stat(secondaryPath)
	if err == nil && info.IsDir() {
		return true, nil
	}

	return false, nil
}

// GetTownName loads the town name from the workspace's town.json config.
// This is used for generating unique tmux session names that avoid collisions
// when running multiple Gas Town instances.
func GetTownName(townRoot string) (string, error) {
	townConfigPath := filepath.Join(townRoot, PrimaryMarker)
	townConfig, err := config.LoadTownConfig(townConfigPath)
	if err != nil {
		return "", fmt.Errorf("loading town config: %w", err)
	}
	return townConfig.Name, nil
}

// GetTownNameFromCwd locates the town root from the current working directory
// and returns the town name from its configuration.
func GetTownNameFromCwd() (string, error) {
	townRoot, err := FindFromCwdOrError()
	if err != nil {
		return "", err
	}
	return GetTownName(townRoot)
}

// MustGetTownName returns the town name or panics if it cannot be loaded.
// Use sparingly - prefer GetTownName with proper error handling.
func MustGetTownName(townRoot string) string {
	name, err := GetTownName(townRoot)
	if err != nil {
		panic(fmt.Sprintf("failed to get town name: %v", err))
	}
	return name
}
