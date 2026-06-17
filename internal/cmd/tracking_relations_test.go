package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrackingDependsOnID_CrossRigWrapsExternal(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0o755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte("{\"prefix\":\"ag-\",\"path\":\"agentcompany/.beads\"}\n"), 0o644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	got := trackingDependsOnID(townRoot, "ag-95s.1")
	want := "external:ag:ag-95s.1"
	if got != want {
		t.Fatalf("trackingDependsOnID() = %q, want %q", got, want)
	}
}

func TestTrackingDependsOnID_HQStaysLocal(t *testing.T) {
	townRoot := t.TempDir()
	got := trackingDependsOnID(townRoot, "hq-cv-test")
	if got != "hq-cv-test" {
		t.Fatalf("trackingDependsOnID() = %q, want %q", got, "hq-cv-test")
	}
}

func TestAddTrackingRelationRetriesVisibilityErrors(t *testing.T) {
	oldMutate := mutateTrackingRelationViaStoreFn
	oldFallback := fallbackTrackingRelationFn
	oldSleep := trackingRelationSleep
	oldBackoffs := trackingRelationVisibilityBackoffs
	t.Cleanup(func() {
		mutateTrackingRelationViaStoreFn = oldMutate
		fallbackTrackingRelationFn = oldFallback
		trackingRelationSleep = oldSleep
		trackingRelationVisibilityBackoffs = oldBackoffs
	})

	trackingRelationVisibilityBackoffs = []time.Duration{0, 0, 0}
	trackingRelationSleep = func(time.Duration) {}
	attempts := 0
	mutateTrackingRelationViaStoreFn = func(townRoot, trackerID, issueID string, add bool) error {
		attempts++
		if !add {
			t.Fatal("addTrackingRelation called mutate with add=false")
		}
		if attempts < 3 {
			return fmt.Errorf("issue %s not found", issueID)
		}
		return nil
	}
	fallbackTrackingRelationFn = func(townRoot, trackerID, issueID string, add bool, storeErr error) error {
		t.Fatalf("fallback called after retry success: %v", storeErr)
		return nil
	}

	if err := addTrackingRelation("/town", "hq-cv", "gt-leg"); err != nil {
		t.Fatalf("addTrackingRelation() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestAddTrackingRelationDoesNotRetryPermanentStoreErrors(t *testing.T) {
	oldMutate := mutateTrackingRelationViaStoreFn
	oldFallback := fallbackTrackingRelationFn
	oldSleep := trackingRelationSleep
	oldBackoffs := trackingRelationVisibilityBackoffs
	t.Cleanup(func() {
		mutateTrackingRelationViaStoreFn = oldMutate
		fallbackTrackingRelationFn = oldFallback
		trackingRelationSleep = oldSleep
		trackingRelationVisibilityBackoffs = oldBackoffs
	})

	trackingRelationVisibilityBackoffs = []time.Duration{0, 0, 0}
	trackingRelationSleep = func(time.Duration) {}
	attempts := 0
	fallbacks := 0
	mutateTrackingRelationViaStoreFn = func(townRoot, trackerID, issueID string, add bool) error {
		attempts++
		return errors.New("dependency cycle")
	}
	fallbackTrackingRelationFn = func(townRoot, trackerID, issueID string, add bool, storeErr error) error {
		fallbacks++
		return storeErr
	}

	if err := addTrackingRelation("/town", "hq-cv", "gt-leg"); err == nil {
		t.Fatal("addTrackingRelation() error = nil, want error")
	}
	if attempts != 1 || fallbacks != 1 {
		t.Fatalf("attempts/fallbacks = %d/%d, want 1/1", attempts, fallbacks)
	}
}

func TestAddTrackingRelationBoundsVisibilityRetries(t *testing.T) {
	oldMutate := mutateTrackingRelationViaStoreFn
	oldFallback := fallbackTrackingRelationFn
	oldSleep := trackingRelationSleep
	oldBackoffs := trackingRelationVisibilityBackoffs
	t.Cleanup(func() {
		mutateTrackingRelationViaStoreFn = oldMutate
		fallbackTrackingRelationFn = oldFallback
		trackingRelationSleep = oldSleep
		trackingRelationVisibilityBackoffs = oldBackoffs
	})

	trackingRelationVisibilityBackoffs = []time.Duration{0, 0}
	trackingRelationSleep = func(time.Duration) {}
	attempts := 0
	mutateTrackingRelationViaStoreFn = func(townRoot, trackerID, issueID string, add bool) error {
		attempts++
		return errors.New("not visible yet")
	}
	fallbackTrackingRelationFn = func(townRoot, trackerID, issueID string, add bool, storeErr error) error {
		return fmt.Errorf("fallback after retries: %w", storeErr)
	}

	if err := addTrackingRelation("/town", "hq-cv", "gt-leg"); err == nil {
		t.Fatal("addTrackingRelation() error = nil, want error")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}
