// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { Textarea } from "../../design/components/forms/Textarea.jsx";

interface HighlightableFieldProps {
  fieldName: string;
  highlightField: string | null;
  highlightNonce: number | null;
  /** fieldName -> body, for fields with an existing unresolved remark — persistent amber accent, not just a jump pulse. */
  remarkedFieldNames: Record<string, string>;
  /** Reviewer-only: lets clicking the field open a draft-comment box for a new remark, on top of (not replacing) any existing unresolved remark shown above. */
  remarkMode?: boolean;
  draftComment?: string;
  onToggleDraft?: () => void;
  onDraftChange?: (body: string) => void;
  open?: boolean;
  style?: CSSProperties;
  children: ReactNode;
}

// Office's version of web/vessel/src/screens/report-form/HighlightableField.tsx
// (apps don't share code — same vendored-copy precedent). Differs from
// the vessel original in one respect: vessel's is pure display (jump
// pulse + a persistent remark annotation it can never edit); office
// layers Remark mode's click-to-flag interaction on top, since office is
// the side that *creates* remarks rather than just receiving them. The
// persistent-remark and jump-pulse halves are otherwise identical so a
// remark-flagged field looks the same on both sides of the sync link.
export function HighlightableField({
  fieldName,
  highlightField,
  highlightNonce,
  remarkedFieldNames,
  remarkMode = false,
  draftComment,
  onToggleDraft,
  onDraftChange,
  open = false,
  style,
  children,
}: HighlightableFieldProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [pulseKey, setPulseKey] = useState<number | null>(null);
  const isTarget = highlightField === fieldName;
  const existingRemark = remarkedFieldNames[fieldName];
  const hasDraft = (draftComment ?? "").trim() !== "";

  useEffect(() => {
    if (!isTarget || highlightNonce == null) return;
    const el = ref.current;
    if (!el) return;
    const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    el.scrollIntoView({ behavior: reduceMotion ? "auto" : "smooth", block: "center" });
    setPulseKey(highlightNonce);
    const timer = window.setTimeout(() => {
      setPulseKey((k) => (k === highlightNonce ? null : k));
    }, 1500);
    return () => window.clearTimeout(timer);
  }, [isTarget, highlightNonce]);

  const flagged = existingRemark != null || hasDraft;

  return (
    <div
      ref={ref}
      onClick={remarkMode ? onToggleDraft : undefined}
      style={{
        position: "relative",
        borderRadius: "var(--shape-small)",
        cursor: remarkMode ? "pointer" : "default",
        padding: flagged || remarkMode ? 8 : 0,
        borderLeft: flagged ? "3px solid var(--color-status-caution)" : "3px solid transparent",
        background: flagged ? "var(--color-status-caution-container)" : "transparent",
        ...style,
      }}
    >
      {children}
      {existingRemark != null ? (
        <div className="md-body-small" style={{ color: "var(--color-status-caution)", marginTop: 6, fontWeight: 600 }}>
          ⚑ {existingRemark}
        </div>
      ) : null}
      {remarkMode && open ? (
        <div style={{ marginTop: 8 }} onClick={(e) => e.stopPropagation()}>
          <Textarea
            label="Comment for reviewer"
            value={draftComment ?? ""}
            onChange={(v) => onDraftChange?.(v)}
            rows={2}
            style={{ width: "100%" }}
          />
        </div>
      ) : null}
      {pulseKey != null ? <span key={pulseKey} aria-hidden="true" className="field-highlight-pulse" /> : null}
    </div>
  );
}
