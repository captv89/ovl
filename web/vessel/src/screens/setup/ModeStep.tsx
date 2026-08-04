// SPDX-License-Identifier: AGPL-3.0-only

import { useState } from "react";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { Radio } from "../../design/components/forms/Radio.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Button } from "../../design/components/core/Button.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import type { Mode } from "../../api/client";

interface ModeOptionProps {
  selected: boolean;
  onSelect: () => void;
  title: string;
  description: string;
  disabled?: boolean;
  disabledNote?: string;
}

function ModeOption({ selected, onSelect, title, description, disabled, disabledNote }: ModeOptionProps) {
  return (
    <Card
      variant={selected ? "filled" : "outlined"}
      onClick={disabled ? undefined : onSelect}
      style={{ opacity: disabled ? "var(--state-disabled-opacity)" : 1, cursor: disabled ? "not-allowed" : "pointer" }}
    >
      <div style={{ display: "flex", gap: 12, alignItems: "flex-start" }}>
        <div style={{ paddingTop: 2 }}>
          <Radio selected={selected} disabled={disabled} onChange={disabled ? undefined : onSelect} />
        </div>
        <div>
          <div className="md-title-small">{title}</div>
          <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", marginTop: 2 }}>
            {description}
          </div>
          {disabledNote ? (
            <div className="md-label-small" style={{ color: "var(--color-tertiary)", marginTop: 4 }}>
              {disabledNote}
            </div>
          ) : null}
        </div>
      </div>
    </Card>
  );
}

export function ModeStep({
  defaultDataDir,
  onNext,
}: {
  defaultDataDir: string;
  onNext: (mode: Mode, dataDir: string) => Promise<void>;
}) {
  const [mode, setMode] = useState<Mode>("standalone");
  const [dataDir, setDataDir] = useState(defaultDataDir);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function handleContinue() {
    setError(null);
    setLoading(true);
    try {
      await onNext(mode, dataDir);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Something went wrong.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <div>
        <div className="md-headline-small">Choose how this vessel runs</div>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", marginTop: 4 }}>
          You can change this later from the admin settings.
        </div>
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        <ModeOption
          selected={mode === "standalone"}
          onSelect={() => setMode("standalone")}
          title="Standalone"
          description="Runs on this one PC. Only this computer's browser can open it."
        />
        <ModeOption
          selected={mode === "server"}
          onSelect={() => setMode("server")}
          title="Vessel server"
          description="Any PC on the ship's network can reach it. Adds live presence and section locking when several people edit the same report."
        />
      </div>

      <TextField
        label="Data directory"
        value={dataDir}
        onChange={setDataDir}
        leadingIcon="folder"
        supportingText="Where reports, attachments, and the local database are stored. Edit this if you'd rather use a different drive or folder."
      />

      {error ? <AlertBanner level="warning" title="Couldn't save this" message={error} /> : null}

      <Button variant="filled" size="large" onClick={() => void handleContinue()} disabled={loading || dataDir.trim() === ""} style={{ alignSelf: "flex-end" }}>
        {loading ? "Saving…" : "Continue"}
      </Button>
    </div>
  );
}
