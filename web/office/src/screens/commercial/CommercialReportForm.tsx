// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useMemo, useState } from "react";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Checkbox } from "../../design/components/forms/Checkbox.jsx";
import { Select } from "../../design/components/forms/Select.jsx";
import { Button } from "../../design/components/core/Button.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { api, ApiError, type FindingView, type SchemaDetail, type SchemaField, type VesselView } from "../../api/client";

type FieldValue = string | boolean;

// Same raw-string-to-JSON-type coercion as web/vessel's own
// reportPersistence.ts (apps don't share code — see that file's own
// precedent for vendored-copy duplication across vessel/office), plus
// one wrinkle vessel's own coercion never needed: no vessel-submitted
// schema renders a raw dateTime-typed field directly (Log Abstract
// splits Date_UTC/Time_UTC instead — see reportPersistence.ts's own
// DATE_TIME_PAIRS), so nothing there has ever had to reconcile the
// native <input type="datetime-local">'s ISO "T"-separated value
// against pkg/validation.dateTimeLayout's OVD-3.13-matching
// "yyyy-mm-dd hh:mm" (space) format — commercial-period's Period_Start/
// Period_End are the first fields to exercise that path.
function coerceFieldValue(field: SchemaField, raw: FieldValue | undefined): unknown {
  if (raw === undefined) return null;
  if (field.type === "boolean") return raw;
  if (typeof raw === "string") {
    if (field.type === "wholeNumber" || field.type === "decimal") {
      if (raw.trim() === "") return null;
      const n = Number(raw);
      return Number.isNaN(n) ? raw : n;
    }
    if (raw.trim() === "") return null;
    if (field.type === "dateTime") return raw.replace("T", " ");
    return raw;
  }
  return raw;
}

function nativeInputType(field: SchemaField): "text" | "number" | "date" | "time" | "datetime-local" {
  switch (field.type) {
    case "wholeNumber":
    case "decimal":
      return "number";
    case "date":
      return "date";
    case "time":
      return "time";
    case "dateTime":
      return "datetime-local";
    // enum has no office-side /api/enums/{name} endpoint the way vessel
    // does (design handoff B8, v1 scope cut, see PROJECT.md) — falls
    // through to a plain text field like everything else untyped.
    default:
      return "text";
  }
}

function vesselOptionLabel(v: VesselView): string {
  return `${v.imo} — ${v.name}`;
}

// Design handoff B8's "single-page schema-driven form... with a health
// check before ready to push" — schema-driven the same way vessel's own
// ReportForm is, but deliberately much smaller: no drafts, no sections,
// no computed/prefill fields, since office-authored commercial reports
// are a one-shot submit (see office/httpapi/commercial.go's own doc
// comment on why nothing persists on a failed health check).
export function CommercialReportForm({
  schemaName,
  vessels,
  onCreated,
  onCancel,
}: {
  schemaName: "commercial-period" | "cargo-nomination";
  vessels: VesselView[];
  onCreated: () => void;
  onCancel: () => void;
}) {
  const [schema, setSchema] = useState<SchemaDetail | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [vesselId, setVesselId] = useState("");
  const [values, setValues] = useState<Record<string, FieldValue>>({});
  const [findings, setFindings] = useState<FindingView[]>([]);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    api
      .listLatestSchemaVersions()
      .then((list) => {
        const summary = list.find((s) => s.schemaName === schemaName);
        if (!summary) throw new Error(`No published schema for ${schemaName} yet.`);
        return api.getSchemaVersion(summary.schemaName, summary.version);
      })
      .then((detail) => {
        if (!cancelled) setSchema(detail);
      })
      .catch((err) => {
        if (!cancelled) setLoadError(err instanceof Error ? err.message : "Could not load this form's schema.");
      });
    return () => {
      cancelled = true;
    };
  }, [schemaName]);

  const findingByField = useMemo(() => {
    const map: Record<string, FindingView> = {};
    for (const f of findings) {
      if (!f.field) continue;
      const existing = map[f.field];
      if (!existing || (f.severity === "error" && existing.severity !== "error")) map[f.field] = f;
    }
    return map;
  }, [findings]);

  function setValue(name: string, value: FieldValue) {
    setValues((prev) => ({ ...prev, [name]: value }));
  }

  // Mirrors vessel/httpapi's own IMO carryForward (schemas.go's
  // hasIMOField/prefill["IMO"]) — every curated schema carries an IMO
  // field, and the officer already named the vessel via the picker
  // above, so re-typing the same number into a second, easy-to-miss
  // "Vessel" field (the schema's own label for IMO) would just be a
  // data-entry footgun. Still a normal editable field afterward, same
  // as vessel's own carryForward class.
  function selectVessel(id: string) {
    setVesselId(id);
    const v = vessels.find((vessel) => vessel.id === id);
    if (v) setValue("IMO", v.imo);
  }

  async function handleSubmit() {
    if (!schema || !vesselId) return;
    setBusy(true);
    setSubmitError(null);
    setFindings([]);
    try {
      const fields: Record<string, unknown> = {};
      for (const f of schema.fields) {
        const coerced = coerceFieldValue(f, values[f.name]);
        if (coerced !== null) fields[f.name] = coerced;
      }
      const resp = await api.createCommercialReport(schemaName, vesselId, fields);
      if (resp.report) {
        onCreated();
      } else {
        setFindings(resp.findings);
      }
    } catch (err) {
      setSubmitError(err instanceof ApiError ? err.message : "Could not submit this report.");
    } finally {
      setBusy(false);
    }
  }

  if (loadError) {
    return <AlertBanner level="warning" title="Couldn't load form" message={loadError} />;
  }
  if (!schema) {
    return (
      <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
        Loading form…
      </div>
    );
  }

  const selectedVessel = vessels.find((v) => v.id === vesselId);
  const errorCount = findings.filter((f) => f.severity === "error").length;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      {submitError ? <AlertBanner level="warning" title="Couldn't submit" message={submitError} onDismiss={() => setSubmitError(null)} /> : null}
      {errorCount > 0 ? (
        <AlertBanner
          level="warning"
          title="Health check found issues"
          message={`${errorCount} error${errorCount === 1 ? "" : "s"} must be fixed before this can be submitted.`}
        />
      ) : null}

      <Select
        label="Vessel"
        value={selectedVessel ? vesselOptionLabel(selectedVessel) : ""}
        options={vessels.map(vesselOptionLabel)}
        onChange={(label) => selectVessel(vessels.find((v) => vesselOptionLabel(v) === label)?.id ?? "")}
        placeholder="Select a vessel…"
      />

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))", gap: 16 }}>
        {schema.fields.map((field) => {
          const finding = findingByField[field.name];
          const value = values[field.name];
          if (field.type === "boolean") {
            return (
              <Checkbox key={field.name} label={field.label} checked={value === true} onChange={(v) => setValue(field.name, v)} />
            );
          }
          return (
            <TextField
              key={field.name}
              label={field.label}
              type={nativeInputType(field)}
              value={typeof value === "string" ? value : ""}
              onChange={(v) => setValue(field.name, v)}
              error={finding?.severity === "error"}
              warning={finding?.severity === "warning"}
              supportingText={finding?.message}
              suffix={field.unit}
              policyOutline={field.schemaMandatory ? "mandatory" : null}
              style={{ width: "100%" }}
            />
          );
        })}
      </div>

      <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
        <Button variant="text" onClick={onCancel} disabled={busy}>
          Cancel
        </Button>
        <Button variant="filled" onClick={() => void handleSubmit()} disabled={busy || !vesselId}>
          {busy ? "Submitting…" : "Submit"}
        </Button>
      </div>
    </div>
  );
}
