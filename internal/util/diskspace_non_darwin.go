//go:build !windows && !darwin

package util

func applyPlatformDiskSpaceAdjustments(_ string, _ *DiskSpaceInfo) {}
