package cmd

import "testing"

func TestDoneContaminationTargetBaseRef(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "defaults to main target",
			target: "main",
			want:   "origin/main",
		},
		{
			name:   "uses explicit target branch",
			target: "upstream-rebuild-main",
			want:   "origin/upstream-rebuild-main",
		},
		{
			name:   "avoids double origin prefix",
			target: "origin/upstream-rebuild-main",
			want:   "origin/upstream-rebuild-main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := doneTargetBaseRef(tt.target)
			if got != tt.want {
				t.Fatalf("doneTargetBaseRef(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}
