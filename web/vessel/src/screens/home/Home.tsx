// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState, type ReactNode } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { ProgressIndicator } from "../../design/components/feedback/ProgressIndicator.jsx";
import { ReportStatusBadge } from "../../design/components/maritime/ReportStatusBadge.jsx";
import { PageContainer } from "../PageContainer";
import {
  api,
  ApiError,
  type EventSuggestion,
  type EventType,
  type FieldPolicyState,
  type ReportView,
  type Schema,
  type SyncStatus,
  type VoyageSummary,
} from "../../api/client";
import {
  cadenceStatus,
  completionFraction,
  formatDurationShort,
  lastSubmittedReport,
  latestReport,
  sortByEventTimeDesc,
  suggestedEventTypes,
} from "./homeData";
import { dismissOverdue, isOverdueDismissed, overdueKeyFor } from "./overdueDismissal";
import { VoyageCard } from "./VoyageCard";
import { formatUtc, voyageNumberOf } from "../../format";
import { EventPickerDialog } from "../EventPickerDialog";

interface HomeData {
  schema: Schema;
  policy: Record<string, FieldPolicyState>;
  // Per-field voyage-event narrowing from the applied config bundle; a draft's
  // completion percentage counts only the fields its own event actually shows.
  fieldEvents: Record<string, string[]>;
  reports: ReportView[];
  eventTypes: EventType[];
  suggestions: EventSuggestion[];
  voyage: VoyageSummary | null;
  // Resolved cadence ceiling from the applied config bundle — drives the
  // overdue banner (replacing homeData.ts's hardcoded 12h).
  maxGapHours: number;
}

// Design handoff A3: "the most important screen on the vessel." Content,
// top to bottom, matches the handoff's own numbered list: overdue banner,
// suggested next report, in-progress, needs attention, recent reports,
// sync status — with a voyage summary card ahead of it all (new scope, not
// in the handoff, requested directly and built entirely from fields
// already captured on reports — see VoyageCard's own comment). Scoped to
// the log-abstract schema — Bunker/EDN/office-authored schemas don't have
// a cadence or "next event" concept.
export function Home({
  enrolled,
  onOpenReportForm,
  onResumeReport,
  onViewReport,
}: {
  enrolled: boolean;
  onOpenReportForm: (schemaName: string, eventType?: string) => void;
  onResumeReport: (schemaName: string, reportId: string) => void;
  /** Design handoff A7: submitted-and-later reports open the read-only detail screen instead. */
  onViewReport: (schemaName: string, reportId: string) => void;
}) {
  const [data, setData] = useState<HomeData | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [config, reports, eventTypes, suggestions, voyage, vesselConfig] = await Promise.all([
          api.getSchema("log-abstract"),
          api.listReports("log-abstract"),
          api.listEventTypes(),
          api.listEventSuggestions(),
          api.getCurrentVoyage(),
          api.getVesselConfig(),
        ]);
        if (cancelled) return;
        setData({
          schema: config.schema,
          policy: config.fieldPolicy,
          fieldEvents: config.fieldEvents ?? {},
          reports,
          eventTypes,
          suggestions,
          voyage,
          maxGapHours: vesselConfig.maxGapHours,
        });
      } catch (err) {
        if (!cancelled) setError(err instanceof ApiError ? err.message : "Could not load Home.");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <PageContainer title="Home">
      {!enrolled ? (
        <AlertBanner
          level="caution"
          title="Not enrolled"
          message="This vessel isn't connected to an office yet. Enroll any time from Settings."
        />
      ) : null}

      {error ? <AlertBanner level="warning" title="Couldn't load Home" message={error} /> : null}

      {data ? (
        <HomeBody
          data={data}
          onOpenReportForm={onOpenReportForm}
          onResumeReport={onResumeReport}
          onViewReport={onViewReport}
          pickerOpen={pickerOpen}
          setPickerOpen={setPickerOpen}
        />
      ) : null}

      <SyncStatusFooter />
    </PageContainer>
  );
}

function HomeBody({
  data,
  onOpenReportForm,
  onResumeReport,
  onViewReport,
  pickerOpen,
  setPickerOpen,
}: {
  data: HomeData;
  onOpenReportForm: (schemaName: string, eventType?: string) => void;
  onResumeReport: (schemaName: string, reportId: string) => void;
  onViewReport: (schemaName: string, reportId: string) => void;
  pickerOpen: boolean;
  setPickerOpen: (open: boolean) => void;
}) {
  const { schema, policy, fieldEvents, reports, eventTypes, suggestions, voyage, maxGapHours } = data;
  const last = latestReport(reports);
  // Overdue cadence must run from the last report actually submitted, not
  // the most recently touched draft (18.07.26 manual-test item 3) — the
  // event-type suggestion above stays on `last` (any state): a draft
  // already in progress is still the best signal for "what's likely
  // next," unaffected by this bug.
  const lastSubmitted = lastSubmittedReport(reports);
  const cadence = cadenceStatus(lastSubmitted?.eventTime, new Date(), maxGapHours);
  const suggested = suggestedEventTypes(suggestions, last?.eventType);

  // 18.07.26 manual-test item 14: the overdue/due-soon banner used to be
  // permanent — see overdueDismissal.ts's own doc comment for why a
  // dismissal is scoped to the current cadence cycle rather than
  // suppressing every future banner.
  const overdueKey = overdueKeyFor(lastSubmitted?.reportId);
  const [dismissedKey, setDismissedKey] = useState<string | null>(() => (isOverdueDismissed(overdueKey) ? overdueKey : null));
  const bannerDismissed = dismissedKey === overdueKey;

  // A draft/ready report is still being edited (opens ReportForm); anything
  // past that is locked there (architecture 8.1) and only viewable read-only
  // (design handoff A7) — matches ReportForm's own isLocked check exactly.
  function openReport(report: ReportView) {
    if (report.state === "draft" || report.state === "ready") {
      onResumeReport("log-abstract", report.reportId);
    } else {
      onViewReport("log-abstract", report.reportId);
    }
  }
  const primarySuggestion = suggested[0];

  const inProgress = sortByEventTimeDesc(reports.filter((r) => r.state === "draft" || r.state === "ready"));
  const needsAttention = sortByEventTimeDesc(reports.filter((r) => r.state === "invalidated" || r.state === "remarked"));
  const recent = sortByEventTimeDesc(reports).slice(0, 8);

  return (
    <>
      {bannerDismissed ? null : cadence.kind === "overdue" ? (
        <AlertBanner
          level="warning"
          title="Report overdue"
          message={`Overdue by ${formatDurationShort(Date.now() - cadence.dueAt.getTime())}`}
          onDismiss={() => {
            dismissOverdue(overdueKey);
            setDismissedKey(overdueKey);
          }}
        />
      ) : cadence.kind === "dueSoon" ? (
        <AlertBanner
          level="caution"
          title="Report due soon"
          message={`Next report due in ${formatDurationShort(cadence.dueAt.getTime() - Date.now())}`}
          onDismiss={() => {
            dismissOverdue(overdueKey);
            setDismissedKey(overdueKey);
          }}
        />
      ) : null}

      <div style={{ display: "flex", gap: 16, alignItems: "stretch", flexWrap: "wrap" }}>
        <VoyageCard voyage={voyage} />

        <Card variant="elevated" style={{ padding: 24, flex: 1, minWidth: 280, display: "flex", flexDirection: "column" }}>
          <div className="md-label-large" style={{ color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em", marginBottom: 8 }}>
            Suggested next report
          </div>
          <div className="md-headline-medium" style={{ color: "var(--color-on-surface)", marginBottom: 4 }}>
            {primarySuggestion}
          </div>
          {/* Complements, doesn't replace, the separate overdue/due-soon
              AlertBanner above — that banner is easy to miss once the page
              has scrolled; this line keeps the urgency visible right next
              to the action that resolves it. */}
          {cadence.kind === "overdue" ? (
            <div className="md-body-medium" style={{ color: "var(--color-error)", fontWeight: 600, marginBottom: "auto" }}>
              Report overdue by {formatDurationShort(Date.now() - cadence.dueAt.getTime())}
            </div>
          ) : cadence.kind === "dueSoon" ? (
            <div className="md-body-medium" style={{ color: "var(--color-status-caution)", fontWeight: 600, marginBottom: "auto" }}>
              Due in {formatDurationShort(cadence.dueAt.getTime() - Date.now())}
            </div>
          ) : (
            <div style={{ marginBottom: 16 }} />
          )}
          <div style={{ display: "flex", gap: 12, alignItems: "center", marginTop: 16 }}>
            <Button variant="filled" size="large" onClick={() => onOpenReportForm("log-abstract", primarySuggestion)}>
              Open
            </Button>
            <Button variant="text" onClick={() => setPickerOpen(true)}>
              Other event…
            </Button>
          </div>
        </Card>
      </div>

      {inProgress.length > 0 ? (
        <div>
          <SectionHeading title="In progress" />
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: 12 }}>
            {inProgress.map((r) => (
              <DraftTile key={r.reportId} report={r} schema={schema} policy={policy} events={fieldEvents} onClick={() => openReport(r)} />
            ))}
          </div>
        </div>
      ) : null}

      {needsAttention.length > 0 ? (
        <Section title="Needs attention">
          {needsAttention.map((r) => (
            <button
              key={r.reportId}
              onClick={() => openReport(r)}
              style={{
                display: "flex", flexDirection: "column", gap: 4, padding: "12px 16px",
                borderLeft: "3px solid var(--color-error)", background: "var(--color-surface-container-low)",
                borderRadius: "var(--shape-small)", border: "none", textAlign: "left", font: "inherit", cursor: "pointer", width: "100%",
              }}
            >
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <span className="md-body-large" style={{ color: "var(--color-on-surface)" }}>{r.eventType || "(no event type)"}</span>
                <ReportStatusBadge status={r.state} resubmitted={r.versionNo > 1} />
              </div>
              <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
                {formatUtc(r.eventTime)}
                {r.invalidatedRules && r.invalidatedRules.length > 0 ? ` · Invalidated: ${r.invalidatedRules.join(", ")}` : ""}
              </div>
            </button>
          ))}
        </Section>
      ) : null}

      {recent.length > 0 ? (
        <div>
          <SectionHeading title="Recent reports" />
          <RecentReportsTable reports={recent} onOpen={openReport} />
        </div>
      ) : null}

      <EventPickerDialog
        open={pickerOpen}
        eventTypes={eventTypes}
        suggested={suggested}
        onPick={(code) => {
          setPickerOpen(false);
          onOpenReportForm("log-abstract", code);
        }}
        onPickOtherReport={(schemaName) => {
          setPickerOpen(false);
          onOpenReportForm(schemaName);
        }}
        onClose={() => setPickerOpen(false)}
      />
    </>
  );
}

function SectionHeading({ title }: { title: string }) {
  return (
    <h2 className="md-title-medium" style={{ color: "var(--color-on-surface)", marginBottom: 12 }}>
      {title}
    </h2>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div>
      <SectionHeading title={title} />
      <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>{children}</div>
    </div>
  );
}

function DraftTile({
  report,
  schema,
  policy,
  events,
  onClick,
}: {
  report: ReportView;
  schema: Schema;
  policy: Record<string, FieldPolicyState>;
  events: Record<string, string[]>;
  onClick: () => void;
}) {
  const fraction = completionFraction(schema.fields, policy, report.fields, events, report.eventType);
  const percent = Math.round(fraction * 100);
  const voyageNumber = voyageNumberOf(report);
  return (
    <button
      onClick={onClick}
      style={{
        textAlign: "left", cursor: "pointer", font: "inherit", width: "100%",
        border: "1px solid var(--color-outline-variant)", borderRadius: "var(--shape-medium)",
        background: "var(--color-surface-container-low)", padding: 16,
        display: "flex", flexDirection: "column", gap: 10,
      }}
    >
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 8, minWidth: 0 }}>
        <span className="md-title-small" style={{ color: "var(--color-on-surface)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", flex: 1, minWidth: 0 }}>
          {report.eventType || "(no event type)"}
        </span>
        <ReportStatusBadge status={report.state} resubmitted={report.versionNo > 1} />
      </div>
      <div className="md-label-medium" style={{ color: "var(--color-on-surface-variant)", fontFamily: "var(--font-mono)" }}>
        {formatUtc(report.eventTime)}
        {voyageNumber ? ` · ${voyageNumber}` : ""}
      </div>
      <ProgressIndicator value={percent} />
      <div className="md-label-small" style={{ color: "var(--color-on-surface-variant)" }}>{percent}% of required fields filled</div>
    </button>
  );
}

function RecentReportsTable({ reports, onOpen }: { reports: ReportView[]; onOpen: (r: ReportView) => void }) {
  return (
    <div style={{ border: "1px solid var(--color-outline-variant)", borderRadius: "var(--shape-medium)", overflow: "hidden" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 16, padding: "0 16px", height: 32, background: "var(--color-surface-container)" }}>
        <span className="md-label-small" style={{ flex: 1, color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em" }}>Event</span>
        <span className="md-label-small" style={{ width: 140, color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em" }}>UTC</span>
        <span className="md-label-small" style={{ width: 90, color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em" }}>Voyage</span>
        <span className="md-label-small" style={{ width: 130, color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em" }}>State</span>
      </div>
      {reports.map((r, i) => (
        <button
          key={r.reportId}
          onClick={() => onOpen(r)}
          style={{
            width: "100%", textAlign: "left", cursor: "pointer", font: "inherit",
            border: "none", borderTop: i === 0 ? "none" : "1px solid var(--color-outline-variant)",
            background: "var(--color-surface-container-low)", padding: "0 16px", height: 44,
            display: "flex", alignItems: "center", gap: 16,
          }}
        >
          <span className="md-body-medium" style={{ flex: 1, minWidth: 0, color: "var(--color-on-surface)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
            {r.eventType || "(no event type)"}
          </span>
          <span className="md-body-small" style={{ width: 140, color: "var(--color-on-surface-variant)", fontFamily: "var(--font-mono)" }}>{formatUtc(r.eventTime)}</span>
          <span className="md-body-small" style={{ width: 90, color: "var(--color-on-surface-variant)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
            {voyageNumberOf(r) || "—"}
          </span>
          <span style={{ width: 130 }}>
            <ReportStatusBadge status={r.state} resubmitted={r.versionNo > 1} />
          </span>
        </button>
      ))}
    </div>
  );
}

// Architecture 11/design handoff A3 item 6. Sync itself has existed
// since Phase 4 (vessel/httpapi's GET /api/sync/status, POST
// /api/sync/now) — this was left as an honest inert stub until now
// because the frontend never actually called those endpoints.
function SyncStatusFooter() {
  const [status, setStatus] = useState<SyncStatus | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void api.getSyncStatus().then((s) => {
      if (!cancelled) setStatus(s);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  async function handleSyncNow() {
    setSyncing(true);
    setError(null);
    try {
      const result = await api.syncNow();
      setStatus(result);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Sync failed.");
    } finally {
      setSyncing(false);
    }
  }

  const label = !status
    ? "Sync: —"
    : !status.enrolled
      ? "Sync: not connected to an office"
      : error || status.lastError
        ? `Sync: failed — ${error || status.lastError}`
        : status.lastSuccess
          ? `Sync: last synced ${formatUtc(status.lastSuccess)}`
          : "Sync: never run";

  return (
    <div
      className="md-body-small"
      style={{
        display: "flex", justifyContent: "space-between", alignItems: "center",
        color: error || status?.lastError ? "var(--color-error)" : "var(--color-on-surface-variant)",
        borderTop: "1px solid var(--color-outline-variant)",
        paddingTop: 12,
      }}
    >
      <span>{label}</span>
      <Button variant="text" size="small" disabled={!status?.enrolled || syncing} onClick={() => void handleSyncNow()}>
        {syncing ? "Syncing…" : "Sync now"}
      </Button>
    </div>
  );
}
