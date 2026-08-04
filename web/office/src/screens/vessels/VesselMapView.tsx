// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useMemo, useRef, useState } from "react";
import { MapContainer, TileLayer, CircleMarker, Tooltip, useMapEvents } from "react-leaflet";
import type { Map as LeafletMap } from "leaflet";
import "leaflet/dist/leaflet.css";
import { Button } from "../../design/components/core/Button.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { Chip } from "../../design/components/surfaces/Chip.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { api, ApiError, type VesselPositionView } from "../../api/client";

// Design handoff B2·M's fleet map — a view toggle inside Vessels (not a
// separate nav destination), built from Tideline's own "Fleet Map
// Landing" template (`templates/fleet-map-landing`): a real Leaflet map
// with a floating search/filter/vessel-list card, a floating alerts
// card, and a detail card anchored to whichever marker is selected. The
// template's own left nav rail + standalone theme toggle are dropped —
// AppShell already supplies both app-wide, so duplicating them here
// would just be two rails and two toggles on screen at once. Marker
// color keeps the same fixed green/amber/red status language B1's KPI
// tiles and B3/B4's state badges already use: red overdue, amber a
// remarked report still awaiting review, green otherwise — the
// template's heading-triangle markers aren't reproduced since OVL has
// no course-over-ground field to point one (see vesselpositions.go's
// own doc comment: lat/lon/status/asOf is all the backend has).
const STATUS_COLOR: Record<VesselPositionView["status"], string> = {
  overdue: "#e53935",
  remarked: "#f0ad4e",
  ok: "#3fa34d",
};

const STATUS_OPTIONS: { key: "all" | VesselPositionView["status"]; label: string }[] = [
  { key: "all", label: "All" },
  { key: "ok", label: "Underway / OK" },
  { key: "remarked", label: "Remarked" },
  { key: "overdue", label: "Overdue" },
];

const ALERT_LEVEL: Partial<Record<VesselPositionView["status"], "warning" | "caution">> = {
  overdue: "warning",
  remarked: "caution",
};

const ALERT_MESSAGE: Partial<Record<VesselPositionView["status"], string>> = {
  overdue: "No report received within the configured cadence — overdue.",
  remarked: "Has a remarked report still awaiting review.",
};

// Anchors the detail card to the selected marker's live screen position,
// re-measuring on pan/zoom/resize — same imperative-pixel-math approach
// as the template (it's genuinely live layout math, not something a
// themeable/static style can express).
function AnchorSync({ selectedId, positions, onAnchor }: { selectedId: string | null; positions: VesselPositionView[]; onAnchor: (pt: { x: number; y: number } | null) => void }) {
  const map = useMapEvents({
    move: recalc,
    zoom: recalc,
    resize: recalc,
  });

  function recalc() {
    const p = positions.find((v) => v.vesselId === selectedId);
    if (!p) {
      onAnchor(null);
      return;
    }
    const pt = map.latLngToContainerPoint([p.lat, p.lon]);
    onAnchor({ x: pt.x, y: pt.y });
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(recalc, [selectedId, positions, map]);
  return null;
}

// Tries right/left/below/above the marker (offset by `gap` so the
// marker itself is never covered), picks whichever overlaps the two
// side panels least, then clamps to the map bounds as a last resort.
function computeAnchorStyle(
  anchor: { x: number; y: number } | null,
  containerSize: { width: number; height: number } | null,
): { left: number; top: number } | null {
  if (!anchor || !containerSize) return null;
  const cardW = 280;
  const cardH = 200;
  const gap = 24;
  const margin = 16;
  const leftPanelX1 = 352; // 16 inset + 320 card width + 16 gap
  const rightPanelX0Offset = 296; // panel width + inset, measured from the right edge
  const { x, y } = anchor;
  const { width: w, height: h } = containerSize;
  const rightPanelX0 = w - rightPanelX0Offset;
  const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(v, hi));

  const candidates = [
    { left: x + gap, top: y - cardH / 2 },
    { left: x - gap - cardW, top: y - cardH / 2 },
    { left: x - cardW / 2, top: y + gap },
    { left: x - cardW / 2, top: y - gap - cardH },
  ].map((c) => ({
    left: clamp(c.left, margin, Math.max(margin, w - cardW - margin)),
    top: clamp(c.top, margin, Math.max(margin, h - cardH - margin)),
  }));

  let best = candidates[0];
  let bestScore = Infinity;
  for (const c of candidates) {
    const leftOverlap = Math.max(0, Math.min(c.left + cardW, leftPanelX1) - Math.max(c.left, 0));
    const rightOverlap = Math.max(0, Math.min(c.left + cardW, w) - Math.max(c.left, rightPanelX0));
    const score = leftOverlap + rightOverlap;
    if (score < bestScore) {
      bestScore = score;
      best = c;
    }
  }
  return best;
}

export function VesselMapView({
  globalGroup,
  onOpenVessel,
  onBackToList,
}: {
  globalGroup: string | null;
  onOpenVessel: (id: string) => void;
  onBackToList: () => void;
}) {
  const [positions, setPositions] = useState<VesselPositionView[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | VesselPositionView["status"]>("all");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [dismissedAlerts, setDismissedAlerts] = useState<Set<string>>(new Set());
  const [anchorPt, setAnchorPt] = useState<{ x: number; y: number } | null>(null);
  const [containerSize, setContainerSize] = useState<{ width: number; height: number } | null>(null);
  const mapRef = useRef<LeafletMap | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    let cancelled = false;
    setPositions(null);
    setError(null);
    setSelectedId(null);
    api
      .listVesselPositions(globalGroup)
      .then((list) => {
        // Defensive against a null array from the backend — see
        // Dashboard.tsx's own comment on the same class of bug.
        if (!cancelled) setPositions(list ?? []);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : "Could not load vessel positions.");
      });
    return () => {
      cancelled = true;
    };
  }, [globalGroup]);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const measure = () => setContainerSize({ width: el.clientWidth, height: el.clientHeight });
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [positions]);

  // Centered on the fleet's own average position once positions load, so
  // a fleet operating in (say) the Baltic doesn't open on a default
  // mid-Atlantic view — falls back to a whole-world view when there's
  // nothing to center on yet.
  const center = useMemo((): [number, number] => {
    if (!positions || positions.length === 0) return [20, 0];
    const lat = positions.reduce((sum, p) => sum + p.lat, 0) / positions.length;
    const lon = positions.reduce((sum, p) => sum + p.lon, 0) / positions.length;
    return [lat, lon];
  }, [positions]);

  const filtered = useMemo(() => {
    if (!positions) return [];
    const q = search.trim().toLowerCase();
    return positions.filter((p) => {
      const matchesQ = !q || p.vesselName.toLowerCase().includes(q) || p.vesselImo.toLowerCase().includes(q);
      const matchesStatus = statusFilter === "all" || p.status === statusFilter;
      return matchesQ && matchesStatus;
    });
  }, [positions, search, statusFilter]);

  const alerts = useMemo(
    () => (positions ?? []).filter((p) => ALERT_LEVEL[p.status] && !dismissedAlerts.has(p.vesselId)),
    [positions, dismissedAlerts],
  );

  const selected = positions?.find((p) => p.vesselId === selectedId) ?? null;
  const anchorStyle = computeAnchorStyle(anchorPt, containerSize);

  function selectVessel(p: VesselPositionView) {
    setSelectedId(p.vesselId);
    const map = mapRef.current;
    if (map) map.flyTo([p.lat, p.lon], Math.max(map.getZoom(), 8), { duration: 0.6 });
  }

  if (error) {
    return (
      <div style={{ padding: 24 }}>
        <Button variant="text" icon="arrow_back" onClick={onBackToList}>Back to vessels</Button>
        <AlertBanner level="warning" title="Couldn't load vessel positions" message={error} />
      </div>
    );
  }

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16, height: "100%", boxSizing: "border-box" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <div className="md-headline-small">Vessels · Map</div>
        <Button variant="outlined" icon="list" onClick={onBackToList}>List</Button>
      </div>

      {positions === null ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>Loading…</div>
      ) : positions.length === 0 ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          No vessels have a plottable position yet — a vessel appears here once one of its Log Abstract
          reports carries a Position (Latitude/Longitude).
        </div>
      ) : (
        <div ref={containerRef} style={{ position: "relative", flex: 1, minHeight: 480, borderRadius: "var(--shape-medium)", overflow: "hidden", border: "1px solid var(--color-outline-variant)" }}>
          <MapContainer ref={mapRef} center={center} zoom={4} style={{ width: "100%", height: "100%" }}>
            <TileLayer
              attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
              url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
            />
            {filtered.map((p) => (
              <CircleMarker
                key={p.vesselId}
                center={[p.lat, p.lon]}
                radius={9}
                pathOptions={{
                  color: p.vesselId === selectedId ? "var(--color-primary)" : "#fff",
                  weight: p.vesselId === selectedId ? 3 : 2,
                  fillColor: STATUS_COLOR[p.status],
                  fillOpacity: 1,
                }}
                eventHandlers={{ click: () => selectVessel(p) }}
              >
                <Tooltip direction="top" offset={[0, -8]}>
                  <strong>{p.vesselName}</strong>
                </Tooltip>
              </CircleMarker>
            ))}
            <AnchorSync selectedId={selectedId} positions={positions} onAnchor={setAnchorPt} />
          </MapContainer>

          {/* Top-left: fleet overview (search + status filter) and vessel list */}
          <div style={{ position: "absolute", top: 16, left: 16, bottom: 16, width: 320, zIndex: 1000, display: "flex", flexDirection: "column", gap: 12, pointerEvents: "none" }}>
            <div style={{ pointerEvents: "auto", flexShrink: 0 }}>
              <Card variant="elevated" style={{ boxShadow: "var(--elevation-2)" }}>
                <div className="md-title-large" style={{ fontFamily: "var(--font-brand)", color: "var(--color-on-surface)" }}>Fleet Overview</div>
                <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", marginTop: 2 }}>{filtered.length} vessels tracked</div>
                <div style={{ marginTop: 10 }}>
                  <TextField label="Search vessels" value={search} onChange={setSearch} variant="outlined" leadingIcon="search" />
                </div>
                <div style={{ display: "flex", gap: 8, marginTop: 10, flexWrap: "wrap" }}>
                  {STATUS_OPTIONS.map((opt) => (
                    <Chip key={opt.key} label={opt.label} type="filter" selected={statusFilter === opt.key} onClick={() => setStatusFilter(opt.key)} />
                  ))}
                </div>
              </Card>
            </div>

            <div style={{ pointerEvents: "auto", flex: 1, minHeight: 0, overflow: "auto" }}>
              <Card variant="outlined" style={{ boxShadow: "var(--elevation-2)", maxHeight: "100%", overflow: "auto" }}>
                {filtered.map((p) => (
                  <div
                    key={p.vesselId}
                    onClick={() => selectVessel(p)}
                    style={{
                      position: "relative", padding: "10px 12px", borderRadius: "var(--shape-small)", cursor: "pointer", marginBottom: 2,
                      background: p.vesselId === selectedId ? "var(--color-secondary-container)" : "transparent",
                    }}
                  >
                    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
                      <span className="md-title-small" style={{ color: "var(--color-on-surface)" }}>{p.vesselName}</span>
                      <StatusDot color={STATUS_COLOR[p.status]} />
                    </div>
                    <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", marginTop: 4 }}>
                      {p.vesselImo} &middot; {new Date(p.asOf).toLocaleString()}
                    </div>
                  </div>
                ))}
                {filtered.length === 0 ? (
                  <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", padding: 16, textAlign: "center" }}>
                    No vessels match this search.
                  </div>
                ) : null}
              </Card>
            </div>
          </div>

          {/* Top-right: alerts */}
          <div style={{ position: "absolute", top: 16, right: 16, width: 280, zIndex: 1000 }}>
            <Card variant="elevated" style={{ boxShadow: "var(--elevation-2)" }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <span className="md-title-small" style={{ color: "var(--color-on-surface)" }}>Alerts</span>
                <span className="md-label-medium" style={{ color: "var(--color-on-surface-variant)" }}>{alerts.length} active</span>
              </div>
              <div style={{ display: "flex", flexDirection: "column", gap: 8, marginTop: 10 }}>
                {alerts.map((p) => (
                  <AlertBanner
                    key={p.vesselId}
                    level={ALERT_LEVEL[p.status]}
                    title={p.vesselName}
                    message={ALERT_MESSAGE[p.status] ?? ""}
                    onDismiss={() => setDismissedAlerts((prev) => new Set(prev).add(p.vesselId))}
                  />
                ))}
                {alerts.length === 0 ? (
                  <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>No active alerts.</div>
                ) : null}
              </div>
            </Card>
          </div>

          {/* Anchored to the selected marker's live screen position */}
          {selected && anchorStyle ? (
            <div style={{ position: "absolute", left: anchorStyle.left, top: anchorStyle.top, width: 280, zIndex: 1000 }}>
              <Card variant="elevated" style={{ boxShadow: "var(--elevation-3)" }}>
                <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between" }}>
                  <div>
                    <div className="md-title-medium" style={{ fontFamily: "var(--font-brand)", color: "var(--color-on-surface)" }}>{selected.vesselName}</div>
                    <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", marginTop: 2 }}>IMO {selected.vesselImo}</div>
                  </div>
                  <Button variant="text" icon="close" onClick={() => setSelectedId(null)} style={{ minWidth: 0, padding: 4 }} aria-label="Close">{""}</Button>
                </div>
                <div style={{ display: "flex", alignItems: "center", gap: 6, marginTop: 10 }}>
                  <StatusDot color={STATUS_COLOR[selected.status]} />
                  <span className="md-body-medium">{STATUS_OPTIONS.find((o) => o.key === selected.status)?.label ?? selected.status}</span>
                </div>
                <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", marginTop: 8 }}>
                  Last position {new Date(selected.asOf).toLocaleString()}
                </div>
                <div style={{ marginTop: 12 }}>
                  <Button variant="outlined" onClick={() => onOpenVessel(selected.vesselId)}>Open vessel</Button>
                </div>
              </Card>
            </div>
          ) : null}
        </div>
      )}
    </div>
  );
}

function StatusDot({ color }: { color: string }) {
  return <span style={{ width: 10, height: 10, borderRadius: "50%", background: color, display: "inline-block", flexShrink: 0 }} />;
}
