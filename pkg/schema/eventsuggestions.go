// SPDX-License-Identifier: AGPL-3.0-only

package schema

import (
	"encoding/json"
	"fmt"
	"io/fs"
)

// EventType is one entry from schemas/ovd-3.13/enums/event-types.json — the
// 33 OVD event-type codes, matching the "event-types" enumRef on the
// curated Event field (architecture 6.2/9.4).
type EventType struct {
	Code          string  `json:"code"`
	Remark        *string `json:"remark"`
	FixedSpelling bool    `json:"fixedSpelling"`
}

type eventTypesDoc struct {
	EnumName   string      `json:"enumName"`
	OvdVersion string      `json:"ovdVersion"`
	Values     []EventType `json:"values"`
}

// LoadEventTypes reads the full OVD event-type enum. Design handoff A3's
// "Other event…" affordance needs the complete list; A9.4's next-event
// suggestion state machine (LoadEventSuggestions) needs it to be the same
// vocabulary its "after"/"suggest" codes reference.
func LoadEventTypes(fsys fs.FS, path string) ([]EventType, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("read event types %s: %w", path, err)
	}
	var doc eventTypesDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse event types %s: %w", path, err)
	}
	return doc.Values, nil
}

// EventSuggestion is one "after this event, suggest these next" entry from
// schemas/ovd-3.13/event-suggestions/event-types.json (architecture 9.4:
// "Next-event suggestion is a state machine over the OVD event types,
// shipped as data in the curated schema").
type EventSuggestion struct {
	After   string   `json:"after"`
	Suggest []string `json:"suggest"`
}

type eventSuggestionsDoc struct {
	OvdVersion  string            `json:"ovdVersion"`
	Suggestions []EventSuggestion `json:"suggestions"`
}

// LoadEventSuggestions reads the curated next-event suggestion table.
func LoadEventSuggestions(fsys fs.FS, path string) ([]EventSuggestion, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("read event suggestions %s: %w", path, err)
	}
	var doc eventSuggestionsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse event suggestions %s: %w", path, err)
	}
	return doc.Suggestions, nil
}
