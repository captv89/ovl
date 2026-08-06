// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useMemo, useRef, useState } from "react";
import { FormWizard, type WizardFillStatus } from "../../design/components/forms-wizard/FormWizard.jsx";
import { Button } from "../../design/components/core/Button.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Dialog } from "../../design/components/feedback/Dialog.jsx";
import { ReportStatusBadge } from "../../design/components/maritime/ReportStatusBadge.jsx";
import {
  api,
  ApiError,
  type ContinuityImpact,
  type Finding,
  type FieldPolicyState,
  type LockConflict,
  type PrefillEntry,
  type ProfileReadiness,
  type ReportEvent,
  type ReportState,
  type ReportView,
  type RobSeries,
  type Schema,
  type SchemaField,
  type SectionLock,
  type UserView,
} from "../../api/client";
import { ROLE_LABELS } from "../account/roleLabels";
import { effectiveState } from "./fieldPolicy";
import { sectionLabel, visibleSectionsInOrder } from "./sections";
import { SectionPanel } from "./SectionPanel";
import { HealthCheckPanel, findingKey } from "./HealthCheckPanel";
import type { FieldValue } from "./FieldRow";
import { computeDerivedValues, computeRobDerivedValues, computeTimeSincePreviousReport, DERIVED_FIELDS } from "./derivedFields";
import { buildChangesBySection, buildFieldsPayload, computeEventTime, decodeFieldsPayload, hasEventTimeEntered } from "./reportPersistence";
import { formatUtc } from "../../format";

// Section-lock heartbeat interval (architecture 9.5): comfortably under
// the 5-minute server-side TTL (vessel/httpapi's lockTTL) so an actively
// open section never expires out from under its holder.
const LOCK_HEARTBEAT_INTERVAL_MS = 60_000;

// pkg/validation/fieldrules.go's RuleFieldRequired — the one rule ID
// EvaluateFieldRules ever emits for an empty mandatory (error) or empty
// recommended (warning) field. Filtered out of the live, pre-Check
// findings feed below (2026-07-13 manual-test feedback, two rounds: only
// the red/amber error/warning treatment should wait for a real Check —
// the mandatory/GHG-relevant policyOutline border marking *which* fields
// are mandatory/recommended/both is static schema/policy metadata, not a
// validation result, and stays visible by default; see FieldRow.tsx's
// own `outline` doc comment). Every other rule class (format/type,
// plausibility) stays live regardless of `checked` — those catch an
// actual mistake as it's typed, unlike "this field is simply still
// empty," which the always-on policyOutline border already conveys.
const RULE_FIELD_REQUIRED = "field.required";

// Extracts the lock-conflict body (vessel/httpapi's lockConflictResponse)
// from a 409 ApiError, or null for anything else — shared by saveChanges
// and the section-lock acquire effect, both of which need to read who
// actually holds a section they were rejected from.
function lockConflictFromError(err: unknown): LockConflict | null {
  if (err instanceof ApiError && err.status === 409 && err.body && typeof err.body === "object" && "lock" in err.body) {
    return err.body as LockConflict;
  }
  return null;
}

const STATE_LABELS: Record<ReportState, string> = {
  draft: "Draft",
  ready: "Ready",
  submitted: "Submitted",
  synced: "Synced",
  pushed: "Pushed",
  remarked: "Remarked",
  invalidated: "Invalidated",
};

// A report stops being editable the moment it leaves draft/ready
// (architecture 8.1) — matches the reports_locked_after_submit DB
// trigger and vessel/httpapi's own loadEditableReport check.
function isLocked(state: ReportState): boolean {
  return state !== "draft" && state !== "ready";
}

interface WizardIssue {
  severity: "error" | "warning";
  message: string;
  sectionLabel?: string;
  field?: string;
}

// Maps health-check findings (pkg/validation.Finding, one per rule
// violation) onto FormWizard's own issue/section-count shape: a finding
// with no Field (a whole-report plausibility rule, e.g. timestamp
// uniqueness) has no section to scope to and only ever shows in the
// "all sections" view, and isn't jumpable (nothing to jump to). Info-
// severity findings are advisory only (architecture 10.2) and have no
// footer-bar representation today.
function findingsToWizardData(findings: Finding[], schema: Schema) {
  const sectionByField = new Map(schema.fields.map((f) => [f.name, f.section]));
  const issues: WizardIssue[] = [];
  const counts = new Map<string, { errors: number; warnings: number }>();
  for (const f of findings) {
    if (f.severity === "info") continue;
    const section = f.field ? sectionByField.get(f.field) : undefined;
    issues.push({ severity: f.severity, message: f.message, sectionLabel: section ? sectionLabel(section) : undefined, field: f.field || undefined });
    if (section) {
      const c = counts.get(section) ?? { errors: 0, warnings: 0 };
      if (f.severity === "error") c.errors++;
      else c.warnings++;
      counts.set(section, c);
    }
  }
  return { issues, counts };
}

// Folds finding_acknowledged events for one version into the set of
// currently-acknowledged findingKeys, last-write-wins (a later event for
// the same ruleId+field overrides an earlier one — un-acknowledging is
// its own event, not a deletion, see pkg/domain.EventFindingAcknowledged's
// own doc comment). Events are already chronological (oldest first, per
// handleListReportEvents), so a plain forward fold is correct.
function acknowledgedKeysFromEvents(events: ReportEvent[], versionNo: number): Set<string> {
  const keys = new Set<string>();
  for (const e of events) {
    if (e.type !== "finding_acknowledged" || e.versionNo !== versionNo) continue;
    const ruleId = typeof e.detail?.ruleId === "string" ? e.detail.ruleId : "";
    const field = typeof e.detail?.field === "string" ? e.detail.field : "";
    const key = `${ruleId}|${field}`;
    if (e.detail?.acknowledged) keys.add(key);
    else keys.delete(key);
  }
  return keys;
}

export function schemaDisplayName(name: string): string {
  return name.replace(/-/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

function initialValues(fields: SchemaField[], prefill: Record<string, PrefillEntry>): Record<string, FieldValue> {
  const values: Record<string, FieldValue> = {};
  for (const f of fields) {
    const entry = prefill[f.name];
    // carryForward and computed prefill directly into the field
    // (editable, architecture 6.4); ghost only ever shows as a hint
    // (FieldRow's supportingText) and never auto-fills.
    if (entry && (entry.class === "carryForward" || entry.class === "computed") && entry.value !== undefined) {
      values[f.name] = typeof entry.value === "boolean" ? entry.value : String(entry.value);
    }
  }
  return values;
}

// Placeholder fill-tracking: counts non-hidden, non-optional fields
// with a non-empty value. Real completeness (and error/warning counts)
// comes from pkg/validation once the live-validation and lifecycle
// wiring exist (separate, not-yet-started Phase 2 items) — this just
// keeps the section nav from looking inert in the meantime.
function sectionFillStatus(
  fields: SchemaField[],
  policy: Record<string, FieldPolicyState>,
  values: Record<string, FieldValue>,
  events: Record<string, string[]>,
  eventType: string | undefined,
): WizardFillStatus {
  const required = fields.filter((f) => {
    const state = effectiveState(f, policy, events, eventType);
    return state !== "hidden" && state !== "optional";
  });
  if (required.length === 0) return "empty";
  const filled = required.filter((f) => {
    const v = values[f.name];
    return f.type === "boolean" ? v !== undefined : v !== undefined && v !== "";
  });
  if (filled.length === 0) return "empty";
  if (filled.length === required.length) return "complete";
  return "partial";
}

export function ReportForm({
  schemaName,
  initialEventType,
  existingReportId,
  user,
  onBack,
}: {
  schemaName: string;
  /** Design handoff A3: the suggested-next-report card opens the form pre-set to a specific event. */
  initialEventType?: string;
  /** Design handoff A3: resuming an in-progress draft card loads that report instead of creating a new one. */
  existingReportId?: string;
  user: UserView;
  onBack: () => void;
}) {
  const [schema, setSchema] = useState<Schema | null>(null);
  const [policy, setPolicy] = useState<Record<string, FieldPolicyState>>({});
  const [fieldEvents, setFieldEvents] = useState<Record<string, string[]>>({});
  const [prefill, setPrefill] = useState<Record<string, PrefillEntry>>({});
  const [robSeries, setRobSeries] = useState<RobSeries[]>([]);
  const [lastReportEventTime, setLastReportEventTime] = useState<string | undefined>(undefined);
  const [enumOptions, setEnumOptions] = useState<Record<string, string[]>>({});
  const [values, setValues] = useState<Record<string, FieldValue>>({});
  const [savedValues, setSavedValues] = useState<Record<string, FieldValue>>({});
  const [overridden, setOverridden] = useState<Set<string>>(new Set());
  const [activeSection, setActiveSection] = useState<string>("");
  const [error, setError] = useState<string | null>(null);

  // Architecture 9.5/design handoff A5: server-mode section soft locks
  // and live presence. Keyed by section key, sparse — a section with no
  // entry is unlocked. heldSectionRef (not state: the lock-acquire
  // effect's own cleanup reads it synchronously, and re-rendering on
  // every heartbeat would be pure waste) tracks which section, if any,
  // this browser tab currently holds, so cleanup/pagehide know what to
  // release without guessing from activeSection (which may have already
  // moved on by the time cleanup runs).
  const [sectionLocks, setSectionLocks] = useState<Record<string, SectionLock>>({});
  const [releasedNotice, setReleasedNotice] = useState<string | null>(null);
  const heldSectionRef = useRef<string | null>(null);

  const [reportId, setReportId] = useState<string | null>(null);
  const [reportState, setReportState] = useState<ReportState>("draft");
  // A freshly-created report is always versionNo 1; only a resumed report
  // (existingReportId) can load a higher one, when it's a correction draft
  // reopened after Start correction. Used to gate Delete draft below — see
  // that button's own comment.
  const [versionNo, setVersionNo] = useState(1);
  // Phase 5 open question 4's resolved default: fields an office Reviewer
  // flagged with a remark carry into the live form as amber-annotated
  // when an officer starts a correction on a remarked report. Keyed by
  // field name (unresolved remarks only) rather than kept as a raw list —
  // HighlightableField just needs a fast per-field lookup.
  const [remarkedFieldNames, setRemarkedFieldNames] = useState<Record<string, string>>({});
  const [findings, setFindings] = useState<Finding[]>([]);
  const [regulatoryReadiness, setRegulatoryReadiness] = useState<ProfileReadiness[]>([]);
  const [continuityImpact, setContinuityImpact] = useState<ContinuityImpact[]>([]);
  // The exact report snapshot the last check ran against — the submit
  // confirm dialog reads event/timestamp/voyage from this rather than
  // reconstructing a ReportView from local form state, so it always shows
  // what was actually persisted and checked, not the wizard's in-progress
  // (and possibly since-edited) values.
  const [checkedReport, setCheckedReport] = useState<ReportView | null>(null);
  const [busy, setBusy] = useState<"save" | "check" | "submit" | "delete" | "sensor" | "vms" | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [highlight, setHighlight] = useState<{ field: string; nonce: number } | null>(null);
  // "Check report" expands FormWizard's own footer panel with the full
  // review (HealthCheckPanel) instead of navigating to a separate screen,
  // and gates the
  // RULE_FIELD_REQUIRED filter on the live-validate effect below — the
  // red/amber error/warning findings a Check surfaces, not the
  // mandatory/GHG-relevant policyOutline border itself (that border is
  // static schema/policy metadata, always shown, see FieldRow's own doc
  // comment — a 2026-07-13 correction after a first pass wrongly gated
  // it the same way). Sticky for the rest of this draft's lifetime
  // (re-editing a field afterward doesn't re-hide its findings; a fresh
  // Check is what refreshes them). Resuming an already-checked draft
  // (reportState "ready") starts `checked` true so findings don't
  // disappear just from reopening the form.
  const [checked, setChecked] = useState(false);
  const [panelExpanded, setPanelExpanded] = useState(false);
  // Which findings (by findingKey) are currently acknowledged for this
  // version — derived from finding_acknowledged audit events (Phase 3),
  // not local component state, so it survives closing/reopening the
  // panel and is the same record office sees once synced.
  const [acknowledgedKeys, setAcknowledgedKeys] = useState<Set<string>>(new Set());
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);

  // Design handoff A6: "tap jumps to it" for a health-check issue — used
  // by both the footer's live-findings issue list and the post-check
  // HealthCheckPanel's own error/warning rows. Switching section and
  // setting the highlight target happen in the same handler, so React
  // batches them into one commit — SectionPanel already has the right
  // fields rendered by the time HighlightableField's effect runs, no
  // separate "wait for the section to switch" step needed. A fresh nonce
  // (not just the field name) is what lets re-clicking the same issue
  // replay the pulse.
  function handleIssueClick(issue: { field?: string; sectionLabel?: string }) {
    if (!issue.field || !schema) return;
    const target = schema.fields.find((f) => f.name === issue.field);
    if (target) setActiveSection(target.section);
    setPanelExpanded(false);
    setHighlight({ field: issue.field, nonce: Date.now() });
  }

  // initialEventType/existingReportId deliberately excluded from the
  // dependency array: this effect represents "start this ReportForm
  // instance" (create or resume, once), not "keep syncing to whatever the
  // props currently say" — the caller (App.tsx) mounts a fresh ReportForm
  // instance for every distinct open/resume action instead of expecting
  // this same instance to react to a changed target mid-render.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const config = await api.getSchema(schemaName);
        if (cancelled) return;
        setSchema(config.schema);
        setPolicy(config.fieldPolicy);
        setFieldEvents(config.fieldEvents ?? {});
        setPrefill(config.prefill);
        setRobSeries(config.robSeries ?? []);
        setLastReportEventTime(config.lastReportEventTime);
        const order = visibleSectionsInOrder(config.schema, config.fieldPolicy, config.fieldEvents ?? {});
        if (order.length > 0) setActiveSection(order[0]);

        // Best-effort: an enumRef with no generic resolver (e.g.
        // "offshore-modes") 404s and that field just falls back to its
        // existing plain-text rendering, same as before this existed.
        const enumRefs = [...new Set(config.schema.fields.map((f) => f.enumRef).filter((r): r is string => !!r))];
        void Promise.allSettled(enumRefs.map((ref) => api.getEnum(ref))).then((results) => {
          if (cancelled) return;
          const loaded: Record<string, string[]> = {};
          results.forEach((result, i) => {
            if (result.status === "fulfilled") loaded[enumRefs[i]] = result.value.values;
          });
          setEnumOptions(loaded);
        });

        if (existingReportId) {
          // Resuming an in-progress draft (design handoff A3's in-progress
          // cards) — load what's actually persisted rather than creating
          // a new report.
          const existing = await api.getReport(existingReportId);
          if (cancelled) return;
          const decoded = decodeFieldsPayload(config.schema, existing.fields);
          setValues(decoded);
          setSavedValues(decoded);
          setReportId(existing.reportId);
          setReportState(existing.state);
          setVersionNo(existing.versionNo);
          // "ready" only happens after a clean check (architecture 8.1) —
          // resuming one shouldn't hide the policy-outline markers a fresh
          // Check would just restore anyway.
          if (existing.state === "ready") {
            setChecked(true);
            try {
              const events = await api.listReportEvents(existing.reportId);
              if (cancelled) return;
              setAcknowledgedKeys(acknowledgedKeysFromEvents(events, existing.versionNo));
            } catch {
              // Best-effort — see the same pattern in handleCheck.
            }
          }

          // Remarks target whichever version they were sent against, but
          // the flag itself still applies once the officer starts a
          // correction (a fresh draft version with no "remarked" state of
          // its own) — so this reads unresolved remarks by reportId
          // (stable across versions), not by the current version number.
          try {
            const remarks = await api.listRemarks(existing.reportId);
            if (cancelled) return;
            const byField: Record<string, string> = {};
            for (const r of remarks) {
              if (!r.resolved) byField[r.fieldName] = r.body;
            }
            setRemarkedFieldNames(byField);
          } catch {
            // Best-effort: a remarks-load failure shouldn't block opening the form.
          }
          // A resumed report can already have sections held by other
          // officers (server mode, architecture 9.5) — a fresh report
          // (the branch below) never can, since nobody else could have
          // acquired a lock before it existed. Land on the first section
          // that isn't locked by someone else instead of always
          // order[0], so opening a report doesn't default into one the
          // viewer can't actually enter.
          try {
            const locks = await api.listLocks(existing.reportId);
            if (cancelled) return;
            if (locks.length > 0) {
              const lockMap: Record<string, SectionLock> = {};
              for (const lock of locks) lockMap[lock.section] = lock;
              setSectionLocks(lockMap);
              const free = order.find((key) => !lockMap[key] || lockMap[key].userId === user.id);
              if (free) setActiveSection(free);
            }
          } catch {
            // Best-effort: the section-lock acquire effect surfaces any real problem once it runs.
          }
          return;
        }

        const initial = initialValues(config.schema.fields, config.prefill);
        if (initialEventType && config.schema.fields.some((f) => f.name === "Event")) {
          initial["Event"] = initialEventType;
        }
        setValues(initial);
        // No api.createReport call here — see ensureReportCreated's own
        // doc comment on why creation is deferred until the officer
        // actually commits to this draft, not the moment the form opens.
      } catch (err) {
        if (!cancelled) setError(err instanceof ApiError ? err.message : "Could not load the schema.");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [schemaName]);

  // Report creation deferred until the officer actually commits to this
  // draft — Save draft or Check report, whichever happens first — rather
  // than the instant the form opens (2026-07-13 manual-test feedback:
  // "as soon as a user clicks on a report it just persists the form in
  // draft automatically... what if the user is simply trying to explore
  // the form"). Before this runs, everything lives in local component
  // state only (`values`) — nothing is visible to any other officer on
  // this vessel (design handoff A5/architecture 9.5's shared "in
  // progress" list only ever reads persisted reports), and navigating
  // away without saving leaves zero trace, matching the same "explore
  // freely" bar Delete draft exists for once a report does get created.
  // Sends the *current* values, not just whatever prefill resolved at
  // mount, since the officer may already have typed real edits before
  // ever clicking Save/Check — that first call already carries them, so
  // saveChanges() right after has nothing left to diff (savedValues is
  // set to the same values here).
  //
  // Safe to call unconditionally at the top of any action needing a
  // persisted report: returns the existing id with no new request once
  // one exists. Returns the id directly rather than relying on callers
  // to read the `reportId` state afterward — setReportId here doesn't
  // take effect in this same closure until a re-render, so a caller
  // needing the id immediately (handleCheck) would otherwise see a
  // stale null.
  async function ensureReportCreated(): Promise<string> {
    if (reportId) return reportId;
    if (!schema) throw new Error("Schema not loaded yet");
    const eventTypeValue = values["Event"];
    const created = await api.createReport({
      schemaName: schema.schemaName,
      eventType: typeof eventTypeValue === "string" ? eventTypeValue : "",
      eventTime: computeEventTime(values),
      fields: buildFieldsPayload(schema, values),
    });
    setReportId(created.reportId);
    setReportState(created.state);
    setSavedValues(values);
    return created.reportId;
  }

  // Persists every changed field, one PATCH per schema section
  // (architecture 9.5: "section saves are independent writes") — a
  // section another officer holds a lock on 409s without blocking the
  // rest of the batch, and every other section's changes still land.
  async function saveChanges(): Promise<ReportState> {
    if (!reportId || !schema) return reportState;
    const bySection = buildChangesBySection(schema, values, savedValues);
    const dirtySections = Object.keys(bySection);
    if (dirtySections.length === 0) return reportState;

    let latestState = reportState;
    const savedFieldNames = new Set<string>();
    const conflictedSections: string[] = [];
    for (const section of dirtySections) {
      try {
        const updated = await api.saveSection(reportId, section, bySection[section]);
        latestState = updated.state;
        for (const name of Object.keys(bySection[section])) savedFieldNames.add(name);
      } catch (err) {
        const conflict = lockConflictFromError(err);
        if (!conflict) throw err;
        setSectionLocks((prev) => ({ ...prev, [section]: conflict.lock }));
        conflictedSections.push(section);
      }
    }

    if (savedFieldNames.size > 0) {
      setSavedValues((prev) => {
        const next = { ...prev };
        for (const name of savedFieldNames) next[name] = values[name];
        return next;
      });
    }
    setReportState(latestState);

    if (conflictedSections.length > 0) {
      const labels = conflictedSections.map(sectionLabel).join(", ");
      throw new ApiError(409, `Could not save ${labels} — locked by someone else. Your other changes were saved.`);
    }
    return latestState;
  }

  // Architecture 9.4/design handoff A5: "per-field validation on blur...
  // plausibility checks run live across the form." A debounce (rather than
  // one call per keystroke) against a stateless preview endpoint that never
  // saves or transitions the report — reflects the officer's still-unsaved
  // edits, distinct from the persisted, audit-logged Check flow below.
  // Paused while a save/check/submit is in flight or the report is locked,
  // to avoid a stale debounced response landing after (and disagreeing
  // with) a real Check's findings; a request-sequence guard covers the
  // remaining case of two live-validate calls racing each other.
  const validateSeq = useRef(0);
  useEffect(() => {
    if (!reportId || !schema || busy !== null || isLocked(reportState)) return;
    const seq = ++validateSeq.current;
    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          const result = await api.validateReport(reportId, buildFieldsPayload(schema, values));
          // Excludes RULE_FIELD_REQUIRED until a real Check has run — see
          // that constant's own doc comment. handleCheck's own setFindings
          // call below keeps the full set once `checked` is true, so
          // mandatory/recommended findings appear immediately after a
          // Check without needing a second live-validate round-trip.
          if (seq === validateSeq.current) {
            setFindings(checked ? result.findings : result.findings.filter((f) => f.ruleId !== RULE_FIELD_REQUIRED));
          }
        } catch {
          // Best-effort: an explicit Check surfaces failures for real.
        }
      })();
    }, 500);
    return () => window.clearTimeout(timer);
  }, [values, reportId, schema, busy, reportState, checked]);

  // Live presence over SSE (architecture 9.5): reflects every section
  // lock on this report, held by anyone, including this tab's own —
  // the sections array below reads `self` off it to suppress the
  // lock icon for a section the viewer holds themselves.
  useEffect(() => {
    if (!reportId) return;
    const source = new EventSource(`/api/reports/${encodeURIComponent(reportId)}/locks/stream`);
    source.addEventListener("locked", (e) => {
      const lock = JSON.parse((e as MessageEvent).data) as SectionLock;
      setSectionLocks((prev) => ({ ...prev, [lock.section]: lock }));
    });
    source.addEventListener("unlocked", (e) => {
      const { section } = JSON.parse((e as MessageEvent).data) as { section: string };
      setSectionLocks((prev) => {
        if (!(section in prev)) return prev;
        const next = { ...prev };
        delete next[section];
        return next;
      });
      // Force-released, or a rare heartbeat/expiry race — either way
      // this tab no longer actually holds it, even though it still
      // thought it did a moment ago.
      if (heldSectionRef.current === section) {
        heldSectionRef.current = null;
        setReleasedNotice("Your access to this section was released — reopen it to keep editing.");
      }
    });
    return () => source.close();
  }, [reportId]);

  // Claims the active section for as long as it's open (architecture
  // 9.5), renewing well under the server's 5-minute inactivity TTL, and
  // releases it on section change/unmount. A conflict here (someone else
  // already holds it) doesn't throw — it just records the real holder,
  // same as saveChanges' own backstop, so a stale client-side selection
  // never silently pretends to have exclusive access it doesn't.
  useEffect(() => {
    if (!reportId || !activeSection || isLocked(reportState)) return;
    let cancelled = false;

    async function claim() {
      try {
        await api.acquireLock(reportId as string, activeSection);
        if (cancelled) return;
        heldSectionRef.current = activeSection;
      } catch (err) {
        const conflict = lockConflictFromError(err);
        if (!conflict || cancelled) return;
        setSectionLocks((prev) => ({ ...prev, [activeSection]: conflict.lock }));
      }
    }
    void claim();
    const heartbeat = window.setInterval(() => void claim(), LOCK_HEARTBEAT_INTERVAL_MS);

    return () => {
      cancelled = true;
      window.clearInterval(heartbeat);
      if (heldSectionRef.current === activeSection) {
        heldSectionRef.current = null;
        void api.releaseLock(reportId as string, activeSection);
      }
    };
  }, [reportId, activeSection, reportState]);

  // Best-effort release on tab close/navigate-away — React's own effect
  // cleanup above never runs for that case. keepalive lets the request
  // outlive the page instead of being aborted mid-flight.
  useEffect(() => {
    function release() {
      if (reportId && heldSectionRef.current) {
        void api.releaseLock(reportId, heldSectionRef.current, true);
      }
    }
    window.addEventListener("pagehide", release);
    return () => window.removeEventListener("pagehide", release);
  }, [reportId]);

  // Design handoff A5: "Master-only force release." Optimistically
  // clears the local lock entry too — the SSE "unlocked" event arrives
  // moments later and would do the same, but there's no reason to wait
  // on it for the officer who just clicked the button.
  async function handleForceRelease(section: string) {
    if (!reportId) return;
    try {
      await api.forceReleaseLock(reportId, section);
      setSectionLocks((prev) => {
        if (!(section in prev)) return prev;
        const next = { ...prev };
        delete next[section];
        return next;
      });
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Could not release that section's lock.");
    }
  }

  async function handleSave() {
    setBusy("save");
    setActionError(null);
    try {
      await ensureReportCreated();
      await saveChanges();
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Could not save the draft.");
    } finally {
      setBusy(null);
    }
  }

  async function handleCheck() {
    setBusy("check");
    setActionError(null);
    try {
      const id = await ensureReportCreated();
      await saveChanges();
      const result = await api.checkReport(id);
      setFindings(result.findings);
      setRegulatoryReadiness(result.regulatoryReadiness);
      setContinuityImpact(result.continuityImpact);
      setCheckedReport(result.report);
      setReportState(result.report.state);
      setChecked(true);
      setPanelExpanded(true);
      // Best-effort: an events-load failure shouldn't block the check
      // itself from completing — acknowledgedKeys just stays whatever it
      // was (empty, on a first check).
      try {
        const events = await api.listReportEvents(id);
        setAcknowledgedKeys(acknowledgedKeysFromEvents(events, result.report.versionNo));
      } catch {
        // Best-effort, see above.
      }
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Could not run the health check.");
    } finally {
      setBusy(null);
    }
  }

  async function handleSubmit() {
    if (!reportId) return;
    setBusy("submit");
    setActionError(null);
    try {
      const updated = await api.submitReport(reportId);
      setReportState(updated.state);
      // The form's own locked/read-only banner takes over from here —
      // there's no separate submitted-state view to switch to anymore.
      setPanelExpanded(false);
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Could not submit the report.");
    } finally {
      setBusy(null);
    }
  }

  // 2026-07-13 manual-test feedback: "there is no way to delete a form in
  // draft status." Draft/ready only (vessel/httpapi's loadEditableReport
  // enforces this server-side too); once submitted a report is locked and
  // this action isn't offered at all (see the actions block below). Never
  // reachable before a report has actually been created (see
  // ensureReportCreated's own doc comment on why creation is now deferred
  // until the first Save/Check) — there's nothing persisted yet to delete
  // in that case, onBack already discards the in-memory draft for free.
  async function handleDeleteDraft() {
    if (!reportId) return;
    setBusy("delete");
    setActionError(null);
    try {
      await api.deleteReport(reportId);
      onBack();
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Could not delete the draft.");
      setBusy(null);
    }
  }

  // Optimistic: flips the key locally before the request resolves (the
  // panel is meant to feel instant, same as every other toggle in this
  // app), then reverts on failure rather than leaving a lie on screen.
  async function handleToggleAcknowledge(finding: Finding, nextAcknowledged: boolean) {
    if (!reportId) return;
    const key = findingKey(finding);
    setAcknowledgedKeys((prev) => {
      const next = new Set(prev);
      if (nextAcknowledged) next.add(key);
      else next.delete(key);
      return next;
    });
    try {
      await api.acknowledgeFinding(reportId, {
        ruleId: finding.ruleId,
        field: finding.field,
        message: finding.message,
        acknowledged: nextAcknowledged,
      });
    } catch (err) {
      setAcknowledgedKeys((prev) => {
        const next = new Set(prev);
        if (nextAcknowledged) next.delete(key);
        else next.add(key);
        return next;
      });
      setActionError(err instanceof ApiError ? err.message : "Could not save that acknowledgement.");
    }
  }

  const fieldsBySection = useMemo(() => {
    const map = new Map<string, SchemaField[]>();
    if (!schema) return map;
    for (const f of schema.fields) {
      const list = map.get(f.section) ?? [];
      list.push(f);
      map.set(f.section, list);
    }
    return map;
  }, [schema]);

  // Per-section unresolved-remark counts, same shape as
  // ReportDetailScreen's own remarkCountBySection — a remark only ever
  // renders inline on its field (HighlightableField's amber annotation),
  // so a remark on a field outside whichever section the wizard happens
  // to default to (order[0]) was otherwise invisible for the rest of the
  // correction: nothing in the nav rail hinted it existed. This feeds
  // FormWizard's own nav-badge/footer-readout rendering (its `remarks`
  // field), not a new UI surface.
  const remarkCountBySection = useMemo(() => {
    const counts: Record<string, number> = {};
    if (!schema) return counts;
    const sectionByField = new Map(schema.fields.map((f) => [f.name, f.section]));
    for (const fieldName of Object.keys(remarkedFieldNames)) {
      const section = sectionByField.get(fieldName);
      if (section) counts[section] = (counts[section] ?? 0) + 1;
    }
    return counts;
  }, [schema, remarkedFieldNames]);

  function handleChange(name: string, value: FieldValue, wasComputed: boolean) {
    setValues((v) => ({ ...v, [name]: value }));
    if (wasComputed) {
      setOverridden((o) => new Set(o).add(name));
    }
  }

  // 18.07.26 manual-test items 4/9: "once the report is open, the user
  // makes the entry of date and time of the event and in the same page
  // should be a button ... to fetch sensor data ... auto populate
  // relevant fields." Merges fetched values into `values` via the same
  // path any manual edit takes (setValues, not a separate "sensor value"
  // concept) — the officer can still edit or reject any of them before
  // Save draft/Check/Submit, and the existing derived-field effect
  // (HFO_ROB/Time_Since_Previous_Report/Wind_Dir/...) picks the change up
  // automatically since it's just another `values` update, no separate
  // wiring needed for the "computed fields will also be updated" half.
  async function handleFetchSensorData() {
    if (!schema) return;
    setBusy("sensor");
    setActionError(null);
    try {
      const result = await api.fetchSensorData(schema.schemaName, computeEventTime(values));
      setValues((v) => {
        const next = { ...v };
        for (const [name, value] of Object.entries(result.fields)) {
          // Fields are string | number | boolean on the wire; keep a
          // real boolean as a boolean (same idiom as
          // reportPersistence.ts's fieldsToFormValues) so FieldRow's
          // `value === true` checkbox check and pkg/validation's
          // field.type check both still see a real boolean, not the
          // string "true"/"false".
          next[name] = typeof value === "boolean" ? value : String(value);
        }
        return next;
      });
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Could not fetch sensor data.");
    } finally {
      setBusy(null);
    }
  }

  // Sensor+VMS stub expansion design doc: the VMS half of "fetch
  // external data into the report," parallel to handleFetchSensorData
  // but hitting the VMS contract instead. Same merge-into-`values`
  // pattern, same "still editable, not locked" guarantee, same
  // automatic pickup by the existing derived-field effect.
  async function handleFetchVMSData() {
    if (!schema) return;
    setBusy("vms");
    setActionError(null);
    try {
      const result = await api.fetchVMSData(schema.schemaName, computeEventTime(values));
      setValues((v) => {
        const next = { ...v };
        for (const [name, value] of Object.entries(result.fields)) {
          // Fields are string | number | boolean on the wire; keep a
          // real boolean as a boolean (same idiom as
          // reportPersistence.ts's fieldsToFormValues) so FieldRow's
          // `value === true` checkbox check and pkg/validation's
          // field.type check both still see a real boolean, not the
          // string "true"/"false".
          next[name] = typeof value === "boolean" ? value : String(value);
        }
        return next;
      });
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Could not fetch voyage data.");
    } finally {
      setBusy(null);
    }
  }

  // Restoring a computed field both writes the calculated value back and
  // clears the override flag — the whole point of always showing the
  // calculate button is that this is reversible, unlike a plain edit.
  function handleRestoreComputed(name: string, computedValue: FieldValue) {
    setValues((v) => ({ ...v, [name]: computedValue }));
    setOverridden((o) => {
      if (!o.has(name)) return o;
      const next = new Set(o);
      next.delete(name);
      return next;
    });
  }

  // Wind_Dir/Wind_Force_Bft/Sea_state_Dir/Swell_Dir/Current_Dir are pure
  // functions of one sibling field already on the form (see
  // derivedFields.ts). HFO_ROB (and any other ROB series
  // vessel/httpapi/schemas.go's RobSeries names) and
  // Time_Since_Previous_Report are the same kind of live recompute, just
  // anchored to a base from the last submitted report
  // (`prefill`/`lastReportEventTime`, from the schema config response)
  // instead of a sibling field's raw value — see derivedFields.ts's own
  // doc comment on why these used to be static demo constants and what
  // that broke (18.07.26 manual-test items 3/6/8/10). All three classes
  // auto-apply on every recompute unless the user has overridden the
  // field, restorable the same way as any other computed field.
  const robBases = useMemo(() => {
    const bases: Record<string, number> = {};
    for (const series of robSeries) {
      const v = prefill[series.robField]?.value;
      if (typeof v === "number") bases[series.robField] = v;
    }
    return bases;
  }, [robSeries, prefill]);

  const derived = useMemo(() => {
    const combined = { ...computeDerivedValues(values), ...computeRobDerivedValues(values, robSeries, robBases) };
    const tsp = computeTimeSincePreviousReport(computeEventTime(values), lastReportEventTime);
    if (tsp) combined["Time_Since_Previous_Report"] = tsp;
    return combined;
  }, [values, robSeries, robBases, lastReportEventTime]);
  const liveComputedFields = useMemo(
    () => [...Object.keys(DERIVED_FIELDS), ...robSeries.map((s) => s.robField), "Time_Since_Previous_Report"],
    [robSeries],
  );
  useEffect(() => {
    setValues((v) => {
      let changed = false;
      const next = { ...v };
      for (const name of liveComputedFields) {
        if (overridden.has(name)) continue;
        const result = derived[name];
        if (result && next[name] !== result.value) {
          next[name] = result.value;
          changed = true;
        }
      }
      return changed ? next : v;
    });
  }, [derived, overridden, liveComputedFields]);

  const effectivePrefill = useMemo(() => {
    const merged: Record<string, PrefillEntry> = { ...prefill };
    for (const name of liveComputedFields) {
      const result = derived[name];
      if (!result && !(name in merged)) continue;
      merged[name] = { class: "computed", value: result?.value, formula: result?.formula };
    }
    return merged;
  }, [prefill, derived, liveComputedFields]);

  if (error) {
    return (
      <div style={{ padding: 40 }}>
        <div className="md-body-large" style={{ color: "var(--color-error)" }}>{error}</div>
        <Button variant="text" onClick={onBack} style={{ marginTop: 16 }}>Back</Button>
      </div>
    );
  }
  if (!schema) {
    return (
      <div style={{ padding: 40 }}>
        <span className="md-body-large" style={{ color: "var(--color-on-surface-variant)" }}>Loading…</span>
      </div>
    );
  }

  const { issues, counts } = findingsToWizardData(findings, schema);
  // The voyage event this report was created under — what per-event field
  // policy resolves against. Read from the Event field's own value rather
  // than kept in separate state because it is fixed for the report's whole
  // life (FieldRow's LOCKED_FIELDS keeps the control read-only), so there is
  // no second source of truth to drift from.
  const reportEventType = typeof values["Event"] === "string" ? (values["Event"] as string) : undefined;
  const order = visibleSectionsInOrder(schema, policy, fieldEvents, reportEventType);
  const sections = order.map((key) => {
    const fields = fieldsBySection.get(key) ?? [];
    const c = counts.get(key);
    const lock = sectionLocks[key];
    const lockedByOther = Boolean(lock && lock.userId !== user.id);
    return {
      key,
      label: sectionLabel(key),
      fillStatus: sectionFillStatus(fields, policy, values, fieldEvents, reportEventType),
      errors: c?.errors ?? 0,
      warnings: c?.warnings ?? 0,
      remarks: remarkCountBySection[key] ?? 0,
      locked: lockedByOther,
      lock: lock
        ? { holderLabel: `${lock.username} (${ROLE_LABELS[lock.role]})`, since: lock.acquiredAt, self: lock.userId === user.id }
        : undefined,
      onForceRelease: user.role === "master" ? handleForceRelease : undefined,
    };
  });

  const locked = isLocked(reportState);
  // No longer requires reportId: Save draft/Check report are exactly the
  // two actions that create it in the first place now
  // (ensureReportCreated), so gating them on its prior existence would
  // make them unclickable. Submit/Delete draft both only ever render
  // once reportId is already set (`checked`/`!locked && reportId`
  // respectively), so relaxing this here doesn't loosen anything for them.
  // Delete draft additionally requires versionNo === 1 (2026-07-14
  // manual-test feedback): a correction draft (versionNo > 1) reopens
  // this same form in state "draft" by design (architecture 8.1/8.2), but
  // it has a prior submitted version and must never be deletable — the
  // server enforces this too (handleDeleteReport), this just keeps the
  // button from being offered in the first place.
  const busyDisabled = busy !== null;
  const hasErrors = findings.some((f) => f.severity === "error");
  const voyageNumber = checkedReport && typeof checkedReport.fields["Voyage_Number"] === "string" ? (checkedReport.fields["Voyage_Number"] as string) : undefined;

  return (
    <div style={{ height: "100%" }}>
      <FormWizard
        breadcrumbs={[{ label: "Reports" }, { label: schemaDisplayName(schema.schemaName) }]}
        title={schemaDisplayName(schema.schemaName)}
        status={<ReportStatusBadge status={reportState} />}
        sections={sections}
        activeKey={activeSection}
        onSelectSection={setActiveSection}
        issues={issues}
        onIssueClick={handleIssueClick}
        expanded={panelExpanded}
        onExpandedChange={setPanelExpanded}
        panel={
          checked ? (
            <HealthCheckPanel
              fields={schema.fields}
              findings={findings}
              regulatoryReadiness={regulatoryReadiness}
              continuityImpact={continuityImpact}
              onJumpToField={(field) => handleIssueClick({ field })}
              acknowledgedKeys={acknowledgedKeys}
              onToggleAcknowledge={(finding, next) => void handleToggleAcknowledge(finding, next)}
            />
          ) : undefined
        }
        actions={
          <>
            {actionError ? (
              <span className="md-body-small" style={{ color: "var(--color-error)" }}>{actionError}</span>
            ) : null}
            <Button variant="text" onClick={onBack}>Back</Button>
            {!locked && reportId && versionNo === 1 ? (
              <Button variant="text" disabled={busyDisabled} style={{ color: "var(--color-error)" }} onClick={() => setDeleteConfirmOpen(true)}>
                {busy === "delete" ? "Deleting…" : "Delete draft"}
              </Button>
            ) : null}
            {!locked ? (
              <>
                <Button
                  variant="text"
                  icon="satellite_alt"
                  disabled={busyDisabled || !hasEventTimeEntered(values)}
                  onClick={() => void handleFetchSensorData()}
                >
                  {busy === "sensor" ? "Fetching…" : "Fetch sensor data"}
                </Button>
                <Button
                  variant="text"
                  icon="directions_boat"
                  disabled={busyDisabled || !hasEventTimeEntered(values)}
                  onClick={() => void handleFetchVMSData()}
                >
                  {busy === "vms" ? "Fetching…" : "Fetch voyage data"}
                </Button>
                <Button variant="outlined" disabled={busyDisabled} onClick={() => void handleSave()}>
                  {busy === "save" ? "Saving…" : "Save draft"}
                </Button>
                <Button variant="filled" disabled={busyDisabled} onClick={() => void handleCheck()}>
                  {busy === "check" ? "Checking…" : "Check report"}
                </Button>
                {checked ? (
                  <Button
                    variant="filled"
                    disabled={hasErrors || busyDisabled || !user.canSubmit}
                    onClick={() => setConfirmOpen(true)}
                  >
                    {!user.canSubmit ? "Ready for Master to submit" : busy === "submit" ? "Submitting…" : "Submit"}
                  </Button>
                ) : null}
              </>
            ) : null}
          </>
        }
      >
        {locked ? (
          <div style={{ marginBottom: "var(--space-5)" }}>
            <AlertBanner
              level="caution"
              title={`This report is ${STATE_LABELS[reportState].toLowerCase()}`}
              message="It is locked and read-only here. Start a correction from this report's page (Reports → open the report → Start correction) to edit it as a new version."
            />
          </div>
        ) : null}
        {releasedNotice ? (
          <div style={{ marginBottom: "var(--space-5)" }}>
            <AlertBanner level="caution" title="Section access released" message={releasedNotice} onDismiss={() => setReleasedNotice(null)} />
          </div>
        ) : null}
        <SectionPanel
          sectionKey={activeSection}
          fields={fieldsBySection.get(activeSection) ?? []}
          policy={policy}
          events={fieldEvents}
          eventType={reportEventType}
          values={values}
          prefill={effectivePrefill}
          overridden={overridden}
          enumOptions={enumOptions}
          onChange={handleChange}
          onRestoreComputed={handleRestoreComputed}
          highlightField={highlight?.field ?? null}
          highlightNonce={highlight?.nonce ?? null}
          remarkedFieldNames={remarkedFieldNames}
          findings={findings}
          reportId={reportId}
          locked={locked}
        />
      </FormWizard>
      <Dialog
        open={confirmOpen}
        title="Submit this report?"
        onClose={() => setConfirmOpen(false)}
        actions={[
          { label: "Cancel", onClick: () => setConfirmOpen(false) },
          { label: "Submit", onClick: () => { setConfirmOpen(false); void handleSubmit(); } },
        ]}
      >
        {checkedReport ? (
          <>
            {checkedReport.eventType} · {formatUtc(checkedReport.eventTime)}
            {voyageNumber ? ` · Voyage ${voyageNumber}` : ""}
          </>
        ) : null}
      </Dialog>
      <Dialog
        open={deleteConfirmOpen}
        title="Delete this draft?"
        onClose={() => setDeleteConfirmOpen(false)}
        actions={[
          { label: "Cancel", onClick: () => setDeleteConfirmOpen(false) },
          { label: "Delete", onClick: () => { setDeleteConfirmOpen(false); void handleDeleteDraft(); } },
        ]}
      >
        This permanently removes the draft and everything entered on it — including any attachments. This cannot be undone.
      </Dialog>
    </div>
  );
}
