//go:build !windows

package util

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSetProcessGroupKillsChildrenOnCancel(t *testing.T) {
	childPID := runShellWithBackgroundChild(t, SetProcessGroup)

	if !waitForProcessExit(childPID, 2*time.Second) {
		t.Fatalf("child PID %d still running after process-group cancellation", childPID)
	}
}

func TestSetDetachedProcessGroupDoesNotKillChildrenOnCancel(t *testing.T) {
	childPID := runShellWithBackgroundChild(t, SetDetachedProcessGroup)
	defer func() { _ = syscall.Kill(childPID, syscall.SIGKILL) }()

	if !processRunning(childPID) {
		t.Skipf("child PID %d exited before assertion", childPID)
	}
}

func runShellWithBackgroundChild(t *testing.T, configure func(*exec.Cmd)) int {
	t.Helper()

	tmp := t.TempDir()
	pidFile := filepath.Join(tmp, "child.pid")
	script := fmt.Sprintf("sleep 600 & echo $! > %s; wait", pidFile)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	configure(cmd)
	cmd.WaitDelay = time.Second

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start shell: %v", err)
	}

	childPID := waitForPIDFile(t, pidFile, 2*time.Second)
	if !processRunning(childPID) {
		cancel()
		_ = cmd.Wait()
		t.Fatalf("child PID %d was not running before cancellation", childPID)
	}

	cancel()
	_ = cmd.Wait()

	return childPID
}

func waitForPIDFile(t *testing.T, path string, timeout time.Duration) int {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatalf("parse pid file %q: %v", path, err)
			}
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("pid file %q did not appear within %s", path, timeout)
	return 0
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processRunning(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return !processRunning(pid)
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return false
	} else if err != nil {
		return true
	}

	status, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err == nil && strings.HasPrefix(strings.TrimSpace(string(status)), "Z") {
		return false
	}
	return true
}
