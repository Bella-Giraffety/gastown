package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/workspace"
)

type fanoutOptions struct {
	Rig       string
	Parent    string
	Rate      time.Duration
	StateFile string
	DryRun    bool
	Labels    []string
	Type      string
	Priority  int
}

type fanoutDeps struct {
	stdin            io.Reader
	stdout           io.Writer
	now              func() time.Time
	sleep            func(time.Duration)
	findTownRoot     func() (string, error)
	rigDirForName    func(string, string) string
	rigNameForPrefix func(string, string) string
	create           func(beads.CreateOptions) (*beads.Issue, error)
	show             func(string) (*beads.Issue, error)
}

type fanoutStateRecord struct {
	Status    string   `json:"status"`
	Title     string   `json:"title"`
	ID        string   `json:"id"`
	Rig       string   `json:"rig"`
	Parent    string   `json:"parent,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	Priority  int      `json:"priority"`
	CreatedAt string   `json:"created_at"`
}

func init() {
	beadCmd.AddCommand(newFanoutCmd())
}

func newFanoutCmd() *cobra.Command {
	opts := fanoutOptions{
		Type:     "task",
		Priority: 2,
		Rate:     500 * time.Millisecond,
	}
	cmd := &cobra.Command{
		Use:   "fanout --rig <rig>",
		Short: "Create many rig-pinned beads from stdin",
		Long: `Create many beads from newline-delimited titles on stdin.

Writes are pinned to the target rig, run serially with a configurable delay,
and recorded to a JSONL state file so interrupted runs can be resumed safely.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFanout(opts, fanoutDeps{
				stdin:  cmd.InOrStdin(),
				stdout: cmd.OutOrStdout(),
			})
		},
	}
	cmd.Flags().StringVar(&opts.Rig, "rig", "", "target rig name (required)")
	cmd.Flags().StringVar(&opts.Parent, "parent", "", "parent bead ID in the target rig")
	cmd.Flags().DurationVar(&opts.Rate, "rate", opts.Rate, "delay between creates")
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "JSONL state file for resumable runs")
	cmd.Flags().BoolVarP(&opts.DryRun, "dry-run", "n", false, "show what would be created without writing")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "label to apply; repeatable and comma-aware")
	cmd.Flags().StringVar(&opts.Type, "type", opts.Type, "bead type label to apply")
	cmd.Flags().IntVar(&opts.Priority, "priority", opts.Priority, "bead priority 0-4")
	_ = cmd.MarkFlagRequired("rig")
	return cmd
}

func runFanout(opts fanoutOptions, deps fanoutDeps) error {
	deps = deps.withDefaults()

	if err := validateFanoutOptions(opts); err != nil {
		return err
	}
	labels, err := fanoutLabels(opts.Type, opts.Labels)
	if err != nil {
		return err
	}

	townRoot, err := deps.findTownRoot()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}
	if err := validateFanoutRig(townRoot, opts.Rig, deps.rigDirForName); err != nil {
		return err
	}
	if err := validateFanoutParent(townRoot, opts, deps); err != nil {
		return err
	}

	if opts.StateFile == "" {
		opts.StateFile = defaultFanoutStateFile(townRoot, opts, labels)
	}

	var stateLock *flock.Flock
	if !opts.DryRun {
		stateLock, err = lockFanoutStateFile(opts.StateFile)
		if err != nil {
			return err
		}
		defer stateLock.Unlock()
	}

	done, err := loadFanoutState(opts.StateFile, opts, labels)
	if err != nil {
		return err
	}

	titles, err := readFanoutTitles(deps.stdin)
	if err != nil {
		return err
	}
	if len(titles) == 0 {
		return fmt.Errorf("no bead titles on stdin")
	}

	var stateFile *os.File
	if !opts.DryRun {
		stateFile, err = openFanoutStateFile(opts.StateFile)
		if err != nil {
			return err
		}
		defer stateFile.Close()
	}

	fmt.Fprintf(deps.stdout, "State file: %s\n", opts.StateFile)

	created := 0
	skipped := 0
	for i, title := range titles {
		key := fanoutStateKey(title, opts, labels)
		if rec, ok := done[key]; ok {
			fmt.Fprintf(deps.stdout, "Skipping %q (already created as %s)\n", title, rec.ID)
			skipped++
			continue
		}

		createOpts := beads.CreateOptions{
			Title:    title,
			Labels:   labels,
			Priority: opts.Priority,
			Parent:   opts.Parent,
			Rig:      opts.Rig,
		}
		if opts.DryRun {
			fmt.Fprintf(deps.stdout, "Would create %q in rig %s\n", title, opts.Rig)
			continue
		}

		pending := fanoutStateRecord{
			Status:    "pending",
			Title:     title,
			Rig:       opts.Rig,
			Parent:    opts.Parent,
			Labels:    labels,
			Priority:  opts.Priority,
			CreatedAt: deps.now().UTC().Format(time.RFC3339Nano),
		}
		if err := appendFanoutState(stateFile, pending); err != nil {
			return fmt.Errorf("recording pending state for %q in %s: %w", title, opts.StateFile, err)
		}

		issue, err := deps.create(createOpts)
		if err != nil {
			return fmt.Errorf("creating %q: %w", title, err)
		}
		if issue == nil || issue.ID == "" {
			return fmt.Errorf("creating %q: empty issue response", title)
		}

		rec := fanoutStateRecord{
			Status:    "created",
			Title:     title,
			ID:        issue.ID,
			Rig:       opts.Rig,
			Parent:    opts.Parent,
			Labels:    labels,
			Priority:  opts.Priority,
			CreatedAt: deps.now().UTC().Format(time.RFC3339Nano),
		}
		if err := appendFanoutState(stateFile, rec); err != nil {
			return fmt.Errorf("created %s for %q but failed to record state in %s: %w", issue.ID, title, opts.StateFile, err)
		}
		done[key] = rec
		created++
		fmt.Fprintf(deps.stdout, "Created %s %q\n", issue.ID, title)

		if opts.Rate > 0 && hasPendingFanoutCreate(titles[i+1:], opts, labels, done) {
			deps.sleep(opts.Rate)
		}
	}

	if opts.DryRun {
		fmt.Fprintf(deps.stdout, "Dry run complete: %d title(s), %d already in state\n", len(titles), skipped)
		return nil
	}
	fmt.Fprintf(deps.stdout, "Fanout complete: %d created, %d skipped\n", created, skipped)
	return nil
}

func (deps fanoutDeps) withDefaults() fanoutDeps {
	if deps.stdin == nil {
		deps.stdin = os.Stdin
	}
	if deps.stdout == nil {
		deps.stdout = os.Stdout
	}
	if deps.now == nil {
		deps.now = time.Now
	}
	if deps.sleep == nil {
		deps.sleep = time.Sleep
	}
	if deps.findTownRoot == nil {
		deps.findTownRoot = workspace.FindFromCwdOrError
	}
	if deps.rigDirForName == nil {
		deps.rigDirForName = beads.GetRigDirForName
	}
	if deps.rigNameForPrefix == nil {
		deps.rigNameForPrefix = beads.GetRigNameForPrefix
	}
	if deps.create == nil || deps.show == nil {
		var client *beads.Beads
		deps.findTownRoot = func(find func() (string, error)) func() (string, error) {
			return func() (string, error) {
				townRoot, err := find()
				if err != nil {
					return "", err
				}
				client = beads.New(townRoot)
				return townRoot, nil
			}
		}(deps.findTownRoot)
		if deps.create == nil {
			deps.create = func(opts beads.CreateOptions) (*beads.Issue, error) {
				if client == nil {
					return nil, fmt.Errorf("beads client not initialized")
				}
				return client.Create(opts)
			}
		}
		if deps.show == nil {
			deps.show = func(id string) (*beads.Issue, error) {
				if client == nil {
					return nil, fmt.Errorf("beads client not initialized")
				}
				return client.Show(id)
			}
		}
	}
	return deps
}

func readFanoutTitles(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	seen := make(map[string]struct{})
	var titles []string
	line := 0
	for scanner.Scan() {
		line++
		title := strings.TrimSpace(scanner.Text())
		if title == "" {
			continue
		}
		if beads.IsFlagLikeTitle(title) {
			return nil, fmt.Errorf("line %d: refusing flag-like title %q", line, title)
		}
		if _, ok := seen[title]; ok {
			return nil, fmt.Errorf("duplicate title %q on stdin", title)
		}
		seen[title] = struct{}{}
		titles = append(titles, title)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	return titles, nil
}

func validateFanoutOptions(opts fanoutOptions) error {
	if opts.Rig == "" {
		return fmt.Errorf("--rig is required")
	}
	if strings.ContainsAny(opts.Rig, `/\\`) || opts.Rig == "." || opts.Rig == ".." || strings.Contains(opts.Rig, "..") {
		return fmt.Errorf("invalid rig %q", opts.Rig)
	}
	if opts.Rate < 0 {
		return fmt.Errorf("--rate must be non-negative")
	}
	if opts.Priority < 0 || opts.Priority > 4 {
		return fmt.Errorf("--priority must be between 0 and 4")
	}
	return nil
}

func fanoutLabels(issueType string, labels []string) ([]string, error) {
	validTypes := map[string]bool{
		"":         true,
		"task":     true,
		"bug":      true,
		"feature":  true,
		"epic":     true,
		"question": true,
		"docs":     true,
		"chore":    true,
	}
	if !validTypes[issueType] {
		return nil, fmt.Errorf("unsupported --type %q", issueType)
	}
	typeLabel := ""
	if issueType != "" {
		typeLabel = "gt:" + issueType
	}

	seen := make(map[string]struct{})
	add := func(label string) {
		label = strings.TrimSpace(label)
		if label == "" {
			return
		}
		if _, ok := seen[label]; ok {
			return
		}
		seen[label] = struct{}{}
	}
	if typeLabel != "" {
		add(typeLabel)
	}
	for _, raw := range labels {
		for _, label := range strings.Split(raw, ",") {
			label = strings.TrimSpace(label)
			if strings.HasPrefix(label, "gt:") && validTypes[strings.TrimPrefix(label, "gt:")] && typeLabel != "" && label != typeLabel {
				return nil, fmt.Errorf("label %q conflicts with --type %s", label, issueType)
			}
			add(label)
		}
	}

	result := make([]string, 0, len(seen))
	for label := range seen {
		result = append(result, label)
	}
	sort.Strings(result)
	return result, nil
}

func validateFanoutRig(townRoot, rig string, rigDirForName func(string, string) string) error {
	if rigDir := rigDirForName(townRoot, rig); rigDir == "" {
		return fmt.Errorf("unknown rig %q", rig)
	}
	return nil
}

func validateFanoutParent(townRoot string, opts fanoutOptions, deps fanoutDeps) error {
	if opts.Parent == "" {
		return nil
	}
	prefix := beads.ExtractPrefix(opts.Parent)
	if prefix == "" {
		return fmt.Errorf("parent %q has no routable prefix", opts.Parent)
	}
	parentRig := deps.rigNameForPrefix(townRoot, prefix)
	if parentRig != opts.Rig {
		return fmt.Errorf("parent %s belongs to rig %q, not target rig %q", opts.Parent, parentRig, opts.Rig)
	}
	if opts.DryRun {
		return nil
	}
	issue, err := deps.show(opts.Parent)
	if err != nil {
		return fmt.Errorf("showing parent %s: %w", opts.Parent, err)
	}
	if issue == nil || issue.ID == "" {
		return fmt.Errorf("parent %s not found", opts.Parent)
	}
	return nil
}

func defaultFanoutStateFile(townRoot string, opts fanoutOptions, labels []string) string {
	stateKey := fanoutStateKey("", opts, labels)
	digest := sha256.Sum256([]byte(stateKey))
	return filepath.Join(townRoot, ".runtime", "fanout", fmt.Sprintf("gt-fanout-%s-%x.jsonl", opts.Rig, digest[:8]))
}

func loadFanoutState(path string, opts fanoutOptions, labels []string) (map[string]fanoutStateRecord, error) {
	done := make(map[string]fanoutStateRecord)
	pending := make(map[string]fanoutStateRecord)
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return done, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening state file %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var rec fanoutStateRecord
		if err := json.Unmarshal([]byte(text), &rec); err != nil {
			return nil, fmt.Errorf("parsing state file %s line %d: %w", path, line, err)
		}
		if rec.Status == "" {
			rec.Status = "created"
		}
		if rec.Title == "" || rec.Rig == "" || (rec.Status == "created" && rec.ID == "") {
			return nil, fmt.Errorf("parsing state file %s line %d: missing title, id, or rig", path, line)
		}
		key := fanoutStateKey(rec.Title, fanoutOptions{
			Rig:      rec.Rig,
			Parent:   rec.Parent,
			Priority: rec.Priority,
		}, rec.Labels)
		if key == fanoutStateKey(rec.Title, opts, labels) {
			switch rec.Status {
			case "pending":
				pending[key] = rec
			case "created":
				done[key] = rec
				delete(pending, key)
			default:
				return nil, fmt.Errorf("parsing state file %s line %d: unknown status %q", path, line, rec.Status)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading state file %s: %w", path, err)
	}
	for _, rec := range pending {
		return nil, fmt.Errorf("state file %s has pending record for %q; previous run may have stopped before recording a created bead, so inspect the target rig and remove or complete the pending state before retrying", path, rec.Title)
	}
	return done, nil
}

func lockFanoutStateFile(path string) (*flock.Flock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("creating state directory for %s: %w", path, err)
	}
	lock := flock.New(path + ".lock")
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("locking state file %s: %w", path, err)
	}
	if !locked {
		return nil, fmt.Errorf("state file %s is locked by another fanout run", path)
	}
	return lock, nil
}

func openFanoutStateFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("creating state directory for %s: %w", path, err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening state file %s for append: %w", path, err)
	}
	return file, nil
}

func appendFanoutState(file *os.File, rec fanoutStateRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}

func fanoutStateKey(title string, opts fanoutOptions, labels []string) string {
	canonicalLabels := append([]string(nil), labels...)
	sort.Strings(canonicalLabels)
	return strings.Join([]string{
		opts.Rig,
		opts.Parent,
		fmt.Sprintf("%d", opts.Priority),
		strings.Join(canonicalLabels, ","),
		title,
	}, "\x00")
}

func hasPendingFanoutCreate(titles []string, opts fanoutOptions, labels []string, done map[string]fanoutStateRecord) bool {
	for _, title := range titles {
		if _, ok := done[fanoutStateKey(title, opts, labels)]; !ok {
			return true
		}
	}
	return false
}
