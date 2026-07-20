package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/plugin"
)

func TestPluginRecordRunSuppressedUnderRunner(t *testing.T) {
	t.Setenv("GT_PLUGIN_RUNNER_ACTIVE", "1")
	t.Setenv("GT_PLUGIN_NAME", "test-plugin")
	oldPlugin, oldResult := pluginRecordPlugin, pluginRecordResult
	t.Cleanup(func() {
		pluginRecordPlugin = oldPlugin
		pluginRecordResult = oldResult
	})
	pluginRecordPlugin = "test-plugin"
	pluginRecordResult = "success"

	if err := runPluginRecordRun(&cobra.Command{}, nil); err != nil {
		t.Fatalf("runPluginRecordRun returned error under runner: %v", err)
	}

	pluginRecordPlugin = "other-plugin"
	if err := runPluginRecordRun(&cobra.Command{}, nil); err != nil {
		t.Fatalf("runPluginRecordRun returned error for cross-plugin runner suppression: %v", err)
	}
}

func TestPluginRecordRunUnderRunnerWritesRecordRequest(t *testing.T) {
	recordFile := filepath.Join(t.TempDir(), "record.json")
	t.Setenv("GT_PLUGIN_RUNNER_ACTIVE", "1")
	t.Setenv("GT_PLUGIN_RUNNER_RECORD_FILE", recordFile)
	oldPlugin, oldResult, oldTitle, oldBody, oldRig, oldLabels := pluginRecordPlugin, pluginRecordResult, pluginRecordTitle, pluginRecordBody, pluginRecordRig, pluginRecordLabels
	t.Cleanup(func() {
		pluginRecordPlugin = oldPlugin
		pluginRecordResult = oldResult
		pluginRecordTitle = oldTitle
		pluginRecordBody = oldBody
		pluginRecordRig = oldRig
		pluginRecordLabels = oldLabels
	})
	pluginRecordPlugin = "test-plugin"
	pluginRecordResult = "warning"
	pluginRecordTitle = "script warning"
	pluginRecordBody = "partial failure"
	pluginRecordRig = "gastown"
	pluginRecordLabels = []string{"source:script"}

	if err := runPluginRecordRun(&cobra.Command{}, nil); err != nil {
		t.Fatalf("runPluginRecordRun returned error under runner: %v", err)
	}
	data, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatalf("read runner record file: %v", err)
	}
	var record plugin.RunnerRecordRun
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("unmarshal runner record: %v", err)
	}
	if record.PluginName != "test-plugin" || record.Result != plugin.RunResult("warning") || record.Title != "script warning" || record.Body != "partial failure" || record.RigName != "gastown" {
		t.Fatalf("unexpected runner record: %+v", record)
	}
	if !stringSliceContains(record.ExtraLabels, "source:script") {
		t.Fatalf("runner record labels missing source:script: %+v", record)
	}
}

func TestPluginRecordRunLabelsManualReceipts(t *testing.T) {
	oldLabels := pluginRecordLabels
	t.Cleanup(func() { pluginRecordLabels = oldLabels })
	pluginRecordLabels = nil

	success := pluginRecordRunLabels(plugin.ResultSuccess)
	for _, want := range []string{plugin.LabelAuthorityManual, plugin.LabelCooldownCounted, plugin.LabelRetryableFalse} {
		if !stringSliceContains(success, want) {
			t.Fatalf("success labels missing %q: %v", want, success)
		}
	}

	failure := pluginRecordRunLabels(plugin.ResultFailure)
	for _, want := range []string{plugin.LabelAuthorityManual, "cooldown:retryable", plugin.LabelRetryableTrue} {
		if !stringSliceContains(failure, want) {
			t.Fatalf("failure labels missing %q: %v", want, failure)
		}
	}
}

func TestPluginRunExecutesRunScriptThroughRuntime(t *testing.T) {
	townRoot := setupPluginCommandTown(t, `#!/usr/bin/env bash
printf '%s %s' "$GT_PLUGIN_TRIGGER" "$GT_PLUGIN_RUNNER_ACTIVE" > "$GT_TOWN_ROOT/plugin-run-marker"
`)
	chdir(t, townRoot)
	resetPluginRunFlags(t)

	if err := runPluginRun(&cobra.Command{}, []string{"script-plugin"}); err != nil {
		t.Fatalf("runPluginRun returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(townRoot, "plugin-run-marker"))
	if err != nil {
		t.Fatalf("reading marker: %v", err)
	}
	if got, want := string(data), "manual 1"; got != want {
		t.Fatalf("marker = %q, want %q", got, want)
	}
}

func TestDogDispatchExecutesScriptBeforeDogAssignment(t *testing.T) {
	townRoot := setupPluginCommandTown(t, `#!/usr/bin/env bash
printf '%s %s' "$GT_PLUGIN_TRIGGER" "$GT_PLUGIN_RUNNER_ACTIVE" > "$GT_TOWN_ROOT/dog-dispatch-marker"
`)
	chdir(t, townRoot)
	resetDogDispatchFlags(t)
	dogDispatchPlugin = "script-plugin"

	if err := runDogDispatch(&cobra.Command{}, nil); err != nil {
		t.Fatalf("runDogDispatch returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(townRoot, "dog-dispatch-marker"))
	if err != nil {
		t.Fatalf("reading marker: %v", err)
	}
	if got, want := string(data), "dog-dispatch 1"; got != want {
		t.Fatalf("marker = %q, want %q", got, want)
	}
	entries, err := os.ReadDir(filepath.Join(townRoot, "deacon", "dogs"))
	if err == nil && len(entries) > 0 {
		t.Fatalf("dog dispatch created/assigned dogs for script-only success: %v", entries)
	}
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("reading dog kennel: %v", err)
	}
}

func TestDogDispatchTargetedScriptSuccessDoesNotReportDog(t *testing.T) {
	townRoot := setupPluginCommandTown(t, `#!/usr/bin/env bash
printf '%s %s' "$GT_PLUGIN_TRIGGER" "$GT_PLUGIN_RUNNER_ACTIVE" > "$GT_TOWN_ROOT/dog-dispatch-marker"
`)
	chdir(t, townRoot)
	resetDogDispatchFlags(t)
	dogDispatchPlugin = "script-plugin"
	dogDispatchDog = "alpha"
	dogDispatchJSON = true

	var runErr error
	output := capturePluginTestStdout(t, func() {
		runErr = runDogDispatch(&cobra.Command{}, nil)
	})
	if runErr != nil {
		t.Fatalf("runDogDispatch returned error: %v", runErr)
	}
	var result dogDispatchResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("unmarshal dispatch JSON %q: %v", output, err)
	}
	if result.Dog != "" || result.SessionStarted || result.WorkConfirmed {
		t.Fatalf("script-only success reported dog assignment: %+v", result)
	}
	if !result.ScriptRan || result.ScriptExitCode != 0 || result.Result != string(plugin.ResultSuccess) {
		t.Fatalf("unexpected script success result: %+v", result)
	}
}

func setupPluginCommandTown(t *testing.T, script string) string {
	t.Helper()
	townRoot := t.TempDir()
	for _, dir := range []string{
		filepath.Join(townRoot, "mayor"),
		filepath.Join(townRoot, ".beads"),
		filepath.Join(townRoot, "plugins", "script-plugin"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"type":"town","version":2,"name":"test"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
	if err := config.SaveRigsConfig(filepath.Join(townRoot, "mayor", "rigs.json"), &config.RigsConfig{Version: config.CurrentRigsVersion, Rigs: map[string]config.RigEntry{}}); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}
	pluginDir := filepath.Join(townRoot, "plugins", "script-plugin")
	pluginMD := "+++\nname = \"script-plugin\"\ndescription = \"script plugin\"\nversion = 1\n+++\n\n# Instructions\nFollow up only after run.sh exits 10.\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.md"), []byte(pluginMD), 0644); err != nil {
		t.Fatalf("write plugin.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "run.sh"), []byte(script), 0755); err != nil {
		t.Fatalf("write run.sh: %v", err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	fakeBD := `#!/bin/sh
case "$1" in
  create) printf '{"id":"gt-rec-test"}\n' ;;
  list) printf '[]\n' ;;
  close) exit 0 ;;
  *) exit 0 ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(fakeBD), 0755); err != nil {
		t.Fatalf("write fake bd: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return townRoot
}

func resetPluginRunFlags(t *testing.T) {
	t.Helper()
	oldForce, oldDryRun := pluginRunForce, pluginRunDryRun
	t.Cleanup(func() {
		pluginRunForce = oldForce
		pluginRunDryRun = oldDryRun
	})
	pluginRunForce = false
	pluginRunDryRun = false
}

func resetDogDispatchFlags(t *testing.T) {
	t.Helper()
	oldPlugin, oldRig, oldDog := dogDispatchPlugin, dogDispatchRig, dogDispatchDog
	oldCreate, oldJSON, oldDryRun := dogDispatchCreate, dogDispatchJSON, dogDispatchDryRun
	t.Cleanup(func() {
		dogDispatchPlugin = oldPlugin
		dogDispatchRig = oldRig
		dogDispatchDog = oldDog
		dogDispatchCreate = oldCreate
		dogDispatchJSON = oldJSON
		dogDispatchDryRun = oldDryRun
	})
	dogDispatchPlugin = ""
	dogDispatchRig = ""
	dogDispatchDog = ""
	dogDispatchCreate = false
	dogDispatchJSON = false
	dogDispatchDryRun = false
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func capturePluginTestStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	os.Stdout = old
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	return string(data)
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
