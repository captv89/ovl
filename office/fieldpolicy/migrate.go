// SPDX-License-Identifier: AGPL-3.0-only

package fieldpolicy

import "github.com/captv89/ovl/pkg/schema"

// MigrationResult is design handoff B6's migration assistant: opening a
// new schema version carries over policy/prefill for unchanged fields,
// flags added fields for review, and lists removed fields for
// acknowledgment.
//
// Every curated schema currently has exactly one published version, so
// this has no real multi-version data to run against yet — same
// "infra ready, no real consumer yet" pattern as several Phase 1/2
// pieces (event-suggestions, AttachmentStore). It is exercised by unit
// tests constructing two synthetic schema.Schema versions directly,
// which pkg/schema.DiffSchemas already supports (it only compares two
// *schema.Schema values, regardless of where they came from).
type MigrationResult struct {
	NewPolicy     *SchemaFieldPolicy
	NewFields     []string // added since the old version; arrive at default policy (optional/none), flagged for review
	RemovedFields []string // present in the old version, gone in the new one; listed for acknowledgment only, nothing to carry
}

// Migrate carries old's policy/event/prefill overrides forward onto
// newSchemaVersion, dropping overrides for fields diff reports removed
// (a removed field's override is meaningless, and if a future version
// ever reintroduces a field with the same name, starting it fresh is
// safer than resurrecting a stale override). Added fields are not given
// an entry: absent-from-map already defaults to optional/none, matching
// architecture 5.3's "new fields arrive as optional... flagged for
// review."
func Migrate(old *SchemaFieldPolicy, newSchemaVersion string, diff *schema.Diff) *MigrationResult {
	removed := make(map[string]bool, len(diff.Removed))
	for _, f := range diff.Removed {
		removed[f.Name] = true
	}

	next := newUnchecked(old.Scope, old.SchemaName, newSchemaVersion)
	for name, state := range old.Policy {
		if removed[name] {
			continue
		}
		next.Policy[name] = state
	}
	for name, events := range old.Events {
		if removed[name] {
			continue
		}
		next.Events[name] = events
	}
	for name, class := range old.Prefill {
		if removed[name] {
			continue
		}
		next.Prefill[name] = class
	}

	result := &MigrationResult{
		NewPolicy:     next,
		RemovedFields: fieldNames(diff.Removed),
	}
	result.NewFields = fieldNames(diff.Added)
	return result
}

func fieldNames(fields []schema.Field) []string {
	if len(fields) == 0 {
		return nil
	}
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
	}
	return names
}
