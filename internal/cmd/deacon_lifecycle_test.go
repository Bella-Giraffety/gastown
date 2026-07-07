package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/deacon"
)

type fakeDeaconLifecycle struct {
	startErr error
	stopErr  error
	calls    []string
}

func (f *fakeDeaconLifecycle) Start(agentOverride string) error {
	f.calls = append(f.calls, "start:"+agentOverride)
	return f.startErr
}

func (f *fakeDeaconLifecycle) Stop() error {
	f.calls = append(f.calls, "stop")
	return f.stopErr
}

func installFakeDeaconLifecycle(t *testing.T, fake *fakeDeaconLifecycle) {
	t.Helper()

	oldNew := newDeaconLifecycle
	oldExists := deaconSessionExists
	oldAttach := attachDeaconSession
	oldAgent := deaconAgentOverride

	newDeaconLifecycle = func() (deaconLifecycle, error) { return fake, nil }
	deaconSessionExists = func(string) (bool, error) { return false, nil }
	attachDeaconSession = func(sessionName string) error {
		fake.calls = append(fake.calls, "attach:"+sessionName)
		return nil
	}
	deaconAgentOverride = ""

	t.Cleanup(func() {
		newDeaconLifecycle = oldNew
		deaconSessionExists = oldExists
		attachDeaconSession = oldAttach
		deaconAgentOverride = oldAgent
	})
}

func assertDeaconLifecycleCalls(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls = %v, want %v", got, want)
		}
	}
}

func TestRunDeaconStartUsesManager(t *testing.T) {
	fake := &fakeDeaconLifecycle{}
	installFakeDeaconLifecycle(t, fake)
	deaconAgentOverride = "codex"

	if err := runDeaconStart(nil, nil); err != nil {
		t.Fatalf("runDeaconStart() error = %v", err)
	}
	assertDeaconLifecycleCalls(t, fake.calls, "start:codex")
}

func TestRunDeaconStartAlreadyRunning(t *testing.T) {
	fake := &fakeDeaconLifecycle{startErr: deacon.ErrAlreadyRunning}
	installFakeDeaconLifecycle(t, fake)

	err := runDeaconStart(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("runDeaconStart() error = %v, want already running", err)
	}
	assertDeaconLifecycleCalls(t, fake.calls, "start:")
}

func TestRunDeaconStopUsesManager(t *testing.T) {
	fake := &fakeDeaconLifecycle{}
	installFakeDeaconLifecycle(t, fake)

	if err := runDeaconStop(nil, nil); err != nil {
		t.Fatalf("runDeaconStop() error = %v", err)
	}
	assertDeaconLifecycleCalls(t, fake.calls, "stop")
}

func TestRunDeaconStopNotRunning(t *testing.T) {
	fake := &fakeDeaconLifecycle{stopErr: deacon.ErrNotRunning}
	installFakeDeaconLifecycle(t, fake)

	err := runDeaconStop(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("runDeaconStop() error = %v, want not running", err)
	}
	assertDeaconLifecycleCalls(t, fake.calls, "stop")
}

func TestRunDeaconRestartStopsThenStarts(t *testing.T) {
	fake := &fakeDeaconLifecycle{}
	installFakeDeaconLifecycle(t, fake)
	deaconAgentOverride = "claude"

	if err := runDeaconRestart(nil, nil); err != nil {
		t.Fatalf("runDeaconRestart() error = %v", err)
	}
	assertDeaconLifecycleCalls(t, fake.calls, "stop", "start:claude")
}

func TestRunDeaconRestartIgnoresNotRunning(t *testing.T) {
	fake := &fakeDeaconLifecycle{stopErr: deacon.ErrNotRunning}
	installFakeDeaconLifecycle(t, fake)

	if err := runDeaconRestart(nil, nil); err != nil {
		t.Fatalf("runDeaconRestart() error = %v", err)
	}
	assertDeaconLifecycleCalls(t, fake.calls, "stop", "start:")
}

func TestRunDeaconAttachAutoStartUsesManager(t *testing.T) {
	fake := &fakeDeaconLifecycle{}
	installFakeDeaconLifecycle(t, fake)
	deaconAgentOverride = "opus"

	if err := runDeaconAttach(nil, nil); err != nil {
		t.Fatalf("runDeaconAttach() error = %v", err)
	}
	assertDeaconLifecycleCalls(t, fake.calls, "start:opus", "attach:hq-deacon")
}

func TestRunDeaconAttachRunningDoesNotStart(t *testing.T) {
	fake := &fakeDeaconLifecycle{}
	installFakeDeaconLifecycle(t, fake)
	deaconSessionExists = func(string) (bool, error) { return true, nil }

	if err := runDeaconAttach(nil, nil); err != nil {
		t.Fatalf("runDeaconAttach() error = %v", err)
	}
	assertDeaconLifecycleCalls(t, fake.calls, "attach:hq-deacon")
}

func TestRunDeaconAttachSessionCheckError(t *testing.T) {
	fake := &fakeDeaconLifecycle{}
	installFakeDeaconLifecycle(t, fake)
	checkErr := errors.New("tmux down")
	deaconSessionExists = func(string) (bool, error) { return false, checkErr }

	err := runDeaconAttach(nil, nil)
	if !errors.Is(err, checkErr) {
		t.Fatalf("runDeaconAttach() error = %v, want %v", err, checkErr)
	}
	assertDeaconLifecycleCalls(t, fake.calls)
}
