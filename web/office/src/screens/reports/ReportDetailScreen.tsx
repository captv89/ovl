// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useMemo, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Tabs } from "../../design/components/navigation/Tabs.jsx";
import { Textarea } from "../../design/components/forms/Textarea.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Select } from "../../design/components/forms/Select.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Dialog } from "../../design/components/feedback/Dialog.jsx";
import { Chip } from "../../design/components/surfaces/Chip.jsx";
import {
  api,
  ApiError,
  MAX_CHAT_BODY_BYTES,
  type AttachmentView,
  type ChatMessageView,
  type RemarkView,
  type ReportDetailView,
  type ReportEvent,
  type ReportState,
  type ReportView,
  type SchemaDetail,
  type DomainEventType,
} from "../../api/client";
import { sectionLabel, sectionsInOrder } from "./sections";
import { HighlightableField } from "./HighlightableField";
import { groupFields } from "./fieldGrouping";

const TABS = ["Report", "Audit trail", "Chat"] as const;
type Tab = (typeof TABS)[number];

const STATE_LABEL: Record<ReportState, string> = {
  draft: "Draft",
  ready: "Ready",
  submitted: "Submitted",
  synced: "Synced",
  pushed: "Pushed",
  remarked: "Remarked",
  invalidated: "Invalidated",
};

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

// Design handoff B4's report detail: read-only, 3 tabs — Report, Audit
// trail (History's version-diff folded in, Phase 5 vessel-UI-rework
// pattern ported verbatim from web/vessel/src/screens/report-detail/
// ReportDetailScreen.tsx), Chat. The former standalone Remarks tab is
// dropped: remark-set creation already auto-posts a linking chat message
// server-side (office/httpapi/remarks.go's remarkChatSummary), so that
// conversation happens in Chat, matching the vessel side's own already-
// shipped decision 5. Unlike vessel's screen this one also drives Remark
// mode (office is the side that *creates* remarks), so the Reviewer-only
// resolve toggle that used to live on the standalone Remarks tab now
// lives inline on each row of the Report tab's "Remarks & findings" box
// instead — the wireframe (B4·3) draws it inside a structured chat
// bubble, but this project's Chat tab is deliberately kept identical on
// both sides of the sync link (plain ChatMessageView rows, matching
// vessel's own "deliberately boring" implementation exactly) rather than
// diverging into a fancier office-only bubble type for the same
// underlying messages.
export function ReportDetailScreen({
  vesselId,
  reportId,
  isReviewer,
  onBack,
}: {
  vesselId: string;
  reportId: string;
  isReviewer: boolean;
  onBack: () => void;
}) {
  const [detail, setDetail] = useState<ReportDetailView | null>(null);
  const [schema, setSchema] = useState<SchemaDetail | null>(null);
  const [events, setEvents] = useState<ReportEvent[]>([]);
  const [versions, setVersions] = useState<ReportView[]>([]);
  const [chat, setChat] = useState<ChatMessageView[]>([]);
  const [remarks, setRemarks] = useState<RemarkView[]>([]);
  const [attachments, setAttachments] = useState<AttachmentView[]>([]);
  const [tab, setTab] = useState<Tab>("Report");
  const [error, setError] = useState<string | null>(null);
  const [remarkMode, setRemarkMode] = useState(false);
  const [draftRemarks, setDraftRemarks] = useState<Record<string, string>>({});

  const reload = () =>
    api.getReport(vesselId, reportId).then(async (d) => {
      const [e, v, c, rm, at, versionsList] = await Promise.all([
        api.listReportEvents(vesselId, reportId),
        api.listReportVersions(vesselId, reportId),
        api.listChat(vesselId, reportId),
        api.listRemarks(vesselId, reportId),
        api.listAttachments(vesselId, reportId),
        api.listLatestSchemaVersions(),
      ]);
      const versionSummary = versionsList.find((s) => s.schemaName === d.latest.schemaName);
      const schemaDetail = versionSummary
        ? await api.getSchemaVersion(versionSummary.schemaName, versionSummary.version)
        : null;
      setDetail(d);
      setEvents(e);
      setVersions(v);
      setChat(c);
      setRemarks(rm);
      setAttachments(at);
      setSchema(schemaDetail);
    });

  useEffect(() => {
    let cancelled = false;
    reload().catch((err) => {
      if (!cancelled) setError(err instanceof ApiError ? err.message : "Could not load this report.");
    });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [vesselId, reportId]);

  if (error) {
    return (
      <div style={{ padding: 24 }}>
        <Button variant="text" icon="arrow_back" onClick={onBack}>
          Back to reports
        </Button>
        <AlertBanner level="warning" title="Couldn't load report" message={error} />
      </div>
    );
  }
  if (!detail) {
    return (
      <div className="md-body-medium" style={{ padding: 24, color: "var(--color-on-surface-variant)" }}>
        Loading report…
      </div>
    );
  }

  const { latest } = detail;
  const flaggedFields = Object.keys(draftRemarks).filter((f) => draftRemarks[f].trim() !== "");
  // 18.07.26 manual-test item 12: dropping a field from draftRemarks is
  // exactly "undo this remark" — flaggedFields/HighlightableField's own
  // `flagged` styling already key off whether a field has a non-empty
  // draft, so removing the key here is enough to un-flag it visually too,
  // no separate "undo" concept needed beyond exposing this action.
  function removeDraft(fieldName: string) {
    setDraftRemarks((prev) => {
      const next = { ...prev };
      delete next[fieldName];
      return next;
    });
  }
  function fieldLabel(fieldName: string) {
    return schema?.fields.find((f) => f.name === fieldName)?.label ?? fieldName;
  }

  async function handleSendRemarkSet() {
    const remarksToSend = flaggedFields.map((fieldName) => ({ fieldName, body: draftRemarks[fieldName] }));
    if (remarksToSend.length === 0) return;
    await api.createRemarkSet(vesselId, reportId, remarksToSend);
    setDraftRemarks({});
    setRemarkMode(false);
    await reload();
  }

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
      <div>
        <Button variant="text" icon="arrow_back" onClick={onBack}>
          Back to reports
        </Button>
        <div style={{ display: "flex", alignItems: "center", gap: 12, marginTop: 4 }}>
          <div className="md-headline-small">{latest.eventType || "(no event type)"}</div>
          <StatePill state={latest.state} resubmitted={latest.versionNo > 1} />
          {isReviewer ? (
            <Button
              variant={remarkMode ? "filled" : "outlined"}
              icon="flag"
              onClick={() => {
                if (remarkMode) setDraftRemarks({});
                setRemarkMode(!remarkMode);
              }}
              style={{ marginLeft: "auto" }}
            >
              {remarkMode ? "Exit remark mode" : "Remark mode"}
            </Button>
          ) : null}
        </div>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          {detail.vesselName} · IMO {detail.vesselImo} · {latest.schemaName} · v{latest.versionNo}
        </div>
      </div>

      {remarkMode ? (
        <AlertBanner
          level="caution"
          title="Remark mode is on"
          message="Click any field below to flag it with a comment. Values are always read-only here."
        />
      ) : null}

      <Tabs items={[...TABS]} selected={tab} onSelect={(t) => setTab(t as Tab)} />

      {tab === "Report" ? (
        schema ? (
          <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
            <ReportValuesTab
              schema={schema}
              report={latest}
              remarks={remarks}
              remarkMode={remarkMode}
              isReviewer={isReviewer}
              draftRemarks={draftRemarks}
              onDraftChange={(fieldName, body) => setDraftRemarks((prev) => ({ ...prev, [fieldName]: body }))}
              onResolvedChange={reload}
            />
            {attachments.length > 0 ? <AttachmentsPreview vesselId={vesselId} reportId={reportId} attachments={attachments} /> : null}
          </div>
        ) : (
          <FlatValuesFallback report={latest} />
        )
      ) : tab === "Audit trail" ? (
        <AuditTab events={events} versions={versions} />
      ) : (
        <ChatTab
          vesselId={vesselId}
          reportId={reportId}
          messages={chat}
          onSent={(m) => setChat((prev) => [...prev, m])}
        />
      )}

      {remarkMode && flaggedFields.length > 0 ? (
        <div
          style={{
            position: "sticky",
            bottom: 0,
            background: "var(--color-surface-container-high)",
            borderRadius: "var(--shape-medium)",
            padding: 16,
            display: "flex",
            flexDirection: "column",
            gap: 12,
            boxShadow: "var(--elevation-2)",
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <span className="md-body-medium">
              {flaggedFields.length} field{flaggedFields.length === 1 ? "" : "s"} flagged
            </span>
            <span className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
              — remove a chip below to undo a field before sending
            </span>
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
            {flaggedFields.map((f) => (
              <Chip key={f} type="input" label={fieldLabel(f)} onDelete={() => removeDraft(f)} />
            ))}
          </div>
          <Button variant="filled" onClick={() => void handleSendRemarkSet()} style={{ alignSelf: "flex-end" }}>
            Send remark set
          </Button>
        </div>
      ) : null}
    </div>
  );
}

export function StatePill({ state, resubmitted }: { state: ReportState; resubmitted: boolean }) {
  const tone: Record<ReportState, { bg: string; fg: string }> = {
    draft: { bg: "var(--color-surface-container-highest)", fg: "var(--color-on-surface-variant)" },
    ready: { bg: "var(--color-tertiary-container)", fg: "var(--color-on-tertiary-container)" },
    submitted: { bg: "var(--color-primary-container)", fg: "var(--color-on-primary-container)" },
    synced: { bg: "var(--color-status-underway-container)", fg: "var(--color-status-underway)" },
    pushed: { bg: "var(--color-status-underway-container)", fg: "var(--color-status-underway)" },
    remarked: { bg: "var(--color-status-caution-container)", fg: "var(--color-status-caution)" },
    invalidated: { bg: "var(--color-status-warning-container)", fg: "var(--color-status-warning)" },
  };
  const { bg, fg } = tone[state];
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
      <span
        className="md-label-medium"
        style={{ display: "inline-flex", alignItems: "center", height: 22, padding: "0 10px", borderRadius: "var(--shape-full)", background: bg, color: fg, fontWeight: 700 }}
      >
        {STATE_LABEL[state]}
      </span>
      {resubmitted ? (
        <span title="Resubmitted" style={{ width: 8, height: 8, borderRadius: "50%", background: "var(--color-primary)" }} />
      ) : null}
    </span>
  );
}

// Rare fallback (a report's schema has no published version office can
// find — shouldn't happen in practice since every submitted report's
// schema was published to reach the vessel in the first place, but a
// missing/renamed schema shouldn't hard-fail the whole screen) — same
// flat alphabetical grid this screen used everywhere before this phase.
function FlatValuesFallback({ report }: { report: ReportDetailView["latest"] }) {
  const entries = Object.entries(report.fields).sort(([a], [b]) => a.localeCompare(b));
  if (entries.length === 0) {
    return <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>No field values.</div>;
  }
  return (
    <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))", gap: 16 }}>
      {entries.map(([name, value]) => (
        <div key={name}>
          <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>{name}</div>
          <div className="md-body-large">{value == null ? "—" : String(value)}</div>
        </div>
      ))}
    </div>
  );
}

// Design handoff B4·1's section rail + B4·1 note 5's collapsible
// "Remarks & findings" box, ported from web/vessel/src/screens/
// report-detail/ReportDetailScreen.tsx's own ReportValuesTab (apps don't
// share code — same vendored-copy precedent as HighlightableField).
// Differs from the vessel version in three ways: always read-only (no
// correction-start affordance — B4's absolute no-office-edits rule), the
// flagged-field treatment is HighlightableField's own click-to-flag
// overlay instead of a jump-only highlight, and each "Remarks & findings"
// row gets a Reviewer-only "Mark resolved" action (see this file's own
// top comment on why that lives here instead of on a chat bubble).
function ReportValuesTab({
  schema,
  report,
  remarks,
  remarkMode,
  isReviewer,
  draftRemarks,
  onDraftChange,
  onResolvedChange,
}: {
  schema: SchemaDetail;
  report: ReportDetailView["latest"];
  remarks: RemarkView[];
  remarkMode: boolean;
  isReviewer: boolean;
  draftRemarks: Record<string, string>;
  onDraftChange: (fieldName: string, body: string) => void;
  onResolvedChange: () => Promise<void> | void;
}) {
  const order = useMemo(() => sectionsInOrder(schema), [schema]);
  const [activeSection, setActiveSection] = useState(order[0] ?? "");
  const [openField, setOpenField] = useState<string | null>(null);
  const [remarksExpanded, setRemarksExpanded] = useState(true);
  const [highlight, setHighlight] = useState<{ field: string; nonce: number } | null>(null);
  const [resolvingId, setResolvingId] = useState<string | null>(null);

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

  async function resolveRemark(remark: RemarkView) {
    setResolvingId(remark.id);
    try {
      await api.setRemarkResolved(remark.id, true);
      await onResolvedChange();
    } finally {
      setResolvingId(null);
    }
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
                    <div
                      key={rm.id}
                      style={{ display: "flex", alignItems: "center", gap: "var(--space-3)", width: "100%", padding: "var(--space-3) var(--space-4)", borderBottom: "1px solid var(--color-outline-variant)" }}
                    >
                      <button
                        onClick={() => jumpToField(rm.fieldName)}
                        style={{ flex: 1, minWidth: 0, border: "none", background: "transparent", cursor: "pointer", textAlign: "left", font: "inherit", padding: 0 }}
                      >
                        <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", marginBottom: 2 }}>
                          {sectionLabel(field?.section ?? "")} › {field?.label ?? rm.fieldName}
                        </div>
                        <div className="md-body-medium" style={{ color: "var(--color-on-surface)" }}>
                          {rm.body} <span style={{ color: "var(--color-primary)", fontWeight: 600 }}>Go to field ›</span>
                        </div>
                      </button>
                      {isReviewer ? (
                        <Button variant="outlined" onClick={() => void resolveRemark(rm)} disabled={resolvingId === rm.id}>
                          {resolvingId === rm.id ? "Resolving…" : "Mark resolved"}
                        </Button>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            ) : null}
          </div>
        ) : null}

        <div className="md-title-medium" style={{ color: "var(--color-on-surface)", marginBottom: "var(--space-4)" }}>{sectionLabel(activeSection)}</div>

        {filledActiveFields.length === 0 ? (
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>No values recorded for this section.</div>
        ) : (
          // 18.07.26 manual-test item 11: same field-grouping gap as
          // vessel's own ReportValuesTab — see fieldGrouping.ts's own doc
          // comment.
          activeFieldGroups.map((g, i) => (
            <div key={i} style={{ marginBottom: "var(--space-5)" }}>
              {g.label ? (
                <div className="md-label-large" style={{ color: "var(--color-on-surface-variant)", marginBottom: "var(--space-2)", textTransform: "uppercase", letterSpacing: "0.02em" }}>
                  {g.label}
                </div>
              ) : null}
              <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))", gap: "var(--space-4)" }}>
                {g.fields.map((f) => (
                  <HighlightableField
                    key={f.name}
                    fieldName={f.name}
                    highlightField={highlight?.field ?? null}
                    highlightNonce={highlight?.nonce ?? null}
                    remarkedFieldNames={remarkedFieldNames}
                    remarkMode={remarkMode}
                    open={openField === f.name}
                    draftComment={draftRemarks[f.name]}
                    onToggleDraft={() => setOpenField(openField === f.name ? null : f.name)}
                    onDraftChange={(body) => onDraftChange(f.name, body)}
                  >
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

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// AttachmentsPreview is architecture 15's "inline preview on vessel and
// office" for its own half: Bunker/EDN attachments the vessel already
// captured (web/vessel's AttachmentsSection is the write side; office
// only ever previews, matching B4's own absolute read-only rule).
// Rendered under the Report tab's field values, only when the report
// actually has any — most reports (Log Abstract) never will.
function AttachmentsPreview({ vesselId, reportId, attachments }: { vesselId: string; reportId: string; attachments: AttachmentView[] }) {
  const [preview, setPreview] = useState<AttachmentView | null>(null);
  return (
    <div>
      <div className="md-label-large" style={{ color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em", marginBottom: 12 }}>
        Attachments
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(140px, 1fr))", gap: 16 }}>
        {attachments.map((a) => (
          <button
            key={a.id}
            onClick={() => setPreview(a)}
            style={{
              display: "flex", flexDirection: "column", gap: 4, padding: 0, cursor: "pointer",
              border: "1px solid var(--color-outline-variant)", borderRadius: "var(--shape-medium)",
              background: "var(--color-surface-container-low)", overflow: "hidden", textAlign: "left", font: "inherit",
            }}
          >
            <div style={{ height: 100, display: "flex", alignItems: "center", justifyContent: "center", background: "var(--color-surface-container-highest)" }}>
              {a.contentType.startsWith("image/") ? (
                <img src={api.attachmentDownloadUrl(vesselId, reportId, a.id)} alt={a.filename} style={{ width: "100%", height: "100%", objectFit: "cover" }} />
              ) : (
                <span className="material-symbols-rounded" style={{ fontSize: 40, color: "var(--color-on-surface-variant)" }}>picture_as_pdf</span>
              )}
            </div>
            <div style={{ padding: "6px 8px 8px" }}>
              <div className="md-body-small" style={{ color: "var(--color-on-surface)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {a.filename}
              </div>
              <div className="md-label-small" style={{ color: "var(--color-on-surface-variant)" }}>{formatBytes(a.sizeBytes)}</div>
            </div>
          </button>
        ))}
      </div>

      <Dialog open={preview !== null} title={preview?.filename ?? ""} onClose={() => setPreview(null)} actions={[{ label: "Close", onClick: () => setPreview(null) }]}>
        {preview ? (
          preview.contentType.startsWith("image/") ? (
            <img src={api.attachmentDownloadUrl(vesselId, reportId, preview.id)} alt={preview.filename} style={{ maxWidth: "100%", borderRadius: "var(--shape-small)" }} />
          ) : (
            <a href={api.attachmentDownloadUrl(vesselId, reportId, preview.id)} target="_blank" rel="noreferrer" className="md-body-large">
              Open PDF in a new tab
            </a>
          )
        ) : null}
      </Dialog>
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
// trail tabs into one chronological, filterable event list — ported
// verbatim from web/vessel's own AuditTab (Phase 5 vessel-UI-rework
// restructure): a "resubmitted" row expands inline to show that
// version's field diff instead of living in its own tab. Office adds a
// third filter (Side: vessel/office) its Audit trail already had before
// this restructure, kept since the office's own findings note its
// authoring side and vessel's screen has no equivalent concept.
function AuditTab({ events, versions }: { events: ReportEvent[]; versions: ReportView[] }) {
  const [eventType, setEventType] = useState("");
  const [actor, setActor] = useState("");
  const [origin, setOrigin] = useState("");
  const [expanded, setExpanded] = useState<Set<number>>(new Set());

  const filtered = useMemo(
    () =>
      events.filter(
        (e) =>
          (!eventType || e.type === eventType) &&
          (!actor || (e.actor ?? "").toLowerCase().includes(actor.toLowerCase())) &&
          (!origin || e.origin === origin),
      ),
    [events, eventType, actor, origin],
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
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 12, alignItems: "flex-end" }}>
        <Select
          label="Event type"
          value={eventType}
          options={["", ...Object.keys(EVENT_LABELS)]}
          onChange={setEventType}
          style={{ width: 200 }}
        />
        <TextField label="Actor" value={actor} onChange={setActor} style={{ width: 160 }} />
        <Select label="Side" value={origin} options={["", "vessel", "office"]} onChange={setOrigin} style={{ width: 140 }} />
      </div>
      {filtered.length === 0 ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          {events.length === 0 ? "No audit events yet." : "No events match the current filters."}
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          {filtered.map((e, i) => {
            const prevVersion = e.type === "resubmitted" ? versions.find((v) => v.versionNo === e.versionNo - 1) : undefined;
            const curVersion = e.type === "resubmitted" ? versions.find((v) => v.versionNo === e.versionNo) : undefined;
            const diffs = prevVersion && curVersion ? diffFields(prevVersion.fields, curVersion.fields) : null;
            const isOpen = expanded.has(i);
            return (
              <div key={i} style={{ paddingBottom: 12, borderBottom: "1px solid var(--color-outline-variant)" }}>
                <div
                  style={{ display: "flex", gap: 16, cursor: diffs ? "pointer" : "default" }}
                  onClick={diffs ? () => toggle(i) : undefined}
                >
                  <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", width: 200, flexShrink: 0 }}>
                    {new Date(e.at).toUTCString()}
                  </div>
                  <div style={{ flex: 1 }}>
                    <div className="md-body-large" style={{ color: "var(--color-on-surface)", display: "flex", alignItems: "center", gap: 8 }}>
                      {EVENT_LABELS[e.type] ?? e.type}
                      {diffs ? (
                        <span className="material-symbols-rounded" style={{ fontSize: 18, color: "var(--color-on-surface-variant)" }}>
                          {isOpen ? "expand_less" : "expand_more"}
                        </span>
                      ) : null}
                    </div>
                    <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
                      {e.actor ? e.actor : "system"} · v{e.versionNo} · {e.origin}
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
                    <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", marginTop: 8 }}>No field changes.</div>
                  ) : (
                    <div style={{ display: "flex", flexDirection: "column", gap: 8, marginTop: 12, marginLeft: 216 }}>
                      {diffs.map((d) => (
                        <div key={d.field} style={{ display: "flex", gap: 16 }}>
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

// ChatTab mirrors web/vessel's own ChatTab: design handoff A8/B4's
// per-report chat wall, text-only, size-capped, no attachments/
// reactions/threading. Office messages align right here (the reverse of
// the vessel screen's own alignment) since "this side's messages on the
// right" is the more legible convention per-viewer, not a fixed global
// left/right per direction.
function ChatTab({
  vesselId,
  reportId,
  messages,
  onSent,
}: {
  vesselId: string;
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
      const posted = await api.postChat(vesselId, reportId, draft);
      onSent(posted);
      setDraft("");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not send this message.");
    } finally {
      setSending(false);
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: 640 }}>
      {error ? <AlertBanner level="warning" title="Couldn't send message" message={error} onDismiss={() => setError(null)} /> : null}

      <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        {messages.length === 0 ? (
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>No messages yet.</div>
        ) : (
          messages.map((m) => (
            <div
              key={m.id}
              style={{
                alignSelf: m.direction === "office" ? "flex-end" : "flex-start",
                maxWidth: "80%",
                background: m.direction === "office" ? "var(--color-primary-container)" : "var(--color-surface-container-highest)",
                color: m.direction === "office" ? "var(--color-on-primary-container)" : "var(--color-on-surface)",
                borderRadius: "var(--shape-medium)",
                padding: "12px 16px",
              }}
            >
              <div className="md-body-large" style={{ whiteSpace: "pre-wrap" }}>{m.body}</div>
              <div className="md-body-small" style={{ opacity: 0.8, marginTop: 4 }}>
                {m.sender} · {new Date(m.sentAt).toUTCString()}
              </div>
            </div>
          ))
        )}
      </div>

      <div>
        <Textarea label="Send a message" value={draft} onChange={setDraft} rows={2} maxLength={MAX_CHAT_BODY_BYTES} />
        <div style={{ display: "flex", justifyContent: "flex-end", marginTop: 8 }}>
          <Button variant="filled" onClick={() => void handleSend()} disabled={sending || !draft.trim() || overCap}>
            {sending ? "Sending…" : "Send"}
          </Button>
        </div>
      </div>
    </div>
  );
}
