//go:build linux

package nudge

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

func pollerProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	return proc.Signal(syscall.Signal(0)) == nil
}

func pollerProcessMatches(pid int, session string) (bool, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false, false
	}
	parts := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "nudge-poller" && parts[i+1] == session {
			return true, true
		}
	}
	return false, true
}
