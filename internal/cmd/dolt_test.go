package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/doltserver"
)

func TestDirSizeHuman(t *testing.T) {
	dir := t.TempDir()

	// Empty directory
	got := dirSizeHuman(dir)
	if got != "0 B" {
		t.Errorf("empty dir: got %q, want %q", got, "0 B")
	}

	// Write a 1024-byte file
	data := make([]byte, 1024)
	if err := os.WriteFile(filepath.Join(dir, "file.dat"), data, 0644); err != nil {
		t.Fatal(err)
	}
	got = dirSizeHuman(dir)
	if got != "1.0 KB" {
		t.Errorf("1KB file: got %q, want %q", got, "1.0 KB")
	}

	// Add a subdirectory with another file
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	data2 := make([]byte, 512)
	if err := os.WriteFile(filepath.Join(subDir, "nested.dat"), data2, 0644); err != nil {
		t.Fatal(err)
	}
	got = dirSizeHuman(dir)
	if got != "1.5 KB" {
		t.Errorf("1.5KB total: got %q, want %q", got, "1.5 KB")
	}
}

func TestDirSizeHuman_NonexistentDir(t *testing.T) {
	got := dirSizeHuman("/nonexistent/path/that/does/not/exist")
	if got != "0 B" {
		t.Errorf("nonexistent dir: got %q, want %q", got, "0 B")
	}
}

func TestRunDoltRestart_StopsIdleMonitorsBeforeStop(t *testing.T) {
	townRoot := t.TempDir()
	var calls []string

	origFindTownRootForDoltCommand := findTownRootForDoltCommand
	origDoltDefaultConfigFn := doltDefaultConfigFn
	origDoltIsRunningFn := doltIsRunningFn
	origDoltStopFn := doltStopFn
	origDoltStopIdleMonitorsFn := doltStopIdleMonitorsFn
	origDoltKillImpostersFn := doltKillImpostersFn
	origDoltListDatabasesFn := doltListDatabasesFn
	origDoltStartFn := doltStartFn
	origDoltLoadStateFn := doltLoadStateFn
	origDoltVerifyDatabasesWithRetryFn := doltVerifyDatabasesWithRetryFn
	origDoltRestartSleepFn := doltRestartSleepFn
	defer func() {
		findTownRootForDoltCommand = origFindTownRootForDoltCommand
		doltDefaultConfigFn = origDoltDefaultConfigFn
		doltIsRunningFn = origDoltIsRunningFn
		doltStopFn = origDoltStopFn
		doltStopIdleMonitorsFn = origDoltStopIdleMonitorsFn
		doltKillImpostersFn = origDoltKillImpostersFn
		doltListDatabasesFn = origDoltListDatabasesFn
		doltStartFn = origDoltStartFn
		doltLoadStateFn = origDoltLoadStateFn
		doltVerifyDatabasesWithRetryFn = origDoltVerifyDatabasesWithRetryFn
		doltRestartSleepFn = origDoltRestartSleepFn
	}()

	findTownRootForDoltCommand = func() (string, error) {
		return townRoot, nil
	}
	doltDefaultConfigFn = func(string) *doltserver.Config {
		return &doltserver.Config{DataDir: filepath.Join(townRoot, ".dolt-data"), Port: 3307}
	}
	doltStopIdleMonitorsFn = func(string) int {
		calls = append(calls, "stop-idle-monitors")
		return 1
	}
	doltIsRunningFn = func(string) (bool, int, error) {
		calls = append(calls, "is-running")
		return true, 4242, nil
	}
	doltStopFn = func(string) error {
		calls = append(calls, "stop")
		return nil
	}
	doltKillImpostersFn = func(string) error {
		calls = append(calls, "kill-imposters")
		return nil
	}
	doltListDatabasesFn = func(string) ([]string, error) {
		calls = append(calls, "list-databases")
		return []string{"gastown"}, nil
	}
	doltStartFn = func(string) error {
		calls = append(calls, "start")
		return nil
	}
	doltLoadStateFn = func(string) (*doltserver.State, error) {
		calls = append(calls, "load-state")
		return &doltserver.State{PID: 4343, DataDir: filepath.Join(townRoot, ".dolt-data"), Databases: []string{"gastown"}}, nil
	}
	doltVerifyDatabasesWithRetryFn = func(string, int) ([]string, []string, error) {
		calls = append(calls, "verify-databases")
		return []string{"gastown"}, nil, nil
	}
	doltRestartSleepFn = func(time.Duration) {
		calls = append(calls, "sleep")
	}

	if err := runDoltRestart(nil, nil); err != nil {
		t.Fatalf("runDoltRestart returned error: %v", err)
	}

	want := []string{
		"stop-idle-monitors",
		"sleep",
		"is-running",
		"stop",
		"kill-imposters",
		"sleep",
		"list-databases",
		"start",
		"load-state",
		"verify-databases",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("call order = %v, want %v", calls, want)
	}
}
