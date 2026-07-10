package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/nudge"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

func TestSessionInfoJSONOutput(t *testing.T) {
	info := &polecat.SessionInfo{
		Polecat:   "alpha",
		SessionID: "gt-alpha",
		Running:   true,
		RigName:   "gastown",
		Attached:  false,
		Created:   time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC),
		Windows:   1,
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if parsed["polecat"] != "alpha" {
		t.Errorf("polecat = %v, want alpha", parsed["polecat"])
	}
	if parsed["session_id"] != "gt-alpha" {
		t.Errorf("session_id = %v, want gt-alpha", parsed["session_id"])
	}
	if parsed["running"] != true {
		t.Errorf("running = %v, want true", parsed["running"])
	}
	if parsed["rig_name"] != "gastown" {
		t.Errorf("rig_name = %v, want gastown", parsed["rig_name"])
	}
}

func TestSessionStatusCmdJSONFlagWiring(t *testing.T) {
	// Verify --json flag is registered on the session status command.
	// This catches regressions where flag binding is accidentally removed,
	// which would silently break formulas that depend on --json output.
	f := sessionStatusCmd.Flags().Lookup("json")
	if f == nil {
		t.Fatal("session status command missing --json flag")
	}
	if f.DefValue != "false" {
		t.Errorf("--json default = %q, want \"false\"", f.DefValue)
	}
}

func TestSessionHealthCmdFlagWiring(t *testing.T) {
	if sessionCmd.Commands() == nil {
		t.Fatal("session command has no subcommands")
	}

	f := sessionHealthCmd.Flags().Lookup("json")
	if f == nil {
		t.Fatal("session health command missing --json flag")
	}
	if f.DefValue != "false" {
		t.Errorf("--json default = %q, want \"false\"", f.DefValue)
	}

	f = sessionHealthCmd.Flags().Lookup("max-inactivity")
	if f == nil {
		t.Fatal("session health command missing --max-inactivity flag")
	}
	if f.DefValue != "0s" {
		t.Errorf("--max-inactivity default = %q, want \"0s\"", f.DefValue)
	}
}

func TestSessionHealthReportJSONContract(t *testing.T) {
	report := newSessionHealthReport("gt-vault", tmux.AgentDead, 30*time.Minute)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if parsed["session"] != "gt-vault" {
		t.Errorf("session = %v, want gt-vault", parsed["session"])
	}
	if parsed["status"] != "agent-dead" {
		t.Errorf("status = %v, want agent-dead", parsed["status"])
	}
	if parsed["healthy"] != false {
		t.Errorf("healthy = %v, want false", parsed["healthy"])
	}
	if parsed["zombie"] != true {
		t.Errorf("zombie = %v, want true", parsed["zombie"])
	}
	if parsed["max_inactivity_seconds"] != float64(1800) {
		t.Errorf("max_inactivity_seconds = %v, want 1800", parsed["max_inactivity_seconds"])
	}
}

func TestResolveSessionHealthTarget(t *testing.T) {
	old := session.DefaultRegistry()
	registry := session.NewPrefixRegistry()
	registry.Register("gt", "gastown")
	registry.Register("do", "dotfiles")
	session.SetDefaultRegistry(registry)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })

	tests := []struct {
		input string
		want  string
	}{
		{input: "gt-witness", want: "gt-witness"},
		{input: "dotfiles/witness", want: "do-witness"},
		{input: "gastown/refinery", want: "gt-refinery"},
		{input: "gastown/polecats/chrome", want: "gt-chrome"},
		{input: "gastown/crew/max", want: "gt-crew-max"},
		{input: "gatsown/witness", want: "gatsown/witness"},
		{input: "raw/session", want: "raw/session"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := resolveSessionHealthTarget(tt.input); got != tt.want {
				t.Fatalf("resolveSessionHealthTarget(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveSessionHealthTargetUsesWorkspaceRigWhenRegistryMissing(t *testing.T) {
	oldRegistry := session.DefaultRegistry()
	session.SetDefaultRegistry(session.NewPrefixRegistry())
	t.Cleanup(func() { session.SetDefaultRegistry(oldRegistry) })

	townRoot := setupSessionHealthWorkspace(t)
	chdirForTest(t, filepath.Join(townRoot, "gastown"))

	tests := []struct {
		input string
		want  string
	}{
		{input: "gastown/ghoul", want: "gt-ghoul"},
		{input: "gastown/polecats/ghoul", want: "gt-ghoul"},
		{input: "gastown/crew/max", want: "gt-crew-max"},
		{input: "gastown/witness", want: "gt-witness"},
		{input: "dotfiles/witness", want: "do-witness"},
		{input: "gatsown/witness", want: "gatsown/witness"},
		{input: "raw/session", want: "raw/session"},
		{input: "gt-ghoul", want: "gt-ghoul"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := resolveSessionHealthTarget(tt.input); got != tt.want {
				t.Fatalf("resolveSessionHealthTarget(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}

	if got := len(session.DefaultRegistry().AllRigs()); got != 0 {
		t.Fatalf("resolveSessionHealthTarget mutated default registry, got %d registered rigs", got)
	}
}

func TestRunSessionHealthJSONAddressActiveOpenCodeNoPromptQueuedNudge(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a Unix shell fake tmux")
	}

	oldRegistry := session.DefaultRegistry()
	session.SetDefaultRegistry(session.NewPrefixRegistry())
	t.Cleanup(func() { session.SetDefaultRegistry(oldRegistry) })
	oldSocket := tmux.GetDefaultSocket()
	tmux.SetDefaultSocket("")
	t.Cleanup(func() { tmux.SetDefaultSocket(oldSocket) })
	t.Setenv("GT_TOWN_SOCKET", "")

	townRoot := setupSessionHealthWorkspace(t)
	chdirForTest(t, filepath.Join(townRoot, "gastown"))
	writeSessionHealthFakeTmux(t, true)

	if err := nudge.Enqueue(townRoot, "gt-guzzle", nudge.QueuedNudge{
		Sender:   "gastown/witness",
		Message:  "status?",
		Priority: nudge.PriorityNormal,
	}); err != nil {
		t.Fatalf("enqueue queued nudge: %v", err)
	}

	parsed := runSessionHealthJSONForTest(t, "gastown/polecats/guzzle")
	if parsed["session"] != "gt-guzzle" {
		t.Fatalf("session = %v, want gt-guzzle", parsed["session"])
	}
	if parsed["status"] != "healthy" {
		t.Fatalf("status = %v, want healthy", parsed["status"])
	}
	if parsed["healthy"] != true {
		t.Fatalf("healthy = %v, want true", parsed["healthy"])
	}
	if parsed["zombie"] != false {
		t.Fatalf("zombie = %v, want false", parsed["zombie"])
	}
	pending, err := nudge.Pending(townRoot, "gt-guzzle")
	if err != nil {
		t.Fatalf("pending queued nudges: %v", err)
	}
	if pending != 1 {
		t.Fatalf("queued nudges changed during health check: got %d, want 1", pending)
	}
}

func TestRunSessionHealthJSONAddressMissingSessionRemainsDead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a Unix shell fake tmux")
	}

	oldRegistry := session.DefaultRegistry()
	session.SetDefaultRegistry(session.NewPrefixRegistry())
	t.Cleanup(func() { session.SetDefaultRegistry(oldRegistry) })
	oldSocket := tmux.GetDefaultSocket()
	tmux.SetDefaultSocket("")
	t.Cleanup(func() { tmux.SetDefaultSocket(oldSocket) })
	t.Setenv("GT_TOWN_SOCKET", "")

	townRoot := setupSessionHealthWorkspace(t)
	chdirForTest(t, filepath.Join(townRoot, "gastown"))
	writeSessionHealthFakeTmux(t, false)

	parsed := runSessionHealthJSONForTest(t, "gastown/polecats/guzzle")
	if parsed["session"] != "gt-guzzle" {
		t.Fatalf("session = %v, want gt-guzzle", parsed["session"])
	}
	if parsed["status"] != "session-dead" {
		t.Fatalf("status = %v, want session-dead", parsed["status"])
	}
	if parsed["healthy"] != false {
		t.Fatalf("healthy = %v, want false", parsed["healthy"])
	}
	if parsed["zombie"] != false {
		t.Fatalf("zombie = %v, want false", parsed["zombie"])
	}
}

func TestRunSessionHealthJSONSessionDead(t *testing.T) {
	oldJSON := sessionHealthJSON
	oldMaxInactivity := sessionHealthMaxInactivity
	oldStdout := os.Stdout
	t.Cleanup(func() {
		sessionHealthJSON = oldJSON
		sessionHealthMaxInactivity = oldMaxInactivity
		os.Stdout = oldStdout
	})

	sessionHealthJSON = true
	sessionHealthMaxInactivity = 0
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = w

	err = runSessionHealth(sessionHealthCmd, []string{"gt-session-health-test-nonexistent"})
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("closing pipe writer: %v", closeErr)
	}
	os.Stdout = oldStdout
	if err != nil {
		t.Fatalf("runSessionHealth failed: %v", err)
	}

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading stdout pipe: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v\noutput: %s", err, string(data))
	}
	if parsed["session"] != "gt-session-health-test-nonexistent" {
		t.Errorf("session = %v, want gt-session-health-test-nonexistent", parsed["session"])
	}
	if parsed["status"] != "session-dead" {
		t.Errorf("status = %v, want session-dead", parsed["status"])
	}
	if parsed["healthy"] != false {
		t.Errorf("healthy = %v, want false", parsed["healthy"])
	}
	if parsed["zombie"] != false {
		t.Errorf("zombie = %v, want false", parsed["zombie"])
	}
}

func setupSessionHealthWorkspace(t *testing.T) string {
	t.Helper()
	townRoot := t.TempDir()
	for _, dir := range []string{
		filepath.Join(townRoot, "mayor"),
		filepath.Join(townRoot, "gastown"),
		filepath.Join(townRoot, "dotfiles"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	rigsJSON := `{"version":1,"rigs":{"gastown":{"git_url":"https://example.invalid/gastown","beads":{"repo":"local","prefix":"gt-"}},"dotfiles":{"git_url":"https://example.invalid/dotfiles","beads":{"repo":"local","prefix":"do"}}}}`
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}
	return townRoot
}

func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })
}

func writeSessionHealthFakeTmux(t *testing.T, sessionExists bool) {
	t.Helper()
	binDir := t.TempDir()
	missingStatus := "0"
	if !sessionExists {
		missingStatus = "1"
	}
	script := `#!/bin/sh
case "$*" in
  *"capture-pane"*) echo "health must not rely on pane capture or prompt text" >&2; exit 1;;
  *"has-session -t =gt-guzzle"*) exit ` + missingStatus + `;;
  *"show-environment -t gt-guzzle GT_PROCESS_NAMES"*) echo "GT_PROCESS_NAMES=sleep"; exit 0;;
  *"show-environment -t gt-guzzle GT_PANE_ID"*) echo "unknown variable: GT_PANE_ID" >&2; exit 1;;
  *"display-message -t gt-guzzle:^ -p #{pane_current_command}"*) echo "sleep"; exit 0;;
  *"display-message -t gt-guzzle:^ -p #{pane_pid}"*) echo "12345"; exit 0;;
  *) echo "unexpected tmux command: $*" >&2; exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "tmux"), []byte(script), 0755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func runSessionHealthJSONForTest(t *testing.T, target string) map[string]interface{} {
	t.Helper()
	oldJSON := sessionHealthJSON
	oldMaxInactivity := sessionHealthMaxInactivity
	oldStdout := os.Stdout
	t.Cleanup(func() {
		sessionHealthJSON = oldJSON
		sessionHealthMaxInactivity = oldMaxInactivity
		os.Stdout = oldStdout
	})

	sessionHealthJSON = true
	sessionHealthMaxInactivity = 0
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = w

	err = runSessionHealth(sessionHealthCmd, []string{target})
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("closing pipe writer: %v", closeErr)
	}
	os.Stdout = oldStdout
	if err != nil {
		t.Fatalf("runSessionHealth failed: %v", err)
	}

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading stdout pipe: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v\noutput: %s", err, string(data))
	}
	return parsed
}

func TestSessionInfoJSONOutputNotRunning(t *testing.T) {
	info := &polecat.SessionInfo{
		Polecat:   "beta",
		SessionID: "gt-beta",
		Running:   false,
		RigName:   "testrig",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if parsed["running"] != false {
		t.Errorf("running = %v, want false", parsed["running"])
	}
}
