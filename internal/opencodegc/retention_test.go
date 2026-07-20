package opencodegc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/util"
)

type fakeRunner struct {
	outputs map[string][]byte
	errors  map[string]error
	calls   []string
}

type retentionStats struct {
	total     int
	tooRecent int
	kept      int
	protected int
	eligible  int
}

func (r *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	call := commandCall(name, args...)
	r.calls = append(r.calls, call)
	if err := r.errors[call]; err != nil {
		return nil, err
	}
	if out, ok := r.outputs[call]; ok {
		return out, nil
	}
	return nil, fmt.Errorf("unexpected command: %s", call)
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{outputs: map[string][]byte{}, errors: map[string]error{}}
}

func commandCall(name string, args ...string) string {
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}

func opencodeCall(args ...string) string {
	return commandCall("opencode", args...)
}

func sessionsJSON(t *testing.T, sessions ...Session) []byte {
	t.Helper()
	out, err := json.Marshal(sessions)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func statsJSON(stats retentionStats) []byte {
	return []byte(fmt.Sprintf(`[{"total":%d,"too_recent":%d,"kept":%d,"protected":%d,"eligible":%d}]`,
		stats.total, stats.tooRecent, stats.kept, stats.protected, stats.eligible))
}

func makeOpenCodeFiles(t *testing.T, dbBytes, walBytes uint64) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	writeSizedFile(t, dbPath, dbBytes)
	if walBytes > 0 {
		writeSizedFile(t, dbPath+"-wal", walBytes)
	}
	return dbPath
}

func writeSizedFile(t *testing.T, path string, size uint64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(int64(size)); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func addAnalyzeOutputs(t *testing.T, runner *fakeRunner, dbPath string, opts Options, protected map[string]struct{}, stats retentionStats, candidates []Session) []string {
	t.Helper()
	opts = opts.withDefaults()
	cutoff := opts.Now.Add(-opts.RetentionAge).UnixMilli()
	calls := []string{
		opencodeCall("--pure", "db", "path"),
		opencodeCall("--pure", "db", sessionStatsSQL(cutoff, opts.KeepPerDir, protected), "--format", "json"),
		opencodeCall("--pure", "db", sessionCandidateSQL(cutoff, opts.KeepPerDir, opts.MaxDeletes, protected), "--format", "json"),
	}
	runner.outputs[calls[0]] = []byte(dbPath + "\n")
	runner.outputs[calls[1]] = statsJSON(stats)
	runner.outputs[calls[2]] = sessionsJSON(t, candidates...)
	return calls
}

func addCheckpointOutputs(runner *fakeRunner) {
	runner.outputs[opencodeCall("--pure", "db", "PRAGMA wal_checkpoint(PASSIVE);", "--format", "json")] = []byte(`[{"busy":0}]`)
	runner.outputs[opencodeCall("--pure", "db", "PRAGMA wal_checkpoint(TRUNCATE);", "--format", "json")] = []byte(`[{"busy":0}]`)
}

func selectedIDs(sessions []Session) []string {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}
	return ids
}

func hasMutation(calls []string) bool {
	for _, call := range calls {
		if strings.Contains(call, "session delete") ||
			strings.Contains(call, "wal_checkpoint(TRUNCATE)") ||
			strings.Contains(call, "VACUUM") {
			return true
		}
	}
	return false
}

func countCallsContaining(calls []string, needle string) int {
	count := 0
	for _, call := range calls {
		if strings.Contains(call, needle) {
			count++
		}
	}
	return count
}

func TestAnalyzeUsesBoundedRankedQueries(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	dbPath := makeOpenCodeFiles(t, 1, 0)
	runner := newFakeRunner()
	opts := Options{Now: now, RetentionAge: 24 * time.Hour, KeepPerDir: 1, MaxDeletes: 2, Runner: runner}
	analyzeCalls := addAnalyzeOutputs(t, runner, dbPath, opts, nil, retentionStats{
		total: 6, tooRecent: 1, kept: 1, protected: 1, eligible: 3,
	}, []Session{
		{ID: "delete-oldest", Directory: "/repo", TimeUpdated: now.Add(-120 * time.Hour).UnixMilli()},
		{ID: "delete-second", Directory: "/repo", TimeUpdated: now.Add(-96 * time.Hour).UnixMilli()},
	})

	report, err := Analyze(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.SessionCount != 6 || report.TooRecent != 1 || report.KeptPerDir != 1 || report.Protected != 1 || report.Eligible != 3 {
		t.Fatalf("unexpected counters: total=%d tooRecent=%d kept=%d protected=%d eligible=%d",
			report.SessionCount, report.TooRecent, report.KeptPerDir, report.Protected, report.Eligible)
	}
	if got, want := selectedIDs(report.Selected), []string{"delete-oldest", "delete-second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected IDs = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(runner.calls, analyzeCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, analyzeCalls)
	}
	candidateCall := runner.calls[len(runner.calls)-1]
	for _, want := range []string{"row_number() over", "partition by directory", "dir_rank > 1", "order by time_updated asc, id asc", "limit 2"} {
		if !strings.Contains(candidateCall, want) {
			t.Fatalf("candidate query missing %q: %s", want, candidateCall)
		}
	}
	for _, call := range runner.calls {
		if strings.Contains(call, "from session order by time_updated desc") {
			t.Fatalf("full session scan should not be used: %s", call)
		}
	}
	if !strings.Contains(report.RemainingReason, "exceed one bounded 2-delete batch") {
		t.Fatalf("RemainingReason = %q", report.RemainingReason)
	}
}

func TestAnalyzeProtectsEnvAndRuntimeSessionIDs(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "gastown", "polecats", "atom", ".runtime"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "gastown", "polecats", "atom", ".runtime", "session_id"), []byte("runtime-live\n2026-07-20T12:00:00Z\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GT_SESSION_ID", "env-live")
	t.Setenv("CLAUDE_SESSION_ID", "claude-live")
	t.Setenv("GT_SESSION_ID_ENV", "CUSTOM_OPENCODE_SESSION")
	t.Setenv("CUSTOM_OPENCODE_SESSION", "custom-live")
	protected := map[string]struct{}{
		"env-live":     {},
		"claude-live":  {},
		"custom-live":  {},
		"runtime-live": {},
	}

	dbPath := makeOpenCodeFiles(t, 1, 0)
	runner := newFakeRunner()
	opts := Options{TownRoot: townRoot, Now: now, RetentionAge: 24 * time.Hour, KeepPerDir: 1, MaxDeletes: 10, Runner: runner}
	addAnalyzeOutputs(t, runner, dbPath, opts, protected, retentionStats{
		total: 6, kept: 1, protected: 4, eligible: 1,
	}, []Session{{ID: "delete-me", Directory: "/repo", TimeUpdated: now.Add(-100 * time.Hour).UnixMilli()}})

	report, err := Analyze(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Protected != 4 || report.KeptPerDir != 1 || report.Eligible != 1 {
		t.Fatalf("unexpected counters: protected=%d kept=%d eligible=%d", report.Protected, report.KeptPerDir, report.Eligible)
	}
	if got, want := selectedIDs(report.Selected), []string{"delete-me"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected IDs = %v, want %v", got, want)
	}
	if !strings.Contains(runner.calls[len(runner.calls)-1], "id not in ('claude-live', 'custom-live', 'env-live', 'runtime-live')") {
		t.Fatalf("candidate query did not exclude protected IDs: %s", runner.calls[len(runner.calls)-1])
	}
}

func TestProtectedIDClauseEscapesSQLLiterals(t *testing.T) {
	protected := map[string]struct{}{"live' OR 1=1 --": {}}
	exclude := protectedIDClause(protected, false)
	if !strings.Contains(exclude, "'live'' OR 1=1 --'") {
		t.Fatalf("protected clause did not escape quote: %s", exclude)
	}
	if strings.Contains(exclude, "live' OR 1=1") {
		t.Fatalf("protected clause contains unescaped literal: %s", exclude)
	}
	if got := protectedIDClause(nil, true); got != " and 1 = 0" {
		t.Fatalf("empty include clause = %q", got)
	}
	if got := protectedIDClause(nil, false); got != "" {
		t.Fatalf("empty exclude clause = %q", got)
	}
}

func TestFixReturnsLockBusyBeforeOpenCodeCalls(t *testing.T) {
	runner := newFakeRunner()
	_, err := Fix(context.Background(), Options{
		TownRoot: t.TempDir(),
		Runner:   runner,
		TryLock: func(path string) (func(), bool, error) {
			return func() {}, false, nil
		},
	})
	if !errors.Is(err, ErrLockBusy) {
		t.Fatalf("Fix error = %v, want ErrLockBusy", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("opencode was called before acquiring lock: %v", runner.calls)
	}
}

func TestFixFailsClosedBeforeDeletesWhenHeadroomLow(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	dbPath := makeOpenCodeFiles(t, 1, 0)
	runner := newFakeRunner()
	opts := Options{TownRoot: t.TempDir(), Now: now, RetentionAge: 24 * time.Hour, KeepPerDir: 1, Runner: runner}
	addAnalyzeOutputs(t, runner, dbPath, opts, nil, retentionStats{total: 2, kept: 1, eligible: 1}, []Session{
		{ID: "delete-me", Directory: "/repo", TimeUpdated: now.Add(-48 * time.Hour).UnixMilli()},
	})
	unlocked := false
	opts.MinDeleteHeadroomBytes = 2
	opts.DiskInfo = func(path string) (*util.DiskSpaceInfo, error) {
		return &util.DiskSpaceInfo{AvailableBytes: 1}, nil
	}
	opts.TryLock = func(path string) (func(), bool, error) {
		return func() { unlocked = true }, true, nil
	}

	report, err := Fix(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "insufficient headroom") {
		t.Fatalf("Fix error = %v, want insufficient headroom", err)
	}
	if report == nil || report.Deleted != 0 {
		t.Fatalf("report.Deleted = %v, want 0", report)
	}
	if !unlocked {
		t.Fatal("lock was not released")
	}
	if hasMutation(runner.calls) || countCallsContaining(runner.calls, "wal_checkpoint(PASSIVE)") != 0 {
		t.Fatalf("destructive command ran despite low headroom: %v", runner.calls)
	}
}

func TestFixUsesSupportedCommandsAndSkipsVacuumWithoutHeadroom(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	dbPath := makeOpenCodeFiles(t, 1024, 0)
	runner := newFakeRunner()
	opts := Options{
		TownRoot:               t.TempDir(),
		Now:                    now,
		RetentionAge:           24 * time.Hour,
		KeepPerDir:             1,
		Runner:                 runner,
		VacuumReserveBytes:     20 * 1024 * 1024,
		WALCheckpointBytes:     256 * 1024 * 1024,
		CommandTimeout:         time.Second,
		MinDeleteHeadroomBytes: 1,
	}
	analyzeCalls := addAnalyzeOutputs(t, runner, dbPath, opts, nil, retentionStats{total: 2, kept: 1, eligible: 1}, []Session{
		{ID: "delete-me", Directory: "/repo", TimeUpdated: now.Add(-48 * time.Hour).UnixMilli()},
	})
	addCheckpointOutputs(runner)
	runner.outputs[opencodeCall("--pure", "session", "delete", "delete-me")] = []byte("")
	opts.DiskInfo = func(path string) (*util.DiskSpaceInfo, error) {
		return &util.DiskSpaceInfo{AvailableBytes: 10 * 1024 * 1024}, nil
	}
	opts.TryLock = func(path string) (func(), bool, error) { return func() {}, true, nil }

	report, err := Fix(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Deleted != 1 || !report.Checkpointed || report.Vacuumed || report.VacuumSkipped == "" {
		t.Fatalf("unexpected report: deleted=%d checkpointed=%v vacuumed=%v skipped=%q",
			report.Deleted, report.Checkpointed, report.Vacuumed, report.VacuumSkipped)
	}
	wantCalls := append(analyzeCalls,
		opencodeCall("--pure", "db", "PRAGMA wal_checkpoint(PASSIVE);", "--format", "json"),
		opencodeCall("--pure", "session", "delete", "delete-me"),
		opencodeCall("--pure", "db", "PRAGMA wal_checkpoint(TRUNCATE);", "--format", "json"),
	)
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
	}
	for _, call := range runner.calls {
		if strings.Contains(strings.ToUpper(call), "DELETE FROM") {
			t.Fatalf("raw SQL delete used: %s", call)
		}
	}
}

func TestFixCheckpointsWalWithoutDeletingSessions(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	dbPath := makeOpenCodeFiles(t, 1024, 32)
	runner := newFakeRunner()
	opts := Options{TownRoot: t.TempDir(), Now: now, Runner: runner, WALCheckpointBytes: 16, MinDeleteHeadroomBytes: 1}
	addAnalyzeOutputs(t, runner, dbPath, opts, nil, retentionStats{}, nil)
	addCheckpointOutputs(runner)
	opts.DiskInfo = func(path string) (*util.DiskSpaceInfo, error) {
		return &util.DiskSpaceInfo{AvailableBytes: 10 * 1024 * 1024}, nil
	}
	opts.TryLock = func(path string) (func(), bool, error) { return func() {}, true, nil }

	report, err := Fix(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Deleted != 0 || !report.Checkpointed || !report.NeedsCleanup() {
		t.Fatalf("unexpected WAL-only report: deleted=%d checkpointed=%v needsCleanup=%v",
			report.Deleted, report.Checkpointed, report.NeedsCleanup())
	}
	if countCallsContaining(runner.calls, "session delete") != 0 || countCallsContaining(runner.calls, "VACUUM") != 0 {
		t.Fatalf("unexpected session delete/vacuum in WAL-only cleanup: %v", runner.calls)
	}
	if countCallsContaining(runner.calls, "wal_checkpoint(TRUNCATE)") != 1 {
		t.Fatalf("TRUNCATE checkpoint not called exactly once: %v", runner.calls)
	}
}

func TestFixRechecksProtectedSessionsBeforeDelete(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	townRoot := t.TempDir()
	dbPath := makeOpenCodeFiles(t, 1, 0)
	runner := newFakeRunner()
	opts := Options{TownRoot: townRoot, Now: now, RetentionAge: 24 * time.Hour, KeepPerDir: 1, Runner: runner, MinDeleteHeadroomBytes: 1}
	addAnalyzeOutputs(t, runner, dbPath, opts, nil, retentionStats{total: 1, eligible: 1}, []Session{
		{ID: "became-live", Directory: "/repo", TimeUpdated: now.Add(-48 * time.Hour).UnixMilli()},
	})
	addCheckpointOutputs(runner)
	opts.DiskInfo = func(path string) (*util.DiskSpaceInfo, error) {
		if err := os.MkdirAll(filepath.Join(townRoot, ".runtime"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(townRoot, ".runtime", "session_id"), []byte("became-live\n"), 0644); err != nil {
			t.Fatal(err)
		}
		return &util.DiskSpaceInfo{AvailableBytes: 10 * 1024 * 1024}, nil
	}
	opts.TryLock = func(path string) (func(), bool, error) { return func() {}, true, nil }

	report, err := Fix(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Deleted != 0 || report.Protected != 1 {
		t.Fatalf("unexpected report after protected recheck: deleted=%d protected=%d", report.Deleted, report.Protected)
	}
	if countCallsContaining(runner.calls, "session delete") != 0 {
		t.Fatalf("deleted newly protected session: %v", runner.calls)
	}
}

func TestAnalyzeFailsClosedOnInvalidCandidateScan(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	dbPath := makeOpenCodeFiles(t, 1, 0)
	runner := newFakeRunner()
	opts := Options{Now: now, Runner: runner}
	addAnalyzeOutputs(t, runner, dbPath, opts, nil, retentionStats{total: 1, eligible: 1}, nil)
	opts = opts.withDefaults()
	cutoff := opts.Now.Add(-opts.RetentionAge).UnixMilli()
	runner.outputs[opencodeCall("--pure", "db", sessionCandidateSQL(cutoff, opts.KeepPerDir, opts.MaxDeletes, nil), "--format", "json")] = []byte(`[{"id":"","directory":"/repo","time_updated":1}]`)

	report, err := Analyze(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "empty id") {
		t.Fatalf("Analyze error = %v, want empty id", err)
	}
	if report == nil || report.Deleted != 0 {
		t.Fatalf("unexpected report after invalid scan: %#v", report)
	}
	if hasMutation(runner.calls) {
		t.Fatalf("destructive command ran after invalid scan: %v", runner.calls)
	}
}

func TestAnalyzeMapsMissingOpenCodeToUnavailable(t *testing.T) {
	runner := newFakeRunner()
	runner.errors[opencodeCall("--pure", "db", "path")] = exec.ErrNotFound

	_, err := Analyze(context.Background(), Options{Runner: runner})
	if !errors.Is(err, ErrOpenCodeUnavailable) {
		t.Fatalf("Analyze error = %v, want ErrOpenCodeUnavailable", err)
	}
}
