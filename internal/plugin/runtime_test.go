package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/dog"
	"github.com/steveyegge/gastown/internal/mail"
)

type fakeRecorder struct {
	records []PluginRunRecord
}

func (f *fakeRecorder) RecordRun(record PluginRunRecord) (string, error) {
	f.records = append(f.records, record)
	return fmt.Sprintf("gt-rec-%d", len(f.records)), nil
}

type fakeDogDispatcher struct {
	body  string
	opts  DogDispatchOptions
	calls int
	err   error
	res   *DogDispatchResult
}

func (f *fakeDogDispatcher) DispatchPlugin(_ context.Context, _ *Plugin, body string, opts DogDispatchOptions) (*DogDispatchResult, error) {
	f.calls++
	f.body = body
	f.opts = opts
	if f.res == nil {
		f.res = &DogDispatchResult{Dog: "alpha", Work: "plugin:test-plugin", SessionStarted: true, WorkConfirmed: true}
	}
	return f.res, f.err
}

type fakeInFlightDispatcher struct {
	fakeDogDispatcher
	inFlight *DogDispatchResult
	inErr    error
}

func (f *fakeInFlightDispatcher) InFlightPlugin(context.Context, *Plugin) (*DogDispatchResult, error) {
	return f.inFlight, f.inErr
}

func testRuntimePlugin(t *testing.T, script string) (*Runtime, *fakeRecorder, *Plugin) {
	t.Helper()
	townRoot := t.TempDir()
	pluginDir := filepath.Join(townRoot, "plugins", "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("mkdir plugin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "run.sh"), []byte(script), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	recorder := &fakeRecorder{}
	runtime := NewRuntime(townRoot, recorder, &fakeDogDispatcher{})
	runtime.DefaultTimeout = time.Second
	p := &Plugin{
		Name:         "test-plugin",
		Description:  "test plugin",
		Path:         pluginDir,
		HasRunScript: true,
		Instructions: "Do the AI part.",
	}
	return runtime, recorder, p
}

func TestRuntimeExecuteScriptSuccessRecordsCooldownCounted(t *testing.T) {
	runtime, recorder, p := testRuntimePlugin(t, "#!/usr/bin/env bash\necho ok\n")
	dispatcher := &fakeDogDispatcher{}
	runtime.DogDispatcher = dispatcher
	outcome, err := runtime.Execute(context.Background(), p, RunOptions{Trigger: TriggerAuto, CreateDog: true})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if outcome.Result != ResultSuccess || !outcome.CooldownCounted || outcome.Retryable {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if dispatcher.calls != 0 {
		t.Fatalf("script success dispatched dog %d time(s), want 0", dispatcher.calls)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("records = %d, want 1", len(recorder.records))
	}
	record := recorder.records[0]
	for _, want := range []string{LabelAuthorityRunner, "executor:script", "trigger:auto", LabelCooldownCounted, LabelRetryableFalse, "exit-code:0"} {
		if !contains(record.ExtraLabels, want) {
			t.Fatalf("record labels missing %q: %v", want, record.ExtraLabels)
		}
	}
}

func TestRuntimeExecuteScriptFailureRetryableNotCooldownCounted(t *testing.T) {
	runtime, recorder, p := testRuntimePlugin(t, "#!/usr/bin/env bash\necho bad\nexit 7\n")
	outcome, err := runtime.Execute(context.Background(), p, RunOptions{Trigger: TriggerManual})
	if !errors.Is(err, ErrScriptFailed) {
		t.Fatalf("Execute error = %v, want ErrScriptFailed", err)
	}
	if outcome.Result != ResultFailure || outcome.CooldownCounted || !outcome.Retryable {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	record := recorder.records[0]
	for _, want := range []string{LabelAuthorityRunner, "executor:script", "trigger:manual", "cooldown:retryable", LabelRetryableTrue, "exit-code:7"} {
		if !contains(record.ExtraLabels, want) {
			t.Fatalf("record labels missing %q: %v", want, record.ExtraLabels)
		}
	}
}

func TestRuntimeExecuteScriptTimeoutRetryable(t *testing.T) {
	runtime, _, p := testRuntimePlugin(t, "#!/usr/bin/env bash\nsleep 2\n")
	runtime.DefaultTimeout = 50 * time.Millisecond
	outcome, err := runtime.Execute(context.Background(), p, RunOptions{Trigger: TriggerAuto, TargetDog: "bravo"})
	if !errors.Is(err, ErrScriptFailed) {
		t.Fatalf("Execute error = %v, want ErrScriptFailed", err)
	}
	if outcome.Result != ResultTimeout || !outcome.Script.TimedOut || outcome.CooldownCounted || !outcome.Retryable {
		t.Fatalf("unexpected timeout outcome: %+v", outcome)
	}
}

func TestRuntimeExecuteExit10DispatchesAgentStepWithoutRerun(t *testing.T) {
	townRoot := t.TempDir()
	pluginDir := filepath.Join(townRoot, "plugins", "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("mkdir plugin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "run.sh"), []byte("#!/usr/bin/env bash\necho precheck\nexit 10\n"), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	recorder := &fakeRecorder{}
	dispatcher := &fakeDogDispatcher{}
	runtime := NewRuntime(townRoot, recorder, dispatcher)
	p := &Plugin{Name: "test-plugin", Description: "test", Path: pluginDir, HasRunScript: true, Instructions: "Analyze."}
	outcome, err := runtime.Execute(context.Background(), p, RunOptions{Trigger: TriggerDogDispatch, CreateDog: true})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if outcome.Result != ResultDogDispatched || outcome.CooldownCounted || outcome.Retryable {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if !contains(recorder.records[0].ExtraLabels, "cooldown:not-counted") {
		t.Fatalf("dispatch record should not count cooldown: %v", recorder.records[0].ExtraLabels)
	}
	if strings.Contains(dispatcher.body, "bash run.sh") || !strings.Contains(dispatcher.body, "Do NOT run `run.sh` again") || !strings.Contains(dispatcher.body, "precheck") {
		t.Fatalf("bad exit10 body:\n%s", dispatcher.body)
	}
	if !dispatcher.opts.CreateIfNeeded {
		t.Fatalf("exit10 dispatch did not carry create-if-needed option: %+v", dispatcher.opts)
	}
}

func TestRuntimeDispatchFailureRetryableNotCooldownCounted(t *testing.T) {
	runtime, recorder, p := testRuntimePlugin(t, "#!/usr/bin/env bash\nexit 10\n")
	runtime.DogDispatcher = &fakeDogDispatcher{err: ErrNoDogAvailable}
	outcome, err := runtime.Execute(context.Background(), p, RunOptions{Trigger: TriggerAuto})
	if !errors.Is(err, ErrNoDogAvailable) {
		t.Fatalf("Execute error = %v, want ErrNoDogAvailable", err)
	}
	if outcome.Result != ResultDispatchFailure || outcome.CooldownCounted || !outcome.Retryable {
		t.Fatalf("unexpected dispatch failure outcome: %+v", outcome)
	}
	record := recorder.records[0]
	for _, want := range []string{LabelAuthorityRunner, "executor:dog", "trigger:auto", "cooldown:retryable", LabelRetryableTrue} {
		if !contains(record.ExtraLabels, want) {
			t.Fatalf("record labels missing %q: %v", want, record.ExtraLabels)
		}
	}
}

func TestRuntimeInFlightDispatchSkipsWithoutCooldown(t *testing.T) {
	runtime, recorder, p := testRuntimePlugin(t, "#!/usr/bin/env bash\nexit 10\n")
	runtime.DogDispatcher = &fakeDogDispatcher{
		err: ErrPluginInFlight,
		res: &DogDispatchResult{Dog: "alpha", Work: "plugin:test-plugin"},
	}
	outcome, err := runtime.Execute(context.Background(), p, RunOptions{Trigger: TriggerAuto})
	if err != nil {
		t.Fatalf("Execute returned error for in-flight dispatch: %v", err)
	}
	if outcome.Result != ResultSkipped || outcome.CooldownCounted || outcome.Retryable {
		t.Fatalf("unexpected in-flight outcome: %+v", outcome)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("records = %d, want 1", len(recorder.records))
	}
	for _, want := range []string{LabelAuthorityRunner, "executor:dog", "trigger:auto", "cooldown:not-counted", LabelRetryableFalse, "dog:alpha"} {
		if !contains(recorder.records[0].ExtraLabels, want) {
			t.Fatalf("record labels missing %q: %v", want, recorder.records[0].ExtraLabels)
		}
	}
}

func TestRuntimeScriptPluginInFlightSkipsBeforeRunScript(t *testing.T) {
	runtime, recorder, p := testRuntimePlugin(t, "#!/usr/bin/env bash\ntouch ran\nexit 10\n")
	dispatcher := &fakeInFlightDispatcher{inFlight: &DogDispatchResult{Dog: "alpha", Work: "plugin:test-plugin"}}
	runtime.DogDispatcher = dispatcher
	outcome, err := runtime.Execute(context.Background(), p, RunOptions{Trigger: TriggerAuto})
	if err != nil {
		t.Fatalf("Execute returned error for in-flight script plugin: %v", err)
	}
	if outcome.Result != ResultSkipped || outcome.CooldownCounted || outcome.Retryable {
		t.Fatalf("unexpected in-flight outcome: %+v", outcome)
	}
	if dispatcher.calls != 0 {
		t.Fatalf("dispatcher called %d time(s), want 0", dispatcher.calls)
	}
	if _, statErr := os.Stat(filepath.Join(p.Path, "ran")); !os.IsNotExist(statErr) {
		t.Fatalf("run.sh executed despite in-flight plugin, stat err=%v", statErr)
	}
	for _, want := range []string{LabelAuthorityRunner, "executor:dog", "trigger:auto", "cooldown:not-counted", LabelRetryableFalse, "dog:alpha"} {
		if !contains(recorder.records[0].ExtraLabels, want) {
			t.Fatalf("record labels missing %q: %v", want, recorder.records[0].ExtraLabels)
		}
	}
}

func TestRuntimeAgentPluginDispatchRecordsDogDispatched(t *testing.T) {
	townRoot := t.TempDir()
	recorder := &fakeRecorder{}
	dispatcher := &fakeDogDispatcher{}
	runtime := NewRuntime(townRoot, recorder, dispatcher)
	p := &Plugin{Name: "agent-plugin", Description: "agent", Path: filepath.Join(townRoot, "plugins", "agent-plugin"), Instructions: "Do agent work."}
	outcome, err := runtime.Execute(context.Background(), p, RunOptions{Trigger: TriggerManual, CreateDog: true})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if outcome.Result != ResultDogDispatched || outcome.Executor != ExecutorDog || outcome.CooldownCounted {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if len(recorder.records) != 1 || recorder.records[0].Result != ResultDogDispatched {
		t.Fatalf("unexpected records: %+v", recorder.records)
	}
	if !strings.Contains(dispatcher.body, "Do agent work.") || strings.Contains(dispatcher.body, "bash run.sh") {
		t.Fatalf("bad agent body:\n%s", dispatcher.body)
	}
	if !dispatcher.opts.CreateIfNeeded {
		t.Fatalf("agent dispatch did not carry create-if-needed option: %+v", dispatcher.opts)
	}
}

func TestRuntimeScriptStartFailureRecordsRetryableReceipt(t *testing.T) {
	runtime, recorder, p := testRuntimePlugin(t, "#!/usr/bin/env bash\necho should not run\n")
	p.Execution = &Execution{Timeout: "not-a-duration"}
	outcome, err := runtime.Execute(context.Background(), p, RunOptions{Trigger: TriggerAuto})
	if !errors.Is(err, ErrScriptFailed) {
		t.Fatalf("Execute error = %v, want ErrScriptFailed", err)
	}
	if outcome.Result != ResultFailure || outcome.CooldownCounted || !outcome.Retryable {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("records = %d, want 1", len(recorder.records))
	}
	for _, want := range []string{LabelAuthorityRunner, "executor:script", "trigger:auto", "cooldown:retryable", LabelRetryableTrue} {
		if !contains(recorder.records[0].ExtraLabels, want) {
			t.Fatalf("record labels missing %q: %v", want, recorder.records[0].ExtraLabels)
		}
	}
}

func TestRunScriptBoundsOutput(t *testing.T) {
	runtime, _, p := testRuntimePlugin(t, "#!/usr/bin/env bash\nprintf 'x%.0s' {1..200}\n")
	runtime.OutputLimit = 80
	result, err := runtime.RunScript(context.Background(), p, TriggerManual)
	if err != nil {
		t.Fatalf("RunScript returned error: %v", err)
	}
	if len(result.Output) > 80 || !result.Truncated {
		t.Fatalf("output not bounded/truncated: len=%d truncated=%v", len(result.Output), result.Truncated)
	}
	if !strings.HasPrefix(result.Output, "xxxxxxxx") {
		t.Fatalf("bounded tail did not retain script output tail: %q", result.Output)
	}
}

func TestRunScriptAllowlistsEnv(t *testing.T) {
	runtime, _, p := testRuntimePlugin(t, "#!/usr/bin/env bash\nenv | sort\n")
	runtime.OutputLimit = 16 * 1024
	t.Setenv("SECRET_TOKEN", "do-not-leak")
	t.Setenv("GT_DOLT_DATA", "do-not-leak")
	result, err := runtime.RunScript(context.Background(), p, TriggerManual)
	if err != nil {
		t.Fatalf("RunScript returned error: %v", err)
	}
	if strings.Contains(result.Output, "SECRET_TOKEN") || strings.Contains(result.Output, "GT_DOLT_DATA") || strings.Contains(result.Output, "do-not-leak") {
		t.Fatalf("runner leaked disallowed env in output:\n%s", result.Output)
	}
}

func TestRuntimeReceiptOmitsScriptOutput(t *testing.T) {
	runtime, recorder, p := testRuntimePlugin(t, "#!/usr/bin/env bash\necho secret-output\nexit 7\n")
	_, err := runtime.Execute(context.Background(), p, RunOptions{Trigger: TriggerAuto})
	if !errors.Is(err, ErrScriptFailed) {
		t.Fatalf("Execute error = %v, want ErrScriptFailed", err)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("records = %d, want 1", len(recorder.records))
	}
	if strings.Contains(recorder.records[0].Body, "secret-output") {
		t.Fatalf("receipt body leaked script output:\n%s", recorder.records[0].Body)
	}
	if !strings.Contains(recorder.records[0].Body, "Output: omitted from receipt") {
		t.Fatalf("receipt body missing output omission note:\n%s", recorder.records[0].Body)
	}
}

func TestRunnerEnvUsesBoundedPathAllowlist(t *testing.T) {
	t.Setenv("PATH", "/tmp/evil:/usr/bin")
	t.Setenv("HOME", "/home/tester")
	env := RunnerEnv("/town", &Plugin{Name: "test-plugin", Path: "/town/plugins/test-plugin"}, TriggerAuto)
	path := envValue(env, "PATH")
	if strings.Contains(path, "/tmp/evil") {
		t.Fatalf("runner PATH inherited disallowed parent entry: %q", path)
	}
	for _, want := range []string{"/home/tester/.local/bin", "/home/tester/go/bin", "/usr/bin", "/bin"} {
		if !strings.Contains(path, want) {
			t.Fatalf("runner PATH missing %q in %q", want, path)
		}
	}
	if strings.Contains(strings.Join(env, "\n"), "GT_DOLT_DATA=") {
		t.Fatalf("runner env leaked disallowed GT_DOLT_DATA: %v", env)
	}
}

func TestRunnerEnvIncludesCanonicalDoltEndpointAliases(t *testing.T) {
	townRoot := t.TempDir()
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	metadata := []byte(`{"dolt_server_host":"10.0.0.7","dolt_server_port":3311}`)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), metadata, 0644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	env := RunnerEnv(townRoot, &Plugin{Name: "test-plugin", Path: filepath.Join(townRoot, "plugins", "test-plugin")}, TriggerAuto)
	for key, want := range map[string]string{
		"GT_DOLT_HOST":           "10.0.0.7",
		"DOLT_HOST":              "10.0.0.7",
		"GT_DOLT_PORT":           "3311",
		"DOLT_PORT":              "3311",
		"BEADS_DOLT_SERVER_HOST": "10.0.0.7",
		"BEADS_DOLT_SERVER_PORT": "3311",
	} {
		if got := envValue(env, key); got != want {
			t.Fatalf("%s = %q, want %q in env %v", key, got, want, env)
		}
	}
}

func TestDogPoolDispatcherAssignsWorkBeforeSessionAndMail(t *testing.T) {
	townRoot := t.TempDir()
	mgr := dog.NewManager(townRoot, nil)
	setupIdleRuntimeDog(t, townRoot, "alpha")
	assertAssigned := func(where string) {
		t.Helper()
		dg, err := mgr.Get("alpha")
		if err != nil {
			t.Fatalf("%s: Get dog: %v", where, err)
		}
		if dg.State != dog.StateWorking || dg.Work != "plugin:test-plugin" {
			t.Fatalf("%s: dog assignment = state %q work %q, want working plugin:test-plugin", where, dg.State, dg.Work)
		}
	}
	dispatcher := &DogPoolDispatcher{
		TownRoot:      townRoot,
		Manager:       mgr,
		Sessions:      fakeSessionEnsurer{assert: assertAssigned},
		Router:        fakeMailSender{assert: assertAssigned},
		EnsureDogBead: func(string) error { return nil },
	}

	result, err := dispatcher.DispatchPlugin(context.Background(), &Plugin{Name: "test-plugin"}, "body", DogDispatchOptions{})
	if err != nil {
		t.Fatalf("DispatchPlugin returned error: %v", err)
	}
	if result == nil || result.Dog != "alpha" || !result.SessionStarted || !result.WorkConfirmed {
		t.Fatalf("unexpected dispatch result: %+v", result)
	}
	assertAssigned("after dispatch")
}

func TestDogPoolDispatcherClearsExactAssignmentOnSessionFailure(t *testing.T) {
	townRoot := t.TempDir()
	mgr := dog.NewManager(townRoot, nil)
	setupIdleRuntimeDog(t, townRoot, "alpha")
	dispatcher := &DogPoolDispatcher{
		TownRoot:      townRoot,
		Manager:       mgr,
		Sessions:      fakeSessionEnsurer{err: errors.New("session refused")},
		Router:        fakeMailSender{},
		EnsureDogBead: func(string) error { return nil },
	}

	_, err := dispatcher.DispatchPlugin(context.Background(), &Plugin{Name: "test-plugin"}, "body", DogDispatchOptions{TargetDog: "alpha"})
	if err == nil || !strings.Contains(err.Error(), "session refused") {
		t.Fatalf("DispatchPlugin error = %v, want session refused", err)
	}
	dg, getErr := mgr.Get("alpha")
	if getErr != nil {
		t.Fatalf("Get dog: %v", getErr)
	}
	if dg.State != dog.StateIdle || dg.Work != "" {
		t.Fatalf("dog assignment after failed session = state %q work %q, want idle empty", dg.State, dg.Work)
	}
}

type fakeSessionEnsurer struct {
	assert func(where string)
	err    error
}

func setupIdleRuntimeDog(t *testing.T, townRoot, name string) {
	t.Helper()
	kennelDir := filepath.Join(townRoot, "deacon", "dogs", name)
	if err := os.MkdirAll(kennelDir, 0755); err != nil {
		t.Fatalf("mkdir dog: %v", err)
	}
	now := time.Now()
	state := &dog.DogState{
		Name:       name,
		State:      dog.StateIdle,
		LastActive: now,
		Worktrees:  map[string]string{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal dog state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(kennelDir, ".dog.json"), data, 0644); err != nil {
		t.Fatalf("write dog state: %v", err)
	}
}

func (f fakeSessionEnsurer) EnsureRunning(_ string, _ dog.SessionStartOptions) (string, error) {
	if f.assert != nil {
		f.assert("session start")
	}
	if f.err != nil {
		return "", f.err
	}
	return "%1", nil
}

type fakeMailSender struct {
	assert func(where string)
	err    error
}

func (f fakeMailSender) Send(_ *mail.Message) error {
	if f.assert != nil {
		f.assert("mail send")
	}
	return f.err
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func envValue(env []string, name string) string {
	prefix := name + "="
	for _, value := range env {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return ""
}
