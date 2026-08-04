// SPDX-License-Identifier: AGPL-3.0-only

import { Button } from "../../design/components/core/Button.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";

export function DoneStep({ enrolled, onStart }: { enrolled: boolean; onStart: () => void }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <div>
        <div className="md-headline-small">You're all set</div>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", marginTop: 4 }}>
          The Master account is ready and this vessel is configured.
        </div>
      </div>

      {!enrolled ? (
        <AlertBanner
          level="caution"
          title="Not enrolled"
          message="This vessel isn't connected to an office yet. Reports stay local until enrollment is completed — you can do this any time from settings."
        />
      ) : null}

      <Button variant="filled" size="large" onClick={onStart} style={{ alignSelf: "flex-end" }}>
        Start
      </Button>
    </div>
  );
}
