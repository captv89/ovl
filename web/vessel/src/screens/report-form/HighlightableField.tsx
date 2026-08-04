// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";

export type PolicyOutline = "mandatory" | "ghgRelevant" | "both" | null;

interface HighlightableFieldProps {
  fieldNames: string[];
  highlightField: string | null;
  highlightNonce: number | null;
  /**
   * Static field-policy annotation, independent of the transient highlight
   * pulse below. The actual mandatory (solid) / GHG-relevant (dashed)
   * marking is drawn by the wrapped field itself now (its `policyOutline`
   * prop — see TextField's doc comment): an earlier version drew that as a
   * second frame overlaid here, a couple of pixels outside the field's own
   * border, and the two edges never quite lined up on screen (caught from
   * a screenshot — reads as a broken/misaligned box, not one field). This
   * prop now only controls the small "also GHG-relevant" corner dot for
   * "both", positioned right on the field's own edge since there's just
   * the one rectangle to anchor to. Always Verdigris Teal
   * (--color-secondary) — deliberately not Brass/tertiary, which the
   * highlight pulse and the maritime "caution" status already both claim
   * (see motion.css), and not red/navy/green, which error
   * severity/focus/underway-status already claim.
   */
  outline?: PolicyOutline;
  /**
   * fieldName -> reviewer comment, for fields an office Reviewer flagged
   * with a remark (Phase 5 open question 4's resolved default: carry
   * flagged fields into the live form as amber-annotated when an officer
   * starts a correction on a remarked report). Unlike the transient
   * highlight pulse above, this is a persistent annotation — it stays
   * amber for as long as the remark is unresolved, not just for one
   * scroll-to-it moment.
   */
  remarkedFieldNames?: Record<string, string>;
  style?: CSSProperties;
  children: ReactNode;
}

// Wraps a field row (or a compound row covering several field names, e.g.
// PositionRow's degree/minutes/hemisphere trio) so a health-check issue's
// "jump to it" (design handoff A6) can scroll straight to it, move focus
// there for keyboard/screen-reader users, and pulse the Tideline
// field-highlight-pulse token once. highlightNonce changing (even to the
// same field, on a repeat click) is what retriggers the pulse — matching
// fieldNames alone would only fire once per section-mount.
export function HighlightableField({ fieldNames, highlightField, highlightNonce, outline = null, remarkedFieldNames, style, children }: HighlightableFieldProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [pulseKey, setPulseKey] = useState<number | null>(null);
  const isTarget = highlightField != null && fieldNames.includes(highlightField);
  const remarkComment = fieldNames.map((n) => remarkedFieldNames?.[n]).find((c) => c != null);

  useEffect(() => {
    if (!isTarget || highlightNonce == null) return;
    const el = ref.current;
    if (!el) return;
    const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    el.scrollIntoView({ behavior: reduceMotion ? "auto" : "smooth", block: "center" });
    el.querySelector<HTMLElement>("input, textarea, select, button")?.focus({ preventScroll: true });
    setPulseKey(highlightNonce);
    const timer = window.setTimeout(() => {
      setPulseKey((k) => (k === highlightNonce ? null : k));
    }, 1500);
    return () => window.clearTimeout(timer);
  }, [isTarget, highlightNonce]);

  return (
    <div
      ref={ref}
      style={{
        position: "relative",
        borderRadius: "var(--shape-small)",
        ...(remarkComment != null
          ? { border: "1.5px solid var(--color-status-caution)", background: "var(--color-status-caution-container)", padding: 8 }
          : null),
        ...style,
      }}
    >
      {children}
      {remarkComment != null ? (
        <div className="md-body-small" style={{ color: "var(--color-status-caution)", marginTop: 6, fontWeight: 600 }}>
          ⚑ Reviewer: {remarkComment}
        </div>
      ) : null}
      {outline === "both" ? (
        <span
          aria-hidden="true"
          title="Also relevant for GHG verification"
          style={{
            position: "absolute",
            top: -5,
            right: -5,
            width: 8,
            height: 8,
            borderRadius: "var(--shape-full)",
            background: "var(--color-secondary)",
            border: "1px solid var(--color-surface)",
          }}
        />
      ) : null}
      {pulseKey != null ? <span key={pulseKey} aria-hidden="true" className="field-highlight-pulse" /> : null}
    </div>
  );
}
