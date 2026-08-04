// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useMemo, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Dialog } from "../../design/components/feedback/Dialog.jsx";
import { Switch } from "../../design/components/forms/Switch.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { ReportStatusBadge } from "../../design/components/maritime/ReportStatusBadge.jsx";
import { DataTable, type DataTableColumn } from "../../design/components/data/DataTable.jsx";
import { PageContainer } from "../PageContainer";
import { formatUtc, voyageNumberOf } from "../../format";
import { api, ApiError, type EventType, type ReportView } from "../../api/client";
import { sortByEventTimeDesc } from "../home/homeData";
import { schemaDisplayName } from "../report-form/ReportForm";
import { EventPickerDialog } from "../EventPickerDialog";

interface ReportRow {
  id: string;
  schemaName: string;
  type: string;
  event: string;
  eventTimeDisplay: string;
  voyage: string;
  state: string;
  resubmitted: boolean;
}

function toRow(r: ReportView): ReportRow {
  return {
    id: r.reportId,
    schemaName: r.schemaName,
    type: schemaDisplayName(r.schemaName),
    event: r.eventType || "(no event type)",
    // "YYYY-MM-DD HH:MM UTC" sorts lexicographically the same as
    // chronologically, so this doubles as the sortable value — no
    // separate raw-timestamp column needed.
    eventTimeDisplay: formatUtc(r.eventTime),
    voyage: voyageNumberOf(r) ?? "—",
    state: r.state,
    resubmitted: r.versionNo > 1,
  };
}

export const NEEDS_ATTENTION_STATES = new Set(["remarked", "invalidated"]);

// Every schema this screen lists reports for (Phase 6: Bunker/EDN join
// Log Abstract here), tagged by each report's own schemaName rather than
// one hardcoded value. Exported for NotificationBell.tsx, which needs
// the exact same cross-schema "does this vessel have anything needing
// attention" scope — a single source of truth rather than a second list
// that could drift.
export const LISTED_SCHEMAS = ["log-abstract", "bunker-report", "edn-report"];

// NEW_REPORT_TYPES (schemas with no Event field/cadence concept, offered
// directly by schema name) lives in EventPickerDialog.tsx now — this
// screen's own "+ New report" and Home's "Start a report" both render it
// there, inside the same shared dialog.

// Design handoff A4: filterable table of every report. DataTable already
// has its own free-text search (covers voyage number/event lookups) and
// per-column checkbox filters (used here for Type, Event, and State) —
// so this only adds controls for things that aren't a simple "match one
// of these exact values": needs-attention is a derived boolean, and the
// date range is a range, not a set membership. Health-indicator and
// last-editor columns from the handoff are omitted — listReports doesn't
// carry per-report finding counts or a last-editor field yet.
export function ReportsScreen({
  onOpenReportForm,
  onResumeReport,
  onViewReport,
}: {
  /** "+ New report"'s type picker (Phase 6) — same contract as Home's onOpenReportForm. */
  onOpenReportForm: (schemaName: string, eventType?: string) => void;
  /** Row click on a draft/ready report — same "resume an existing report" contract as Home's onResumeReport. */
  onResumeReport: (schemaName: string, reportId: string) => void;
  onViewReport: (schemaName: string, reportId: string) => void;
}) {
  const [reports, setReports] = useState<ReportView[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [needsAttentionOnly, setNeedsAttentionOnly] = useState(false);
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [pickerOpen, setPickerOpen] = useState(false);
  const [eventTypes, setEventTypes] = useState<EventType[]>([]);
  // 2026-07-14 manual-test feedback: "there is no grid action in reports
  // to delete a report in status draft." Only ever offered for a true
  // never-submitted draft (state draft, versionNo 1 — the `!resubmitted`
  // check below) — same versionNo === 1 boundary handleDeleteReport
  // enforces server-side (see its own doc comment), so a correction
  // draft of an already-submitted report never shows this action.
  const [deleteTarget, setDeleteTarget] = useState<ReportRow | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [lists, types] = await Promise.all([
          Promise.all(LISTED_SCHEMAS.map((name) => api.listReports(name))),
          api.listEventTypes(),
        ]);
        if (cancelled) return;
        setReports(lists.flat());
        setEventTypes(types);
      } catch (err) {
        if (!cancelled) setError(err instanceof ApiError ? err.message : "Could not load reports.");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const visible = useMemo(() => {
    if (!reports) return [];
    let out = sortByEventTimeDesc(reports);
    if (needsAttentionOnly) out = out.filter((r) => NEEDS_ATTENTION_STATES.has(r.state));
    if (fromDate) out = out.filter((r) => r.eventTime.slice(0, 10) >= fromDate);
    if (toDate) out = out.filter((r) => r.eventTime.slice(0, 10) <= toDate);
    return out;
  }, [reports, needsAttentionOnly, fromDate, toDate]);

  const rows = useMemo(() => visible.map(toRow), [visible]);

  const columns: DataTableColumn[] = [
    { key: "type", label: "Type", type: "text", sortable: true, filterable: true },
    { key: "event", label: "Event", type: "text", sortable: true, filterable: true },
    { key: "eventTimeDisplay", label: "UTC Timestamp", type: "text", sortable: true },
    { key: "voyage", label: "Voyage", type: "text", sortable: true },
    {
      key: "state",
      label: "State",
      type: "text",
      sortable: true,
      filterable: true,
      render: (row) => <ReportStatusBadge status={row.state} resubmitted={row.resubmitted} />,
    },
    {
      key: "actions",
      label: "",
      type: "text",
      render: (row) =>
        row.state === "draft" && !row.resubmitted ? (
          <div style={{ display: "flex", justifyContent: "flex-end" }}>
            <Button variant="text" size="small" style={{ color: "var(--color-error)" }} onClick={() => setDeleteTarget(row as ReportRow)}>
              Delete
            </Button>
          </div>
        ) : null,
    },
  ];

  async function handleDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await api.deleteReport(deleteTarget.id);
      setReports((prev) => (prev ? prev.filter((r) => r.reportId !== deleteTarget.id) : prev));
      setDeleteTarget(null);
    } catch (err) {
      setDeleteError(err instanceof ApiError ? err.message : "Could not delete this draft.");
    } finally {
      setDeleting(false);
    }
  }

  function openRow(row: Record<string, unknown>) {
    const report = reports?.find((r) => r.reportId === row.id);
    if (!report) return;
    // Matches Home's own openReport split (architecture 8.1): draft/ready
    // is still being edited, anything past that is locked and read-only
    // (design handoff A7).
    if (report.state === "draft" || report.state === "ready") {
      onResumeReport(report.schemaName, report.reportId);
    } else {
      onViewReport(report.schemaName, report.reportId);
    }
  }

  return (
    <PageContainer
      title="Reports"
      actions={
        <Button variant="filled" icon="add" onClick={() => setPickerOpen(true)}>
          New report
        </Button>
      }
    >
      {error ? <AlertBanner level="warning" title="Couldn't load reports" message={error} /> : null}
      {deleteError ? <AlertBanner level="warning" title="Couldn't delete draft" message={deleteError} onDismiss={() => setDeleteError(null)} /> : null}

      <div style={{ display: "flex", alignItems: "center", gap: "var(--space-5)", flexWrap: "wrap" }}>
        <label style={{ display: "flex", alignItems: "center", gap: "var(--space-3)" }}>
          <Switch checked={needsAttentionOnly} onChange={setNeedsAttentionOnly} />
          <span className="md-body-medium">Needs attention only</span>
        </label>
        <TextField label="From" value={fromDate} onChange={setFromDate} type="date" style={{ width: 180 }} />
        <TextField label="To" value={toDate} onChange={setToDate} type="date" style={{ width: 180 }} />
      </div>

      {reports === null ? (
        <span className="md-body-large" style={{ color: "var(--color-on-surface-variant)" }}>Loading…</span>
      ) : (
        <DataTable
          columns={columns}
          rows={rows}
          onRowAction={openRow}
          searchPlaceholder="Search type, event, voyage…"
          emptyMessage="No reports match these filters."
        />
      )}

      <EventPickerDialog
        open={pickerOpen}
        eventTypes={eventTypes}
        suggested={[]}
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

      <Dialog
        open={deleteTarget !== null}
        title="Delete this draft?"
        onClose={() => !deleting && setDeleteTarget(null)}
        actions={[
          { label: "Cancel", onClick: () => setDeleteTarget(null) },
          { label: deleting ? "Deleting…" : "Delete", onClick: () => void handleDelete() },
        ]}
      >
        {deleteTarget ? (
          <>
            This permanently removes the {deleteTarget.type} draft ({deleteTarget.event}) and everything entered on it —
            including any attachments. This cannot be undone.
          </>
        ) : null}
      </Dialog>
    </PageContainer>
  );
}
