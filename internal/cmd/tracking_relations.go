package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	beadsdk "github.com/steveyegge/beads"
	"github.com/steveyegge/gastown/internal/beads"
)

var (
	addTrackingRelationFn              = addTrackingRelation
	removeTrackingRelationFn           = removeTrackingRelation
	mutateTrackingRelationViaStoreFn   = mutateTrackingRelationViaStore
	fallbackTrackingRelationFn         = fallbackTrackingRelation
	trackingRelationSleep              = time.Sleep
	trackingRelationVisibilityBackoffs = []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}
)

func addTrackingRelation(townRoot, trackerID, issueID string) error {
	var err error
	for attempt := 0; ; attempt++ {
		err = mutateTrackingRelationViaStoreFn(townRoot, trackerID, issueID, true)
		if err == nil {
			return nil
		}
		if !isTrackingVisibilityError(err) || attempt >= len(trackingRelationVisibilityBackoffs) {
			break
		}
		trackingRelationSleep(trackingRelationVisibilityBackoffs[attempt])
	}
	return fallbackTrackingRelationFn(townRoot, trackerID, issueID, true, err)
}

func removeTrackingRelation(townRoot, trackerID, issueID string) error {
	if err := mutateTrackingRelationViaStoreFn(townRoot, trackerID, issueID, false); err != nil {
		return fallbackTrackingRelationFn(townRoot, trackerID, issueID, false, err)
	}
	return nil
}

func isTrackingVisibilityError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"not found",
		"not visible",
		"does not exist",
		"no such issue",
		"unknown issue",
		"could not find",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func mutateTrackingRelationViaStore(townRoot, trackerID, issueID string, add bool) error {
	resolvedBeads := beads.ResolveBeadsDir(townRoot)
	if resolvedBeads == "" {
		return fmt.Errorf("resolving town beads dir")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	b := beads.NewWithBeadsDir(townRoot, resolvedBeads)
	store, cleanup, err := b.OpenStore(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	targetID := trackingDependsOnID(townRoot, issueID)
	actor := os.Getenv("BD_ACTOR")
	if actor == "" {
		actor = detectSender()
	}

	if add {
		dep := &beadsdk.Dependency{
			IssueID:     trackerID,
			DependsOnID: targetID,
			Type:        beadsdk.DependencyType("tracks"),
		}
		return store.AddDependency(ctx, dep, actor)
	}

	return store.RemoveDependency(ctx, trackerID, targetID, actor)
}

func fallbackTrackingRelation(townRoot, trackerID, issueID string, add bool, storeErr error) error {
	args := []string{"dep", "add", trackerID, issueID, "--type=tracks"}
	if !add {
		args = []string{"dep", "remove", trackerID, issueID, "--type=tracks"}
	}

	if out, err := BdCmd(args...).Dir(townRoot).WithAutoCommit().StripBeadsDir().CombinedOutput(); err != nil {
		output := strings.TrimSpace(string(out))
		if output == "" {
			return fmt.Errorf("tracking relation via store failed: %w; fallback bd path failed: %w", storeErr, err)
		}
		return fmt.Errorf("tracking relation via store failed: %w; fallback bd path failed: %w; output: %s", storeErr, err, output)
	}

	return nil
}

func trackingDependsOnID(townRoot, issueID string) string {
	if strings.HasPrefix(issueID, "external:") {
		return issueID
	}

	prefix := beads.ExtractPrefix(issueID)
	if prefix == "" {
		return issueID
	}

	if rigName := beads.GetRigNameForPrefix(townRoot, prefix); rigName != "" {
		return fmt.Sprintf("external:%s:%s", strings.TrimSuffix(prefix, "-"), issueID)
	}

	return issueID
}
