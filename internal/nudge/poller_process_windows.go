//go:build windows

package nudge

import (
	"fmt"
	"math"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"
)

func pollerProcessAlive(pid int) bool {
	if pid <= 0 || pid > math.MaxUint32 {
		return false
	}

	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return err == windows.ERROR_ACCESS_DENIED
	}
	_ = windows.CloseHandle(handle)
	return true
}

func pollerProcessMatches(pid int, session string) (bool, bool) {
	filter := fmt.Sprintf("ProcessId=%d", pid)
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", fmt.Sprintf("(Get-CimInstance Win32_Process -Filter '%s').CommandLine", filter)).Output()
	if err != nil || len(out) == 0 {
		out, err = exec.Command("wmic", "process", "where", filter, "get", "CommandLine", "/value").Output()
	}
	if err != nil {
		return false, false
	}
	cmdline := string(out)
	return strings.Contains(cmdline, "nudge-poller") && strings.Contains(cmdline, session), true
}
