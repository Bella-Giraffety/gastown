//go:build integration

package cmd

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/testutil"
)

var healthTestCounter atomic.Int32

func TestRunHealthJSONIncludesVisibleSharedServerDatabases(t *testing.T) {
	requireDoltServer(t)

	port := testutil.DoltContainerPort()
	t.Setenv("GT_DOLT_PORT", port)
	t.Setenv("BEADS_DOLT_PORT", port)

	n := healthTestCounter.Add(1)
	hqPrefix := fmt.Sprintf("hhq%d", n)
	gtPrefix := fmt.Sprintf("hgt%d", n)
	trPrefix := fmt.Sprintf("htr%d", n)

	townRoot := setupRoutingTestTownWithPrefixes(t, hqPrefix, gtPrefix, trPrefix)
	bridgeDoltPidToTown(t, townRoot)
	writeHealthTestState(t, townRoot, port)
	t.Setenv("GT_TOWN_ROOT", townRoot)

	initBeadsDBWithPrefix(t, townRoot, hqPrefix)
	gastownRigPath := filepath.Join(townRoot, "gastown", "mayor", "rig")
	testrigRigPath := filepath.Join(townRoot, "testrig", "mayor", "rig")
	initBeadsDBWithPrefix(t, gastownRigPath, gtPrefix)
	initBeadsDBWithPrefix(t, testrigRigPath, trPrefix)

	wantDatabases := []string{
		readHealthTestDatabaseName(t, filepath.Join(townRoot, ".beads", "metadata.json")),
		readHealthTestDatabaseName(t, filepath.Join(gastownRigPath, ".beads", "metadata.json")),
		readHealthTestDatabaseName(t, filepath.Join(testrigRigPath, ".beads", "metadata.json")),
	}

	visibleDatabases := queryHealthTestDatabases(t, port)
	visibleSet := make(map[string]bool, len(visibleDatabases))
	for _, name := range visibleDatabases {
		visibleSet[name] = true
	}
	for _, want := range wantDatabases {
		if !visibleSet[want] {
			t.Fatalf("SHOW DATABASES missing %q; visible=%v", want, visibleDatabases)
		}
	}

	oldHealthJSON := healthJSON
	healthJSON = true
	t.Cleanup(func() { healthJSON = oldHealthJSON })

	output := captureHealthTestStdout(t, func() {
		if err := runHealth(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runHealth: %v", err)
		}
	})

	var report HealthReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("parse health JSON: %v\noutput:\n%s", err, output)
	}

	reportedSet := make(map[string]bool, len(report.Databases))
	for _, db := range report.Databases {
		reportedSet[db.Name] = true
	}
	for _, want := range wantDatabases {
		if !reportedSet[want] {
			t.Fatalf("health report missing visible database %q; reported=%v", want, report.Databases)
		}
	}
	if len(report.Databases) < len(wantDatabases) {
		t.Fatalf("expected at least %d reported databases, got %d", len(wantDatabases), len(report.Databases))
	}
	if !report.Server.Running {
		t.Fatal("expected health report to mark Dolt server as running")
	}
	if report.Server.Port == 0 {
		t.Fatal("expected health report to include Dolt port")
	}
}

func writeHealthTestState(t *testing.T, townRoot, port string) {
	t.Helper()

	daemonDir := filepath.Join(townRoot, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatalf("mkdir daemon: %v", err)
	}
	state := doltserver.State{Port: mustAtoi(t, port)}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(daemonDir, "dolt-state.json"), data, 0644); err != nil {
		t.Fatalf("write dolt-state.json: %v", err)
	}
}

func readHealthTestDatabaseName(t *testing.T, metadataPath string) string {
	t.Helper()

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read metadata %s: %v", metadataPath, err)
	}
	var metadata struct {
		Database string `json:"dolt_database"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("parse metadata %s: %v", metadataPath, err)
	}
	if metadata.Database == "" {
		t.Fatalf("metadata %s missing dolt_database", metadataPath)
	}
	return metadata.Database
}

func queryHealthTestDatabases(t *testing.T, port string) []string {
	t.Helper()

	dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%s)/?timeout=5s&readTimeout=10s", port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open mysql connection: %v", err)
	}
	defer db.Close()

	rows, err := db.Query("SHOW DATABASES")
	if err != nil {
		t.Fatalf("SHOW DATABASES: %v", err)
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan database name: %v", err)
		}
		if doltserver.IsSystemDatabase(name) {
			continue
		}
		databases = append(databases, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate databases: %v", err)
	}
	return databases
}

func captureHealthTestStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = originalStdout }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	var captured bytes.Buffer
	if _, err := io.Copy(&captured, r); err != nil {
		t.Fatalf("copy stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	return captured.String()
}

func mustAtoi(t *testing.T, value string) int {
	t.Helper()

	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
		t.Fatalf("parse int %q: %v", value, err)
	}
	return parsed
}
