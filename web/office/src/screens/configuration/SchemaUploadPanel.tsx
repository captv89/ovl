// SPDX-License-Identifier: AGPL-3.0-only

import { useRef, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { api, type SchemaField, type SchemaUploadPreview } from "../../api/client";

function FieldList({ title, fields }: { title: string; fields: SchemaField[] }) {
  if (fields.length === 0) return null;
  return (
    <div>
      <div className="md-title-small">
        {title} ({fields.length})
      </div>
      <ul style={{ margin: "4px 0 0", paddingLeft: 20 }}>
        {fields.map((f) => (
          <li key={f.name} className="md-body-medium">
            {f.label} ({f.name})
          </li>
        ))}
      </ul>
    </div>
  );
}

function NameList({ title, names }: { title: string; names: string[] }) {
  if (names.length === 0) return null;
  return (
    <div>
      <div className="md-title-small">
        {title} ({names.length})
      </div>
      <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
        {names.join(", ")}
      </div>
    </div>
  );
}

// Design handoff B5's upload flow: upload -> meta-schema validation
// (hard stop with errors) -> diff review screen -> confirm and name the
// new version -> published (immutable from here). This is the mechanism
// the user described for carrying field-policy annotations forward
// across OVD revisions (B6's migration assistant consumes exactly the
// version this publishes).
export function SchemaUploadPanel({
  schemaName,
  onCancel,
  onPublished,
}: {
  schemaName: string;
  onCancel: () => void;
  onPublished: () => void;
}) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [content, setContent] = useState<string | null>(null);
  const [fileName, setFileName] = useState<string | null>(null);
  const [preview, setPreview] = useState<SchemaUploadPreview | null>(null);
  const [version, setVersion] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function handleFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setFileName(file.name);
    const reader = new FileReader();
    reader.onload = () => setContent(String(reader.result ?? ""));
    reader.readAsText(file);
  }

  async function runPreview() {
    if (!content) return;
    setBusy(true);
    setError(null);
    try {
      const result = await api.previewSchemaUpload(schemaName, content);
      setPreview(result);
      if (result.valid && result.parsedVersion) {
        setVersion(result.parsedVersion);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not validate the uploaded file.");
    } finally {
      setBusy(false);
    }
  }

  async function publish() {
    if (!content) return;
    setBusy(true);
    setError(null);
    try {
      await api.publishSchemaVersion(schemaName, version, content);
      onPublished();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not publish the new version.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16, maxWidth: 640 }}>
      <div>
        <Button variant="text" icon="arrow_back" onClick={onCancel}>
          Back to field policy
        </Button>
        <div className="md-headline-small" style={{ marginTop: 4 }}>
          Upload new version — {schemaName}
        </div>
      </div>

      {error ? <AlertBanner level="warning" title="Something went wrong" message={error} onDismiss={() => setError(null)} /> : null}

      <Card variant="outlined" style={{ padding: 20, display: "flex", flexDirection: "column", gap: 12 }}>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          Select the updated OVD schema JSON for {schemaName} (e.g. downloaded from the version list, edited
          for the new OVD revision).
        </div>
        <input ref={fileInputRef} type="file" accept="application/json" onChange={handleFile} />
        {fileName ? (
          <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
            Selected: {fileName}
          </div>
        ) : null}
        <div>
          <Button variant="filled" onClick={runPreview} disabled={!content || busy}>
            Validate and preview
          </Button>
        </div>
      </Card>

      {preview && !preview.valid ? (
        <AlertBanner level="warning" title="Validation failed" message={preview.error ?? "The document does not match the OVD meta-schema."} />
      ) : null}

      {preview?.valid ? (
        <Card variant="outlined" style={{ padding: 20, display: "flex", flexDirection: "column", gap: 16 }}>
          <div className="md-title-medium">Diff review</div>
          {preview.diff ? (
            preview.diff.added.length +
              preview.diff.removed.length +
              preview.diff.typeChanged.length +
              preview.diff.mandatorinessChanged.length +
              preview.diff.enumChanged.length ===
            0 ? (
              <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
                No differences from the current published version.
              </div>
            ) : (
              <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
                <FieldList title="Added" fields={preview.diff.added} />
                <FieldList title="Removed" fields={preview.diff.removed} />
                <NameList title="Type changed" names={preview.diff.typeChanged} />
                <NameList title="Mandatoriness changed" names={preview.diff.mandatorinessChanged} />
                <NameList title="Enum changed" names={preview.diff.enumChanged} />
              </div>
            )
          ) : (
            <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
              No current published version to compare against — this will be the first version.
            </div>
          )}

          <TextField label="Version name" value={version} onChange={setVersion} supportingText='e.g. "3.14" or "3.13-company-r2"' />
          <div>
            <Button variant="filled" onClick={publish} disabled={busy || !version.trim()}>
              Publish version
            </Button>
          </div>
        </Card>
      ) : null}
    </div>
  );
}
