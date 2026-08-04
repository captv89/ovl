// SPDX-License-Identifier: AGPL-3.0-only

import { useState } from "react";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Button } from "../../design/components/core/Button.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";

export function EnrollmentStep({
  onSubmit,
  onSkip,
}: {
  onSubmit: (officeURL: string, code: string) => Promise<void>;
  onSkip: () => Promise<void>;
}) {
  const [officeURL, setOfficeURL] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState<"connect" | "skip" | null>(null);

  async function handleConnect() {
    setError(null);
    setLoading("connect");
    try {
      await onSubmit(officeURL, code);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Something went wrong.");
    } finally {
      setLoading(null);
    }
  }

  async function handleSkip() {
    setError(null);
    setLoading("skip");
    try {
      await onSkip();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Something went wrong.");
    } finally {
      setLoading(null);
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <div>
        <div className="md-headline-small">Connect to your office</div>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", marginTop: 4 }}>
          Enter the office URL and enrollment code from the printed enrollment sheet. You can skip this and
          enroll later — the vessel works fully offline either way.
        </div>
      </div>

      <TextField label="Office URL" value={officeURL} onChange={setOfficeURL} leadingIcon="link" />
      <TextField label="Enrollment code" value={code} onChange={setCode} leadingIcon="key" />

      {error ? <AlertBanner level="warning" title="Couldn't connect" message={error} /> : null}

      <div style={{ display: "flex", justifyContent: "space-between" }}>
        <Button variant="text" onClick={() => void handleSkip()} disabled={loading !== null}>
          {loading === "skip" ? "Skipping…" : "Skip for now"}
        </Button>
        <Button
          variant="filled"
          size="large"
          onClick={() => void handleConnect()}
          disabled={loading !== null || officeURL.trim() === "" || code.trim() === ""}
        >
          {loading === "connect" ? "Connecting…" : "Connect and verify"}
        </Button>
      </div>
    </div>
  );
}
