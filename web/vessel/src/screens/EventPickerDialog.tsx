// SPDX-License-Identifier: AGPL-3.0-only

import { Dialog } from "../design/components/feedback/Dialog.jsx";
import type { EventType } from "../api/client";

// Bunker/EDN have no Event field/cadence concept (unlike Log Abstract),
// so they're offered directly by schema name rather than through the
// event-code list below. Log Abstract can't be created without an
// eventType — FieldRow.tsx's LOCKED_FIELDS unconditionally disables the
// Event field with no way to ever set it afterward ("Start a new report
// from Home to change it") — so it's only reachable through this
// dialog's event-code list.
export const NEW_REPORT_TYPES: { schemaName: string; label: string }[] = [
  { schemaName: "bunker-report", label: "Bunker Report" },
  { schemaName: "edn-report", label: "EDN Report" },
];

// Shared by Home's "Start a report" action and ReportsScreen's own "+ New
// report" button (2026-07-14 manual-test feedback: the Reports page
// picker only offered Bunker/EDN, leaving Log Abstract — the only schema
// with a cadence/event concept — reachable from Home alone). One dialog,
// two entry points, so neither can drift out of sync with the other on
// which report types are actually startable.
export function EventPickerDialog({
  open,
  eventTypes,
  suggested,
  onPick,
  onPickOtherReport,
  onClose,
}: {
  open: boolean;
  eventTypes: EventType[];
  /** Home passes its cadence-derived "likely next event" codes first; callers with no such concept (ReportsScreen) pass []. */
  suggested: string[];
  onPick: (code: string) => void;
  onPickOtherReport: (schemaName: string) => void;
  onClose: () => void;
}) {
  const rest = eventTypes.map((e) => e.code).filter((c) => !suggested.includes(c));
  const eventButton = (code: string) => (
    <button
      key={code}
      onClick={() => onPick(code)}
      className="md-body-medium"
      style={{
        padding: "8px 4px", border: "none", background: "none", color: "var(--color-on-surface)",
        textAlign: "left", cursor: "pointer", font: "inherit", borderRadius: "var(--shape-small)",
      }}
    >
      {code}
    </button>
  );
  const reportTypeButton = (s: { schemaName: string; label: string }) => (
    <button
      key={s.schemaName}
      onClick={() => onPickOtherReport(s.schemaName)}
      className="md-body-medium"
      style={{
        padding: "8px 4px", border: "none", background: "none", color: "var(--color-on-surface)",
        textAlign: "left", cursor: "pointer", font: "inherit", borderRadius: "var(--shape-small)",
      }}
    >
      {s.label}
    </button>
  );

  return (
    <Dialog open={open} title="Choose an event" onClose={onClose} actions={[{ label: "Cancel", onClick: onClose }]}>
      <div style={{ display: "flex", flexDirection: "column", gap: 4, maxHeight: 320, overflowY: "auto" }}>
        {suggested.map(eventButton)}
        {suggested.length > 0 && rest.length > 0 ? (
          <div style={{ borderTop: "1px solid var(--color-outline-variant)", margin: "6px 4px" }} />
        ) : null}
        {rest.map(eventButton)}
        <div className="md-label-small" style={{ color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em", borderTop: "1px solid var(--color-outline-variant)", margin: "6px 4px 0", paddingTop: 10 }}>
          Other reports
        </div>
        {NEW_REPORT_TYPES.map(reportTypeButton)}
      </div>
    </Dialog>
  );
}
