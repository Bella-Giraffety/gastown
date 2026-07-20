package cmd

import "testing"

func TestDoctorDoesNotRegisterDoltConfigCheck(t *testing.T) {
	d := newDoctorForCommand("")
	for _, check := range d.Checks() {
		if check.Name() == "dolt-config" {
			t.Fatalf("dolt-config check must not be registered; it writes runtime Dolt keys into tracked .beads/config.yaml")
		}
	}
}

func TestDoctorRegistersOpenCodeRetentionAfterDiskSpace(t *testing.T) {
	d := newDoctorForCommand("")
	indexes := map[string]int{}
	for i, check := range d.Checks() {
		name := check.Name()
		if _, exists := indexes[name]; exists && name == "opencode-retention" {
			t.Fatalf("opencode-retention registered more than once")
		}
		indexes[name] = i
	}

	opencodeIndex, ok := indexes["opencode-retention"]
	if !ok {
		t.Fatal("opencode-retention check is not registered")
	}
	if diskIndex, ok := indexes["disk-space"]; !ok || diskIndex >= opencodeIndex {
		t.Fatalf("opencode-retention should run after disk-space; disk=%d opencode=%d", diskIndex, opencodeIndex)
	}
	if staleBinaryIndex, ok := indexes["stale-binary"]; !ok || opencodeIndex >= staleBinaryIndex {
		t.Fatalf("opencode-retention should run before infrastructure checks; opencode=%d stale-binary=%d", opencodeIndex, staleBinaryIndex)
	}
}
