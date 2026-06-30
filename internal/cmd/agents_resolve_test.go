package cmd

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestAgentBeadMatchesDescriptionAndIDFallback(t *testing.T) {
	tests := []struct {
		name  string
		issue *beads.Issue
		role  string
		rig   string
		want  bool
	}{
		{
			name: "description matches legacy random wisp ID",
			issue: &beads.Issue{
				ID:          "au-wisp-0ti",
				Description: "Agent\n\nrole_type: refinery\nrig: alleago_ui",
			},
			role: "refinery",
			rig:  "alleago_ui",
			want: true,
		},
		{
			name: "canonical ID fallback matches sparse wisp metadata",
			issue: &beads.Issue{
				ID: "gt-gastown-witness",
			},
			role: "witness",
			rig:  "gastown",
			want: true,
		},
		{
			name: "collapsed prefix-rig ID fallback matches sparse metadata",
			issue: &beads.Issue{
				ID: "cp-refinery",
			},
			role: "refinery",
			rig:  "cp",
			want: true,
		},
		{
			name: "role mismatch",
			issue: &beads.Issue{
				ID:          "gt-gastown-witness",
				Description: "Agent\n\nrole_type: witness\nrig: gastown",
			},
			role: "refinery",
			rig:  "gastown",
			want: false,
		},
		{
			name: "rig mismatch",
			issue: &beads.Issue{
				ID:          "gt-gastown-refinery",
				Description: "Agent\n\nrole_type: refinery\nrig: gastown",
			},
			role: "refinery",
			rig:  "other",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agentBeadMatches(tt.issue, tt.role, tt.rig)
			if got != tt.want {
				t.Fatalf("agentBeadMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPickBestAgentBead(t *testing.T) {
	candidates := []agentBeadCandidate{
		candidate("town-issue", agentSourceTownIssues, "open"),
		candidate("rig-issue", agentSourceRigIssues, "open"),
		candidate("town-wisp", agentSourceTownWisps, "open"),
		candidate("rig-wisp", agentSourceRigWisps, "open"),
	}

	got, err := pickBestAgentBead(candidates)
	if err != nil {
		t.Fatalf("pickBestAgentBead returned error: %v", err)
	}
	if got == nil || got.ID != "rig-wisp" {
		t.Fatalf("pickBestAgentBead picked %v, want rig-wisp", got)
	}
}

func TestPickBestAgentBeadSkipsClosed(t *testing.T) {
	candidates := []agentBeadCandidate{
		candidate("closed-rig-wisp", agentSourceRigWisps, "closed"),
		candidate("open-rig-issue", agentSourceRigIssues, "open"),
	}

	got, err := pickBestAgentBead(candidates)
	if err != nil {
		t.Fatalf("pickBestAgentBead returned error: %v", err)
	}
	if got == nil || got.ID != "open-rig-issue" {
		t.Fatalf("pickBestAgentBead picked %v, want open-rig-issue", got)
	}
}

func TestPickBestAgentBeadRejectsSameRankDuplicates(t *testing.T) {
	candidates := []agentBeadCandidate{
		candidate("rig-wisp-a", agentSourceRigWisps, "open"),
		candidate("rig-wisp-b", agentSourceRigWisps, "open"),
		candidate("rig-issue", agentSourceRigIssues, "open"),
	}

	got, err := pickBestAgentBead(candidates)
	if err == nil {
		t.Fatalf("pickBestAgentBead picked %v, want duplicate error", got)
	}
	if !strings.Contains(err.Error(), "multiple matching agent beads") {
		t.Fatalf("error = %q, want duplicate diagnostic", err)
	}
}

func TestPickBestAgentBeadCollapsesSameIDIssueWispDuplicate(t *testing.T) {
	candidates := []agentBeadCandidate{
		candidate("hq-deacon", agentSourceTownWisps, "open"),
		candidate("hq-deacon", agentSourceTownIssues, "open"),
	}

	got, err := pickBestAgentBead(candidates)
	if err != nil {
		t.Fatalf("pickBestAgentBead returned error: %v", err)
	}
	if got == nil || got.ID != "hq-deacon" || got.Source != agentSourceTownIssues {
		t.Fatalf("pickBestAgentBead picked %#v, want durable hq-deacon issue", got)
	}
}

func TestLoadAgentBeadsUsesSourceSpecificQueriesForDuplicateID(t *testing.T) {
	installTestBD(t, `#!/bin/sh
cmd=""
for arg in "$@"; do
  case "$arg" in --*) ;; *) cmd="$arg"; break ;; esac
done
case "$cmd" in
  version)
    echo "bd test"
    ;;
  list)
    echo 'list should not be used for agent identity lookup' >&2
    exit 9
    ;;
  query)
    args="$*"
    case "$args" in
      *"ephemeral=false"*) echo '[{"id":"hq-deacon","title":"Deacon","status":"open","description":"role_type: deacon\nagent_state: idle","labels":["gt:agent"]}]' ;;
      *"ephemeral=true"*) echo '[{"id":"hq-deacon","title":"Deacon heartbeat","status":"open","description":"agent_state: idle","labels":["gt:agent"],"ephemeral":true}]' ;;
      *) echo '[]' ;;
    esac
    ;;
  *)
    echo '[]'
    ;;
esac
`)

	candidates, err := loadAgentBeadsFromDir(t.TempDir(), agentSourceTownIssues, agentSourceTownWisps)
	if err != nil {
		t.Fatalf("loadAgentBeadsFromDir returned error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1: %#v", len(candidates), candidates)
	}
	if candidates[0].ID != "hq-deacon" || candidates[0].Source != agentSourceTownIssues {
		t.Fatalf("candidate = %#v, want durable hq-deacon issue", candidates[0])
	}
	if !agentBeadMatches(candidates[0].Issue, "deacon", "") {
		t.Fatalf("deduped hq-deacon issue no longer matches deacon role: %#v", candidates[0].Issue)
	}
}

func candidate(id string, source agentBeadSource, status string) agentBeadCandidate {
	return agentBeadCandidate{
		ID:     id,
		Source: source,
		Status: status,
		Issue:  &beads.Issue{ID: id, Status: status},
	}
}
