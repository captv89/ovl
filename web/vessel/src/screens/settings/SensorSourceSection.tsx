// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Switch } from "../../design/components/forms/Switch.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { api, ApiError, type SensorSourceTestResult, type SensorSourceView } from "../../api/client";

// 18.07.26 manual-test items 4/9: Master-only config for the vessel's
// onboard sensor-data REST service (the decided architecture — vessel
// pulls, see vessel/sensorclient's own doc comment). Mirrors
// BackupSection.tsx's own shape (its own SectionCard in
// SettingsScreen.tsx, Master-gated the same way). apiKey always arrives
// masked from the server (vessel/httpapi's sensorSourceView) — this
// screen never shows the real value back, only accepts a fresh one to
// change it.
export function SensorSourceSection() {
  const [source, setSource] = useState<SensorSourceView | null>(null);
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [enabled, setEnabled] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<SensorSourceTestResult | null>(null);

  useEffect(() => {
    void api
      .getSensorSource()
      .then((s) => {
        setSource(s);
        setBaseUrl(s.baseUrl);
        setEnabled(s.enabled);
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : "Could not load the sensor source."));
  }, []);

  async function handleSave() {
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      const result = await api.saveSensorSource({ baseUrl, apiKey, enabled });
      setSource(result);
      setApiKey("");
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save the sensor source.");
    } finally {
      setSaving(false);
    }
  }

  async function handleTest() {
    setTesting(true);
    setError(null);
    setTestResult(null);
    try {
      setTestResult(await api.testSensorSource({ baseUrl, apiKey }));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not test the connection.");
    } finally {
      setTesting(false);
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}>
      <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
        Configures the onboard sensor-data service the report form's "Fetch sensor data" button queries. The vessel
        pulls readings from this URL for a time window — nothing is ever pushed to the vessel unsolicited.
      </div>

      {error ? <AlertBanner level="warning" title="Sensor source" message={error} /> : null}
      {saved ? <AlertBanner level="ok" title="Saved" message="The sensor source config was updated." onDismiss={() => setSaved(false)} /> : null}
      {testResult ? (
        <AlertBanner
          level={testResult.ok ? "ok" : "warning"}
          title="Test connection"
          message={testResult.message}
          onDismiss={() => setTestResult(null)}
        />
      ) : null}

      <TextField label="Base URL" value={baseUrl} onChange={setBaseUrl} placeholder="https://sensors.example.com" style={{ width: "100%" }} />
      <TextField
        label={source?.configured ? `API key (currently ${source.apiKey})` : "API key"}
        value={apiKey}
        onChange={setApiKey}
        type="password"
        placeholder={source?.configured ? "Enter a new key to change it" : ""}
        style={{ width: "100%" }}
      />
      <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)" }}>
        <Switch checked={enabled} onChange={setEnabled} />
        <span className="md-body-medium">{enabled ? "Enabled" : "Disabled"}</span>
      </div>

      <div style={{ display: "flex", gap: "var(--space-3)" }}>
        <Button
          variant="filled"
          disabled={saving || !baseUrl || (!source?.configured && !apiKey)}
          onClick={() => void handleSave()}
        >
          {saving ? "Saving…" : "Save"}
        </Button>
        <Button
          variant="outlined"
          disabled={testing || !baseUrl || (!source?.configured && !apiKey)}
          onClick={() => void handleTest()}
        >
          {testing ? "Testing…" : "Test connection"}
        </Button>
      </div>
    </div>
  );
}
