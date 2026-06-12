package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/steveyegge/gastown/internal/beads"
)

func TestRunFanoutCreatesWithRigParentAndLabels(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.jsonl")
	var out bytes.Buffer
	var created []beads.CreateOptions

	err := runFanout(fanoutOptions{
		Rig:       "gastown",
		Parent:    "gt-parent",
		Rate:      0,
		StateFile: stateFile,
		Labels:    []string{"cleanup"},
		Type:      "task",
		Priority:  2,
	}, testFanoutDeps("First bead\n", &out, func(opts beads.CreateOptions) (*beads.Issue, error) {
		created = append(created, opts)
		return &beads.Issue{ID: "gt-child"}, nil
	}))
	if err != nil {
		t.Fatalf("runFanout() error = %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("created %d beads, want 1", len(created))
	}
	got := created[0]
	if got.Title != "First bead" || got.Rig != "gastown" || got.Parent != "gt-parent" || got.Priority != 2 {
		t.Fatalf("create opts = %+v", got)
	}
	if !hasFanoutTestLabel(got.Labels, "gt:task") || !hasFanoutTestLabel(got.Labels, "cleanup") {
		t.Fatalf("labels = %v, want gt:task and cleanup", got.Labels)
	}

	recs := readFanoutRecords(t, stateFile)
	if len(recs) != 2 || recs[0].Status != "pending" || recs[1].Status != "created" || recs[1].ID != "gt-child" || recs[1].Parent != "gt-parent" {
		t.Fatalf("state records = %+v", recs)
	}
	if !strings.Contains(out.String(), "Created gt-child") {
		t.Fatalf("output = %q, want created message", out.String())
	}
}

func TestRunFanoutDryRunDoesNotCreateWriteOrSleep(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.jsonl")
	var out bytes.Buffer
	created := false
	slept := false

	err := runFanout(fanoutOptions{
		Rig:       "gastown",
		Rate:      time.Second,
		StateFile: stateFile,
		DryRun:    true,
		Type:      "task",
		Priority:  2,
	}, testFanoutDeps("One\nTwo\n", &out, func(opts beads.CreateOptions) (*beads.Issue, error) {
		created = true
		return nil, nil
	}).withSleep(func(time.Duration) { slept = true }))
	if err != nil {
		t.Fatalf("runFanout() error = %v", err)
	}
	if created {
		t.Fatal("dry run called create")
	}
	if slept {
		t.Fatal("dry run slept")
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("state file stat err = %v, want not exist", err)
	}
	if !strings.Contains(out.String(), "Would create") {
		t.Fatalf("output = %q, want dry-run create text", out.String())
	}
}

func TestRunFanoutResumeSkipsRecordedTitles(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.jsonl")
	writeFanoutRecord(t, stateFile, fanoutStateRecord{
		Status:    "pending",
		Title:     "Old",
		Rig:       "gastown",
		Labels:    []string{"gt:task"},
		Priority:  2,
		CreatedAt: "2026-06-12T00:00:00Z",
	})
	writeFanoutRecord(t, stateFile, fanoutStateRecord{
		Status:    "created",
		Title:     "Old",
		ID:        "gt-old",
		Rig:       "gastown",
		Labels:    []string{"gt:task"},
		Priority:  2,
		CreatedAt: "2026-06-12T00:00:00Z",
	})
	var out bytes.Buffer
	var created []string

	err := runFanout(fanoutOptions{
		Rig:       "gastown",
		Rate:      0,
		StateFile: stateFile,
		Type:      "task",
		Priority:  2,
	}, testFanoutDeps("Old\nNew\n", &out, func(opts beads.CreateOptions) (*beads.Issue, error) {
		created = append(created, opts.Title)
		return &beads.Issue{ID: "gt-new"}, nil
	}))
	if err != nil {
		t.Fatalf("runFanout() error = %v", err)
	}
	if len(created) != 1 || created[0] != "New" {
		t.Fatalf("created = %v, want only New", created)
	}
	recs := readFanoutRecords(t, stateFile)
	if len(recs) != 4 || recs[3].Title != "New" || recs[3].Status != "created" || recs[3].ID != "gt-new" {
		t.Fatalf("state records = %+v, want appended New", recs)
	}
	if !strings.Contains(out.String(), "Skipping \"Old\"") {
		t.Fatalf("output = %q, want skip", out.String())
	}
}

func TestRunFanoutFailsWhenStateLockHeld(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.jsonl")
	lock := flock.New(stateFile + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()

	created := false
	err := runFanout(fanoutOptions{
		Rig:       "gastown",
		StateFile: stateFile,
		Type:      "task",
		Priority:  2,
	}, testFanoutDeps("Title\n", &bytes.Buffer{}, func(opts beads.CreateOptions) (*beads.Issue, error) {
		created = true
		return &beads.Issue{ID: "gt-new"}, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "locked by another fanout run") {
		t.Fatalf("runFanout() error = %v, want lock error", err)
	}
	if created {
		t.Fatal("created while state lock held")
	}
}

func TestRunFanoutRejectsMissingParentBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name string
		show func(string) (*beads.Issue, error)
		want string
	}{
		{name: "show error", show: func(string) (*beads.Issue, error) { return nil, errors.New("boom") }, want: "showing parent"},
		{name: "nil parent", show: func(string) (*beads.Issue, error) { return nil, nil }, want: "not found"},
		{name: "empty parent", show: func(string) (*beads.Issue, error) { return &beads.Issue{}, nil }, want: "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created := false
			deps := testFanoutDeps("Title\n", &bytes.Buffer{}, func(opts beads.CreateOptions) (*beads.Issue, error) {
				created = true
				return &beads.Issue{ID: "gt-new"}, nil
			})
			deps.show = tc.show
			err := runFanout(fanoutOptions{
				Rig:       "gastown",
				Parent:    "gt-parent",
				StateFile: filepath.Join(t.TempDir(), "state.jsonl"),
				Type:      "task",
				Priority:  2,
			}, deps)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runFanout() error = %v, want %q", err, tc.want)
			}
			if created {
				t.Fatal("created after parent validation failure")
			}
		})
	}
}

func TestRunFanoutSleepsBetweenCreates(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.jsonl")
	var created []string
	var sleeps []time.Duration

	err := runFanout(fanoutOptions{
		Rig:       "gastown",
		Rate:      250 * time.Millisecond,
		StateFile: stateFile,
		Type:      "task",
		Priority:  2,
	}, testFanoutDeps("One\nTwo\nThree\n", &bytes.Buffer{}, func(opts beads.CreateOptions) (*beads.Issue, error) {
		created = append(created, opts.Title)
		return &beads.Issue{ID: "gt-" + strings.ToLower(opts.Title)}, nil
	}).withSleep(func(d time.Duration) { sleeps = append(sleeps, d) }))
	if err != nil {
		t.Fatalf("runFanout() error = %v", err)
	}
	if strings.Join(created, ",") != "One,Two,Three" {
		t.Fatalf("created = %v", created)
	}
	if len(sleeps) != 2 || sleeps[0] != 250*time.Millisecond || sleeps[1] != 250*time.Millisecond {
		t.Fatalf("sleeps = %v, want two 250ms sleeps", sleeps)
	}
}

func TestRunFanoutDryRunSkipsParentLookup(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.jsonl")
	showCalled := false
	deps := testFanoutDeps("Title\n", &bytes.Buffer{}, func(opts beads.CreateOptions) (*beads.Issue, error) {
		t.Fatal("dry run called create")
		return nil, nil
	})
	deps.show = func(id string) (*beads.Issue, error) {
		showCalled = true
		return &beads.Issue{ID: id}, nil
	}

	err := runFanout(fanoutOptions{
		Rig:       "gastown",
		Parent:    "gt-parent",
		StateFile: stateFile,
		DryRun:    true,
		Type:      "task",
		Priority:  2,
	}, deps)
	if err != nil {
		t.Fatalf("runFanout() error = %v", err)
	}
	if showCalled {
		t.Fatal("dry-run parent validation called show")
	}
}

func TestRunFanoutRejectsDuplicateTitlesBeforeWriting(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.jsonl")
	created := false
	err := runFanout(fanoutOptions{
		Rig:       "gastown",
		StateFile: stateFile,
		Type:      "task",
		Priority:  2,
	}, testFanoutDeps("Dup\n  Dup  \n", &bytes.Buffer{}, func(opts beads.CreateOptions) (*beads.Issue, error) {
		created = true
		return &beads.Issue{ID: "gt-dup"}, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "duplicate title") {
		t.Fatalf("runFanout() error = %v, want duplicate title", err)
	}
	if created {
		t.Fatal("created after duplicate input")
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("state file stat err = %v, want not exist", err)
	}
}

func TestRunFanoutRejectsBadValidationBeforeWriting(t *testing.T) {
	tests := []struct {
		name string
		opts fanoutOptions
		want string
	}{
		{name: "bad rig", opts: fanoutOptions{Rig: "../other", Type: "task", Priority: 2}, want: "invalid rig"},
		{name: "bad rate", opts: fanoutOptions{Rig: "gastown", Rate: -time.Second, Type: "task", Priority: 2}, want: "--rate"},
		{name: "bad priority", opts: fanoutOptions{Rig: "gastown", Type: "task", Priority: 5}, want: "--priority"},
		{name: "bad type", opts: fanoutOptions{Rig: "gastown", Type: "unknown", Priority: 2}, want: "unsupported --type"},
		{name: "conflicting type label", opts: fanoutOptions{Rig: "gastown", Type: "task", Labels: []string{"gt:bug"}, Priority: 2}, want: "conflicts with --type"},
		{name: "unknown rig", opts: fanoutOptions{Rig: "unknown", Type: "task", Priority: 2}, want: "unknown rig"},
		{name: "parent other rig", opts: fanoutOptions{Rig: "gastown", Parent: "bd-parent", Type: "task", Priority: 2}, want: "not target rig"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created := false
			err := runFanout(tt.opts, testFanoutDeps("Title\n", &bytes.Buffer{}, func(opts beads.CreateOptions) (*beads.Issue, error) {
				created = true
				return &beads.Issue{ID: "gt-new"}, nil
			}))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("runFanout() error = %v, want %q", err, tt.want)
			}
			if created {
				t.Fatal("created after validation failure")
			}
		})
	}
}

func TestRunFanoutStopsOnPendingStateBeforeWriting(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.jsonl")
	writeFanoutRecord(t, stateFile, fanoutStateRecord{
		Status:   "pending",
		Title:    "Maybe created",
		Rig:      "gastown",
		Labels:   []string{"gt:task"},
		Priority: 2,
	})
	created := false
	err := runFanout(fanoutOptions{
		Rig:       "gastown",
		StateFile: stateFile,
		Type:      "task",
		Priority:  2,
	}, testFanoutDeps("Maybe created\n", &bytes.Buffer{}, func(opts beads.CreateOptions) (*beads.Issue, error) {
		created = true
		return &beads.Issue{ID: "gt-new"}, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "pending record") {
		t.Fatalf("runFanout() error = %v, want pending record", err)
	}
	if created {
		t.Fatal("created after pending state")
	}
}

func TestRunFanoutRejectsBadStateBeforeWriting(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.jsonl")
	if err := os.WriteFile(stateFile, []byte("not json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	created := false
	err := runFanout(fanoutOptions{
		Rig:       "gastown",
		StateFile: stateFile,
		Type:      "task",
		Priority:  2,
	}, testFanoutDeps("Title\n", &bytes.Buffer{}, func(opts beads.CreateOptions) (*beads.Issue, error) {
		created = true
		return &beads.Issue{ID: "gt-new"}, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "parsing state file") {
		t.Fatalf("runFanout() error = %v, want state parse error", err)
	}
	if created {
		t.Fatal("created after bad state")
	}
}

func TestRunFanoutOpensStateBeforeCreating(t *testing.T) {
	stateFile := t.TempDir()
	created := false
	err := runFanout(fanoutOptions{
		Rig:       "gastown",
		StateFile: stateFile,
		Type:      "task",
		Priority:  2,
	}, testFanoutDeps("Title\n", &bytes.Buffer{}, func(opts beads.CreateOptions) (*beads.Issue, error) {
		created = true
		return &beads.Issue{ID: "gt-new"}, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "state file") {
		t.Fatalf("runFanout() error = %v, want state open error", err)
	}
	if created {
		t.Fatal("created before state file was writable")
	}
}

func testFanoutDeps(input string, out *bytes.Buffer, create func(beads.CreateOptions) (*beads.Issue, error)) fanoutDeps {
	return fanoutDeps{
		stdin:  strings.NewReader(input),
		stdout: out,
		now: func() time.Time {
			return time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC)
		},
		findTownRoot: func() (string, error) { return "/town", nil },
		rigDirForName: func(townRoot, rig string) string {
			if townRoot == "/town" && rig == "gastown" {
				return "/town/gastown/mayor/rig"
			}
			return ""
		},
		rigNameForPrefix: func(townRoot, prefix string) string {
			switch prefix {
			case "gt-":
				return "gastown"
			case "bd-":
				return "beads"
			default:
				return ""
			}
		},
		show: func(id string) (*beads.Issue, error) {
			return &beads.Issue{ID: id}, nil
		},
		create: create,
	}
}

func (deps fanoutDeps) withSleep(sleep func(time.Duration)) fanoutDeps {
	deps.sleep = sleep
	return deps
}

func hasFanoutTestLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}

func readFanoutRecords(t *testing.T, path string) []fanoutStateRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records []fanoutStateRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var rec fanoutStateRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		records = append(records, rec)
	}
	return records
}

func writeFanoutRecord(t *testing.T, path string, rec fanoutStateRecord) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
}
