// SPDX-License-Identifier: AGPL-3.0-only

import { useCallback, useEffect, useMemo, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { Select } from "../../design/components/forms/Select.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Checkbox } from "../../design/components/forms/Checkbox.jsx";
import { SegmentedButtonMenu } from "../../design/components/forms/SegmentedButtonMenu.jsx";
import { MultiSelectMenu } from "../../design/components/forms/MultiSelectMenu.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Tooltip } from "../../design/components/feedback/Tooltip.jsx";
import { DataTable, type DataTableColumn } from "../../design/components/data/DataTable.jsx";
import {
  api,
  type FieldPolicyAssignmentView,
  type FieldPolicyView,
  type SchemaField,
  type Scope,
  type SchemaVersionSummary,
  type UserView,
  type VesselView,
} from "../../api/client";
import { SchemaUploadPanel } from "./SchemaUploadPanel";
import { ScopeSelector } from "./ScopeSelector";
import { scopeLabel, scopesEqual } from "./complianceLogic";
import { POLICY_STATES, PREFILL_CLASSES, effectiveState, effectivePrefill, visibleFieldCount, sectionsInOrder } from "./fieldPolicyLogic";

// The "Preview as" option meaning "do not filter by any one event" — the
// default, and what the screen showed before per-event policy existed.
const ALL_EVENTS_PREVIEW = "All events";

// Design handoff B7's override-precedence explanation ("vessel > group >
// fleet") applies here too, but the 2026-07 mockup (Configuration
// Redesign.dc.html) does not show it as a standing banner on this screen
// the way it does on Regulatory profiles/Rule severities/Bundles — it's
// folded into an info tooltip on the heading instead, since this screen's
// vertical space is already contested by the section rail + table.
const OVERRIDE_PRECEDENCE_TEXT =
  "Vessel and group settings override the fleet default. Most specific wins: vessel > group > fleet. Unset scopes inherit the level above.";

type Mode = { kind: "editor" } | { kind: "upload" };

// Shape of one DataTable row for the field grid — flat label/type/
// relevance/policyState/prefill string fields so DataTable's generic
// cellText (which reads row[column.key] directly for a `render` column)
// has real values to sort/filter on, plus the nested SchemaField for
// each column's own render logic.
type FieldPolicyRow = {
  id: string;
  field: SchemaField;
  label: string;
  type: string;
  relevance: string;
  policyState: string;
  prefill: string;
  events: string[];
};

// Design handoff B6, the field policy editor — "the heaviest office
// screen, treat with care." Section tree left, field table center with a
// 5-value policy-state control and a prefill class control
// (schemaMandatory locked; both rendered as a compact SegmentedButtonMenu
// trigger + popover, not inline, to keep the table narrow), search/filter
// toolbar, migration assistant when opening a version newer than the one
// field policy was last saved against. 2026-07-25 redesign: the section
// rail's own per-section "Total · Visible" counts replace what used to be
// a separate right-rail "live impact preview" panel — the mockup dropped
// that panel once the rail carried the same numbers, so this screen
// doesn't duplicate it either.
export function FieldPolicyScreen({ user }: { user: UserView }) {
  const canEdit = user.roles.includes("configManager");

  const [schemas, setSchemas] = useState<SchemaVersionSummary[] | null>(null);
  const [selectedSchema, setSelectedSchema] = useState<string | null>(null);
  const [mode, setMode] = useState<Mode>({ kind: "editor" });
  const [fp, setFp] = useState<FieldPolicyView | null>(null);
  const [error, setError] = useState<string | null>(null);

  const [vessels, setVessels] = useState<VesselView[]>([]);
  const [scope, setScope] = useState<Scope>({ type: "fleet" });
  const [assignments, setAssignments] = useState<FieldPolicyAssignmentView[]>([]);

  useEffect(() => {
    api.listVessels().then(setVessels).catch(() => setVessels([]));
  }, []);

  const [policy, setPolicy] = useState<Record<string, string>>({});
  const [prefill, setPrefill] = useState<Record<string, string>>({});
  // Per-field voyage-event narrowing. A field absent from this map applies to
  // every event, which is why an untouched row shows "All events".
  const [events, setEvents] = useState<Record<string, string[]>>({});
  // Which event the table and the section counts are previewed for. Purely a
  // view control — it never changes what is saved.
  const [previewEvent, setPreviewEvent] = useState<string>(ALL_EVENTS_PREVIEW);
  const [reviewed, setReviewed] = useState<Set<string>>(new Set());
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [saving, setSaving] = useState(false);

  const [search, setSearch] = useState("");
  const [sectionFilter, setSectionFilter] = useState<string | null>(null);
  // Reported by DataTable's onVisibleRowsChange — the set actually on
  // screen after its own column sort/filter run on top of search +
  // section, so "select all" never selects a row a column filter is
  // currently hiding.
  const [tableVisibleRows, setTableVisibleRows] = useState<FieldPolicyRow[]>([]);

  useEffect(() => {
    api
      .listLatestSchemaVersions()
      .then((list) => {
        setSchemas(list);
        if (list.length > 0) setSelectedSchema((prev) => prev ?? list[0].schemaName);
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load schemas."));
  }, []);

  const loadFieldPolicy = (schemaName: string, forScope: Scope) => {
    setFp(null);
    setSelected(new Set());
    setReviewed(new Set());
    api
      .getFieldPolicy(schemaName, forScope)
      .then((view) => {
        setFp(view);
        setPolicy(view.policy);
        setPrefill(view.prefill);
        setEvents(view.events ?? {});
        setPreviewEvent(ALL_EVENTS_PREVIEW);
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load field policy."));
  };

  const reloadAssignments = (schemaName: string) => {
    api
      .listFieldPolicyAssignments(schemaName)
      .then(setAssignments)
      .catch(() => setAssignments([]));
  };

  // Switching either the schema or the scope re-fetches from scratch —
  // same "silently discard any unsaved edits" behavior this screen
  // already had when only the schema could change.
  useEffect(() => {
    if (selectedSchema) {
      loadFieldPolicy(selectedSchema, scope);
      reloadAssignments(selectedSchema);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedSchema, scope]);

  // A pending migration proposal is never persisted yet even though it
  // already matches policy/prefill's initial state (both were seeded
  // from the same GET response) — without this, Save silently has
  // nothing to do until the user also happens to touch a field, so a
  // migration could sit forever as an unsaved "preview" no control ever
  // flushes to the store.
  const dirty =
    fp != null &&
    (fp.migration != null ||
      JSON.stringify(policy) !== JSON.stringify(fp.policy) ||
      JSON.stringify(prefill) !== JSON.stringify(fp.prefill) ||
      JSON.stringify(events) !== JSON.stringify(fp.events ?? {}));

  const sections = useMemo(() => (fp ? sectionsInOrder(fp.fields) : []), [fp]);
  // Empty for a schema with no event concept of its own — the whole
  // applies-to-events column and the preview control disappear for those
  // rather than offering a narrowing no report could ever match.
  const eventTypes = useMemo(() => fp?.eventTypes ?? [], [fp]);
  const eventful = eventTypes.length > 0;
  // The event the preview control is pointed at, or undefined for "All
  // events" — undefined is what makes visibleFieldCount/effectiveState give
  // their event-agnostic answer.
  const previewEventType = previewEvent === ALL_EVENTS_PREVIEW ? undefined : previewEvent;
  const impact = useMemo(
    () => (fp ? visibleFieldCount(fp.fields, policy, events, previewEventType) : { total: 0, bySection: {} }),
    [fp, policy, events, previewEventType],
  );

  // Search + section are this screen's own toolbar controls (mirrored in
  // the mockup); Relevance/Policy state/Prefill are filtered via the
  // grid's own column-header filter icons instead (DataTable's built-in
  // filterable columns below), not a second, redundant set of Selects.
  const visibleFields = useMemo(() => {
    if (!fp) return [];
    const q = search.trim().toLowerCase();
    return fp.fields.filter((f) => {
      if (sectionFilter && f.section !== sectionFilter) return false;
      if (q && !f.name.toLowerCase().includes(q) && !f.label.toLowerCase().includes(q)) return false;
      return true;
    });
  }, [fp, search, sectionFilter]);

  const tableRows = useMemo<FieldPolicyRow[]>(
    () =>
      visibleFields.map((f) => ({
        id: f.name,
        field: f,
        label: f.label,
        type: f.type + (f.unit ? ` (${f.unit})` : ""),
        relevance: f.relevance,
        policyState: effectiveState(f, policy, events, previewEventType),
        prefill: effectivePrefill(f, prefill),
        events: events[f.name] ?? [],
      })),
    [visibleFields, policy, prefill, events, previewEventType],
  );

  const allVisibleSelected = tableVisibleRows.length > 0 && tableVisibleRows.every((r) => selected.has(r.field.name));

  const toggleSelectAllVisible = useCallback(() => {
    setSelected((prev) => {
      const next = new Set(prev);
      const shouldSelect = !(tableVisibleRows.length > 0 && tableVisibleRows.every((r) => selected.has(r.field.name)));
      for (const r of tableVisibleRows) {
        if (shouldSelect) {
          if (!r.field.schemaMandatory) next.add(r.field.name);
        } else {
          next.delete(r.field.name);
        }
      }
      return next;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tableVisibleRows, selected]);

  // Always writes an explicit entry, even for "optional" — unlike
  // Prefill's "none" (whose absent-key default is unconditionally
  // "none", see effectivePrefill), an absent policy key does NOT
  // default to "optional": pkg/validation.FieldPolicy.StateFor (the
  // source of truth this mirrors) defaults an unlisted GHG-relevant
  // field to "recommended" instead, specifically so it "shouldn't
  // quietly collapse behind optional." Deleting the key here for
  // "optional" used to silently un-set nothing for a GHG-relevant field
  // like Offhire_Reasons — the dropdown showed Optional selected, but
  // effectiveState recomputed straight back to "recommended" on the
  // very next render, since there was no explicit override left to read.
  function setFieldState(name: string, state: string) {
    setPolicy((prev) => ({ ...prev, [name]: state }));
  }

  // An empty list means "every event", so it is stored as the absence of an
  // entry rather than an empty array — keeping the saved map to real
  // narrowings only, exactly as setFieldPrefill does for "none".
  function setFieldEvents(name: string, next: string[]) {
    setEvents((prev) => {
      const out = { ...prev };
      if (next.length === 0) delete out[name];
      else out[name] = next;
      return out;
    });
  }

  function setFieldPrefill(name: string, cls: string) {
    setPrefill((prev) => {
      const next = { ...prev };
      if (cls === "none") {
        delete next[name];
      } else {
        next[name] = cls;
      }
      return next;
    });
  }

  const [bulkState, setBulkState] = useState("optional");
  const [bulkPrefill, setBulkPrefill] = useState("none");
  const [bulkEvents, setBulkEvents] = useState<string[]>([]);

  const clearSelection = useCallback(() => setSelected(new Set()), []);

  // A bulk apply is otherwise silent: the changed rows are usually
  // scrolled out of view at 409 fields, so nothing on screen confirms it
  // landed. Carries its own id so re-applying the same action restarts
  // the timer instead of the message appearing stuck.
  const [bulkNotice, setBulkNotice] = useState<{ text: string; id: number } | null>(null);

  useEffect(() => {
    if (!bulkNotice) return undefined;
    const t = setTimeout(() => setBulkNotice(null), 4000);
    return () => clearTimeout(t);
  }, [bulkNotice]);

  function notifyBulk(text: string) {
    setBulkNotice({ text, id: Date.now() });
  }

  // schemaMandatory fields are skipped by the state/events bulk applies
  // (architecture 6.1 — the schema decides, not the company), so the
  // confirmation counts what actually changed, not what was ticked.
  const editableSelectionSize = useMemo(
    () => [...selected].filter((name) => !fp?.fields.find((f) => f.name === name)?.schemaMandatory).length,
    [selected, fp],
  );

  function applyBulkState() {
    setPolicy((prev) => {
      const next = { ...prev };
      for (const name of selected) {
        const field = fp?.fields.find((f) => f.name === name);
        if (field?.schemaMandatory) continue;
        next[name] = bulkState;
      }
      return next;
    });
    notifyBulk(`State set to ${bulkState} on ${editableSelectionSize} ${editableSelectionSize === 1 ? "field" : "fields"}`);
  }

  // Bulk-applies one event narrowing across the selection — the reason the
  // per-event model stays workable at 409 fields: "these 6 tug/berth fields
  // are Arrival and Departure only" is one action, not six.
  function applyBulkEvents() {
    setEvents((prev) => {
      const next = { ...prev };
      for (const name of selected) {
        const field = fp?.fields.find((f) => f.name === name);
        if (field?.schemaMandatory) continue;
        if (bulkEvents.length === 0) delete next[name];
        else next[name] = bulkEvents;
      }
      return next;
    });
    const scope = bulkEvents.length === 0 ? "all events" : bulkEvents.join(", ");
    notifyBulk(`Applies-to set to ${scope} on ${editableSelectionSize} ${editableSelectionSize === 1 ? "field" : "fields"}`);
  }

  function applyBulkPrefill() {
    setPrefill((prev) => {
      const next = { ...prev };
      for (const name of selected) {
        if (bulkPrefill === "none") delete next[name];
        else next[name] = bulkPrefill;
      }
      return next;
    });
    notifyBulk(`Prefill set to ${bulkPrefill} on ${selected.size} ${selected.size === 1 ? "field" : "fields"}`);
  }

  // Memoized so its reference stays stable across renders that don't
  // change any of its closed-over values — DataTable's onVisibleRowsChange
  // (below) feeds tableVisibleRows back into this component's own state,
  // and a fresh columns array on every render would otherwise re-trigger
  // that effect in a loop the instant a search query or column filter is
  // active (the only case where DataTable's own filtered rows aren't
  // already reference-stable across renders).
  const columns = useMemo<DataTableColumn[]>(
    () => [
      {
        key: "select",
        label: "",
        headerRender: () => (canEdit ? <Checkbox checked={allVisibleSelected} onChange={toggleSelectAllVisible} /> : null),
        render: (row) =>
          canEdit && !row.field.schemaMandatory ? (
            <Checkbox
              checked={selected.has(row.field.name)}
              onChange={(checked) => {
                setSelected((prev) => {
                  const next = new Set(prev);
                  if (checked) next.add(row.field.name);
                  else next.delete(row.field.name);
                  return next;
                });
              }}
            />
          ) : null,
      },
      {
        key: "label",
        label: "Field",
        sortable: true,
        render: (row) => {
          const f = row.field;
          const isNew = fp?.migration?.newFields.includes(f.name) ?? false;
          return (
            <div>
              <div className="md-title-small">{f.label}</div>
              <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
                <span style={{ fontFamily: "var(--font-mono)" }}>{f.name}</span>
                {isNew ? (
                  <span
                    onClick={() =>
                      setReviewed((prev) => {
                        const next = new Set(prev);
                        if (next.has(f.name)) next.delete(f.name);
                        else next.add(f.name);
                        return next;
                      })
                    }
                    className="md-label-medium"
                    style={{
                      marginLeft: 8, padding: "0 8px", borderRadius: "var(--shape-full)", cursor: "pointer",
                      background: reviewed.has(f.name) ? "var(--color-status-underway-container)" : "var(--color-status-caution-container)",
                      color: reviewed.has(f.name) ? "var(--color-status-underway)" : "var(--color-status-caution)",
                    }}
                  >
                    {reviewed.has(f.name) ? "Reviewed" : "New — review"}
                  </span>
                ) : null}
              </div>
            </div>
          );
        },
      },
      {
        key: "type",
        label: "Type",
        sortable: true,
        render: (row) => <span className="md-body-medium">{row.type}</span>,
      },
      {
        key: "relevance",
        label: "Relevance",
        filterable: true,
        render: (row) => <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>{row.relevance}</span>,
      },
      {
        key: "policyState",
        label: "Policy state",
        filterable: true,
        render: (row) => {
          const f = row.field;
          return (
            <SegmentedButtonMenu
              options={POLICY_STATES.map((s) => ({
                ...s,
                // The schemaMandatory option itself is never
                // user-selectable; if the field IS schemaMandatory, no
                // option is (the schema decides, not the company —
                // architecture 6.1).
                locked: f.schemaMandatory || s.value === "schemaMandatory",
              }))}
              value={row.policyState}
              onChange={(v) => canEdit && !f.schemaMandatory && setFieldState(f.name, v)}
              style={{ width: 170 }}
            />
          );
        },
      },
      {
        key: "events",
        label: "Applies to events",
        render: (row) => {
          const f = row.field;
          // schemaMandatory is immutable (architecture 6.1), so it can no more
          // be narrowed to some events than it can be re-stated — the control
          // renders locked for the same reason the policy-state one does.
          return (
            <MultiSelectMenu
              options={eventTypes}
              value={row.events}
              allLabel="All events"
              locked={!canEdit || f.schemaMandatory}
              onChange={(next) => setFieldEvents(f.name, next)}
              style={{ width: 170 }}
            />
          );
        },
      },
      {
        key: "prefill",
        label: "Prefill",
        filterable: true,
        render: (row) => {
          const value = row.prefill;
          const active = value !== "none";
          return (
            <SegmentedButtonMenu
              options={PREFILL_CLASSES.map((v) => ({ value: v, label: v }))}
              value={value}
              onChange={(v) => canEdit && setFieldPrefill(row.field.name, v)}
              style={{
                width: 160,
                background: active ? "var(--color-tertiary-container)" : "var(--color-surface-container-highest)",
                color: active ? "var(--color-on-tertiary-container)" : "var(--color-on-surface-variant)",
              }}
            />
          );
        },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [canEdit, fp?.migration, reviewed, selected, allVisibleSelected, toggleSelectAllVisible, eventTypes],
  );

  async function save() {
    if (!fp) return;
    setSaving(true);
    setError(null);
    try {
      const saved = await api.saveFieldPolicy(fp.schemaName, scope, policy, prefill, events);
      setFp(saved);
      setPolicy(saved.policy);
      setPrefill(saved.prefill);
      setEvents(saved.events ?? {});
      reloadAssignments(fp.schemaName);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save field policy.");
    } finally {
      setSaving(false);
    }
  }

  function discard() {
    if (!fp) return;
    setPolicy(fp.policy);
    setPrefill(fp.prefill);
    setEvents(fp.events ?? {});
  }

  if (mode.kind === "upload" && fp) {
    return (
      <SchemaUploadPanel
        schemaName={fp.schemaName}
        onCancel={() => setMode({ kind: "editor" })}
        onPublished={() => {
          setMode({ kind: "editor" });
          loadFieldPolicy(fp.schemaName, scope);
          reloadAssignments(fp.schemaName);
        }}
      />
    );
  }

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16, height: "100%", boxSizing: "border-box" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 16 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <div className="md-headline-small">Field policy</div>
          <Tooltip label={OVERRIDE_PRECEDENCE_TEXT} maxWidth={260}>
            <span
              className="material-symbols-rounded"
              style={{ fontSize: 18, color: "var(--color-on-surface-variant)", cursor: "help", display: "flex" }}
              aria-label="How overriding works"
            >
              info
            </span>
          </Tooltip>
        </div>
        {fp ? (
          <div style={{ display: "flex", gap: 8 }}>
            <a href={api.downloadSchemaVersionUrl(fp.schemaName, fp.version)} download>
              <Button variant="outlined" icon="download">
                Download JSON
              </Button>
            </a>
            {canEdit ? (
              <Button variant="outlined" icon="upload" onClick={() => setMode({ kind: "upload" })}>
                Upload new version
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>

      {error ? <AlertBanner level="warning" title="Something went wrong" message={error} onDismiss={() => setError(null)} /> : null}

      {assignments && assignments.length > 0 ? (
        <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
          Overrides exist for:{" "}
          {assignments
            .filter((a) => !scopesEqual(a.scope, scope))
            .map((a) => scopeLabel(a.scope, vessels))
            .join(", ") || "no other scope yet"}
        </div>
      ) : null}

      {!fp ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          Loading…
        </div>
      ) : (
        <>
          {fp.migration ? (
            <AlertBanner
              level="caution"
              title={`Migrating field policy from v${fp.migration.fromVersion}`}
              message={`Carried-over policy is pre-filled below. ${fp.migration.newFields.length} new field(s) need review${
                fp.migration.removedFields.length > 0 ? `; ${fp.migration.removedFields.length} field(s) were removed and no longer appear` : ""
              }. Nothing is saved until you click Save.`}
            />
          ) : null}

          <div style={{ display: "flex", gap: 16, flex: 1, minHeight: 0 }}>
            {/* Left: section tree */}
            <Card variant="elevated" style={{ padding: 8, width: 220, flexShrink: 0, overflowY: "auto", boxShadow: "none" }}>
              <div style={{ padding: "10px 12px 8px", display: "flex", justifyContent: "space-between" }}>
                <span className="md-label-small" style={{ color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em" }}>
                  Section
                </span>
                <span className="md-label-small" style={{ color: "var(--color-on-surface-variant)" }}>
                  Total · Visible
                </span>
              </div>
              <div
                onClick={() => setSectionFilter(null)}
                className="md-title-small"
                style={{
                  padding: "10px 12px", borderRadius: "var(--shape-small)", cursor: "pointer", marginBottom: 2,
                  display: "flex", justifyContent: "space-between", alignItems: "center",
                  background: sectionFilter === null ? "var(--color-secondary-container)" : "transparent",
                  color: sectionFilter === null ? "var(--color-on-secondary-container)" : "var(--color-on-surface)",
                }}
              >
                <span>All sections</span>
                <SectionCounts total={fp.fields.length} visible={impact.total} />
              </div>
              {sections.map((section) => {
                const count = fp.fields.filter((f) => f.section === section).length;
                return (
                  <div
                    key={section}
                    onClick={() => setSectionFilter(section)}
                    className="md-body-medium"
                    style={{
                      padding: "9px 12px", borderRadius: "var(--shape-small)", cursor: "pointer",
                      display: "flex", justifyContent: "space-between", alignItems: "center",
                      background: sectionFilter === section ? "var(--color-secondary-container)" : "transparent",
                      color: "var(--color-on-surface)",
                    }}
                  >
                    <span>{section}</span>
                    <SectionCounts total={count} visible={impact.bySection[section] ?? 0} />
                  </div>
                );
              })}
            </Card>

            {/* Center: toolbar + table */}
            <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 12, minWidth: 0 }}>
              <div style={{ display: "flex", gap: 12, justifyContent: "space-between", alignItems: "flex-end", flexWrap: "wrap" }}>
                <div style={{ display: "flex", gap: 10 }}>
                  {schemas && selectedSchema ? (
                    <Select
                      label="Schema"
                      value={selectedSchema}
                      options={schemas.map((s) => s.schemaName)}
                      onChange={setSelectedSchema}
                      style={{ width: 180 }}
                    />
                  ) : null}
                  <ScopeSelector scope={scope} onChange={setScope} vessels={vessels} />
                  {eventful ? (
                    <Select
                      label="Preview as"
                      value={previewEvent}
                      options={[ALL_EVENTS_PREVIEW, ...eventTypes]}
                      onChange={setPreviewEvent}
                      style={{ width: 220 }}
                    />
                  ) : null}
                </div>
                <TextField label="Search fields" value={search} onChange={setSearch} leadingIcon="search" style={{ width: 300 }} />
              </div>

              {selected.size > 0 && canEdit ? (
                // A contextual selection toolbar, not a panel: it exists only
                // while rows are ticked, so it reads as a state of the grid
                // (primary edge accent, elevated surface) rather than another
                // slab of chrome. The count and its Clear sit together as one
                // unit at the head — the selection was previously a dead-end,
                // stating "17 selected" with no way out of it short of
                // hunting down 17 checkboxes across a 409-row scroller.
                <div
                  role="toolbar"
                  aria-label={`Bulk actions for ${selected.size} selected fields`}
                  onKeyDown={(e) => {
                    if (e.key === "Escape") clearSelection();
                  }}
                  style={{
                    display: "flex", alignItems: "center", gap: 14, flexWrap: "wrap",
                    padding: "10px 14px", borderRadius: "var(--shape-medium)",
                    background: "var(--color-surface-container-high)",
                    border: "1px solid var(--color-outline-variant)",
                    borderLeft: "3px solid var(--color-primary)",
                  }}
                >
                  <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
                    <span className="md-title-small" style={{ fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>
                      {selected.size} selected
                    </span>
                    <Button variant="text" size="small" icon="close" onClick={clearSelection}>
                      Clear
                    </Button>
                  </div>

                  <BulkDivider />

                  {/* Each action is one labelled group — select plus its own
                      Apply, fenced by dividers. Three bare Apply buttons
                      spaced like siblings read as three copies of the same
                      button rather than as the tail of three separate
                      actions. */}
                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <Select
                      label="Set state to"
                      value={bulkState}
                      options={POLICY_STATES.filter((s) => s.value !== "schemaMandatory").map((s) => s.value)}
                      onChange={setBulkState}
                      style={{ width: 180 }}
                    />
                    <Button variant="tonal" size="small" onClick={applyBulkState}>Apply</Button>
                  </div>

                  <BulkDivider />

                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <Select
                      label="Set prefill to"
                      value={bulkPrefill}
                      options={PREFILL_CLASSES}
                      onChange={setBulkPrefill}
                      style={{ width: 180 }}
                    />
                    <Button variant="tonal" size="small" onClick={applyBulkPrefill}>Apply</Button>
                  </div>

                  {eventful ? (
                    <>
                      <BulkDivider />
                      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                        <MultiSelectMenu
                          label="Set events to"
                          options={eventTypes}
                          value={bulkEvents}
                          allLabel="All events"
                          onChange={setBulkEvents}
                          style={{ width: 180 }}
                        />
                        <Button variant="tonal" size="small" onClick={applyBulkEvents}>Apply</Button>
                      </div>
                    </>
                  ) : null}

                  {bulkNotice ? (
                    <div
                      role="status"
                      aria-live="polite"
                      className="md-body-small"
                      style={{
                        marginLeft: "auto", display: "flex", alignItems: "center", gap: 6,
                        color: "var(--color-primary)",
                      }}
                    >
                      <span className="material-symbols-rounded" style={{ fontSize: 18 }}>check_circle</span>
                      {bulkNotice.text}
                    </div>
                  ) : null}
                </div>
              ) : null}

              <div style={{ flex: 1, overflow: "auto" }}>
                <DataTable
                  hideSearch
                  emptyMessage="No fields match the current filters."
                  columns={columns}
                  rows={tableRows}
                  groupBy={(row) => row.field.section}
                  onVisibleRowsChange={(rows) => setTableVisibleRows(rows as FieldPolicyRow[])}
                  rowStyle={(row) => (row.field.schemaMandatory ? { borderLeft: "3px solid var(--color-tertiary)" } : undefined)}
                />
              </div>

              <div style={{ display: "flex", gap: 8, alignItems: "center", justifyContent: "space-between" }}>
                <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
                  {canEdit ? (
                    <>
                      <Button variant="filled" onClick={save} disabled={!dirty || saving}>
                        Save
                      </Button>
                      <Button variant="text" onClick={discard} disabled={!dirty || saving}>
                        Discard changes
                      </Button>
                      {dirty ? <span className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>Unsaved changes</span> : null}
                    </>
                  ) : null}
                </div>
                <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
                  v{fp.version} · {fp.fields.length} fields
                </div>
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

// Fences one action group off from the next in the bulk-actions toolbar,
// so its Apply reads as belonging to the control on its left rather than
// as one of three identical loose buttons.
function BulkDivider() {
  return (
    <span
      aria-hidden="true"
      style={{ width: 1, alignSelf: "stretch", minHeight: 32, background: "var(--color-outline-variant)", flexShrink: 0 }}
    />
  );
}

// Total · visible chip for one section row in the left rail — visible
// tracks the live policy edits (impact.bySection), not just the static
// field count, so hiding fields shows up immediately without opening
// the right-rail preview.
function SectionCounts({ total, visible }: { total: number; visible: number }) {
  return (
    <span className="md-label-medium" style={{ color: "var(--color-on-surface-variant)", display: "flex", alignItems: "center", gap: 6 }}>
      {total}
      <span
        style={{
          display: "inline-flex", alignItems: "center", gap: 3, padding: "1px 6px",
          borderRadius: "var(--shape-extra-small)", background: "var(--color-surface-container-highest)",
          color: "var(--color-on-surface-variant)",
        }}
      >
        <span className="material-symbols-rounded" style={{ fontSize: 13 }}>visibility</span>
        {visible}
      </span>
    </span>
  );
}
