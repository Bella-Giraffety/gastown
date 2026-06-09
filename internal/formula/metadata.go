package formula

import "strings"

// WorkflowInteractiveField marks a materialized workflow step that must stay
// with the originating human session instead of being auto-dispatched.
const WorkflowInteractiveField = "workflow_interactive"

// IsWorkflowInteractiveDescription reports whether a materialized workflow
// step description carries the durable interactive marker.
func IsWorkflowInteractiveDescription(description string) bool {
	for _, line := range strings.Split(description, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(key), WorkflowInteractiveField) {
			continue
		}
		value = strings.TrimSpace(value)
		return strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "yes")
	}
	return false
}
