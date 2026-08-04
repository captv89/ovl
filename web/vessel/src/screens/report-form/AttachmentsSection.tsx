// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useRef, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Dialog } from "../../design/components/feedback/Dialog.jsx";
import { api, ApiError, type AttachmentView } from "../../api/client";
import { formatUtc } from "../../format";
import { formatBytes, processAttachmentFile } from "./attachmentProcessing";

const ACCEPT = "image/*,application/pdf";

// AttachmentsSection is design handoff A5·B's Bunker/EDN-only section:
// "capture or pick file, client-side compression progress, thumbnail
// grid, per-file preview and remove. Show final compressed size
// ('1.2 MB → 340 KB')." Rendered by SectionPanel when sectionKey ===
// "attachments" — a pseudo-section with no schema fields of its own
// (Phase 6), same special-case pattern as the weather section's
// WeatherVane widget.
export function AttachmentsSection({ reportId, locked }: { reportId: string | null; locked: boolean }) {
  const [attachments, setAttachments] = useState<AttachmentView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [uploading, setUploading] = useState<string | null>(null);
  const [preview, setPreview] = useState<AttachmentView | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!reportId) {
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    api
      .listAttachments(reportId)
      .then((list) => {
        if (!cancelled) setAttachments(list);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : "Could not load attachments.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [reportId]);

  async function handleFiles(files: FileList | null) {
    if (!files || !reportId) return;
    setError(null);
    for (const file of Array.from(files)) {
      setUploading(file.name);
      try {
        const processed = await processAttachmentFile(file);
        const uploaded = await api.uploadAttachment(reportId, processed.blob, processed.filename);
        setAttachments((prev) => [...prev, uploaded]);
        if (processed.warning) setError(processed.warning);
      } catch (err) {
        setError(err instanceof ApiError ? err.message : err instanceof Error ? err.message : "Could not attach this file.");
      } finally {
        setUploading(null);
      }
    }
  }

  async function handleRemove(a: AttachmentView) {
    if (!reportId) return;
    setError(null);
    try {
      await api.deleteAttachment(reportId, a.id);
      setAttachments((prev) => prev.filter((x) => x.id !== a.id));
      setPreview(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not remove this attachment.");
    }
  }

  if (!reportId) {
    return <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>Save this report before adding attachments.</div>;
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}>
      {error ? <AlertBanner level="warning" title="Attachments" message={error} onDismiss={() => setError(null)} /> : null}

      {!locked ? (
        <div>
          <input
            ref={inputRef}
            type="file"
            accept={ACCEPT}
            multiple
            style={{ display: "none" }}
            onChange={(e) => {
              void handleFiles(e.target.files);
              e.target.value = "";
            }}
          />
          <Button variant="outlined" icon="attach_file" disabled={uploading !== null} onClick={() => inputRef.current?.click()}>
            {uploading ? `Processing ${uploading}…` : "Add attachment"}
          </Button>
        </div>
      ) : null}

      {loading ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>Loading…</div>
      ) : attachments.length === 0 ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>No attachments yet.</div>
      ) : (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(140px, 1fr))", gap: "var(--space-4)" }}>
          {attachments.map((a) => (
            <button
              key={a.id}
              onClick={() => setPreview(a)}
              style={{
                display: "flex", flexDirection: "column", gap: 4, padding: 0, cursor: "pointer",
                border: "1px solid var(--color-outline-variant)", borderRadius: "var(--shape-medium)",
                background: "var(--color-surface-container-low)", overflow: "hidden", textAlign: "left", font: "inherit",
              }}
            >
              <div style={{ height: 100, display: "flex", alignItems: "center", justifyContent: "center", background: "var(--color-surface-container-highest)" }}>
                {a.contentType.startsWith("image/") ? (
                  <img src={api.attachmentDownloadUrl(reportId, a.id)} alt={a.filename} style={{ width: "100%", height: "100%", objectFit: "cover" }} />
                ) : (
                  <span className="material-symbols-rounded" style={{ fontSize: 40, color: "var(--color-on-surface-variant)" }}>picture_as_pdf</span>
                )}
              </div>
              <div style={{ padding: "6px 8px 8px" }}>
                <div className="md-body-small" style={{ color: "var(--color-on-surface)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {a.filename}
                </div>
                <div className="md-label-small" style={{ color: "var(--color-on-surface-variant)" }}>
                  {formatBytes(a.sizeBytes)}{a.synced ? "" : " · pending sync"}
                </div>
              </div>
            </button>
          ))}
        </div>
      )}

      <Dialog
        open={preview !== null}
        title={preview?.filename ?? ""}
        onClose={() => setPreview(null)}
        actions={[
          { label: "Close", onClick: () => setPreview(null) },
          ...(!locked && preview ? [{ label: "Remove", onClick: () => void handleRemove(preview) }] : []),
        ]}
      >
        {preview ? (
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
            {preview.contentType.startsWith("image/") ? (
              <img src={api.attachmentDownloadUrl(reportId, preview.id)} alt={preview.filename} style={{ maxWidth: "100%", borderRadius: "var(--shape-small)" }} />
            ) : (
              <a href={api.attachmentDownloadUrl(reportId, preview.id)} target="_blank" rel="noreferrer" className="md-body-large">
                Open PDF in a new tab
              </a>
            )}
            <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
              {formatBytes(preview.sizeBytes)} · {preview.uploadedBy} · {formatUtc(preview.uploadedAt)}
            </div>
          </div>
        ) : null}
      </Dialog>
    </div>
  );
}
