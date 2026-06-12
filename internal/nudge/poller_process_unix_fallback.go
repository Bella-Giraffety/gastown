//go:build !linux && !windows

package nudge

import (
	"os"
	"os/exec"
	"strconv"
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
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return false, false
	}
	cmdline := string(out)
	return strings.Contains(cmdline, "nudge-poller") && strings.Contains(cmdline, session), true
}
