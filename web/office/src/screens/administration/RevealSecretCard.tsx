// SPDX-License-Identifier: AGPL-3.0-only

import { Card } from "../../design/components/surfaces/Card.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Button } from "../../design/components/core/Button.jsx";

// Design handoff B10's reveal-once secret card — same pattern
// VesselDetailScreen's own enrollment-issue flow already established (a
// plaintext secret shown exactly once, never recoverable from any later
// GET). Generalized from what was originally UsersTab.tsx's own
// user-specific RevealPasswordCard so Administration's API Access tab
// can reuse the same pattern with accurate copy for a bearer key
// (permanent, no username) instead of a misleading "temporary password"
// label.
export function RevealSecretCard({
  title,
  warningTitle,
  warningMessage,
  fields,
  onDone,
}: {
  title: string;
  warningTitle: string;
  warningMessage: string;
  fields: { label: string; value: string }[];
  onDone: () => void;
}) {
  return (
    <Card variant="outlined" style={{ padding: 20, maxWidth: 480, display: "flex", flexDirection: "column", gap: 12 }}>
      <div className="md-title-medium">{title}</div>
      <AlertBanner level="caution" title={warningTitle} message={warningMessage} />
      {fields.map((f) => (
        <div key={f.label}>
          <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
            {f.label}
          </div>
          <div className="md-body-large">{f.value}</div>
        </div>
      ))}
      <div>
        <Button variant="filled" onClick={onDone}>
          I've saved this
        </Button>
      </div>
    </Card>
  );
}
