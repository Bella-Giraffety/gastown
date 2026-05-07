package util

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// DiskSpaceInfo contains filesystem space information.
type DiskSpaceInfo struct {
	// AvailableBytes is the effective number of bytes available to non-root users.
	// On APFS volumes this may include purgeable space that macOS can reclaim.
	AvailableBytes uint64

	// PurgeableBytes is macOS APFS space the OS can reclaim on demand.
	PurgeableBytes uint64

	// TotalBytes is the total filesystem capacity.
	TotalBytes uint64

	// UsedBytes is the number of bytes in use.
	UsedBytes uint64

	// UsedPercent is the usage percentage (0-100).
	UsedPercent float64
}

func evaluateDiskSpace(info *DiskSpaceInfo) (DiskSpaceLevel, string) {
	availMB := info.AvailableMB()

	if availMB < DiskSpaceMinimumMB || info.UsedPercent >= DiskSpaceCriticalPercent {
		return DiskSpaceCritical,
			fmt.Sprintf("CRITICAL: only %s free (%.1f%% used) — disk space exhausted, operations blocked",
				info.AvailableHuman(), info.UsedPercent)
	}

	if availMB < DiskSpaceWarningMB {
		return DiskSpaceWarning,
			fmt.Sprintf("WARNING: only %s free (%.1f%% used) — disk space low, reduce workload",
				info.AvailableHuman(), info.UsedPercent)
	}

	return DiskSpaceOK, ""
}

func applyPurgeableSpace(info *DiskSpaceInfo, purgeableBytes uint64) {
	if info == nil || purgeableBytes == 0 {
		return
	}

	maxExtra := uint64(0)
	if info.TotalBytes > info.AvailableBytes {
		maxExtra = info.TotalBytes - info.AvailableBytes
	}
	if purgeableBytes > maxExtra {
		purgeableBytes = maxExtra
	}
	if purgeableBytes == 0 {
		return
	}

	info.PurgeableBytes = purgeableBytes
	info.AvailableBytes += purgeableBytes
	if purgeableBytes >= info.UsedBytes {
		info.UsedBytes = 0
	} else {
		info.UsedBytes -= purgeableBytes
	}
	if info.TotalBytes > 0 {
		info.UsedPercent = float64(info.UsedBytes) / float64(info.TotalBytes) * 100
	}
}

func parseDiskutilPurgeableBytes(plist string) (uint64, error) {
	decoder := xml.NewDecoder(strings.NewReader(plist))
	var currentKey string

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				return 0, nil
			}
			return 0, fmt.Errorf("decode diskutil plist: %w", err)
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		switch start.Name.Local {
		case "key":
			var key string
			if err := decoder.DecodeElement(&key, &start); err != nil {
				return 0, fmt.Errorf("decode diskutil key: %w", err)
			}
			currentKey = strings.TrimSpace(key)

		case "integer":
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return 0, fmt.Errorf("decode diskutil integer: %w", err)
			}
			if currentKey != "APFSPurgeableSpace" {
				continue
			}

			purgeableBytes, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse APFSPurgeableSpace %q: %w", value, err)
			}
			return purgeableBytes, nil
		}
	}
}

// AvailableMB returns available space in megabytes.
func (d *DiskSpaceInfo) AvailableMB() uint64 {
	return d.AvailableBytes / (1024 * 1024)
}

// AvailableGB returns available space in gigabytes (truncated).
func (d *DiskSpaceInfo) AvailableGB() float64 {
	return float64(d.AvailableBytes) / (1024 * 1024 * 1024)
}

// AvailableHuman returns a human-readable string for available space.
func (d *DiskSpaceInfo) AvailableHuman() string {
	return FormatBytesHuman(d.AvailableBytes)
}

// FormatBytesHuman formats bytes into a human-readable string.
func FormatBytesHuman(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// Default thresholds for disk space checks.
const (
	// DiskSpaceMinimumMB is the absolute minimum free space (in MB) below which
	// operations that require significant disk I/O should be blocked.
	// 500 MB provides enough buffer for Dolt operations, git worktrees, etc.
	DiskSpaceMinimumMB uint64 = 500

	// DiskSpaceWarningMB is the threshold (in MB) at which warnings are emitted.
	// At 1 GB free, the system is at risk and should shed load.
	DiskSpaceWarningMB uint64 = 1024

	// DiskSpaceCriticalPercent is the usage percentage above which operations
	// should be blocked regardless of absolute free space.
	DiskSpaceCriticalPercent float64 = 95.0
)

// DiskSpaceLevel represents the severity of disk space status.
type DiskSpaceLevel int

const (
	// DiskSpaceOK means disk space is adequate.
	DiskSpaceOK DiskSpaceLevel = iota
	// DiskSpaceWarning means disk space is getting low.
	DiskSpaceWarning
	// DiskSpaceCritical means disk space is critically low — block new operations.
	DiskSpaceCritical
)

// String returns a human-readable label.
func (l DiskSpaceLevel) String() string {
	switch l {
	case DiskSpaceOK:
		return "ok"
	case DiskSpaceWarning:
		return "warning"
	case DiskSpaceCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// CheckDiskSpace evaluates disk space at the given path and returns the level
// and a human-readable message. Returns DiskSpaceOK with empty message if fine.
func CheckDiskSpace(path string) (DiskSpaceLevel, string, error) {
	info, err := GetDiskSpace(path)
	if err != nil {
		return DiskSpaceOK, "", err
	}

	level, msg := evaluateDiskSpace(info)
	return level, msg, nil
}
