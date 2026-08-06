# schemas

`meta-schema.json` is the JSON Schema that validates every curated OVD
schema document in this repository, and every schema uploaded through the
office UI (architecture handoff section 5.2). It currently defines 8
field types (`text`, `wholeNumber`, `decimal`, `date`, `time`, `dateTime`,
`boolean`, `enum`) — `dateTime` and `boolean` were added beyond the
architecture doc's original 6 to match real OVD 3.13 data — plus an
optional `unsupportedByDnv` field
property. Validation is not yet wired into CI (no CI pipeline exists
yet); it has been checked with a one-off script instead.

`ovd-3.13/` holds the five curated OVD 3.13 schema JSONs (Log Abstract —
409 fields, Bunker Report — 23, EDN Report — 9, Commercial Period — 6,
Cargo Nomination — 22) plus their shared `enums/` (event types, fuel
types, port call purposes, incoterms, charter types, offshore modes).
Authored from `handoff/OVD 3.13 interface description.xlsx` via a one-off
extraction script (not committed — an assisted xlsx importer is
explicitly out of scope for v1, see architecture section 18).

`ovd-3.13/event-suggestions/event-types.json` is the next-event
suggestion state machine (architecture 9.4): for every one of the 33
`event-types` codes, a short list of plausible next events for the
vessel Home screen's "suggested next report" card. It lives in its own
subdirectory, parallel to `enums/`, so the flat per-file schema-document
scan (`TestLoad_RealCuratedSchemas`) skips it the same way it already
skips `enums/`. Always overridable in the UI — the full event list is
one tap away — so this is a UX hint, not a validation rule; no
meta-schema governs it (same reasoning as `enums/`). No Go loader exists
yet (nothing consumes it until the Phase 2 Home screen is built),
mirroring how `enums/*.json` also has no loader yet.

Published schema versions are immutable: a new version is always a new
file/record, never an edit in place.
