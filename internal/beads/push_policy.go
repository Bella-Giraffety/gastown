package beads

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PushPolicyMetadataKey is the durable issue-level authority for remote push behavior.
const PushPolicyMetadataKey = "push_policy"

type PushPolicy string

const (
	PushPolicyRemote    PushPolicy = "remote"
	PushPolicyLocalOnly PushPolicy = "local-only"
)

// NormalizePushPolicy canonicalizes persisted push-policy values.
func NormalizePushPolicy(raw string) (PushPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "remote", "default":
		return PushPolicyRemote, nil
	case "local", "local-only":
		return PushPolicyLocalOnly, nil
	default:
		return "", fmt.Errorf("invalid %s %q: must be remote or local-only", PushPolicyMetadataKey, raw)
	}
}

// IssuePushPolicy resolves the durable issue-level push policy.
// Metadata is authoritative; legacy structured merge_strategy: local is read only
// as a compatibility fallback so old local-only issues still fail closed.
func IssuePushPolicy(issue *Issue) (PushPolicy, error) {
	if issue == nil {
		return PushPolicyRemote, nil
	}
	if policy, ok, err := pushPolicyFromMetadata(issue.Metadata); ok || err != nil {
		return policy, err
	}
	if fields := ParseAttachmentFields(issue); fields != nil && strings.EqualFold(strings.TrimSpace(fields.MergeStrategy), "local") {
		return PushPolicyLocalOnly, nil
	}
	return PushPolicyRemote, nil
}

func IssueForbidsRemotePush(issue *Issue) (bool, error) {
	policy, err := IssuePushPolicy(issue)
	if err != nil {
		return true, err
	}
	return policy == PushPolicyLocalOnly, nil
}

func pushPolicyFromMetadata(metadata json.RawMessage) (PushPolicy, bool, error) {
	trimmed := strings.TrimSpace(string(metadata))
	if trimmed == "" || trimmed == "null" {
		return PushPolicyRemote, false, nil
	}

	meta := make(map[string]json.RawMessage)
	if err := json.Unmarshal(metadata, &meta); err != nil {
		return "", false, fmt.Errorf("invalid issue metadata: %w", err)
	}
	raw, ok := meta[PushPolicyMetadataKey]
	if !ok || strings.TrimSpace(string(raw)) == "null" {
		return PushPolicyRemote, false, nil
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", true, fmt.Errorf("invalid %s metadata: expected JSON string", PushPolicyMetadataKey)
	}
	policy, err := NormalizePushPolicy(value)
	return policy, true, err
}

// SetIssuePushPolicy stores the durable issue-level push policy without clobbering
// unrelated metadata keys.
func (b *Beads) SetIssuePushPolicy(id string, policy PushPolicy) error {
	normalized, err := NormalizePushPolicy(string(policy))
	if err != nil {
		return err
	}

	target, err := b.forIssueID(id)
	if err != nil {
		return err
	}
	if target != b {
		return target.SetIssuePushPolicy(id, normalized)
	}
	if b.store != nil {
		return b.storeSetIssuePushPolicy(id, normalized)
	}

	if normalized == PushPolicyRemote {
		_, err = b.run("update", id, "--unset-metadata="+PushPolicyMetadataKey)
		return err
	}
	value, err := json.Marshal(string(normalized))
	if err != nil {
		return err
	}
	_, err = b.run("update", id, "--set-metadata="+PushPolicyMetadataKey+"="+string(value))
	return err
}

func (b *Beads) storeSetIssuePushPolicy(id string, policy PushPolicy) error {
	ctx, cancel := storeCtx()
	defer cancel()

	si, err := b.store.GetIssue(ctx, id)
	if err != nil {
		return fmt.Errorf("fetching issue for push policy: %w", err)
	}

	var metadata json.RawMessage
	if policy == PushPolicyRemote {
		metadata, err = deleteMetadataKey(si.Metadata, PushPolicyMetadataKey)
	} else {
		metadata, err = mergeMetadataKey(si.Metadata, PushPolicyMetadataKey, string(policy))
	}
	if err != nil {
		return fmt.Errorf("building push policy metadata: %w", err)
	}

	return b.store.UpdateIssue(ctx, id, map[string]interface{}{"metadata": metadata}, b.getActor())
}
