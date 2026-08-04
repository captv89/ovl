// SPDX-License-Identifier: AGPL-3.0-only

import { Card } from "../../design/components/surfaces/Card.jsx";
import { ProgressIndicator } from "../../design/components/feedback/ProgressIndicator.jsx";
import { DataReadout } from "../../design/components/maritime/DataReadout.jsx";
import type { VoyageSummary } from "../../api/client";
import { formatUtc } from "../../format";

// Design handoff A3 doesn't spec this card — it's new scope the user asked
// for directly: a landing-page voyage summary, entirely derived from fields
// already captured on submitted/draft reports (see vessel/httpapi/voyage.go)
// rather than a manually maintained entity, so it can never drift from what
// was actually reported. Text pieces (ports, ETA, departed, position) degrade
// honestly when their source field hasn't been entered yet — no fabricated
// value. The progress bar + ship marker are the one deliberate exception
// (2026-07-13 feedback): they always render, defaulting to 0% (ship docked
// at the departure end) rather than disappearing whenever Distance/
// Distance_To_Go haven't been reported yet — the wireframe's own "always
// show this" intent, and a 0% default is still honest (it never claims
// progress that hasn't happened).
export function VoyageCard({ voyage }: { voyage: VoyageSummary | null }) {
  if (!voyage || !voyage.voyageNumber) {
    return (
      <Card variant="outlined" style={{ padding: 20, flex: 1, minWidth: 280 }}>
        <div
          className="md-label-large"
          style={{ color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em", marginBottom: 4 }}
        >
          Voyage
        </div>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          Voyage details will appear here once a report carries a voyage number.
        </div>
      </Card>
    );
  }

  // Degrades to a static 0%-filled bar with the ship docked at the left
  // edge when the underlying Distance/Distance_To_Go fields haven't been
  // reported yet, rather than hiding the whole progress visual (2026-07-13
  // feedback: "even if the required figures are not available show the
  // bar... you can't go wrong this way"). Passing `null` through to
  // ProgressIndicator would render its *indeterminate* loading animation
  // instead — explicitly defaulting to 0 here avoids that.
  const progressPercent = voyage.progressPercent ?? 0;

  return (
    <Card variant="elevated" style={{ padding: 24, flex: 1, minWidth: 280 }}>
      <div
        className="md-label-large"
        style={{ color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em", marginBottom: 12 }}
      >
        Voyage {voyage.voyageNumber}
      </div>

      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 16 }}>
        <div>
          <div className="md-title-large" style={{ color: "var(--color-on-surface)" }}>{voyage.fromPort || "—"}</div>
          {voyage.departedAt ? (
            <div className="md-label-medium" style={{ color: "var(--color-on-surface-variant)", fontFamily: "var(--font-mono)", marginTop: 2 }}>
              Departed {formatUtc(voyage.departedAt)}
            </div>
          ) : null}
        </div>
        <div style={{ textAlign: "right" }}>
          <div className="md-title-large" style={{ color: "var(--color-on-surface)" }}>{voyage.toPort || "—"}</div>
          {voyage.eta ? (
            <div className="md-label-medium" style={{ color: "var(--color-on-surface-variant)", fontFamily: "var(--font-mono)", marginTop: 2 }}>
              ETA {formatUtc(voyage.eta)}
            </div>
          ) : null}
        </div>
      </div>

      <div style={{ marginTop: 24 }}>
        <div style={{ position: "relative" }}>
          <ProgressIndicator value={progressPercent} />
          <span
            aria-hidden="true"
            className="material-symbols-rounded"
            style={{
              position: "absolute",
              top: "50%",
              left: `${progressPercent}%`,
              transform: "translate(-50%, -50%)",
              width: 24,
              height: 24,
              borderRadius: "var(--shape-full)",
              background: "var(--color-primary)",
              color: "var(--color-on-primary)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              fontSize: 16,
            }}
          >
            directions_boat
          </span>
        </div>
        <div style={{ display: "flex", justifyContent: "space-between", marginTop: 6 }}>
          <span className="md-label-small" style={{ color: "var(--color-on-surface-variant)" }}>
            {voyage.distanceSailedNm != null ? `${voyage.distanceSailedNm.toFixed(0)} NM sailed` : "—"}
          </span>
          <span className="md-label-small" style={{ color: "var(--color-on-surface-variant)" }}>
            {voyage.distanceRemainingNm != null ? `${voyage.distanceRemainingNm.toFixed(0)} NM remaining` : "—"}
          </span>
        </div>
      </div>

      {voyage.position ? (
        <div style={{ marginTop: 20 }}>
          <DataReadout label="Position" value={voyage.position.text} />
        </div>
      ) : null}
    </Card>
  );
}
