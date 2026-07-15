package cmd

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/testutil"
)

func TestBuildWitnessPatrolVars_NilContext(t *testing.T) {
	ctx := RoleContext{}
	vars := buildWitnessPatrolVars(ctx)
	if len(vars) != 0 {
		t.Errorf("expected empty vars for nil context, got %v", vars)
	}
}

func TestBuildWitnessPatrolVars_InjectsRigAndPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	if err := os.MkdirAll(rigDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildWitnessPatrolVars(ctx)
	if len(vars) != 2 {
		t.Fatalf("expected 2 vars (rig, prefix), got %v", vars)
	}
	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}
	if got := varMap["rig"]; got != "testrig" {
		t.Errorf("rig = %q, want %q", got, "testrig")
	}
	if got := varMap["prefix"]; got != "gt" {
		t.Errorf("prefix = %q, want %q (default fallback)", got, "gt")
	}
}

func TestBuildRefineryPatrolVars_NilContext(t *testing.T) {
	ctx := RoleContext{}
	vars := buildRefineryPatrolVars(ctx)
	if len(vars) != 0 {
		t.Errorf("expected empty vars for nil context, got %v", vars)
	}
}

func TestBuildRefineryPatrolVars_MissingSettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	if err := os.MkdirAll(filepath.Join(rigDir, "settings"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)
	// target_branch should always be present (falls back to "main" without rig config)
	if len(vars) != 1 {
		t.Errorf("expected 1 var (target_branch) when settings file missing, got %v", vars)
	}
	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}
	if got := varMap["target_branch"]; got != "main" {
		t.Errorf("target_branch = %q, want %q", got, "main")
	}
}

func TestBuildRefineryPatrolVars_NilMergeQueue(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write settings with no merge_queue
	settings := config.RigSettings{
		Type:    "rig-settings",
		Version: 1,
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)
	// target_branch should always be present (falls back to "main" without rig config)
	if len(vars) != 1 {
		t.Errorf("expected 1 var (target_branch) when merge_queue is nil, got %v", vars)
	}
	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}
	if got := varMap["target_branch"]; got != "main" {
		t.Errorf("target_branch = %q, want %q", got, "main")
	}
}

func TestBuildRefineryPatrolVars_FullConfig(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write rig config.json with default_branch (source of truth for default branch)
	rigConfig := map[string]interface{}{"type": "rig", "version": 1, "name": "testrig"}
	rigData, _ := json.Marshal(rigConfig)
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), rigData, 0o644); err != nil {
		t.Fatal(err)
	}

	mq := config.DefaultMergeQueueConfig()
	settings := config.RigSettings{
		Type:       "rig-settings",
		Version:    1,
		MergeQueue: mq,
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)

	// DefaultMergeQueueConfig: refinery_enabled=true, auto_land=false, run_tests=true,
	// test_command="" (language-agnostic), target_branch="main" (from rig config),
	// delete_merged_branches=true, judgment_enabled=false, review_depth="standard"
	// merge_strategy is omitted when not explicitly set (formula default "direct" applies)
	// New commands (setup, typecheck, lint, build) default to empty = omitted
	// judgment_enabled defaults to false, review_depth defaults to "standard"
	expected := map[string]string{
		"integration_branch_refinery_enabled": "true",
		"integration_branch_auto_land":        "false",
		"run_tests":                           "true",
		"target_branch":                       "main",
		"delete_merged_branches":              "true",
		"judgment_enabled":                    "false",
		"review_depth":                        "standard",
		"require_review":                      "false",
	}

	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}

	for key, want := range expected {
		got, ok := varMap[key]
		if !ok {
			t.Errorf("missing var %q", key)
			continue
		}
		if got != want {
			t.Errorf("var %q = %q, want %q", key, got, want)
		}
	}

	// Verify empty commands and unset strategy are NOT included
	for _, shouldBeAbsent := range []string{"setup_command", "typecheck_command", "lint_command", "build_command", "merge_strategy"} {
		if _, ok := varMap[shouldBeAbsent]; ok {
			t.Errorf("%q should be omitted when empty/unset", shouldBeAbsent)
		}
	}

	if len(vars) != len(expected) {
		t.Errorf("expected %d vars, got %d: %v", len(expected), len(vars), vars)
	}
}

func TestBuildRefineryPatrolVars_AllCommandsSet(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mq := config.DefaultMergeQueueConfig()
	mq.SetupCommand = "pnpm install"
	mq.TypecheckCommand = "tsc --noEmit"
	mq.LintCommand = "eslint ."
	mq.BuildCommand = "pnpm build"
	settings := config.RigSettings{
		Type:       "rig-settings",
		Version:    1,
		MergeQueue: mq,
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)

	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}

	// All configured commands should be present (test_command is empty by default)
	commandExpected := map[string]string{
		"setup_command":     "pnpm install",
		"typecheck_command": "tsc --noEmit",
		"lint_command":      "eslint .",
		"build_command":     "pnpm build",
	}
	for key, want := range commandExpected {
		got, ok := varMap[key]
		if !ok {
			t.Errorf("missing var %q", key)
			continue
		}
		if got != want {
			t.Errorf("var %q = %q, want %q", key, got, want)
		}
	}
}

func TestBuildRefineryPatrolVars_EmptyTestCommand(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	falseVal := false
	trueVal2 := true
	mq := &config.MergeQueueConfig{
		Enabled:              true,
		RunTests:             &falseVal,
		TestCommand:          "", // empty - should be omitted
		DeleteMergedBranches: &trueVal2,
	}
	settings := config.RigSettings{
		Type:       "rig-settings",
		Version:    1,
		MergeQueue: mq,
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)

	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}

	// test_command should not be present when empty
	if _, ok := varMap["test_command"]; ok {
		t.Error("test_command should be omitted when empty")
	}

	// All command vars should be omitted when empty
	for _, cmd := range []string{"setup_command", "typecheck_command", "lint_command", "build_command"} {
		if _, ok := varMap[cmd]; ok {
			t.Errorf("%q should be omitted when empty", cmd)
		}
	}

	// run_tests should be "false"
	if got := varMap["run_tests"]; got != "false" {
		t.Errorf("run_tests = %q, want %q", got, "false")
	}
}

func TestBuildRefineryPatrolVars_BoolFormat(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write rig config.json with default_branch = "develop"
	rigConfig := map[string]interface{}{"type": "rig", "version": 1, "name": "testrig", "default_branch": "develop"}
	rigData, _ := json.Marshal(rigConfig)
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), rigData, 0o644); err != nil {
		t.Fatal(err)
	}

	trueVal := true
	falseVal2 := false
	mq := &config.MergeQueueConfig{
		Enabled:                          true,
		IntegrationBranchAutoLand:        &trueVal,
		IntegrationBranchRefineryEnabled: &trueVal,
		RunTests:                         &trueVal,
		SetupCommand:                     "npm ci",
		TypecheckCommand:                 "tsc --noEmit",
		LintCommand:                      "eslint .",
		TestCommand:                      "make test",
		BuildCommand:                     "make build",
		DeleteMergedBranches:             &falseVal2,
	}
	settings := config.RigSettings{
		Type:       "rig-settings",
		Version:    1,
		MergeQueue: mq,
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)

	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}

	// Check bool format is "true"/"false" strings
	if got := varMap["integration_branch_auto_land"]; got != "true" {
		t.Errorf("integration_branch_auto_land = %q, want %q", got, "true")
	}
	if got := varMap["delete_merged_branches"]; got != "false" {
		t.Errorf("delete_merged_branches = %q, want %q", got, "false")
	}
	if got := varMap["target_branch"]; got != "develop" {
		t.Errorf("target_branch = %q, want %q", got, "develop")
	}
	if got := varMap["test_command"]; got != "make test" {
		t.Errorf("test_command = %q, want %q", got, "make test")
	}
	if got := varMap["setup_command"]; got != "npm ci" {
		t.Errorf("setup_command = %q, want %q", got, "npm ci")
	}
	if got := varMap["typecheck_command"]; got != "tsc --noEmit" {
		t.Errorf("typecheck_command = %q, want %q", got, "tsc --noEmit")
	}
	if got := varMap["lint_command"]; got != "eslint ." {
		t.Errorf("lint_command = %q, want %q", got, "eslint .")
	}
	if got := varMap["build_command"]; got != "make build" {
		t.Errorf("build_command = %q, want %q", got, "make build")
	}
}

func TestBuildRefineryPatrolVars_DefaultBranchWithoutMQ(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	if err := os.MkdirAll(rigDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write rig config with custom default_branch but NO settings/config.json
	rigConfig := map[string]interface{}{
		"type": "rig", "version": 1, "name": "testrig",
		"default_branch": "gastown",
	}
	rigData, _ := json.Marshal(rigConfig)
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), rigData, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)

	// target_branch must be "gastown" even without merge_queue settings
	if len(vars) != 1 {
		t.Errorf("expected 1 var (target_branch), got %d: %v", len(vars), vars)
	}
	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}
	if got := varMap["target_branch"]; got != "gastown" {
		t.Errorf("target_branch = %q, want %q (should read rig config even without MQ settings)", got, "gastown")
	}
}

func TestBuildRefineryPatrolVars_MergeStrategy(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mq := config.DefaultMergeQueueConfig()
	mq.MergeStrategy = "pr"
	settings := config.RigSettings{
		Type:       "rig-settings",
		Version:    1,
		MergeQueue: mq,
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)

	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}

	if got := varMap["merge_strategy"]; got != "pr" {
		t.Errorf("merge_strategy = %q, want %q (rig-level config must override formula default)", got, "pr")
	}
}

func TestBuildRefineryPatrolVars_MergeStrategyDefaultOmitted(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// MergeStrategy not set — should not be injected (formula default "direct" applies)
	mq := config.DefaultMergeQueueConfig()
	settings := config.RigSettings{
		Type:       "rig-settings",
		Version:    1,
		MergeQueue: mq,
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)

	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}

	// merge_strategy should be absent when not explicitly configured
	if _, ok := varMap["merge_strategy"]; ok {
		t.Error("merge_strategy should be omitted when not configured (let formula default apply)")
	}
}

func TestBuildRefineryPatrolVars_RequireReview(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mq := config.DefaultMergeQueueConfig()
	mq.MergeStrategy = "pr"
	requireReview := true
	mq.RequireReview = &requireReview
	settings := config.RigSettings{
		Type:       "rig-settings",
		Version:    1,
		MergeQueue: mq,
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)

	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}

	if got := varMap["require_review"]; got != "true" {
		t.Errorf("require_review = %q, want %q", got, "true")
	}
	if got := varMap["merge_strategy"]; got != "pr" {
		t.Errorf("merge_strategy = %q, want %q", got, "pr")
	}
}

// splitFirstEquals splits a string on the first '=' only.
func splitFirstEquals(s string) []string {
	idx := -1
	for i, c := range s {
		if c == '=' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}

func TestBuildPatrolConfig_DeaconUsesCanonicalAssignee(t *testing.T) {
	cfg, err := buildPatrolConfig(RoleContext{Role: RoleDeacon, TownRoot: "/town"}, RoleDeacon)
	if err != nil {
		t.Fatalf("buildPatrolConfig: %v", err)
	}
	if cfg.Assignee != "deacon/" {
		t.Fatalf("Deacon patrol assignee = %q, want %q", cfg.Assignee, "deacon/")
	}
}

func TestGetAgentAssigneeIdentity_TownLevelCanonical(t *testing.T) {
	cases := []struct {
		name string
		ctx  RoleContext
		want string
	}{
		{name: "mayor", ctx: RoleContext{Role: RoleMayor}, want: "mayor/"},
		{name: "deacon", ctx: RoleContext{Role: RoleDeacon}, want: "deacon/"},
		{name: "boot", ctx: RoleContext{Role: RoleBoot}, want: "deacon/boot"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := getAgentAssigneeIdentity(tc.ctx); got != tc.want {
				t.Fatalf("getAgentAssigneeIdentity() = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- Patrol discovery tests (findActivePatrol) ---

func requireBd(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not installed, skipping patrol test")
	}
}

func setupPatrolTestDB(t *testing.T) (string, *beads.Beads) {
	t.Helper()
	testutil.RequireDoltContainer(t)
	port, _ := strconv.Atoi(testutil.DoltContainerPort())
	tmpDir := t.TempDir()
	b := beads.NewIsolatedWithPort(tmpDir, port)
	// Use a unique prefix per test run to avoid cross-run contamination
	// in the shared Dolt database.
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	prefix := "pt" + hex.EncodeToString(buf[:])
	if err := b.Init(prefix); err != nil {
		t.Fatalf("bd init: %v", err)
	}

	// Clean up the test database after the test to avoid leaking
	// beads_pt* databases on the shared Dolt server.
	dbName := "beads_" + prefix
	t.Cleanup(func() {
		dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%s)/", testutil.DoltContainerPort())
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			t.Logf("cleanup: failed to connect to dolt server to drop %s: %v", dbName, err)
			return
		}
		defer db.Close()
		if _, err := db.Exec("DROP DATABASE IF EXISTS `" + dbName + "`"); err != nil {
			t.Logf("cleanup: failed to drop %s: %v", dbName, err)
		}
		// Purge dropped databases to prevent accumulation on disk
		db.Exec("CALL dolt_purge_dropped_databases()") //nolint:errcheck
	})

	return tmpDir, b
}

// createHookedPatrol creates a bead with a patrol title and hooks it.
// If withOpenChild is true, creates an open child bead to simulate an active patrol.
func createHookedPatrol(t *testing.T, b *beads.Beads, molName, assignee string, withOpenChild bool) string {
	t.Helper()
	root, err := b.Create(beads.CreateOptions{
		Title:    molName + " (wisp)",
		Priority: -1,
	})
	if err != nil {
		t.Fatalf("create patrol root: %v", err)
	}

	hooked := beads.StatusHooked
	if err := b.Update(root.ID, beads.UpdateOptions{
		Status:   &hooked,
		Assignee: &assignee,
	}); err != nil {
		t.Fatalf("hook patrol: %v", err)
	}

	if withOpenChild {
		_, err := b.Create(beads.CreateOptions{
			Title:    "inbox-check",
			Parent:   root.ID,
			Priority: -1,
		})
		if err != nil {
			t.Fatalf("create child: %v", err)
		}
	}
	return root.ID
}

func TestFindActivePatrolHooked(t *testing.T) {
	requireBd(t)
	tmpDir, b := setupPatrolTestDB(t)

	molName := "mol-test-patrol"
	assignee := "testrig/witness"

	rootID := createHookedPatrol(t, b, molName, assignee, true /* withOpenChild */)

	cfg := PatrolConfig{
		PatrolMolName: molName,
		BeadsDir:      tmpDir,
		Assignee:      assignee,
		Beads:         b,
	}

	patrolID, _, found, findErr := findActivePatrol(cfg)
	if findErr != nil {
		t.Fatalf("findActivePatrol error: %v", findErr)
	}
	if !found {
		t.Fatal("expected to find active patrol, got not found")
	}
	if patrolID != rootID {
		t.Errorf("patrolID = %q, want %q", patrolID, rootID)
	}

	// Verify the patrol is still hooked (not closed)
	issue, err := b.Show(rootID)
	if err != nil {
		t.Fatalf("show patrol: %v", err)
	}
	if issue.Status != beads.StatusHooked {
		t.Errorf("patrol status = %q, want %q", issue.Status, beads.StatusHooked)
	}
}

func TestRunPatrolReportNoActivePatrolStartsReplacement(t *testing.T) {
	requireBd(t)
	tmpDir, b := setupPatrolTestDB(t)

	oldSpawner := autoSpawnPatrolForReport
	called := 0
	autoSpawnPatrolForReport = func(cfg PatrolConfig) (string, error) {
		called++
		if cfg.RoleName != "witness" || cfg.PatrolMolName != "mol-test-patrol" || cfg.Assignee != "testrig/witness" {
			t.Fatalf("unexpected patrol config: %+v", cfg)
		}
		return "pt-wisp-new", nil
	}
	t.Cleanup(func() { autoSpawnPatrolForReport = oldSpawner })

	err := runPatrolReportWithConfig(PatrolConfig{
		RoleName:      "witness",
		PatrolMolName: "mol-test-patrol",
		BeadsDir:      tmpDir,
		Assignee:      "testrig/witness",
		Beads:         b,
	})
	if err != nil {
		t.Fatalf("runPatrolReportWithConfig: %v", err)
	}
	if called != 1 {
		t.Fatalf("autoSpawnPatrolForReport calls = %d, want 1", called)
	}
}

func TestRunPatrolReportClosesCanonicalDeaconHook(t *testing.T) {
	requireBd(t)
	tmpDir, b := setupPatrolTestDB(t)

	legacyID := createHookedPatrol(t, b, constants.MolDeaconPatrol, "deacon", true)
	canonicalID := createHookedPatrol(t, b, constants.MolDeaconPatrol, "deacon/", true)

	oldSpawner := autoSpawnPatrolForReport
	oldSummary := patrolReportSummary
	oldSteps := patrolReportSteps
	patrolReportSummary = "canonical cycle complete"
	patrolReportSteps = ""
	called := 0
	autoSpawnPatrolForReport = func(cfg PatrolConfig) (string, error) {
		called++
		if cfg.RoleName != "deacon" || cfg.PatrolMolName != constants.MolDeaconPatrol || cfg.Assignee != "deacon/" {
			t.Fatalf("unexpected patrol config: %+v", cfg)
		}
		return "hq-wisp-next", nil
	}
	t.Cleanup(func() {
		autoSpawnPatrolForReport = oldSpawner
		patrolReportSummary = oldSummary
		patrolReportSteps = oldSteps
	})

	err := runPatrolReportWithConfig(PatrolConfig{
		RoleName:      "deacon",
		PatrolMolName: constants.MolDeaconPatrol,
		BeadsDir:      tmpDir,
		Assignee:      "deacon/",
		Beads:         b,
	})
	if err != nil {
		t.Fatalf("runPatrolReportWithConfig: %v", err)
	}
	if called != 1 {
		t.Fatalf("autoSpawnPatrolForReport calls = %d, want 1", called)
	}

	canonical, err := b.Show(canonicalID)
	if err != nil {
		t.Fatalf("show canonical patrol: %v", err)
	}
	if canonical.Status != "closed" {
		t.Fatalf("canonical patrol status = %q, want closed", canonical.Status)
	}

	legacy, err := b.Show(legacyID)
	if err != nil {
		t.Fatalf("show legacy patrol: %v", err)
	}
	if legacy.Status != beads.StatusHooked {
		t.Fatalf("legacy patrol status = %q, want still hooked until replacement cleanup", legacy.Status)
	}
}

func TestFindActivePatrolClosedChildrenReportable(t *testing.T) {
	requireBd(t)
	tmpDir, b := setupPatrolTestDB(t)

	molName := "mol-test-patrol"
	assignee := "testrig/witness"

	// Create a patrol with a closed child (simulates completed work awaiting report).
	rootID := createHookedPatrol(t, b, molName, assignee, true /* with child */)

	// Close the child; the hooked root should remain reportable.
	children, err := b.List(beads.ListOptions{Parent: rootID, Status: "all", Priority: -1})
	if err != nil {
		t.Fatalf("list children: %v", err)
	}
	for _, child := range children {
		if closeErr := b.ForceCloseWithReason("test cleanup", child.ID); closeErr != nil {
			t.Fatalf("close child: %v", closeErr)
		}
	}

	cfg := PatrolConfig{
		PatrolMolName: molName,
		BeadsDir:      tmpDir,
		Assignee:      assignee,
		Beads:         b,
	}

	patrolID, _, found, findErr := findActivePatrol(cfg)
	if findErr != nil {
		t.Fatalf("findActivePatrol error: %v", findErr)
	}
	if !found {
		t.Fatal("expected completed hooked patrol to remain reportable")
	}
	if patrolID != rootID {
		t.Errorf("patrolID = %q, want %q", patrolID, rootID)
	}

	// Discovery must be read-only; report closeout owns closure.
	issue, err := b.Show(rootID)
	if err != nil {
		t.Fatalf("show patrol: %v", err)
	}
	if issue.Status != beads.StatusHooked {
		t.Errorf("completed patrol status = %q, want %q", issue.Status, beads.StatusHooked)
	}
}

func TestFindActivePatrolZeroChildren(t *testing.T) {
	requireBd(t)
	tmpDir, b := setupPatrolTestDB(t)

	molName := "mol-test-patrol"
	assignee := "testrig/witness"

	// Create a patrol with NO children — simulates a freshly created wisp
	// whose steps haven't materialized yet. Should be treated as active,
	// not stale, to prevent race condition.
	rootID := createHookedPatrol(t, b, molName, assignee, false /* no children */)

	cfg := PatrolConfig{
		PatrolMolName: molName,
		BeadsDir:      tmpDir,
		Assignee:      assignee,
		Beads:         b,
	}

	patrolID, _, found, findErr := findActivePatrol(cfg)
	if findErr != nil {
		t.Fatalf("findActivePatrol error: %v", findErr)
	}
	if !found {
		t.Fatal("expected zero-children patrol to be treated as active (not stale)")
	}
	if patrolID != rootID {
		t.Errorf("patrolID = %q, want %q", patrolID, rootID)
	}

	// Verify it was NOT closed
	issue, err := b.Show(rootID)
	if err != nil {
		t.Fatalf("show patrol: %v", err)
	}
	if issue.Status != beads.StatusHooked {
		t.Errorf("zero-children patrol status = %q, want %q (should remain hooked)", issue.Status, beads.StatusHooked)
	}
}

func TestFindActivePatrolMultiple(t *testing.T) {
	requireBd(t)
	tmpDir, b := setupPatrolTestDB(t)

	molName := "mol-test-patrol"
	assignee := "testrig/witness"

	// Create 2 completed patrols and 1 patrol with an open child. Child state no
	// longer decides active status; all hooked patrol roots are reportable.
	completed1 := createHookedPatrol(t, b, molName, assignee, true)
	completed2 := createHookedPatrol(t, b, molName, assignee, true)
	openChildID := createHookedPatrol(t, b, molName, assignee, true)

	// Close children of completed patrols to simulate work awaiting report.
	for _, completedID := range []string{completed1, completed2} {
		children, err := b.List(beads.ListOptions{Parent: completedID, Status: "all", Priority: -1})
		if err != nil {
			t.Fatalf("list children of %s: %v", completedID, err)
		}
		for _, child := range children {
			if closeErr := b.ForceCloseWithReason("test cleanup", child.ID); closeErr != nil {
				t.Fatalf("close child: %v", closeErr)
			}
		}
	}

	cfg := PatrolConfig{
		PatrolMolName: molName,
		BeadsDir:      tmpDir,
		Assignee:      assignee,
		Beads:         b,
	}

	patrolID, _, found, findErr := findActivePatrol(cfg)
	if findErr != nil {
		t.Fatalf("findActivePatrol error: %v", findErr)
	}
	if !found {
		t.Fatal("expected to find active patrol")
	}
	created := map[string]bool{completed1: true, completed2: true, openChildID: true}
	if !created[patrolID] {
		t.Errorf("patrolID = %q, want one of %v", patrolID, created)
	}

	// Verify discovery did not close any reportable patrol roots.
	for _, id := range []string{completed1, completed2, openChildID} {
		issue, showErr := b.Show(id)
		if showErr != nil {
			t.Fatalf("show patrol %s: %v", id, showErr)
		}
		if issue.Status != beads.StatusHooked {
			t.Errorf("patrol %s status = %q, want %q", id, issue.Status, beads.StatusHooked)
		}
	}
}

func TestFindActivePatrolUsesCanonicalDeaconAssignee(t *testing.T) {
	cases := []struct {
		name  string
		order string
	}{
		{name: "legacy older", order: "legacy-first"},
		{name: "legacy newer", order: "canonical-first"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireBd(t)
			tmpDir, b := setupPatrolTestDB(t)

			var legacyID, canonicalID string
			if tc.order == "legacy-first" {
				legacyID = createHookedPatrol(t, b, constants.MolDeaconPatrol, "deacon", false)
				canonicalID = createHookedPatrol(t, b, constants.MolDeaconPatrol, "deacon/", false)
			} else {
				canonicalID = createHookedPatrol(t, b, constants.MolDeaconPatrol, "deacon/", false)
				legacyID = createHookedPatrol(t, b, constants.MolDeaconPatrol, "deacon", false)
			}

			cfg := PatrolConfig{
				RoleName:      "deacon",
				PatrolMolName: constants.MolDeaconPatrol,
				BeadsDir:      tmpDir,
				Assignee:      "deacon/",
				Beads:         b,
			}

			patrolID, _, found, findErr := findActivePatrol(cfg)
			if findErr != nil {
				t.Fatalf("findActivePatrol error: %v", findErr)
			}
			if !found {
				t.Fatal("expected canonical Deacon patrol to be found")
			}
			if patrolID != canonicalID {
				t.Fatalf("patrolID = %q, want canonical %q", patrolID, canonicalID)
			}

			legacy, err := b.Show(legacyID)
			if err != nil {
				t.Fatalf("show legacy patrol: %v", err)
			}
			if legacy.Status != beads.StatusHooked {
				t.Fatalf("legacy patrol status = %q, want still hooked until replacement cleanup", legacy.Status)
			}
		})
	}
}

func TestFindActivePatrolDoesNotCleanupCompletedPatrols(t *testing.T) {
	requireBd(t)
	tmpDir, b := setupPatrolTestDB(t)

	molName := "mol-test-patrol"
	assignee := "testrig/witness"

	numPatrols := 3
	patrolIDs := make([]string, numPatrols)
	for i := 0; i < numPatrols; i++ {
		id := createHookedPatrol(t, b, molName, assignee, true /* with child */)
		patrolIDs[i] = id

		// Close the child to simulate a completed patrol awaiting report.
		children, err := b.List(beads.ListOptions{Parent: id, Status: "all", Priority: -1})
		if err != nil {
			t.Fatalf("list children of %s: %v", id, err)
		}
		for _, child := range children {
			if closeErr := b.ForceCloseWithReason("test cleanup", child.ID); closeErr != nil {
				t.Fatalf("close child of %s: %v", id, closeErr)
			}
		}
	}

	cfg := PatrolConfig{
		PatrolMolName: molName,
		BeadsDir:      tmpDir,
		Assignee:      assignee,
		Beads:         b,
	}

	patrolID, _, found, findErr := findActivePatrol(cfg)
	if findErr != nil {
		t.Fatalf("findActivePatrol error: %v", findErr)
	}
	if !found {
		t.Fatal("expected completed hooked patrols to remain reportable")
	}
	created := map[string]bool{}
	for _, id := range patrolIDs {
		created[id] = true
	}
	if !created[patrolID] {
		t.Fatalf("patrolID = %q, want one of %v", patrolID, created)
	}

	for _, id := range patrolIDs {
		issue, err := b.Show(id)
		if err != nil {
			t.Fatalf("show patrol %s: %v", id, err)
		}
		if issue.Status != beads.StatusHooked {
			t.Errorf("patrol %s status = %q, want %q", id, issue.Status, beads.StatusHooked)
		}
	}
}

func TestBurnPreviousPatrolWisps(t *testing.T) {
	requireBd(t)
	tmpDir, b := setupPatrolTestDB(t)

	molName := "mol-test-patrol"
	assignee := "testrig/witness"

	// Create 3 hooked patrol wisps (simulating accumulated orphans)
	id1 := createHookedPatrol(t, b, molName, assignee, true)
	id2 := createHookedPatrol(t, b, molName, assignee, false)
	id3 := createHookedPatrol(t, b, molName, assignee, true)

	cfg := PatrolConfig{
		PatrolMolName: molName,
		BeadsDir:      tmpDir,
		Assignee:      assignee,
		Beads:         b,
	}

	burnPreviousPatrolWisps(cfg)

	// All 3 patrols should now be closed
	for _, id := range []string{id1, id2, id3} {
		issue, err := b.Show(id)
		if err != nil {
			t.Fatalf("show %s: %v", id, err)
		}
		if issue.Status != "closed" {
			t.Errorf("patrol %s status = %q, want %q after burn", id, issue.Status, "closed")
		}
	}
}

func TestBurnPreviousPatrolWisps_CollapsesLegacyDeaconAssignee(t *testing.T) {
	requireBd(t)
	tmpDir, b := setupPatrolTestDB(t)

	canonicalID := createHookedPatrol(t, b, constants.MolDeaconPatrol, "deacon/", false)
	legacyID := createHookedPatrol(t, b, constants.MolDeaconPatrol, "deacon", false)

	cfg := PatrolConfig{
		RoleName:      "deacon",
		PatrolMolName: constants.MolDeaconPatrol,
		BeadsDir:      tmpDir,
		Assignee:      "deacon/",
		Beads:         b,
	}

	burnPreviousPatrolWisps(cfg)

	for _, id := range []string{canonicalID, legacyID} {
		issue, err := b.Show(id)
		if err != nil {
			t.Fatalf("show patrol %s: %v", id, err)
		}
		if issue.Status != "closed" {
			t.Errorf("patrol %s status = %q, want closed after burn", id, issue.Status)
		}
	}
}

func TestBurnPreviousPatrolWisps_IgnoresOtherBeads(t *testing.T) {
	requireBd(t)
	tmpDir, b := setupPatrolTestDB(t)

	molName := "mol-test-patrol"
	assignee := "testrig/witness"

	// Create a patrol wisp (should be burned)
	patrolID := createHookedPatrol(t, b, molName, assignee, true)

	// Create a non-patrol hooked bead (should NOT be burned)
	other, err := b.Create(beads.CreateOptions{
		Title:    "some-other-work",
		Priority: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	hooked := beads.StatusHooked
	if err := b.Update(other.ID, beads.UpdateOptions{
		Status:   &hooked,
		Assignee: &assignee,
	}); err != nil {
		t.Fatal(err)
	}

	cfg := PatrolConfig{
		PatrolMolName: molName,
		BeadsDir:      tmpDir,
		Assignee:      assignee,
		Beads:         b,
	}

	burnPreviousPatrolWisps(cfg)

	// Patrol should be closed
	issue, err := b.Show(patrolID)
	if err != nil {
		t.Fatalf("show patrol: %v", err)
	}
	if issue.Status != "closed" {
		t.Errorf("patrol status = %q, want %q", issue.Status, "closed")
	}

	// Non-patrol bead should still be hooked
	otherIssue, err := b.Show(other.ID)
	if err != nil {
		t.Fatalf("show other: %v", err)
	}
	if otherIssue.Status != beads.StatusHooked {
		t.Errorf("non-patrol bead status = %q, want %q (should not be burned)", otherIssue.Status, beads.StatusHooked)
	}
}
