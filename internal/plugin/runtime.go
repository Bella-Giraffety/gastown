package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/dog"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/util"
)

const (
	DefaultScriptTimeout = 5 * time.Minute
	DefaultOutputLimit   = 32 * 1024
	DefaultRecordLimit   = 32 * 1024

	TriggerAuto        = "auto"
	TriggerManual      = "manual"
	TriggerDogDispatch = "dog-dispatch"

	ExecutorScript = "script"
	ExecutorDog    = "dog"
)

var (
	ErrNoDogAvailable = errors.New("no idle dogs available")
	ErrScriptFailed   = errors.New("plugin script failed")
	ErrPluginInFlight = errors.New("plugin already dispatched")
)

type RunRecorder interface {
	RecordRun(PluginRunRecord) (string, error)
}

type DogDispatcher interface {
	DispatchPlugin(ctx context.Context, p *Plugin, body string, opts DogDispatchOptions) (*DogDispatchResult, error)
}

type InFlightChecker interface {
	InFlightPlugin(ctx context.Context, p *Plugin) (*DogDispatchResult, error)
}

type DogSessionEnsurer interface {
	EnsureRunning(dogName string, opts dog.SessionStartOptions) (string, error)
}

type MailSender interface {
	Send(msg *mail.Message) error
}

type Runtime struct {
	TownRoot       string
	Recorder       RunRecorder
	DogDispatcher  DogDispatcher
	OutputLimit    int
	DefaultTimeout time.Duration
}

type RunOptions struct {
	Trigger   string
	TargetDog string
	CreateDog bool
	From      string
	DryRun    bool
}

type RunOutcome struct {
	Plugin          string             `json:"plugin"`
	PluginRig       string             `json:"plugin_rig,omitempty"`
	PluginPath      string             `json:"plugin_path"`
	Trigger         string             `json:"trigger"`
	Executor        string             `json:"executor"`
	Result          RunResult          `json:"result"`
	ReceiptID       string             `json:"receipt_id,omitempty"`
	Script          *ScriptResult      `json:"script,omitempty"`
	Dispatch        *DogDispatchResult `json:"dispatch,omitempty"`
	DryRun          bool               `json:"dry_run,omitempty"`
	CooldownCounted bool               `json:"cooldown_counted"`
	Retryable       bool               `json:"retryable"`
}

type ScriptResult struct {
	ExitCode    int              `json:"exit_code"`
	TimedOut    bool             `json:"timed_out"`
	Duration    time.Duration    `json:"duration"`
	Output      string           `json:"output,omitempty"`
	Truncated   bool             `json:"truncated,omitempty"`
	RecordedRun *RunnerRecordRun `json:"recorded_run,omitempty"`
}

type DogDispatchOptions struct {
	Trigger        string
	TargetDog      string
	CreateIfNeeded bool
	From           string
}

type DogDispatchResult struct {
	Dog            string   `json:"dog,omitempty"`
	DogCreated     bool     `json:"dog_created,omitempty"`
	Work           string   `json:"work,omitempty"`
	SessionStarted bool     `json:"session_started"`
	WorkConfirmed  bool     `json:"work_confirmed"`
	Warnings       []string `json:"warnings,omitempty"`
}

func NewRuntime(townRoot string, recorder RunRecorder, dispatcher DogDispatcher) *Runtime {
	if recorder == nil {
		recorder = NewRecorder(townRoot)
	}
	return &Runtime{
		TownRoot:       townRoot,
		Recorder:       recorder,
		DogDispatcher:  dispatcher,
		OutputLimit:    DefaultOutputLimit,
		DefaultTimeout: DefaultScriptTimeout,
	}
}

func (r *Runtime) Execute(ctx context.Context, p *Plugin, opts RunOptions) (*RunOutcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	trigger := opts.Trigger
	if trigger == "" {
		trigger = TriggerManual
	}
	outcome := &RunOutcome{
		Plugin:     p.Name,
		PluginRig:  p.RigName,
		PluginPath: p.Path,
		Trigger:    trigger,
		DryRun:     opts.DryRun,
	}

	if opts.DryRun {
		if p.HasRunScript {
			outcome.Executor = ExecutorScript
		} else {
			outcome.Executor = ExecutorDog
		}
		return outcome, nil
	}

	if p.HasRunScript {
		outcome.Executor = ExecutorDog
		inFlight, inFlightErr := r.inFlightPlugin(ctx, p)
		if inFlightErr != nil {
			return r.recordOutcome(outcome, p, ResultDispatchFailure, nil, nil, true, false, inFlightErr)
		}
		if inFlight != nil {
			outcome.Dispatch = inFlight
			return r.recordOutcome(outcome, p, ResultSkipped, nil, inFlight, false, false, nil)
		}
		outcome.Executor = ExecutorScript
		script, err := r.RunScript(ctx, p, trigger)
		outcome.Script = script
		if err != nil {
			return r.recordOutcome(outcome, p, ResultFailure, script, nil, true, false, fmt.Errorf("%w: %v", ErrScriptFailed, err))
		}
		if script.TimedOut {
			return r.recordOutcome(outcome, p, ResultTimeout, script, nil, true, false, fmt.Errorf("%w: timed out", ErrScriptFailed))
		}
		switch script.ExitCode {
		case 0:
			result := ResultSuccess
			if script.RecordedRun != nil && script.RecordedRun.Result != "" {
				result = script.RecordedRun.Result
			}
			retryable, cooldownCounted := resultOutcomePolicy(result)
			return r.recordOutcome(outcome, p, result, script, nil, retryable, cooldownCounted, nil)
		case 10:
			body := p.FormatAgentStepMailBody(script.Output)
			dispatch, dispatchErr := r.dispatchDog(ctx, p, body, DogDispatchOptions{
				Trigger:        trigger,
				TargetDog:      opts.TargetDog,
				CreateIfNeeded: opts.CreateDog,
				From:           opts.From,
			})
			outcome.Dispatch = dispatch
			outcome.Executor = ExecutorDog
			if dispatchErr != nil {
				if errors.Is(dispatchErr, ErrPluginInFlight) {
					return r.recordOutcome(outcome, p, ResultSkipped, script, dispatch, false, false, nil)
				}
				return r.recordOutcome(outcome, p, ResultDispatchFailure, script, dispatch, true, false, dispatchErr)
			}
			return r.recordOutcome(outcome, p, ResultDogDispatched, script, dispatch, false, false, nil)
		default:
			return r.recordOutcome(outcome, p, ResultFailure, script, nil, true, false, fmt.Errorf("%w: exit code %d", ErrScriptFailed, script.ExitCode))
		}
	}

	body := p.FormatMailBody()
	dispatch, dispatchErr := r.dispatchDog(ctx, p, body, DogDispatchOptions{
		Trigger:        trigger,
		TargetDog:      opts.TargetDog,
		CreateIfNeeded: opts.CreateDog,
		From:           opts.From,
	})
	outcome.Dispatch = dispatch
	outcome.Executor = ExecutorDog
	if dispatchErr != nil {
		if errors.Is(dispatchErr, ErrPluginInFlight) {
			return r.recordOutcome(outcome, p, ResultSkipped, nil, dispatch, false, false, nil)
		}
		return r.recordOutcome(outcome, p, ResultDispatchFailure, nil, dispatch, true, false, dispatchErr)
	}
	return r.recordOutcome(outcome, p, ResultDogDispatched, nil, dispatch, false, false, nil)
}

func (r *Runtime) RunScript(ctx context.Context, p *Plugin, trigger string) (*ScriptResult, error) {
	if p == nil {
		return nil, fmt.Errorf("plugin is nil")
	}
	scriptPath := filepath.Join(p.Path, "run.sh")
	info, err := os.Lstat(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("stat run.sh: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("run.sh is not a regular file: %s", scriptPath)
	}

	timeout := r.DefaultTimeout
	if timeout <= 0 {
		timeout = DefaultScriptTimeout
	}
	if p.Execution != nil && p.Execution.Timeout != "" {
		parsed, parseErr := time.ParseDuration(p.Execution.Timeout)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing plugin timeout %q: %w", p.Execution.Timeout, parseErr)
		}
		timeout = parsed
	}

	limit := r.OutputLimit
	if limit <= 0 {
		limit = DefaultOutputLimit
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bashPath, err := resolveBash()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(runCtx, bashPath, "./run.sh")
	cmd.Dir = p.Path
	recordPath, cleanupRecord, err := runnerRecordPath()
	if err != nil {
		return nil, err
	}
	defer cleanupRecord()
	cmd.Env = append(RunnerEnv(r.TownRoot, p, trigger), "GT_PLUGIN_RUNNER_RECORD_FILE="+recordPath)
	util.SetProcessGroup(cmd)

	output := newBoundedTail(limit)
	cmd.Stdout = output
	cmd.Stderr = output

	started := time.Now()
	err = cmd.Run()
	result := &ScriptResult{
		ExitCode:  0,
		TimedOut:  errors.Is(runCtx.Err(), context.DeadlineExceeded),
		Duration:  time.Since(started),
		Output:    output.String(),
		Truncated: output.Truncated(),
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else if result.TimedOut {
			result.ExitCode = -1
		} else {
			result.ExitCode = -1
			return result, fmt.Errorf("running run.sh: %w", err)
		}
	}
	recorded, recordErr := readRunnerRecord(recordPath)
	if recordErr != nil {
		return result, recordErr
	}
	result.RecordedRun = recorded
	return result, nil
}

func (r *Runtime) dispatchDog(ctx context.Context, p *Plugin, body string, opts DogDispatchOptions) (*DogDispatchResult, error) {
	if r.DogDispatcher == nil {
		return nil, ErrNoDogAvailable
	}
	return r.DogDispatcher.DispatchPlugin(ctx, p, body, opts)
}

func (r *Runtime) inFlightPlugin(ctx context.Context, p *Plugin) (*DogDispatchResult, error) {
	checker, ok := r.DogDispatcher.(InFlightChecker)
	if !ok || checker == nil {
		return nil, nil
	}
	return checker.InFlightPlugin(ctx, p)
}

func (r *Runtime) recordOutcome(outcome *RunOutcome, p *Plugin, result RunResult, script *ScriptResult, dispatch *DogDispatchResult, retryable bool, cooldownCounted bool, outcomeErr error) (*RunOutcome, error) {
	outcome.Result = result
	outcome.Retryable = retryable
	outcome.CooldownCounted = cooldownCounted
	labels := []string{
		LabelAuthorityRunner,
		"executor:" + outcome.Executor,
		"trigger:" + outcome.Trigger,
	}
	if retryable {
		labels = append(labels, LabelRetryableTrue)
	} else {
		labels = append(labels, LabelRetryableFalse)
	}
	if cooldownCounted {
		labels = append(labels, LabelCooldownCounted)
	} else if retryable {
		labels = append(labels, "cooldown:retryable")
	} else {
		labels = append(labels, "cooldown:not-counted")
	}
	if script != nil {
		labels = append(labels, fmt.Sprintf("exit-code:%d", script.ExitCode))
		if script.TimedOut {
			labels = append(labels, "timed-out:true")
		}
	}
	if dispatch != nil && dispatch.Dog != "" {
		labels = append(labels, "dog:"+dispatch.Dog)
	}

	body := formatOutcomeBody(result, script, dispatch, outcomeErr)
	title := fmt.Sprintf("Plugin run: %s (%s)", p.Name, result)
	rigName := p.RigName
	if script != nil && script.RecordedRun != nil {
		if script.RecordedRun.RigName != "" {
			rigName = script.RecordedRun.RigName
		}
		if script.RecordedRun.Title != "" {
			title = script.RecordedRun.Title
		}
		if script.RecordedRun.Body != "" {
			body = script.RecordedRun.Body
		}
		labels = append(labels, runnerRecordExtraLabels(script.RecordedRun.ExtraLabels)...)
	}
	receiptID, recordErr := r.Recorder.RecordRun(PluginRunRecord{
		PluginName:  p.Name,
		RigName:     rigName,
		Result:      result,
		Title:       title,
		Body:        body,
		ExtraLabels: labels,
	})
	if recordErr != nil {
		if outcomeErr != nil {
			return outcome, fmt.Errorf("%w; recording plugin outcome: %v", outcomeErr, recordErr)
		}
		return outcome, fmt.Errorf("recording plugin outcome: %w", recordErr)
	}
	outcome.ReceiptID = receiptID
	return outcome, outcomeErr
}

func resultOutcomePolicy(result RunResult) (retryable bool, cooldownCounted bool) {
	switch result {
	case ResultSuccess, ResultSkipped:
		return false, true
	case ResultDogDispatched:
		return false, false
	default:
		return true, false
	}
}

func runnerRecordExtraLabels(labels []string) []string {
	filtered := make([]string, 0, len(labels))
	for _, label := range labels {
		if hasLabelPrefix([]string{label}, "authority:") || hasLabelPrefix([]string{label}, "cooldown:") || hasLabelPrefix([]string{label}, "retryable:") || hasLabelPrefix([]string{label}, "result:") || hasLabelPrefix([]string{label}, "plugin:") || label == "type:plugin-run" {
			continue
		}
		filtered = append(filtered, label)
	}
	return filtered
}

func runnerRecordPath() (string, func(), error) {
	dir, err := os.MkdirTemp("", "gt-plugin-record-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating runner record dir: %w", err)
	}
	return filepath.Join(dir, "record.json"), func() { _ = os.RemoveAll(dir) }, nil
}

func readRunnerRecord(path string) (*RunnerRecordRun, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening runner record file: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, DefaultRecordLimit+1))
	if err != nil {
		return nil, fmt.Errorf("reading runner record file: %w", err)
	}
	if len(data) > DefaultRecordLimit {
		return nil, fmt.Errorf("runner record file exceeds %d bytes", DefaultRecordLimit)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, nil
	}
	var record RunnerRecordRun
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("parsing runner record file: %w", err)
	}
	return &record, nil
}

func formatOutcomeBody(result RunResult, script *ScriptResult, dispatch *DogDispatchResult, outcomeErr error) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Result: %s\n", result))
	if outcomeErr != nil {
		sb.WriteString(fmt.Sprintf("Error: %v\n", outcomeErr))
	}
	if script != nil {
		sb.WriteString(fmt.Sprintf("Exit code: %d\n", script.ExitCode))
		sb.WriteString(fmt.Sprintf("Timed out: %v\n", script.TimedOut))
		sb.WriteString(fmt.Sprintf("Duration: %s\n", script.Duration.Truncate(time.Millisecond)))
		if script.Output != "" {
			if script.Truncated {
				sb.WriteString(fmt.Sprintf("Output: omitted from receipt (%d-byte bounded tail was truncated)\n", len(script.Output)))
			} else {
				sb.WriteString(fmt.Sprintf("Output: omitted from receipt (%d bytes captured)\n", len(script.Output)))
			}
		}
	}
	if dispatch != nil {
		sb.WriteString(fmt.Sprintf("Dog: %s\n", dispatch.Dog))
		sb.WriteString(fmt.Sprintf("Work: %s\n", dispatch.Work))
		sb.WriteString(fmt.Sprintf("Session started: %v\n", dispatch.SessionStarted))
		sb.WriteString(fmt.Sprintf("Work confirmed: %v\n", dispatch.WorkConfirmed))
		for _, warning := range dispatch.Warnings {
			sb.WriteString("Warning: ")
			sb.WriteString(warning)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func RunnerEnv(townRoot string, p *Plugin, trigger string) []string {
	home := os.Getenv("HOME")
	env := []string{
		"GT_TOWN_ROOT=" + townRoot,
		"GT_ROOT=" + townRoot,
		"GT_PLUGIN_NAME=" + p.Name,
		"GT_PLUGIN_DIR=" + p.Path,
		"GT_PLUGIN_RUNNER_ACTIVE=1",
		"GT_PLUGIN_RUNNER_RECEIPTS=authoritative",
		"GT_ROLE=daemon",
		"BD_ACTOR=daemon",
		"TERM=dumb",
		"GIT_CEILING_DIRECTORIES=" + townRoot,
	}
	if p.RigName != "" {
		env = append(env, "GT_PLUGIN_RIG="+p.RigName)
	}
	if trigger != "" {
		env = append(env, "GT_PLUGIN_TRIGGER="+trigger)
	}
	if home != "" {
		env = append(env, "HOME="+home)
	}
	if tmp := os.Getenv("TMPDIR"); tmp != "" {
		env = append(env, "TMPDIR="+tmp)
	} else {
		env = append(env, "TMPDIR="+os.TempDir())
	}
	if lang := os.Getenv("LANG"); lang != "" {
		env = append(env, "LANG="+lang)
	} else {
		env = append(env, "LANG=C.UTF-8")
	}
	if lcAll := os.Getenv("LC_ALL"); lcAll != "" {
		env = append(env, "LC_ALL="+lcAll)
	}
	env = append(env, "PATH="+safePath(home))
	bdEnv := beads.BuildMutationRoutingBDEnv(nil, beads.ResolveBeadsDir(townRoot))
	env = append(env, doltEndpointAliases(bdEnv)...)
	env = append(env, bdEnv...)
	return env
}

func doltEndpointAliases(env []string) []string {
	host := runnerEnvValue(env, "BEADS_DOLT_SERVER_HOST")
	port := runnerEnvValue(env, "BEADS_DOLT_SERVER_PORT")
	if port == "" {
		port = runnerEnvValue(env, "BEADS_DOLT_PORT")
	}
	aliases := make([]string, 0, 4)
	if host != "" {
		aliases = append(aliases, "GT_DOLT_HOST="+host, "DOLT_HOST="+host)
	}
	if port != "" {
		aliases = append(aliases, "GT_DOLT_PORT="+port, "DOLT_PORT="+port)
	}
	return aliases
}

func runnerEnvValue(env []string, key string) string {
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name == key {
			return value
		}
	}
	return ""
}

func safePath(home string) string {
	seen := map[string]bool{}
	dirs := []string{
		"/usr/local/bin",
		"/opt/homebrew/bin",
		"/usr/bin",
		"/bin",
		"/usr/sbin",
		"/sbin",
	}
	if home != "" {
		dirs = append([]string{
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, "go", "bin"),
		}, dirs...)
	}
	kept := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" || !filepath.IsAbs(dir) || seen[dir] {
			continue
		}
		seen[dir] = true
		kept = append(kept, dir)
	}
	return strings.Join(kept, string(os.PathListSeparator))
}

func resolveBash() (string, error) {
	for _, path := range []string{"/bin/bash", "/usr/bin/bash", "/usr/local/bin/bash", "/opt/homebrew/bin/bash"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("bash not found in allowlisted paths")
}

type DogPoolDispatcher struct {
	TownRoot      string
	Manager       *dog.Manager
	Sessions      DogSessionEnsurer
	Router        MailSender
	Logger        *log.Logger
	CreateDogName func(*dog.Manager) string
	EnsureDogBead func(name string) error
}

func (d *DogPoolDispatcher) DispatchPlugin(ctx context.Context, p *Plugin, body string, opts DogDispatchOptions) (*DogDispatchResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.Manager == nil || d.Sessions == nil || d.Router == nil {
		return nil, fmt.Errorf("dog dispatcher is not configured")
	}
	from := opts.From
	if from == "" {
		from = "deacon/"
	}
	workDesc := fmt.Sprintf("plugin:%s", p.Name)
	if inFlight, inFlightErr := d.inFlightDispatch(workDesc); inFlightErr != nil {
		return nil, inFlightErr
	} else if inFlight != nil {
		return inFlight, ErrPluginInFlight
	}
	candidates, err := d.candidateDogs(opts.TargetDog)
	if err != nil {
		return nil, err
	}
	createdDog := ""
	if len(candidates) == 0 && opts.CreateIfNeeded && opts.TargetDog == "" {
		created, createErr := d.createDog()
		if createErr != nil {
			return nil, createErr
		}
		createdDog = created.Name
		candidates = []*dog.Dog{created}
	}
	if len(candidates) == 0 {
		return nil, ErrNoDogAvailable
	}

	var lastErr error
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if inFlight, inFlightErr := d.inFlightDispatch(workDesc); inFlightErr != nil {
			return nil, inFlightErr
		} else if inFlight != nil {
			return inFlight, ErrPluginInFlight
		}

		result := &DogDispatchResult{Dog: candidate.Name, DogCreated: candidate.Name == createdDog, Work: workDesc}
		assigned, assignErr := d.Manager.AssignWorkIfIdle(candidate.Name, workDesc)
		if assignErr != nil {
			lastErr = assignErr
			if errors.Is(assignErr, dog.ErrDogWorking) {
				if inFlight, inFlightErr := d.inFlightDispatch(workDesc); inFlightErr != nil {
					return nil, inFlightErr
				} else if inFlight != nil {
					return inFlight, ErrPluginInFlight
				}
				if opts.TargetDog != "" {
					return result, fmt.Errorf("assigning work to dog %s: %w", candidate.Name, assignErr)
				}
				continue
			}
			return result, fmt.Errorf("assigning work to dog %s: %w", candidate.Name, assignErr)
		}

		if err := d.ensureDogAgentBead(candidate.Name); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("could not create agent bead: %v", err))
		}

		if err := ctx.Err(); err != nil {
			d.clearAssignment(candidate.Name, workDesc, assigned.WorkStartedAt, "context cancellation")
			return result, err
		}

		if _, startErr := d.Sessions.EnsureRunning(candidate.Name, dog.SessionStartOptions{WorkDesc: workDesc}); startErr != nil {
			d.clearAssignment(candidate.Name, workDesc, assigned.WorkStartedAt, "session start failure")
			lastErr = fmt.Errorf("starting dog session %s: %w", candidate.Name, startErr)
			if opts.TargetDog == "" {
				continue
			}
			return result, lastErr
		}
		result.SessionStarted = true

		if err := ctx.Err(); err != nil {
			d.clearAssignment(candidate.Name, workDesc, assigned.WorkStartedAt, "context cancellation")
			return result, err
		}

		msg := mail.NewMessage(from, fmt.Sprintf("deacon/dogs/%s", candidate.Name), fmt.Sprintf("Plugin: %s", p.Name), body)
		msg.Type = mail.TypeTask
		msg.Timestamp = time.Now()
		if sendErr := d.Router.Send(msg); sendErr != nil {
			d.clearAssignment(candidate.Name, workDesc, assigned.WorkStartedAt, "mail failure")
			return result, fmt.Errorf("sending plugin mail to dog %s: %w", candidate.Name, sendErr)
		}

		if current, getErr := d.Manager.Get(candidate.Name); getErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("could not verify work assignment: %v", getErr))
		} else if current.Work == workDesc && current.WorkStartedAt.Equal(assigned.WorkStartedAt) {
			result.WorkConfirmed = true
		} else {
			result.Warnings = append(result.Warnings, "work assignment changed after dispatch")
		}
		return result, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNoDogAvailable
}

func (d *DogPoolDispatcher) InFlightPlugin(ctx context.Context, p *Plugin) (*DogDispatchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.Manager == nil {
		return nil, fmt.Errorf("dog dispatcher is not configured")
	}
	return d.inFlightDispatch(fmt.Sprintf("plugin:%s", p.Name))
}

func (d *DogPoolDispatcher) inFlightDispatch(workDesc string) (*DogDispatchResult, error) {
	dogs, err := d.Manager.List()
	if err != nil {
		return nil, err
	}
	for _, dg := range dogs {
		if dg.State == dog.StateWorking && dg.Work == workDesc {
			return &DogDispatchResult{Dog: dg.Name, Work: workDesc}, nil
		}
	}
	return nil, nil
}

func (d *DogPoolDispatcher) createDog() (*dog.Dog, error) {
	if d.CreateDogName == nil {
		return nil, ErrNoDogAvailable
	}
	name := d.CreateDogName(d.Manager)
	created, err := d.Manager.Add(name)
	if err != nil {
		return nil, fmt.Errorf("creating dog %s: %w", name, err)
	}
	return created, nil
}

func (d *DogPoolDispatcher) candidateDogs(target string) ([]*dog.Dog, error) {
	if target != "" {
		dg, err := d.Manager.Get(target)
		if err != nil {
			return nil, err
		}
		if dg.State != dog.StateIdle {
			return nil, dog.ErrDogWorking
		}
		return []*dog.Dog{dg}, nil
	}
	dogs, err := d.Manager.List()
	if err != nil {
		return nil, err
	}
	sort.Slice(dogs, func(i, j int) bool { return dogs[i].Name < dogs[j].Name })
	idle := make([]*dog.Dog, 0, len(dogs))
	for _, dg := range dogs {
		if dg.State == dog.StateIdle {
			idle = append(idle, dg)
		}
	}
	return idle, nil
}

func (d *DogPoolDispatcher) ensureDogAgentBead(name string) error {
	if d.EnsureDogBead != nil {
		return d.EnsureDogBead(name)
	}
	b := beads.New(d.TownRoot)
	if existing, _ := b.FindDogAgentBead(name); existing != nil {
		return nil
	}
	_, err := b.CreateDogAgentBead(name, filepath.Join("deacon", "dogs", name))
	return err
}

func (d *DogPoolDispatcher) clearAssignment(name, work string, started time.Time, reason string) {
	cleared, err := d.Manager.ClearWorkIfMatches(name, work, started)
	if d.Logger == nil {
		return
	}
	if err != nil {
		d.Logger.Printf("plugin dispatch: failed to clear dog %s after %s: %v", name, reason, err)
	} else if !cleared {
		d.Logger.Printf("plugin dispatch: skipped clearing dog %s after %s: work assignment changed", name, reason)
	}
}

type boundedTail struct {
	mu    sync.Mutex
	limit int
	buf   []byte
	total int
}

func newBoundedTail(limit int) *boundedTail {
	return &boundedTail{limit: limit}
}

func (b *boundedTail) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total += len(p)
	if b.limit <= 0 {
		return len(p), nil
	}
	if len(p) >= b.limit {
		b.buf = append(b.buf[:0], p[len(p)-b.limit:]...)
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.limit {
		copy(b.buf, b.buf[len(b.buf)-b.limit:])
		b.buf = b.buf[:b.limit]
	}
	return len(p), nil
}

func (b *boundedTail) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func (b *boundedTail) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.total > len(b.buf)
}
