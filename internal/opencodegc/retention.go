package opencodegc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/lock"
	"github.com/steveyegge/gastown/internal/util"
)

const (
	DefaultRetentionAge     = 7 * 24 * time.Hour
	DefaultKeepPerDirectory = 20
	DefaultMaxDeletes       = 500
	DefaultCommandTimeout   = 30 * time.Second

	DefaultMinDeleteHeadroomBytes = 512 * 1024 * 1024
	DefaultVacuumReserveBytes     = 2 * 1024 * 1024 * 1024
	DefaultWALCheckpointBytes     = 256 * 1024 * 1024
)

var (
	ErrOpenCodeUnavailable = errors.New("opencode CLI unavailable")
	ErrLockBusy            = errors.New("opencode retention already running")
)

const sessionScanSQL = "select id, coalesce(directory, '') as directory, time_updated from session order by time_updated desc;"

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type CommandRunnerFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

func (f CommandRunnerFunc) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f(ctx, name, args...)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
		}
		return out, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}

type DiskInfoFunc func(path string) (*util.DiskSpaceInfo, error)

type LockFunc func(path string) (unlock func(), locked bool, err error)

type Options struct {
	TownRoot       string
	Now            time.Time
	RetentionAge   time.Duration
	KeepPerDir     int
	MaxDeletes     int
	CommandTimeout time.Duration

	MinDeleteHeadroomBytes uint64
	VacuumReserveBytes     uint64
	WALCheckpointBytes     uint64

	Runner   CommandRunner
	DiskInfo DiskInfoFunc
	TryLock  LockFunc
	LockPath string
}

type Session struct {
	ID          string `json:"id"`
	Directory   string `json:"directory"`
	TimeUpdated int64  `json:"time_updated"`
}

type Report struct {
	DBPath       string
	DBBytes      uint64
	WALBytes     uint64
	SHMBytes     uint64
	SessionCount int

	TooRecent       int
	Protected       int
	KeptPerDir      int
	Eligible        int
	Selected        []Session
	Deleted         int
	Checkpointed    bool
	Vacuumed        bool
	VacuumSkipped   string
	DeleteHeadroom  string
	RemainingReason string
}

func (r *Report) NeedsCleanup() bool {
	if r == nil {
		return false
	}
	return r.Eligible > 0 || r.WALBytes >= DefaultWALCheckpointBytes
}

func (r *Report) Details() []string {
	if r == nil {
		return nil
	}
	details := []string{
		fmt.Sprintf("OpenCode DB: %s", r.DBPath),
		fmt.Sprintf("Sizes: db=%s wal=%s shm=%s", util.FormatBytesHuman(r.DBBytes), util.FormatBytesHuman(r.WALBytes), util.FormatBytesHuman(r.SHMBytes)),
		fmt.Sprintf("Sessions: total=%d too-recent=%d protected=%d kept-per-directory=%d eligible=%d selected=%d deleted=%d",
			r.SessionCount, r.TooRecent, r.Protected, r.KeptPerDir, r.Eligible, len(r.Selected), r.Deleted),
	}
	if r.DeleteHeadroom != "" {
		details = append(details, "Delete headroom: "+r.DeleteHeadroom)
	}
	if r.Checkpointed {
		details = append(details, "WAL checkpoint: complete")
	}
	if r.Vacuumed {
		details = append(details, "VACUUM: complete")
	} else if r.VacuumSkipped != "" {
		details = append(details, "VACUUM skipped: "+r.VacuumSkipped)
	}
	if r.RemainingReason != "" {
		details = append(details, r.RemainingReason)
	}
	return details
}

func Analyze(ctx context.Context, opts Options) (*Report, error) {
	opts = opts.withDefaults()
	return analyze(ctx, opts)
}

func Fix(ctx context.Context, opts Options) (*Report, error) {
	opts = opts.withDefaults()
	if err := os.MkdirAll(filepath.Dir(opts.LockPath), 0755); err != nil {
		return nil, fmt.Errorf("creating opencode retention lock dir: %w", err)
	}
	unlock, locked, err := opts.TryLock(opts.LockPath)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, ErrLockBusy
	}
	defer unlock()

	report, err := analyze(ctx, opts)
	if err != nil {
		return report, err
	}
	if len(report.Selected) == 0 && report.WALBytes < opts.WALCheckpointBytes {
		return report, nil
	}

	info, err := opts.DiskInfo(filepath.Dir(report.DBPath))
	if err != nil {
		return report, fmt.Errorf("checking opencode db headroom: %w", err)
	}
	report.DeleteHeadroom = fmt.Sprintf("%s available", info.AvailableHuman())
	if info.AvailableBytes < opts.MinDeleteHeadroomBytes {
		return report, fmt.Errorf("insufficient headroom for OpenCode retention: %s available, need at least %s",
			info.AvailableHuman(), util.FormatBytesHuman(opts.MinDeleteHeadroomBytes))
	}

	if err := runCheckpoint(ctx, opts, "PASSIVE"); err != nil {
		return report, err
	}

	for _, candidate := range report.Selected {
		if _, err := runOpenCode(ctx, opts, "session", "delete", candidate.ID); err != nil {
			return report, fmt.Errorf("deleting opencode session %s: %w", candidate.ID, err)
		}
		report.Deleted++
	}

	if report.Deleted > 0 || report.WALBytes >= opts.WALCheckpointBytes {
		if err := runCheckpoint(ctx, opts, "TRUNCATE"); err != nil {
			return report, err
		}
		report.Checkpointed = true
	}

	if report.Deleted > 0 {
		if err := refreshSizes(report); err != nil {
			return report, err
		}
		if shouldVacuum(report, opts) {
			if _, err := runOpenCode(ctx, opts, "db", "VACUUM;"); err != nil {
				return report, fmt.Errorf("vacuuming opencode db: %w", err)
			}
			report.Vacuumed = true
			_ = refreshSizes(report)
		} else {
			report.VacuumSkipped = fmt.Sprintf("requires at least db size plus reserve (%s + %s) free",
				util.FormatBytesHuman(report.DBBytes), util.FormatBytesHuman(opts.VacuumReserveBytes))
		}
	}

	return report, nil
}

func (opts Options) withDefaults() Options {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.RetentionAge == 0 {
		opts.RetentionAge = DefaultRetentionAge
	}
	if opts.KeepPerDir == 0 {
		opts.KeepPerDir = DefaultKeepPerDirectory
	}
	if opts.MaxDeletes == 0 {
		opts.MaxDeletes = DefaultMaxDeletes
	}
	if opts.CommandTimeout == 0 {
		opts.CommandTimeout = DefaultCommandTimeout
	}
	if opts.MinDeleteHeadroomBytes == 0 {
		opts.MinDeleteHeadroomBytes = DefaultMinDeleteHeadroomBytes
	}
	if opts.VacuumReserveBytes == 0 {
		opts.VacuumReserveBytes = DefaultVacuumReserveBytes
	}
	if opts.WALCheckpointBytes == 0 {
		opts.WALCheckpointBytes = DefaultWALCheckpointBytes
	}
	if opts.Runner == nil {
		opts.Runner = ExecRunner{}
	}
	if opts.DiskInfo == nil {
		opts.DiskInfo = util.GetDiskSpace
	}
	if opts.TryLock == nil {
		opts.TryLock = lock.FlockTryAcquire
	}
	if opts.LockPath == "" && opts.TownRoot != "" {
		opts.LockPath = filepath.Join(opts.TownRoot, ".runtime", "opencode-retention.lock")
	}
	return opts
}

func analyze(ctx context.Context, opts Options) (*Report, error) {
	dbPath, err := opencodeDBPath(ctx, opts)
	if err != nil {
		return nil, err
	}
	report := &Report{DBPath: dbPath}
	if err := refreshSizes(report); err != nil {
		if os.IsNotExist(err) {
			return report, nil
		}
		return report, err
	}

	sessions, err := loadSessions(ctx, opts)
	if err != nil {
		return report, err
	}
	protected, err := collectProtectedSessionIDs(opts.TownRoot)
	if err != nil {
		return report, err
	}
	report.SessionCount = len(sessions)
	selectCandidates(report, sessions, protected, opts)
	return report, nil
}

func opencodeDBPath(ctx context.Context, opts Options) (string, error) {
	out, err := runOpenCode(ctx, opts, "db", "path")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrOpenCodeUnavailable
		}
		return "", err
	}
	dbPath := strings.TrimSpace(string(out))
	if dbPath == "" {
		return "", fmt.Errorf("opencode db path returned empty output")
	}
	return dbPath, nil
}

func loadSessions(ctx context.Context, opts Options) ([]Session, error) {
	out, err := runOpenCode(ctx, opts, "db", sessionScanSQL, "--format", "json")
	if err != nil {
		return nil, err
	}
	var sessions []Session
	if err := json.Unmarshal(out, &sessions); err != nil {
		return nil, fmt.Errorf("parsing opencode session scan: %w", err)
	}
	for i := range sessions {
		if sessions[i].ID == "" {
			return nil, fmt.Errorf("opencode session scan returned a row with empty id")
		}
		if sessions[i].TimeUpdated <= 0 {
			return nil, fmt.Errorf("opencode session %s has invalid time_updated %d", sessions[i].ID, sessions[i].TimeUpdated)
		}
		if sessions[i].TimeUpdated < 1_000_000_000_000 {
			sessions[i].TimeUpdated *= 1000
		}
	}
	return sessions, nil
}

func runOpenCode(ctx context.Context, opts Options, args ...string) ([]byte, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, opts.CommandTimeout)
	defer cancel()
	allArgs := append([]string{"--pure"}, args...)
	out, err := opts.Runner.Run(cmdCtx, "opencode", allArgs...)
	if errors.Is(err, context.DeadlineExceeded) {
		return out, fmt.Errorf("opencode %s timed out after %s", strings.Join(args, " "), opts.CommandTimeout)
	}
	return out, err
}

func runCheckpoint(ctx context.Context, opts Options, mode string) error {
	out, err := runOpenCode(ctx, opts, "db", fmt.Sprintf("PRAGMA wal_checkpoint(%s);", mode), "--format", "json")
	if err != nil {
		return fmt.Errorf("checkpointing opencode WAL: %w", err)
	}
	var rows []struct {
		Busy int `json:"busy"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return fmt.Errorf("parsing opencode checkpoint result: %w", err)
	}
	for _, row := range rows {
		if row.Busy != 0 {
			return fmt.Errorf("opencode WAL checkpoint reported busy=%d", row.Busy)
		}
	}
	return nil
}

func refreshSizes(report *Report) error {
	db, err := fileSize(report.DBPath)
	if err != nil {
		return err
	}
	wal, err := fileSize(report.DBPath + "-wal")
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	shm, err := fileSize(report.DBPath + "-shm")
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	report.DBBytes = db
	report.WALBytes = wal
	report.SHMBytes = shm
	return nil
}

func fileSize(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if info.Size() < 0 {
		return 0, nil
	}
	return uint64(info.Size()), nil
}

func shouldVacuum(report *Report, opts Options) bool {
	info, err := opts.DiskInfo(filepath.Dir(report.DBPath))
	if err != nil {
		return false
	}
	needed := report.DBBytes + report.WALBytes + opts.VacuumReserveBytes
	return info.AvailableBytes > needed
}

func selectCandidates(report *Report, sessions []Session, protected map[string]struct{}, opts Options) {
	sorted := append([]Session(nil), sessions...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].TimeUpdated == sorted[j].TimeUpdated {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].TimeUpdated > sorted[j].TimeUpdated
	})

	keep := make(map[string]struct{})
	counts := make(map[string]int)
	for _, session := range sorted {
		dir := filepath.Clean(session.Directory)
		if session.Directory == "" || dir == "." {
			dir = "<unknown>"
		}
		if counts[dir] < opts.KeepPerDir {
			keep[session.ID] = struct{}{}
			counts[dir]++
			continue
		}
		counts[dir]++
	}

	cutoff := opts.Now.Add(-opts.RetentionAge).UnixMilli()
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].TimeUpdated == sorted[j].TimeUpdated {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].TimeUpdated < sorted[j].TimeUpdated
	})

	for _, session := range sorted {
		if session.TimeUpdated >= cutoff {
			report.TooRecent++
			continue
		}
		if _, ok := protected[session.ID]; ok {
			report.Protected++
			continue
		}
		if _, ok := keep[session.ID]; ok {
			report.KeptPerDir++
			continue
		}
		report.Eligible++
		if len(report.Selected) < opts.MaxDeletes {
			report.Selected = append(report.Selected, session)
		}
	}
	if report.Eligible > len(report.Selected) {
		report.RemainingReason = fmt.Sprintf("%d eligible session(s) remain after this bounded %d-delete batch",
			report.Eligible-len(report.Selected), opts.MaxDeletes)
	}
}

func collectProtectedSessionIDs(townRoot string) (map[string]struct{}, error) {
	protected := make(map[string]struct{})
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id != "" {
			protected[id] = struct{}{}
		}
	}
	add(os.Getenv("GT_SESSION_ID"))
	add(os.Getenv("CLAUDE_SESSION_ID"))
	if envName := os.Getenv("GT_SESSION_ID_ENV"); envName != "" {
		add(os.Getenv(envName))
	}
	if townRoot == "" {
		return protected, nil
	}

	skipDirs := map[string]bool{
		".git": true, ".repo.git": true, ".beads": true, ".beads-wisp": true,
		".dolt": true, ".dolt-data": true, ".claude": true, ".opencode": true,
		".logs": true, "node_modules": true, "vendor": true, "__pycache__": true,
	}
	err := filepath.WalkDir(townRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == ".runtime" {
			data, err := os.ReadFile(filepath.Join(path, "session_id"))
			if err == nil {
				line, _, _ := strings.Cut(string(data), "\n")
				add(line)
			} else if !os.IsNotExist(err) {
				return err
			}
			return filepath.SkipDir
		}
		if skipDirs[name] {
			return filepath.SkipDir
		}
		return nil
	})
	return protected, err
}
