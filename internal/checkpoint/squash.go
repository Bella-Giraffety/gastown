package checkpoint

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/steveyegge/gastown/internal/util"
)

// WIPCommitPrefix is the commit message prefix used by checkpoint_dog auto-commits.
const WIPCommitPrefix = "WIP: checkpoint (auto)"

// CountWIPCommits returns the number of WIP checkpoint commits between
// the merge-base of baseRef and HEAD.
func CountWIPCommits(workDir, baseRef string) (int, error) {
	mergeBase, err := gitOutput(workDir, "merge-base", baseRef, "HEAD")
	if err != nil {
		return 0, fmt.Errorf("finding merge-base: %w", err)
	}

	// Count only commit subjects; body lines can legitimately mention the WIP prefix.
	logOut, err := gitOutput(workDir, "log", "--format=%s", mergeBase+"..HEAD")
	if err != nil {
		return 0, fmt.Errorf("listing commits: %w", err)
	}

	if logOut == "" {
		return 0, nil
	}

	count := 0
	for _, line := range strings.Split(logOut, "\n") {
		if strings.HasPrefix(line, WIPCommitPrefix) {
			count++
		}
	}
	return count, nil
}

// SquashWIPCommits collapses all commits from merge-base..HEAD into a single
// commit, preserving non-WIP commit messages in the body. Returns the number
// of WIP commits that were squashed.
//
// Callers should use this only at a publish boundary: if checkpoint commits are
// present, the branch history is rewritten while preserving the committed HEAD
// tree. The current index is intentionally ignored so tolerated runtime-only
// dirt cannot be swept into the replacement commit.
func SquashWIPCommits(workDir, baseRef string) (int, error) {
	mergeBase, err := gitOutput(workDir, "merge-base", baseRef, "HEAD")
	if err != nil {
		return 0, fmt.Errorf("finding merge-base: %w", err)
	}
	originalHead, err := gitOutput(workDir, "rev-parse", "HEAD")
	if err != nil {
		return 0, fmt.Errorf("resolving HEAD: %w", err)
	}

	// List commit messages from merge-base..HEAD. Use full messages so attribution
	// trailers on real commits survive the publish-boundary squash.
	logOut, err := gitOutput(workDir, "log", "--reverse", "--format=%B%x00", mergeBase+"..HEAD")
	if err != nil {
		return 0, fmt.Errorf("listing commits: %w", err)
	}

	if logOut == "" {
		return 0, nil // No commits to squash
	}

	messages := strings.Split(logOut, "\x00")
	wipCount := 0
	var nonWIPMessages []string
	for _, message := range messages {
		message = strings.Trim(message, "\n")
		if message == "" {
			continue
		}
		subj := message
		if i := strings.IndexByte(subj, '\n'); i >= 0 {
			subj = subj[:i]
		}
		if strings.HasPrefix(subj, WIPCommitPrefix) {
			wipCount++
		} else {
			nonWIPMessages = append(nonWIPMessages, message)
		}
	}

	if wipCount == 0 {
		return 0, nil // No WIP commits to squash
	}
	tree, err := gitOutput(workDir, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return 0, fmt.Errorf("resolving HEAD tree: %w", err)
	}

	// Build combined commit message
	var msg strings.Builder
	if len(nonWIPMessages) > 0 {
		for i, message := range nonWIPMessages {
			if i > 0 {
				msg.WriteString("\n\n")
			}
			msg.WriteString(message)
		}
	} else {
		// All commits were WIP — use a generic message
		msg.WriteString("squashed WIP checkpoint commits")
	}

	newCommit, err := gitOutput(workDir, "commit-tree", tree, "-p", mergeBase, "-m", msg.String())
	if err != nil {
		return 0, fmt.Errorf("squash commit: %w", err)
	}

	if _, err := gitOutput(workDir, "reset", "--soft", newCommit); err != nil {
		if _, rollbackErr := gitOutput(workDir, "reset", "--soft", originalHead); rollbackErr != nil {
			return 0, fmt.Errorf("updating HEAD to squash commit: %w; rollback to %s failed: %v", err, originalHead, rollbackErr)
		}
		return 0, fmt.Errorf("updating HEAD to squash commit: %w", err)
	}

	return wipCount, nil
}

// gitOutput runs a git command and returns trimmed stdout.
func gitOutput(workDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	util.SetDetachedProcessGroup(cmd)

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return "", fmt.Errorf("%s: %s", err, stderr)
			}
		}
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}
