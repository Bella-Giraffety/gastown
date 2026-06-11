package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/dog"
)

func writeDogStateForDispatchTest(t *testing.T, townRoot, name string, state *dog.DogState) {
	t.Helper()
	dogPath := filepath.Join(townRoot, "deacon", "dogs", name)
	if err := os.MkdirAll(dogPath, 0755); err != nil {
		t.Fatalf("mkdir dog path: %v", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal dog state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dogPath, ".dog.json"), data, 0644); err != nil {
		t.Fatalf("write dog state: %v", err)
	}
}

func TestDogDispatchInfoClearWorkIfMatchesUsesAssignmentTimestamp(t *testing.T) {
	townRoot := t.TempDir()
	rigsConfig := &config.RigsConfig{Version: 1, Rigs: map[string]config.RigEntry{}}
	now := time.Now().Truncate(time.Second)
	workStarted := now.Add(-time.Minute)
	writeDogStateForDispatchTest(t, townRoot, "alpha", &dog.DogState{
		Name:          "alpha",
		State:         dog.StateWorking,
		Work:          "mol-dog-reaper",
		WorkStartedAt: workStarted,
		LastActive:    now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	staleDispatch := &DogDispatchInfo{
		DogName:       "alpha",
		townRoot:      townRoot,
		workDesc:      "mol-dog-reaper",
		workStartedAt: workStarted.Add(time.Second),
		ownsWork:      true,
		rigsConfig:    rigsConfig,
	}
	cleared, err := staleDispatch.clearWorkIfMatches()
	if err != nil {
		t.Fatalf("clearWorkIfMatches stale dispatch error = %v", err)
	}
	if cleared {
		t.Fatal("stale dispatch cleared dog work with a newer assignment timestamp")
	}

	mgr := dog.NewManager(townRoot, rigsConfig)
	got, err := mgr.Get("alpha")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.State != dog.StateWorking || got.Work != "mol-dog-reaper" || !got.WorkStartedAt.Equal(workStarted) {
		t.Fatalf("stale dispatch mutated dog state: state=%q work=%q started=%v", got.State, got.Work, got.WorkStartedAt)
	}

	matchingDispatch := &DogDispatchInfo{
		DogName:       "alpha",
		townRoot:      townRoot,
		workDesc:      "mol-dog-reaper",
		workStartedAt: workStarted,
		ownsWork:      true,
		rigsConfig:    rigsConfig,
	}
	cleared, err = matchingDispatch.clearWorkIfMatches()
	if err != nil {
		t.Fatalf("clearWorkIfMatches matching dispatch error = %v", err)
	}
	if !cleared {
		t.Fatal("matching dispatch did not clear dog work")
	}
	got, err = mgr.Get("alpha")
	if err != nil {
		t.Fatalf("Get() after clear error = %v", err)
	}
	if got.State != dog.StateIdle || got.Work != "" || !got.WorkStartedAt.IsZero() {
		t.Fatalf("matching dispatch did not clear state: state=%q work=%q started=%v", got.State, got.Work, got.WorkStartedAt)
	}
}

func TestDogDispatchInfoClearWorkIfMatchesSkipsReusedWork(t *testing.T) {
	townRoot := t.TempDir()
	rigsConfig := &config.RigsConfig{Version: 1, Rigs: map[string]config.RigEntry{}}
	now := time.Now().Truncate(time.Second)
	workStarted := now.Add(-time.Minute)
	writeDogStateForDispatchTest(t, townRoot, "alpha", &dog.DogState{
		Name:          "alpha",
		State:         dog.StateWorking,
		Work:          "mol-dog-reaper",
		WorkStartedAt: workStarted,
		LastActive:    now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	reusedDispatch := &DogDispatchInfo{
		DogName:       "alpha",
		townRoot:      townRoot,
		workDesc:      "mol-dog-reaper",
		workStartedAt: workStarted,
		ownsWork:      false,
		rigsConfig:    rigsConfig,
	}
	cleared, err := reusedDispatch.clearWorkIfMatches()
	if err != nil {
		t.Fatalf("clearWorkIfMatches reused dispatch error = %v", err)
	}
	if cleared {
		t.Fatal("reused dispatch cleared dog work it did not create")
	}

	got, err := dog.NewManager(townRoot, rigsConfig).Get("alpha")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.State != dog.StateWorking || got.Work != "mol-dog-reaper" || !got.WorkStartedAt.Equal(workStarted) {
		t.Fatalf("reused dispatch mutated dog state: state=%q work=%q started=%v", got.State, got.Work, got.WorkStartedAt)
	}
}
