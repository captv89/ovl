-- +goose Up

-- Adds the per-voyage-event dimension to field policy (architecture 6.1,
-- extended 2026-07-28). Before this, a field's company policy state was the
-- same on every report: the number of tugs used had to be either shown on a
-- Noon at sea report, where it is meaningless, or hidden on an Arrival and
-- Departure, where it matters.
--
-- One JSONB object per assignment, field name -> array of voyage event type
-- codes from schemas/ovd-3.13/enums/event-types.json, e.g.
--   {"Tugs_Used": ["Arrival", "Departure"]}
-- A field absent from the object applies to every event, so the '{}' default
-- makes every pre-existing assignment behave exactly as it did before — this
-- migration changes no vessel's rendered form until an event list is
-- actually authored in the office field policy editor.
--
-- Stored alongside policy rather than in a table of its own because a field's
-- state and the events it applies to are one authoring decision on one editor
-- row, and office/fieldpolicy.EffectiveFieldPolicy must resolve them together
-- from a single scope (see winningRule): splitting them across tables would
-- invite resolving a vessel-scope state against a fleet-scope event list.
ALTER TABLE field_policy_assignments ADD COLUMN events JSONB NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE field_policy_assignments DROP COLUMN events;
