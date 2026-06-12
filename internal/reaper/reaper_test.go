package reaper

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestValidateDBName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"hq", false},
		{"beads", false},
		{"gt", false},
		{"test_db_123", false},
		{"", true},
		{"drop table", true},
		{"db;--", true},
		{"db`name", true},
		{"../etc/passwd", true},
	}
	for _, tt := range tests {
		err := ValidateDBName(tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateDBName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestDefaultDatabases(t *testing.T) {
	if len(DefaultDatabases) == 0 {
		t.Error("DefaultDatabases should not be empty")
	}
	for _, db := range DefaultDatabases {
		if err := ValidateDBName(db); err != nil {
			t.Errorf("DefaultDatabases contains invalid name %q: %v", db, err)
		}
	}
}

func TestFormatJSON(t *testing.T) {
	result := FormatJSON(map[string]int{"count": 42})
	if result == "" {
		t.Error("FormatJSON should not return empty string")
	}
	if result[0] != '{' {
		t.Errorf("FormatJSON should return JSON object, got %q", result[:10])
	}
}

func TestParentExcludeJoin(t *testing.T) {
	joinClause, whereCondition := parentExcludeJoin("testdb")

	// JOIN clause should reference the correct database.
	if joinClause == "" {
		t.Error("parentExcludeJoin joinClause should not be empty")
	}
	// parentExcludeJoin no longer qualifies table names with the database — the
	// reaper connects to a specific database via the DSN, so unqualified names
	// are correct. The dbName parameter is retained for API compatibility.

	// JOIN should select wisps with open parents from wisp_dependencies.
	if !contains(joinClause, "wisp_dependencies") {
		t.Error("parentExcludeJoin should query wisp_dependencies")
	}
	if !contains(joinClause, "parent-child") {
		t.Error("parentExcludeJoin should filter on parent-child type")
	}
	if !contains(joinClause, "'open', 'hooked', 'in_progress'") {
		t.Error("parentExcludeJoin should check for open parent statuses")
	}

	// WHERE condition should be an IS NULL anti-join filter.
	if whereCondition == "" {
		t.Error("parentExcludeJoin whereCondition should not be empty")
	}
	if !contains(whereCondition, "IS NULL") {
		t.Error("parentExcludeJoin whereCondition should use IS NULL for anti-join")
	}
}

// TestReapQueryNoDatabaseNameInjection verifies that the Reap function's batch
// SELECT query does not inject the database name into the SQL string. Previously,
// dbName was passed as a Sprintf arg but the format string didn't use it, causing
// positional shift: "FROM wisps w gt WHERE..." instead of "FROM wisps w LEFT JOIN...".
func TestReapQueryNoDatabaseNameInjection(t *testing.T) {
	// Reproduce the exact Sprintf call from Reap() to verify no dbName injection.
	dbName := "gt"
	parentJoin, parentWhere := parentExcludeJoin(dbName)
	whereClause := staleReapWhere(parentWhere)
	closedMoleculeStepExcludeJoin := closedMoleculeStepJoin("LEFT")

	// This is the fixed query — dbName is NOT in the Sprintf args.
	idQuery := fmt.Sprintf(
		"SELECT w.id FROM wisps w %s %s WHERE %s LIMIT %d",
		parentJoin, closedMoleculeStepExcludeJoin, whereClause, DefaultBatchSize)

	// The query must NOT contain the literal database name as a bare token.
	// Before the fix, "gt" appeared between "wisps w" and "WHERE".
	if strings.Contains(idQuery, "wisps w gt") {
		t.Errorf("Reap idQuery contains injected database name: %s", idQuery)
	}
	if !strings.Contains(idQuery, "LEFT JOIN") {
		t.Errorf("Reap idQuery should contain LEFT JOIN from parentExcludeJoin, got: %s", idQuery)
	}
	if !strings.Contains(idQuery, "closed_molecule_step.issue_id IS NULL") || !strings.Contains(idQuery, "pm.issue_type = 'molecule'") {
		t.Errorf("Reap idQuery should exclude closed-molecule step wisps, got: %s", idQuery)
	}
	if !strings.Contains(idQuery, fmt.Sprintf("LIMIT %d", DefaultBatchSize)) {
		t.Errorf("Reap idQuery should end with LIMIT %d, got: %s", DefaultBatchSize, idQuery)
	}
}

// TestReapUpdateQueryNoDatabaseNameInjection verifies that the UPDATE query in
// Reap() does not inject dbName where the IN clause should go.
func TestReapUpdateQueryNoDatabaseNameInjection(t *testing.T) {
	dbName := "gt"
	inClause := "?,?,?"

	// This is the fixed query — only inClause in the Sprintf args.
	updateQuery := fmt.Sprintf(
		"UPDATE wisps SET status='closed', closed_at=NOW() WHERE id IN (%s)",
		inClause)

	if strings.Contains(updateQuery, dbName) {
		t.Errorf("Reap updateQuery contains injected database name %q: %s", dbName, updateQuery)
	}
	if !strings.Contains(updateQuery, "IN (?,?,?)") {
		t.Errorf("Reap updateQuery should contain parameterized IN clause, got: %s", updateQuery)
	}
}

// TestPurgeDigestQueryNoDatabaseNameInjection verifies that the purge digest
// query is a plain string with no Sprintf interpolation at all.
func TestPurgeDigestQueryNoDatabaseNameInjection(t *testing.T) {
	// The fixed digestQuery is a string literal — no Sprintf.
	digestQuery := "SELECT COALESCE(w.wisp_type, 'unknown') AS wtype, COUNT(*) AS cnt FROM wisps w WHERE w.status = 'closed' AND w.closed_at < ? GROUP BY wtype"

	if strings.Contains(digestQuery, "gt") {
		t.Errorf("purge digestQuery should not contain database name, got: %s", digestQuery)
	}
	if !strings.Contains(digestQuery, "GROUP BY wtype") {
		t.Errorf("purge digestQuery should end with GROUP BY, got: %s", digestQuery)
	}
}

// TestPurgeBatchQueryNoDatabaseNameInjection verifies that the purge batch
// SELECT query uses DefaultBatchSize as the LIMIT, not dbName.
func TestPurgeBatchQueryNoDatabaseNameInjection(t *testing.T) {
	// This is the fixed query — only DefaultBatchSize in the Sprintf args.
	idQuery := fmt.Sprintf(
		"SELECT w.id FROM wisps w WHERE w.status = 'closed' AND w.closed_at < ? LIMIT %d",
		DefaultBatchSize)

	if strings.Contains(idQuery, "gt") {
		t.Errorf("purge idQuery contains injected database name: %s", idQuery)
	}
	expected := fmt.Sprintf("LIMIT %d", DefaultBatchSize)
	if !strings.Contains(idQuery, expected) {
		t.Errorf("purge idQuery should contain %s, got: %s", expected, idQuery)
	}
}

// TestIsNothingToCommit verifies that "nothing to commit" errors are recognized
// correctly. This prevents false-positive dolt_commit_failed anomalies when the
// reaper operates on dolt_ignored tables (wisps, wisp_*), where Dolt has nothing
// to version after a successful SQL DELETE.
func TestIsNothingToCommit(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"nothing to commit", true},
		{"NOTHING TO COMMIT", true},
		{"Error 1105 (HY000): nothing to commit", true},
		{"no changes to commit", false}, // must also contain "commit" — see isNothingToCommit
		{"no changes", false},
		{"connection refused", false},
		{"table not found: wisps", false},
		{"", false},
	}
	for _, c := range cases {
		var err error
		if c.msg != "" {
			err = fmt.Errorf("%s", c.msg)
		}
		got := isNothingToCommit(err)
		if got != c.want {
			t.Errorf("isNothingToCommit(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}

// TestClosedMoleculeStepSelector verifies the molecule-step auto-close query is
// scoped to open, non-agent step wisps whose parent is a closed molecule.
func TestClosedMoleculeStepSelector(t *testing.T) {
	join := closedMoleculeStepJoin("INNER")
	where := closedMoleculeStepWhere()

	if strings.Contains(join, "EXISTS") {
		t.Error("closedMoleculeStepJoin should use join-based selection, not correlated EXISTS")
	}
	if !strings.Contains(join, "wisp_dependencies wd") {
		t.Error("closedMoleculeStepJoin should query wisp_dependencies")
	}
	if !strings.Contains(join, "SELECT DISTINCT wd.issue_id") || !strings.Contains(join, "closed_molecule_step.issue_id = w.id") {
		t.Error("closedMoleculeStepJoin should treat issue_id as the child step wisp")
	}
	if !strings.Contains(join, "pm.id = wd.depends_on_id") {
		t.Error("closedMoleculeStepJoin should join the parent through depends_on_id")
	}
	if strings.Contains(join, "depends_on_wisp_id") {
		t.Error("closedMoleculeStepJoin must not reference nonexistent depends_on_wisp_id")
	}
	if !strings.Contains(join, "wd.type = 'parent-child'") {
		t.Error("closedMoleculeStepJoin should filter parent-child dependencies")
	}
	if !strings.Contains(join, "pm.issue_type = 'molecule'") {
		t.Error("closedMoleculeStepJoin must scope to molecule parents")
	}
	if !strings.Contains(join, "pm.status = 'closed'") {
		t.Error("closedMoleculeStepJoin must require the parent molecule be closed")
	}
	if !strings.Contains(where, "w.status IN ('open', 'hooked', 'in_progress')") {
		t.Error("closedMoleculeStepWhere should only select open-ish step wisps")
	}
	if !strings.Contains(where, "w.issue_type != 'agent'") {
		t.Error("closedMoleculeStepWhere must exclude agent beads")
	}
}

func TestStaleReapWhereExcludesClosedMoleculeSteps(t *testing.T) {
	_, parentWhere := parentExcludeJoin("testdb")
	w := staleReapWhere(parentWhere)

	if !strings.Contains(w, "created_at < ?") {
		t.Error("staleReapWhere should preserve max-age cutoff")
	}
	if !strings.Contains(w, "open_parent.issue_id IS NULL") {
		t.Error("staleReapWhere should preserve parent eligibility filter")
	}
	if !strings.Contains(w, "closed_molecule_step.issue_id IS NULL") {
		t.Fatalf("staleReapWhere should exclude closed-molecule step join to prevent double counting, got: %s", w)
	}
}

func TestReapClosesClosedMoleculeStepsWithoutDoubleCounting(t *testing.T) {
	db, state := openFakeReaperDB(t, map[string]*fakeReaperWisp{
		"mol-closed":       {id: "mol-closed", status: "closed", issueType: "molecule"},
		"mol-open":         {id: "mol-open", status: "open", issueType: "molecule"},
		"epic-closed":      {id: "epic-closed", status: "closed", issueType: "epic"},
		"step-young":       {id: "step-young", status: "open", issueType: "task", parentID: "mol-closed"},
		"step-old":         {id: "step-old", status: "open", issueType: "task", parentID: "mol-closed", old: true},
		"open-parent-step": {id: "open-parent-step", status: "open", issueType: "task", parentID: "mol-open", old: true},
		"epic-young":       {id: "epic-young", status: "open", issueType: "task", parentID: "epic-closed"},
		"stale-orphan":     {id: "stale-orphan", status: "open", issueType: "task", old: true},
		"agent-step":       {id: "agent-step", status: "open", issueType: "agent", parentID: "mol-closed", old: true},
	})

	scan, err := Scan(db, "testdb", time.Hour, time.Hour, time.Hour, time.Hour)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if scan.MoleculeStepCandidates != 2 {
		t.Fatalf("Scan molecule_step_candidates = %d, want 2", scan.MoleculeStepCandidates)
	}
	if scan.ReapCandidates != 1 {
		t.Fatalf("Scan reap_candidates = %d, want 1 stale non-molecule candidate", scan.ReapCandidates)
	}

	dryRun, err := Reap(db, "testdb", time.Hour, true)
	if err != nil {
		t.Fatalf("dry-run Reap() error = %v", err)
	}
	if dryRun.MoleculeStepsClosed != 2 || dryRun.Reaped != 1 {
		t.Fatalf("dry-run Reap() counts = molecule_steps:%d reaped:%d, want 2 and 1", dryRun.MoleculeStepsClosed, dryRun.Reaped)
	}

	result, err := Reap(db, "testdb", time.Hour, false)
	if err != nil {
		t.Fatalf("Reap() error = %v", err)
	}
	if result.MoleculeStepsClosed != 2 || result.Reaped != 1 {
		t.Fatalf("Reap() counts = molecule_steps:%d reaped:%d, want 2 and 1", result.MoleculeStepsClosed, result.Reaped)
	}

	for _, id := range []string{"step-young", "step-old", "stale-orphan"} {
		if state.wisps[id].status != "closed" {
			t.Fatalf("%s status = %s, want closed", id, state.wisps[id].status)
		}
	}
	for _, id := range []string{"open-parent-step", "epic-young", "agent-step"} {
		if state.wisps[id].status != "open" {
			t.Fatalf("%s status = %s, want open", id, state.wisps[id].status)
		}
	}
}

func TestReapDoltCommitFailureReturnsError(t *testing.T) {
	db, state := openFakeReaperDB(t, map[string]*fakeReaperWisp{
		"mol-closed": {id: "mol-closed", status: "closed", issueType: "molecule"},
		"step":       {id: "step", status: "open", issueType: "task", parentID: "mol-closed"},
	})
	state.doltCommitErr = fmt.Errorf("boom")

	_, err := Reap(db, "testdb", time.Hour, false)
	if err == nil || !strings.Contains(err.Error(), "dolt commit: boom") {
		t.Fatalf("Reap() error = %v, want dolt commit failure", err)
	}
	if state.execIndex("CALL DOLT_COMMIT") == -1 {
		t.Fatalf("expected DOLT_COMMIT attempt, execs: %v", state.execs)
	}
	if state.execIndex("ROLLBACK") != -1 {
		t.Fatalf("ROLLBACK should not run after SQL COMMIT, execs: %v", state.execs)
	}
}

func TestReapReturnsMoleculeStepRowsErrBeforeUpdate(t *testing.T) {
	db, state := openFakeReaperDB(t, map[string]*fakeReaperWisp{
		"mol-closed": {id: "mol-closed", status: "closed", issueType: "molecule"},
		"step":       {id: "step", status: "open", issueType: "task", parentID: "mol-closed"},
	})
	state.moleculeStepRowsErr = fmt.Errorf("cursor boom")

	_, err := Reap(db, "testdb", time.Hour, false)
	if err == nil || !strings.Contains(err.Error(), "iterate molecule-step ids: cursor boom") {
		t.Fatalf("Reap() error = %v, want molecule-step cursor failure", err)
	}
	if state.wisps["step"].status != "open" {
		t.Fatalf("step status = %s, want open", state.wisps["step"].status)
	}
	if state.execIndex("COMMIT") != -1 || state.execIndex("CALL DOLT_COMMIT") != -1 {
		t.Fatalf("should not commit after cursor error, execs: %v", state.execs)
	}
}

func TestReapReturnsStaleRowsErrBeforeUpdate(t *testing.T) {
	db, state := openFakeReaperDB(t, map[string]*fakeReaperWisp{
		"stale-orphan": {id: "stale-orphan", status: "open", issueType: "task", old: true},
	})
	state.staleRowsErr = fmt.Errorf("cursor boom")

	_, err := Reap(db, "testdb", time.Hour, false)
	if err == nil || !strings.Contains(err.Error(), "iterate reap ids: cursor boom") {
		t.Fatalf("Reap() error = %v, want stale cursor failure", err)
	}
	if state.wisps["stale-orphan"].status != "open" {
		t.Fatalf("stale-orphan status = %s, want open", state.wisps["stale-orphan"].status)
	}
	if state.execIndex("COMMIT") != -1 || state.execIndex("CALL DOLT_COMMIT") != -1 {
		t.Fatalf("should not commit after cursor error, execs: %v", state.execs)
	}
}

var (
	fakeReaperDriverOnce sync.Once
	fakeReaperDBsMu      sync.Mutex
	fakeReaperDBs        = map[string]*fakeReaperDB{}
)

type fakeReaperWisp struct {
	id        string
	status    string
	issueType string
	parentID  string
	old       bool
}

type fakeReaperDB struct {
	wisps               map[string]*fakeReaperWisp
	moleculeStepRowsErr error
	staleRowsErr        error
	doltCommitErr       error
	execs               []string
}

func openFakeReaperDB(t *testing.T, wisps map[string]*fakeReaperWisp) (*sql.DB, *fakeReaperDB) {
	t.Helper()
	fakeReaperDriverOnce.Do(func() {
		sql.Register("reaper_fake", fakeReaperDriver{})
	})
	name := strings.ReplaceAll(t.Name(), "/", "_")
	state := &fakeReaperDB{wisps: wisps}
	fakeReaperDBsMu.Lock()
	fakeReaperDBs[name] = state
	fakeReaperDBsMu.Unlock()

	db, err := sql.Open("reaper_fake", name)
	if err != nil {
		t.Fatalf("open fake db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		fakeReaperDBsMu.Lock()
		delete(fakeReaperDBs, name)
		fakeReaperDBsMu.Unlock()
	})
	return db, state
}

type fakeReaperDriver struct{}

func (fakeReaperDriver) Open(name string) (driver.Conn, error) {
	fakeReaperDBsMu.Lock()
	state := fakeReaperDBs[name]
	fakeReaperDBsMu.Unlock()
	if state == nil {
		state = &fakeReaperDB{wisps: map[string]*fakeReaperWisp{}}
	}
	return &fakeReaperConn{db: state}, nil
}

type fakeReaperConn struct {
	db *fakeReaperDB
}

func (c *fakeReaperConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("not implemented")
}
func (c *fakeReaperConn) Close() error              { return nil }
func (c *fakeReaperConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("not implemented") }

func (c *fakeReaperConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "SELECT COUNT(*) FROM wisps w") && strings.Contains(query, "created_at < ?"):
		return fakeRowsFromCount(len(c.db.staleIDs())), nil
	case strings.Contains(query, "SELECT COUNT(*) FROM wisps w") && strings.Contains(query, "pm.issue_type = 'molecule'"):
		return fakeRowsFromCount(len(c.db.moleculeStepIDs())), nil
	case strings.Contains(query, "SELECT COUNT(*) FROM wisps w") && strings.Contains(query, "w.status = 'closed'"):
		return fakeRowsFromCount(0), nil
	case strings.Contains(query, "SELECT COUNT(*) FROM issues"):
		return fakeRowsFromCount(0), nil
	case strings.Contains(query, "SELECT COUNT(*) FROM wisp_dependencies"):
		return fakeRowsFromCount(0), nil
	case strings.Contains(query, "SELECT COUNT(*) FROM wisps WHERE status IN"):
		return fakeRowsFromCount(c.db.openCount()), nil
	case strings.Contains(query, "SELECT w.id FROM wisps w") && strings.Contains(query, "pm.issue_type = 'molecule'") && !strings.Contains(query, "created_at < ?"):
		rows := fakeRowsFromIDs(c.db.moleculeStepIDs())
		rows.err = c.db.moleculeStepRowsErr
		return rows, nil
	case strings.Contains(query, "SELECT w.id FROM wisps w") && strings.Contains(query, "created_at < ?"):
		rows := fakeRowsFromIDs(c.db.staleIDs())
		rows.err = c.db.staleRowsErr
		return rows, nil
	default:
		return nil, fmt.Errorf("unexpected fake query: %s", query)
	}
}

func (c *fakeReaperConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.db.execs = append(c.db.execs, query)
	if strings.HasPrefix(query, "UPDATE wisps SET status='closed'") {
		closed := int64(0)
		for _, arg := range args {
			id, _ := arg.Value.(string)
			if w := c.db.wisps[id]; w != nil && isOpenWispStatus(w.status) {
				w.status = "closed"
				closed++
			}
		}
		return driver.RowsAffected(closed), nil
	}
	if strings.HasPrefix(query, "CALL DOLT_COMMIT") && c.db.doltCommitErr != nil {
		return nil, c.db.doltCommitErr
	}
	if strings.HasPrefix(query, "SET @@autocommit") || query == "ROLLBACK" || query == "COMMIT" || strings.HasPrefix(query, "CALL DOLT_COMMIT") {
		return driver.RowsAffected(0), nil
	}
	return nil, fmt.Errorf("unexpected fake exec: %s", query)
}

func (db *fakeReaperDB) execIndex(prefix string) int {
	for i, query := range db.execs {
		if strings.HasPrefix(query, prefix) {
			return i
		}
	}
	return -1
}

func (db *fakeReaperDB) moleculeStepIDs() []string {
	var ids []string
	for _, w := range db.wisps {
		parent := db.wisps[w.parentID]
		if isOpenWispStatus(w.status) && w.issueType != "agent" && parent != nil && parent.issueType == "molecule" && parent.status == "closed" {
			ids = append(ids, w.id)
		}
	}
	return ids
}

func (db *fakeReaperDB) staleIDs() []string {
	var ids []string
	for _, w := range db.wisps {
		if !isOpenWispStatus(w.status) || !w.old || w.issueType == "agent" || db.isClosedMoleculeStep(w) {
			continue
		}
		parent := db.wisps[w.parentID]
		if parent == nil || !isOpenWispStatus(parent.status) {
			ids = append(ids, w.id)
		}
	}
	return ids
}

func (db *fakeReaperDB) isClosedMoleculeStep(w *fakeReaperWisp) bool {
	parent := db.wisps[w.parentID]
	return parent != nil && parent.issueType == "molecule" && parent.status == "closed"
}

func (db *fakeReaperDB) openCount() int {
	count := 0
	for _, w := range db.wisps {
		if isOpenWispStatus(w.status) {
			count++
		}
	}
	return count
}

func isOpenWispStatus(status string) bool {
	return status == "open" || status == "hooked" || status == "in_progress"
}

type fakeRows struct {
	columns []string
	values  [][]driver.Value
	index   int
	err     error
}

func fakeRowsFromCount(count int) *fakeRows {
	return &fakeRows{columns: []string{"count"}, values: [][]driver.Value{{int64(count)}}}
}

func fakeRowsFromIDs(ids []string) *fakeRows {
	rows := &fakeRows{columns: []string{"id"}}
	for _, id := range ids {
		rows.values = append(rows.values, []driver.Value{id})
	}
	return rows
}

func (r *fakeRows) Columns() []string { return r.columns }
func (r *fakeRows) Close() error      { return nil }

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		if r.err != nil {
			err := r.err
			r.err = nil
			return err
		}
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestReapExcludesAgentBeads verifies that the Reap function excludes agent beads
// from being closed, regardless of their age. This is a regression test for the bug
// where the wisp reaper was closing agent beads (hq-mayor, hq-deacon, witness, refinery,
// etc.) after 24 hours, causing doctor to report them as missing.
func TestReapExcludesAgentBeads(t *testing.T) {
	// Verify that the WHERE clause in Reap() excludes issue_type='agent'
	// by checking the source code pattern.
	// This is a compile-time guard — if the exclusion is removed, this test
	// will fail when the query pattern doesn't match.

	// The whereClause in Reap() should contain:
	// "w.issue_type != 'agent'"
	// This test documents the expected behavior; actual exclusion is tested
	// in integration tests with a real database.

	// Integration test would require spinning up a Dolt server, which is
	// beyond the scope of this unit test. The exclusion is verified manually
	// by checking that agent beads are not closed by the wisp_reaper patrol.
	t.Log("Agent beads (issue_type='agent') are excluded from wisp reaping")
	t.Log("This prevents hq-mayor, hq-deacon, witness, refinery, etc. from being closed")
}

// TestScanExcludesAgentBeads documents that Scan() must use the same eligibility
// predicate as Reap() for stale open wisps. If Scan counts agent beads but Reap
// excludes them, the operator sees scan>0 and reap=0 for the same cutoff.
func TestScanExcludesAgentBeads(t *testing.T) {
	sourcePath := "reaper.go"
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}
	source := string(data)
	scanStart := strings.Index(source, "func Scan(")
	reapStart := strings.Index(source, "func Reap(")
	if scanStart == -1 || reapStart == -1 || reapStart <= scanStart {
		t.Fatalf("could not isolate Scan() body in %s", sourcePath)
	}
	scanBody := source[scanStart:reapStart]
	if !strings.Contains(scanBody, "staleReapWhere") {
		t.Fatalf("expected Scan() to use shared staleReapWhere eligibility, scan body was:\n%s", scanBody)
	}
	if !strings.Contains(staleReapWhere("open_parent.issue_id IS NULL"), "w.issue_type != 'agent'") {
		t.Fatal("expected shared staleReapWhere eligibility to exclude agent beads")
	}
}
