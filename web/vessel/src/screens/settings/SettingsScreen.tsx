// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState, type ReactNode } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { Chip } from "../../design/components/surfaces/Chip.jsx";
import { Radio } from "../../design/components/forms/Radio.jsx";
import { Select } from "../../design/components/forms/Select.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { PageContainer } from "../PageContainer";
import { BackupSection } from "./BackupSection";
import { SensorSourceSection } from "./SensorSourceSection";
import { VMSSourceSection } from "./VMSSourceSection";
import { clearThemeOverride, currentTheme, hasThemeOverride, setTheme, subscribeTheme, type Theme } from "../../theme";
import { api, ApiError, type SetupStatus, type SyncStatus, type SyncSettings, type SystemInfo, type UserView } from "../../api/client";
import { formatUtc } from "../../format";

function SectionCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card variant="outlined" style={{ padding: "var(--space-5)" }}>
      <div className="md-title-medium" style={{ color: "var(--color-on-surface)", marginBottom: "var(--space-4)" }}>{title}</div>
      {children}
    </Card>
  );
}

function LabelValue({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ display: "flex", justifyContent: "space-between", gap: "var(--space-4)" }}>
      <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>{label}</span>
      <span className="md-body-medium" style={{ color: "var(--color-on-surface)", fontFamily: "var(--font-mono)", textAlign: "right" }}>{value}</span>
    </div>
  );
}

function Loading() {
  return <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>Loading…</span>;
}

// Preset choices for architecture 11.2's "configurable interval" —
// vessel/httpapi's syncIntervalMinSeconds/MaxSeconds (30s..24h) bound
// what's accepted server-side; these are just a sensible curated subset
// of that range rather than a free-text seconds field, matching this
// screen's existing preference for Radio/Select presets over raw number
// entry elsewhere (e.g. AppearanceSection). 30s and 1 minute are the
// "good connectivity, near real-time" end the 2026-07-20 manual-test
// feedback asked for; 4h/24h are the constrained-satellite-link end.
const SYNC_INTERVAL_PRESETS: { label: string; seconds: number }[] = [
  { label: "30 seconds (near real-time)", seconds: 30 },
  { label: "1 minute", seconds: 60 },
  { label: "5 minutes", seconds: 300 },
  { label: "15 minutes", seconds: 900 },
  { label: "30 minutes", seconds: 1800 },
  { label: "1 hour", seconds: 3600 },
  { label: "4 hours", seconds: 14400 },
  { label: "24 hours (limited connectivity)", seconds: 86400 },
];

function syncIntervalLabel(seconds: number): string {
  return SYNC_INTERVAL_PRESETS.find((p) => p.seconds === seconds)?.label ?? `${seconds}s`;
}

type AppearanceChoice = Theme | "system";

// Reads its selection off theme.ts on every render rather than owning
// its own copy of "which theme is active" — AppShell's top-bar toggle
// can change it from outside this component at any time (both are on
// screen together), so subscribeTheme is what keeps this in sync, not
// local state derived once.
function AppearanceSection() {
  const [, forceRender] = useState(0);
  useEffect(() => subscribeTheme(() => forceRender((n) => n + 1)), []);

  const choice: AppearanceChoice = hasThemeOverride() ? currentTheme() : "system";

  function choose(next: AppearanceChoice) {
    if (next === "system") clearThemeOverride();
    else setTheme(next);
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
      <Radio selected={choice === "light"} onChange={() => choose("light")} label="Light" />
      <Radio selected={choice === "dark"} onChange={() => choose("dark")} label="Dark (Night Bridge)" />
      <Radio selected={choice === "system"} onChange={() => choose("system")} label="Follow system" />
    </div>
  );
}

// Design handoff A10: Sync / Backup & restore / Appearance / Diagnostics,
// stacked on one page rather than a sectioned screen with its own
// internal nav — only four sections exist today, so a second-level rail
// would be overhead without enough content to justify it. Absorbs the
// old standalone Backup screen (admin/BackupScreen.tsx, now deleted) as
// its own section instead of a separate rail destination.
export function SettingsScreen({ user }: { user: UserView }) {
  const [status, setStatus] = useState<SetupStatus | null>(null);
  const [info, setInfo] = useState<SystemInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [syncStatus, setSyncStatus] = useState<SyncStatus | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [syncError, setSyncError] = useState<string | null>(null);
  const [syncSettings, setSyncSettings] = useState<SyncSettings | null>(null);
  const [savingInterval, setSavingInterval] = useState(false);
  const [intervalError, setIntervalError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [statusResult, infoResult, syncResult, syncSettingsResult] = await Promise.all([
          api.getSetupStatus(),
          api.getSystemInfo(),
          api.getSyncStatus(),
          api.getSyncSettings(),
        ]);
        if (cancelled) return;
        setStatus(statusResult);
        setInfo(infoResult);
        setSyncStatus(syncResult);
        setSyncSettings(syncSettingsResult);
      } catch (err) {
        if (!cancelled) setError(err instanceof ApiError ? err.message : "Could not load settings.");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  async function handleChangeSyncInterval(label: string) {
    const preset = SYNC_INTERVAL_PRESETS.find((p) => p.label === label);
    if (!preset) return;
    setSavingInterval(true);
    setIntervalError(null);
    try {
      setSyncSettings(await api.saveSyncSettings(preset.seconds));
    } catch (err) {
      setIntervalError(err instanceof ApiError ? err.message : "Could not save sync interval.");
    } finally {
      setSavingInterval(false);
    }
  }

  async function handleSyncNow() {
    setSyncing(true);
    setSyncError(null);
    try {
      setSyncStatus(await api.syncNow());
    } catch (err) {
      setSyncError(err instanceof ApiError ? err.message : "Sync failed.");
    } finally {
      setSyncing(false);
    }
  }

  return (
    <PageContainer title="Settings" width="narrow">
      {error ? <AlertBanner level="warning" title="Couldn't load settings" message={error} /> : null}

      <SectionCard title="Sync">
        {status ? (
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
            <LabelValue label="Mode" value={status.mode === "server" ? "Vessel server" : "Standalone"} />
            <LabelValue label="Office URL" value={status.enrollment.officeURL || "—"} />
            <div>
              <Chip label={status.enrollment.submitted ? "Enrolled" : "Not enrolled"} selected={status.enrollment.submitted} />
            </div>
            {syncStatus?.enrolled ? (
              <>
                <LabelValue label="Last synced" value={syncStatus.lastSuccess ? formatUtc(syncStatus.lastSuccess) : "Never"} />
                {syncError || syncStatus.lastError ? (
                  <div className="md-body-small" style={{ color: "var(--color-error)" }}>{syncError || syncStatus.lastError}</div>
                ) : null}
                <div>
                  <Button variant="outlined" size="small" disabled={syncing} onClick={() => void handleSyncNow()}>
                    {syncing ? "Syncing…" : "Sync now"}
                  </Button>
                </div>
              </>
            ) : (
              <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
                Sync starts once this vessel is enrolled with an office.
              </div>
            )}
            {syncSettings ? (
              user.role === "master" ? (
                <Select
                  label="Sync interval"
                  value={syncIntervalLabel(syncSettings.syncIntervalSeconds)}
                  options={SYNC_INTERVAL_PRESETS.map((p) => p.label)}
                  onChange={(label) => void handleChangeSyncInterval(label)}
                  disabled={savingInterval}
                  supportingText={intervalError ?? "How often the vessel syncs automatically in the background, on top of Sync now. Shorter is near real-time; lengthen it on a constrained satellite link."}
                  error={!!intervalError}
                />
              ) : (
                <LabelValue label="Sync interval" value={syncIntervalLabel(syncSettings.syncIntervalSeconds)} />
              )
            ) : null}
          </div>
        ) : (
          <Loading />
        )}
      </SectionCard>

      {user.role === "master" ? (
        <SectionCard title="Backup & restore">
          <BackupSection />
        </SectionCard>
      ) : null}

      {user.role === "master" ? (
        <SectionCard title="Sensor data source">
          <SensorSourceSection />
        </SectionCard>
      ) : null}

      {user.role === "master" ? (
        <SectionCard title="VMS data source">
          <VMSSourceSection />
        </SectionCard>
      ) : null}

      <SectionCard title="Appearance">
        <AppearanceSection />
      </SectionCard>

      <SectionCard title="Diagnostics">
        {info ? (
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
            <LabelValue label="Version" value={info.version} />
            <LabelValue label="Commit" value={info.commit} />
            <LabelValue label="Build date" value={info.date} />
            <div className="md-label-large" style={{ color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em", marginTop: "var(--space-2)" }}>
              Schemas
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
              {info.schemas.map((s) => (
                <div key={s.schemaName} style={{ display: "flex", justifyContent: "space-between", gap: "var(--space-4)" }}>
                  <span className="md-body-medium" style={{ color: "var(--color-on-surface)" }}>{s.schemaName}</span>
                  <span className="md-body-small" style={{ color: "var(--color-on-surface-variant)", fontFamily: "var(--font-mono)" }}>
                    v{s.version} · OVD {s.ovdVersion}
                  </span>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <Loading />
        )}
      </SectionCard>
    </PageContainer>
  );
}
