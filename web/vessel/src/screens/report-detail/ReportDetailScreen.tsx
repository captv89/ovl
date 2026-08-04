// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useMemo, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Tabs } from "../../design/components/navigation/Tabs.jsx";
import { Textarea } from "../../design/components/forms/Textarea.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Select } from "../../design/components/forms/Select.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Dialog } from "../../design/components/feedback/Dialog.jsx";
import { ReportStatusBadge } from "../../design/components/maritime/ReportStatusBadge.jsx";
import { PageContainer } from "../PageContainer";
import {
  api,
  ApiError,
  MAX_CHAT_BODY_BYTES,
  type ChatMessageView,
  type DomainEventType,
  type InvalidationNoticeView,
  type RemarkView,
  type ReportEvent,
  type ReportView,
  type Schema,
} from "../../api/client";
import { sectionLabel, sectionsInOrder } from "../report-form/sections";
import { groupFields } from "../report-form/fieldLayout";
import { AttachmentsSection } from "../report-form/AttachmentsSection";
import { HighlightableField } from "../report-form/HighlightableField";
import { formatUtc } from "../../format";
import { markReportChatRead } from "./chatReadMarker";

const EVENT_LABELS: Record<DomainEventType, string> = {
  created: "Created",
  section_saved: "Section saved",
  health_check_result: "Health check run",
  submitted: "Submitted",
  synced: "Synced",
  remarked: "Remarked",
  correction_started: "Correction started",
  resubmitted: "Resubmitted",
  invalidated: "Invalidated",
  restore_applied: "Restore applied",
  finding_acknowledged: "Warning acknowledged",
};

// Design handoff A7: "Report detail (read-only)" for submitted-and-later
// reports. Three tabs (Phase 5 vessel-UI-rework restructure, down from an
// earlier five): Report, Audit (chronological events merged with the
// former standalone History version-diff view — a resubmit row expands
// inline to show that version's field diff), and Chat. The standalone
// Remarks tab is dropped — office remark creation now auto-posts a
// linking chat message (office/httpapi/remarks.go) so that conversation
// happens in Chat instead; the underlying Remark domain type/
// MarkRemarked lifecycle event/resubmit flow are unchanged.
export function ReportDetailScreen({
  schemaName,
  reportId,
  onBack,
  onStartCorrection,
}: {
  schemaName: string;
  reportId: string;
  onBack: () => void;
  /** Design handoff A7's "Start correction" — the caller navigates to the new draft version (architecture 8.1). */
  onStartCorrection: (schemaName: string, reportId: string) => void;
}) {
  const [schema, setSchema] = useState<Schema | null>(null);
  const [report, setReport] = useState<ReportView | null>(null);
  const [events, setEvents] = useState<ReportEvent[]>([]);
  const [versions, setVersions] = useState<ReportView[]>([]);
  const [chat, setChat] = useState<ChatMessageView[]>([]);
  const [invalidationNotices, setInvalidationNotices] = useState<InvalidationNoticeView[]>([]);
  const [tab, setTab] = useState("Report");
  const [error, setError] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [starting, setStarting] = useState(false);
  const [correctionError, setCorrectionError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [config, r, e, v, c, notices] = await Promise.all([
          api.getSchema(schemaName),
          api.getReport(reportId),
          api.listReportEvents(reportId),
          api.listReportVersions(reportId),
          api.getChat(reportId),
          api.listInvalidationNotices(reportId),
        ]);
        if (cancelled) return;
        setSchema(config.schema);
        setReport(r);
        setEvents(e);
        setVersions(v);
        setChat(c);
        setInvalidationNotices(notices);
      } catch (err) {
        if (!cancelled) setError(err instanceof ApiError ? err.message : "Could not load this report.");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [schemaName, reportId]);

  // Opening the Chat tab marks this report's thread as read (A8's
  // localStorage-only read marker — see chatReadMarker.ts's own doc
  // comment on why no server-side read cursor is needed for Phase 5).
  useEffect(() => {
    if (tab === "Chat") markReportChatRead(reportId);
  }, [tab, reportId]);

  if (error) {
    return (
      <div style={{ padding: 40 }}>
        <AlertBanner level="warning" title="Couldn't load report" message={error} />
        <Button variant="text" onClick={onBack} style={{ marginTop: 16 }}>Back</Button>
      </div>
    );
  }
  if (!schema || !report) {
    return (
      <div style={{ padding: 40 }}>
        <span className="md-body-large" style={{ color: "var(--color-on-surface-variant)" }}>Loading…</span>
      </div>
    );
  }

  // NewCorrection's own precondition (pkg/domain/lifecycle.go): only a
  // report that has left draft/ready can be corrected. This screen is only
  // ever reached for those states (Home routes draft/ready into ReportForm
  // instead), but checking here too keeps the button honest if that ever
  // changes.
  const canCorrect = report.state !== "draft" && report.state !== "ready";

  async function handleStartCorrection() {
    setStarting(true);
    setCorrectionError(null);
    try {
      await api.startCorrection(reportId);
      setConfirmOpen(false);
      onStartCorrection(schemaName, reportId);
    } catch (err) {
      setCorrectionError(err instanceof ApiError ? err.message : "Could not start a correction.");
      setStarting(false);
    }
  }

  return (
    <PageContainer
      title={report.eventType || "(no event type)"}
      leading={<Button variant="text" icon="arrow_back" onClick={onBack}>Back</Button>}
      actions={
        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)" }}>
          <ReportStatusBadge status={report.state} resubmitted={report.versionNo > 1} />
          {canCorrect ? (
            <Button variant="outlined" onClick={() => setConfirmOpen(true)}>
              Start correction
            </Button>
          ) : null}
        </div>
      }
    >
      {correctionError ? <AlertBanner level="warning" title="Couldn't start correction" message={correctionError} /> : null}

      {report.state === "invalidated" && invalidationNotices.length > 0 ? (
        <AlertBanner
          level="warning"
          title="This report was invalidated by a later chain change"
          message={`Broken rule(s): ${invalidationNotices[invalidationNotices.length - 1].brokenRules.join(", ")}`}
        />
      ) : null}

      <Tabs items={["Report", "Audit", "Chat"]} selected={tab} onSelect={setTab} />

      {tab === "Report" ? (
        <ReportValuesTab schema={schema} report={report} reportId={reportId} />
      ) : tab === "Audit" ? (
        <AuditTab events={events} versions={versions} />
      ) : (
        <ChatTab reportId={reportId} messages={chat} onSent={(m) => setChat((prev) => [...prev, m])} />
      )}

      <Dialog
        open={confirmOpen}
        title="Start a correction?"
        onClose={() => !starting && setConfirmOpen(false)}
        actions={[
          { label: "Cancel", onClick: () => setConfirmOpen(false) },
          { label: starting ? "Starting…" : "Start correction", onClick: () => void handleStartCorrection() },
        ]}
      >
        This creates a new draft version, seeded with a copy of the current values. The original version stays in history and this one goes through Check and Submit again.
      </Dialog>
    </PageContainer>
  );
}

// Design handoff A7 note 6: "the report should look similar to the edit
// report style just with not-edit fields" (2026-07-14 p2 manual-test
// feedback) — this used to be a single flat column stacking every
// section's fields one below another, nothing like the edit form's own
// section-by-section navigation. Rebuilt to the same section-rail +
// one-section-at-a-time shell the wireframe draws (A7's own "same
// section-wise nav as the form" note 1), with a collapsible "Remarks &
// findings" box (note 5) above the field grid. Deliberately *not* a
// reuse of FormWizard itself: that component owns a header (breadcrumbs/
// title/status) and a footer (error/warning counts), both already
// rendered by this screen's own PageContainer/header above the Tabs row
// — bundling a second copy of either would duplicate chrome, exactly
// what FormWizard's own doc comment says to avoid ("copy this file as a
// starting point instead" when the layout differs). Fields render
// boxless (wireframe note 6: "All fields are read-only (boxless) here"),
// wrapped in the same HighlightableField the edit form uses so a
// remark-flagged field gets the identical amber accent + inline reviewer
// comment in both places, not a bespoke read-view-only treatment.
function ReportValuesTab({ schema, report, reportId }: { schema: Schema; report: ReportView; reportId: string }) {
  const order = useMemo(() => sectionsInOrder(schema), [schema]);
  const [activeSection, setActiveSection] = useState(order[0] ?? "");
  const [remarks, setRemarks] = useState<RemarkView[]>([]);
  const [remarksExpanded, setRemarksExpanded] = useState(true);
  const [highlight, setHighlight] = useState<{ field: string; nonce: number } | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const list = await api.listRemarks(reportId);
        if (!cancelled) setRemarks(list);
      } catch {
        // Best-effort: the read view still works without remark context,
        // same tolerance ReportForm's own remarks fetch has.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [reportId]);

  const sectionOfField = useMemo(() => {
    const map: Record<string, string> = {};
    for (const f of schema.fields) map[f.name] = f.section;
    return map;
  }, [schema]);

  const unresolvedRemarks = useMemo(() => remarks.filter((rm) => !rm.resolved), [remarks]);
  const remarkedFieldNames = useMemo(() => {
    const map: Record<string, string> = {};
    for (const rm of unresolvedRemarks) map[rm.fieldName] = rm.body;
    return map;
  }, [unresolvedRemarks]);
  const remarkCountBySection = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const rm of unresolvedRemarks) {
      const section = sectionOfField[rm.fieldName];
      if (section) counts[section] = (counts[section] ?? 0) + 1;
    }
    return counts;
  }, [unresolvedRemarks, sectionOfField]);

  function jumpToField(fieldName: string) {
    const section = sectionOfField[fieldName];
    if (section) setActiveSection(section);
    setHighlight({ field: fieldName, nonce: Date.now() });
  }

  const activeFields = schema.fields.filter((f) => f.section === activeSection);
  const filledActiveFields = activeFields.filter((f) => report.fields[f.name] !== undefined && report.fields[f.name] !== null && report.fields[f.name] !== "");
  const activeFieldGroups = groupFields(filledActiveFields);

  return (
    <div style={{ display: "flex", border: "1px solid var(--color-outline-variant)", borderRadius: "var(--shape-medium)", minHeight: 480 }}>
      <nav style={{ width: 240, flexShrink: 0, borderRight: "1px solid var(--color-outline-variant)", overflowY: "auto", padding: "var(--space-2) 0" }}>
        {order.map((key) => {
          const active = key === activeSection;
          const count = remarkCountBySection[key];
          return (
            <button
              key={key}
              onClick={() => setActiveSection(key)}
              className="md-body-large"
              style={{
                display: "flex", alignItems: "center", justifyContent: "space-between", gap: "var(--space-2)",
                width: "100%", padding: "var(--space-3) var(--space-4)", border: "none", font: "inherit", textAlign: "left", cursor: "pointer",
                borderLeft: active ? "3px solid var(--color-primary)" : "3px solid transparent",
                background: active ? "var(--color-surface-container-high)" : "transparent",
                color: "var(--color-on-surface)",
              }}
            >
              <span style={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{sectionLabel(key)}</span>
              {count ? (
                <span className="md-label-medium" style={{ display: "flex", alignItems: "center", gap: 3, color: "var(--color-status-caution)", flexShrink: 0 }}>
                  <span className="material-symbols-rounded" style={{ fontSize: 14 }}>chat</span>
                  {count}
                </span>
              ) : null}
            </button>
          );
        })}
      </nav>

      <div style={{ flex: 1, minWidth: 0, padding: "var(--space-5) var(--space-6)", overflowY: "auto" }}>
        {unresolvedRemarks.length > 0 ? (
          <div style={{ border: "1px solid var(--color-outline-variant)", borderLeft: "3px solid var(--color-status-caution)", borderRadius: "var(--shape-medium)", marginBottom: "var(--space-5)" }}>
            <button
              onClick={() => setRemarksExpanded((v) => !v)}
              style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", width: "100%", padding: "var(--space-3) var(--space-4)", border: "none", background: "transparent", cursor: "pointer", textAlign: "left", font: "inherit" }}
            >
              <span className="material-symbols-rounded" style={{ fontSize: 18, color: "var(--color-on-surface-variant)" }}>
                {remarksExpanded ? "expand_more" : "chevron_right"}
              </span>
              <span className="md-title-small" style={{ color: "var(--color-on-surface)" }}>Remarks &amp; findings</span>
              <span className="md-label-medium" style={{ color: "var(--color-status-caution)" }}>{unresolvedRemarks.length} remark{unresolvedRemarks.length === 1 ? "" : "s"}</span>
            </button>
            {remarksExpanded ? (
              <div style={{ borderTop: "1px solid var(--color-outline-variant)" }}>
                {unresolvedRemarks.map((rm) => {
                  const field = schema.fields.find((f) => f.name === rm.fieldName);
                  return (
                    <button
                      key={rm.id}
                      onClick={() => jumpToField(rm.fieldName)}
                      style={{ display: "block", width: "100%", boxSizing: "border-box", padding: "var(--space-3) var(--space-4)", border: "none", borderBottom: "1px solid var(--color-outline-variant)", background: "transparent", cursor: "pointer", textAlign: "left", font: "inherit" }}
                    >
                      <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", marginBottom: 2 }}>
                        {sectionLabel(field?.section ?? "")} › {field?.label ?? rm.fieldName}
                      </div>
                      <div className="md-body-medium" style={{ color: "var(--color-on-surface)" }}>
                        {rm.body} <span style={{ color: "var(--color-primary)", fontWeight: 600 }}>Go to field ›</span>
                      </div>
                    </button>
                  );
                })}
              </div>
            ) : null}
          </div>
        ) : null}

        <div className="md-title-medium" style={{ color: "var(--color-on-surface)", marginBottom: "var(--space-4)" }}>{sectionLabel(activeSection)}</div>

        {activeSection === "attachments" ? (
          // The Attachments pseudo-section (Phase 6, Bunker/EDN forms) has
          // no schema fields at all — same special-case this screen's
          // report-form counterpart (SectionPanel.tsx) makes, reusing the
          // same AttachmentsSection component locked (this screen only
          // ever shows submitted-and-later reports, always read-only).
          <AttachmentsSection reportId={reportId} locked />
        ) : filledActiveFields.length === 0 ? (
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>No values recorded for this section.</div>
        ) : (
          // 18.07.26 manual-test item 11: field grouping (buildRenderGroups'
          // own name-token clustering — ME_Consumption_HFO/LFO/... under
          // "ME", Wind_Dir/Wind_Force_* under "Wind") existed in edit mode's
          // SectionPanel.tsx but not here, so a section with 40+ fields read
          // as one undifferentiated grid. Reuses groupFields directly
          // (not the full buildRenderGroups/collapseCompounds — this view
          // has no editable compound-entry control to collapse the
          // lat/long triple into, so each part just renders as its own
          // plain field within its group instead of inventing a new
          // read-only compound display).
          activeFieldGroups.map((g, i) => (
            <div key={i} style={{ marginBottom: "var(--space-5)" }}>
              {g.label ? (
                <div className="md-label-large" style={{ color: "var(--color-on-surface-variant)", marginBottom: "var(--space-2)", textTransform: "uppercase", letterSpacing: "0.02em" }}>
                  {g.label}
                </div>
              ) : null}
              <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))", gap: "var(--space-4)" }}>
                {g.fields.map((f) => (
                  <HighlightableField key={f.name} fieldNames={[f.name]} highlightField={highlight?.field ?? null} highlightNonce={highlight?.nonce ?? null} remarkedFieldNames={remarkedFieldNames}>
                    <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>{f.label}</div>
                    <div className="md-body-large" style={{ color: "var(--color-on-surface)" }}>
                      {String(report.fields[f.name])}
                      {f.unit ? ` ${f.unit}` : ""}
                    </div>
                  </HighlightableField>
                ))}
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

interface FieldDiff {
  field: string;
  before: unknown;
  after: unknown;
}

function diffFields(before: Record<string, unknown>, after: Record<string, unknown>): FieldDiff[] {
  const fields = new Set([...Object.keys(before), ...Object.keys(after)]);
  const diffs: FieldDiff[] = [];
  for (const field of fields) {
    if (JSON.stringify(before[field]) !== JSON.stringify(after[field])) {
      diffs.push({ field, before: before[field], after: after[field] });
    }
  }
  return diffs.sort((a, b) => a.field.localeCompare(b.field));
}

// AuditTab merges the former separate History (version-diff) and Audit
// trail tabs into one chronological, filterable event list (Phase 5
// vessel-UI-rework restructure): same event-type/actor filters
// AuditTrailTab always had, plus a "resubmitted" row now expands inline
// to show that version's field diff instead of living in its own tab —
// reuses diffFields verbatim (paired against the previous version by
// versionNo), no new diffing logic.
function AuditTab({ events, versions }: { events: ReportEvent[]; versions: ReportView[] }) {
  const [eventType, setEventType] = useState("");
  const [actor, setActor] = useState("");
  const [expanded, setExpanded] = useState<Set<number>>(new Set());

  const filtered = useMemo(
    () =>
      events.filter(
        (e) => (!eventType || e.type === eventType) && (!actor || (e.actor ?? "").toLowerCase().includes(actor.toLowerCase())),
      ),
    [events, eventType, actor],
  );

  function toggle(i: number) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(i)) next.delete(i);
      else next.add(i);
      return next;
    });
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}>
      <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-3)", alignItems: "flex-end" }}>
        <Select
          label="Event type"
          value={eventType}
          options={["", ...Object.keys(EVENT_LABELS)]}
          onChange={setEventType}
          style={{ width: 200 }}
        />
        <TextField label="Actor" value={actor} onChange={setActor} style={{ width: 160 }} />
      </div>
      {filtered.length === 0 ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          {events.length === 0 ? "No audit events yet." : "No events match the current filters."}
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
          {filtered.map((e, i) => {
            const prevVersion = e.type === "resubmitted" ? versions.find((v) => v.versionNo === e.versionNo - 1) : undefined;
            const curVersion = e.type === "resubmitted" ? versions.find((v) => v.versionNo === e.versionNo) : undefined;
            const diffs = prevVersion && curVersion ? diffFields(prevVersion.fields, curVersion.fields) : null;
            const isOpen = expanded.has(i);
            return (
              <div key={i} style={{ paddingBottom: "var(--space-3)", borderBottom: "1px solid var(--color-outline-variant)" }}>
                <div
                  style={{ display: "flex", gap: "var(--space-4)", cursor: diffs ? "pointer" : "default" }}
                  onClick={diffs ? () => toggle(i) : undefined}
                >
                  <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", width: 170, flexShrink: 0 }}>{formatUtc(e.at)}</div>
                  <div style={{ flex: 1 }}>
                    <div className="md-body-large" style={{ color: "var(--color-on-surface)", display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
                      {EVENT_LABELS[e.type] ?? e.type}
                      {diffs ? (
                        <span className="material-symbols-rounded" style={{ fontSize: 18, color: "var(--color-on-surface-variant)" }}>
                          {isOpen ? "expand_less" : "expand_more"}
                        </span>
                      ) : null}
                    </div>
                    <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
                      {e.actor ? e.actor : "system"} · v{e.versionNo}
                    </div>
                    {e.type === "finding_acknowledged" && typeof e.detail?.message === "string" ? (
                      <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", marginTop: 2 }}>
                        {e.detail.acknowledged ? "" : "Un-"}Acknowledged: {e.detail.message}
                      </div>
                    ) : null}
                  </div>
                </div>
                {diffs && isOpen ? (
                  diffs.length === 0 ? (
                    <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", marginTop: "var(--space-2)" }}>No field changes.</div>
                  ) : (
                    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)", marginTop: "var(--space-3)", marginLeft: 186 }}>
                      {diffs.map((d) => (
                        <div key={d.field} style={{ display: "flex", gap: "var(--space-4)" }}>
                          <div className="md-body-medium" style={{ width: 200, flexShrink: 0 }}>{d.field}</div>
                          <div className="md-body-medium" style={{ color: "var(--color-status-warning)", textDecoration: "line-through" }}>
                            {d.before == null ? "—" : String(d.before)}
                          </div>
                          <span className="material-symbols-rounded" style={{ color: "var(--color-on-surface-variant)" }}>arrow_forward</span>
                          <div className="md-body-medium">{d.after == null ? "—" : String(d.after)}</div>
                        </div>
                      ))}
                    </div>
                  )
                ) : null}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ChatTab is design handoff A8's per-report chat wall: text-only,
// size-capped, both directions — deliberately boring, no attachments/
// reactions/threading. Office messages are visually distinct from
// vessel ones via alignment/background, not a label, since the sender
// name already appears on every message.
function ChatTab({
  reportId,
  messages,
  onSent,
}: {
  reportId: string;
  messages: ChatMessageView[];
  onSent: (m: ChatMessageView) => void;
}) {
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const overCap = new TextEncoder().encode(draft).length > MAX_CHAT_BODY_BYTES;

  async function handleSend() {
    if (!draft.trim() || overCap) return;
    setSending(true);
    setError(null);
    try {
      const posted = await api.postChat(reportId, draft);
      onSent(posted);
      setDraft("");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not send this message.");
    } finally {
      setSending(false);
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)", maxWidth: 640 }}>
      {error ? <AlertBanner level="warning" title="Couldn't send message" message={error} onDismiss={() => setError(null)} /> : null}

      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
        {messages.length === 0 ? (
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>No messages yet.</div>
        ) : (
          messages.map((m) => (
            <div
              key={m.id}
              style={{
                alignSelf: m.direction === "vessel" ? "flex-end" : "flex-start",
                maxWidth: "80%",
                background: m.direction === "vessel" ? "var(--color-primary-container)" : "var(--color-surface-container-highest)",
                color: m.direction === "vessel" ? "var(--color-on-primary-container)" : "var(--color-on-surface)",
                borderRadius: "var(--shape-medium)",
                padding: "var(--space-3) var(--space-4)",
              }}
            >
              <div className="md-body-large" style={{ whiteSpace: "pre-wrap" }}>{m.body}</div>
              <div className="md-body-small" style={{ opacity: 0.8, marginTop: 4 }}>
                {m.sender} · {formatUtc(m.sentAt)}
              </div>
            </div>
          ))
        )}
      </div>

      <div>
        <Textarea label="Send a message" value={draft} onChange={setDraft} rows={2} maxLength={MAX_CHAT_BODY_BYTES} />
        <div style={{ display: "flex", justifyContent: "flex-end", marginTop: "var(--space-2)" }}>
          <Button variant="filled" onClick={() => void handleSend()} disabled={sending || !draft.trim() || overCap}>
            {sending ? "Sending…" : "Send"}
          </Button>
        </div>
      </div>
    </div>
  );
}
