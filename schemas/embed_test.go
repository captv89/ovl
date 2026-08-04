// SPDX-License-Identifier: AGPL-3.0-only

package schemas

import (
	"testing"

	"github.com/captv89/ovl/pkg/schema"
)

// TestFS_LoadsRealSchemas confirms the embedded FS actually works with
// pkg/schema's loader — the point of embedding this at all is so
// vessel/office binaries can load schemas without a repo-relative disk
// path, so this is worth verifying directly rather than trusting the
// embed directive silently.
func TestFS_LoadsRealSchemas(t *testing.T) {
	v, err := schema.NewValidator(FS, "meta-schema.json")
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	for _, name := range []string{"log-abstract", "bunker-report", "edn-report", "commercial-period", "cargo-nomination"} {
		t.Run(name, func(t *testing.T) {
			s, err := schema.Load(FS, "ovd-3.13/"+name+".json", v)
			if err != nil {
				t.Fatalf("Load(%s): %v", name, err)
			}
			if len(s.Fields) == 0 {
				t.Errorf("%s: schema has no fields", name)
			}
		})
	}
}

// TestFS_EventTypesAndSuggestionsAreConsistent loads the real embedded
// event-type enum and next-event suggestion table (design handoff A3;
// architecture 9.4) and checks the self-consistency the 2026-07-04
// decisions-log entry describes verifying "by script" at curation time —
// now a real, repo-checked test instead of a one-off. Every event-type
// code must appear exactly once as a suggestion-table "after", and every
// "suggest" target must be a real event-type code.
func TestFS_EventTypesAndSuggestionsAreConsistent(t *testing.T) {
	eventTypes, err := schema.LoadEventTypes(FS, "ovd-3.13/enums/event-types.json")
	if err != nil {
		t.Fatalf("LoadEventTypes: %v", err)
	}
	if len(eventTypes) == 0 {
		t.Fatal("no event types loaded")
	}
	suggestions, err := schema.LoadEventSuggestions(FS, "ovd-3.13/event-suggestions/event-types.json")
	if err != nil {
		t.Fatalf("LoadEventSuggestions: %v", err)
	}

	codes := make(map[string]bool, len(eventTypes))
	for _, et := range eventTypes {
		codes[et.Code] = true
	}

	afterSeen := make(map[string]int, len(suggestions))
	for _, s := range suggestions {
		afterSeen[s.After]++
		if !codes[s.After] {
			t.Errorf("suggestion entry %q is not a real event-type code", s.After)
		}
		for _, target := range s.Suggest {
			if !codes[target] {
				t.Errorf("suggestion after %q targets %q, not a real event-type code", s.After, target)
			}
		}
	}
	for code := range codes {
		if afterSeen[code] != 1 {
			t.Errorf("event type %q appears %d times as a suggestion-table \"after\", want exactly 1", code, afterSeen[code])
		}
	}
}
