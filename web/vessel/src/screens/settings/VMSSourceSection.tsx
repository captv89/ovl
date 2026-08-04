// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Switch } from "../../design/components/forms/Switch.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { api, ApiError, type VMSSourceTestResult, type VMSSourceView } from "../../api/client";

// Sensor+VMS stub expansion design doc: Master-only config for the
// vessel's VMS (voyage management system) reference-data REST service.
// Mirrors SensorSourceSection.tsx's own shape exactly, but is a wholly
// separate config — independent configure/enable/fail states from the
// sensor source, per that doc's own "separate, not merged" decision.
// apiKey always arrives masked from the server (vessel/httpapi's
// vmsSourceView) — this screen never shows the real value back, only
// accepts a fresh one to change it.
export function VMSSourceSection() {
  const [source, setSource] = useState<VMSSourceView | null>(null);
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [enabled, setEnabled] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<VMSSourceTestResult | null>(null);

  useEffect(() => {
    void api
      .getVMSSource()
      .then((s) => {
        setSource(s);
        setBaseUrl(s.baseUrl);
        setEnabled(s.enabled);
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : "Could not load the VMS source."));
  }, []);

  async function handleSave() {
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      const result = await api.saveVMSSource({ baseUrl, apiKey, enabled });
      setSource(result);
      setApiKey("");
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save the VMS source.");
    } finally {
      setSaving(false);
    }
  }

  async function handleTest() {
    setTesting(true);
    setError(null);
    setTestResult(null);
    try {
      setTestResult(await api.testVMSSource({ baseUrl, apiKey }));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not test the connection.");
    } finally {
      setTesting(false);
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}>
      <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
        Configures the VMS (voyage management system) service the report form's "Fetch voyage data" button queries.
        The vessel pulls voyage plan and cargo manifest data from this URL — nothing is ever pushed to the vessel
        unsolicited.
      </div>

      {error ? <AlertBanner level="warning" title="VMS source" message={error} /> : null}
      {saved ? <AlertBanner level="ok" title="Saved" message="The VMS source config was updated." onDismiss={() => setSaved(false)} /> : null}
      {testResult ? (
        <AlertBanner
          level={testResult.ok ? "ok" : "warning"}
          title="Test connection"
          message={testResult.message}
          onDismiss={() => setTestResult(null)}
        />
      ) : null}

      <TextField label="Base URL" value={baseUrl} onChange={setBaseUrl} placeholder="https://vms.example.com" style={{ width: "100%" }} />
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
