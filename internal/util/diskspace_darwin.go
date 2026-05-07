//go:build darwin

package util

import "fmt"

func applyPlatformDiskSpaceAdjustments(path string, info *DiskSpaceInfo) {
	plist, err := ExecWithOutput("", "diskutil", "info", "-plist", path)
	if err != nil {
		return
	}

	purgeableBytes, err := parseDiskutilPurgeableBytes(plist)
	if err != nil {
		return
	}

	applyPurgeableSpace(info, purgeableBytes)
}

var _ = fmt.Sprintf
