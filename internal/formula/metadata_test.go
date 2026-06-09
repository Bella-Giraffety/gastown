package formula

import "testing"

func TestIsWorkflowInteractiveDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		description string
		want        bool
	}{
		{"missing", "body only", false},
		{"true", "workflow_interactive: true\n\nbody", true},
		{"case insensitive key and value", "Workflow_Interactive: YES\n\nbody", true},
		{"numeric true", "workflow_interactive: 1\n\nbody", true},
		{"false", "workflow_interactive: false\n\nbody", false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsWorkflowInteractiveDescription(tt.description); got != tt.want {
				t.Fatalf("IsWorkflowInteractiveDescription() = %v, want %v", got, tt.want)
			}
		})
	}
}
