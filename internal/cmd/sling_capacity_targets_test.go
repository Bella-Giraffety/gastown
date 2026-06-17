package cmd

import "testing"

func TestIsCapacityNeutralTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{name: "dog pool path", target: "deacon/dogs", want: true},
		{name: "dog named path", target: "deacon/dogs/rex", want: true},
		{name: "dog pool shorthand", target: "dog:", want: true},
		{name: "dog named shorthand", target: "dog:rex", want: true},
		{name: "mayor", target: "mayor", want: true},
		{name: "mayor alias", target: "may", want: true},
		{name: "deacon", target: "deacon", want: true},
		{name: "deacon alias", target: "dea", want: true},
		{name: "witness", target: "gastown/witness", want: true},
		{name: "refinery", target: "gastown/refinery", want: true},
		{name: "named crew", target: "gastown/crew/alex", want: true},
		{name: "bare rig", target: "gastown", want: false},
		{name: "explicit polecat", target: "gastown/polecats/nux", want: false},
		{name: "ambiguous shorthand", target: "gastown/alex", want: false},
		{name: "self", target: ".", want: false},
		{name: "empty", target: "", want: false},
		{name: "malformed crew", target: "gastown/crew", want: false},
		{name: "malformed dog", target: "deacon/dogs/a/b", want: false},
		{name: "unknown rig singleton", target: "unknownrig/witness", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isCapacityNeutralTarget(tt.target); got != tt.want {
				t.Fatalf("isCapacityNeutralTarget(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}
