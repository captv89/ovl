// SPDX-License-Identifier: AGPL-3.0-only

import { useMemo, useState } from "react";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Dialog } from "../../design/components/feedback/Dialog.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { ProgressIndicator } from "../../design/components/feedback/ProgressIndicator.jsx";
import { api, ApiError, type UserView } from "../../api/client";
import { MIN_PASSWORD_LENGTH, passwordStrength } from "./passwordStrength";

// The self-service "Change password" flow, reachable any time from the
// user menu. Ported verbatim from web/vessel's own ChangePasswordDialog
// (same field shape, same passwordStrength helper) — office never had
// this despite api.changePassword already existing in client.ts.
export function ChangePasswordDialog({
  open,
  onClose,
  onChanged,
}: {
  open: boolean;
  onClose: () => void;
  onChanged: (user: UserView) => void;
}) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const meter = useMemo(() => passwordStrength(newPassword), [newPassword]);
  const passwordsMatch = confirmPassword === "" || confirmPassword === newPassword;
  const canSubmit =
    currentPassword !== "" && newPassword.length >= MIN_PASSWORD_LENGTH && newPassword === confirmPassword;

  function reset() {
    setCurrentPassword("");
    setNewPassword("");
    setConfirmPassword("");
    setError(null);
  }

  async function handleSubmit() {
    if (loading || !canSubmit) return;
    setLoading(true);
    setError(null);
    try {
      const user = await api.changePassword(currentPassword, newPassword);
      reset();
      onChanged(user);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not change the password.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <Dialog
      open={open}
      title="Change password"
      onClose={() => {
        if (loading) return;
        reset();
        onClose();
      }}
      actions={[
        { label: "Cancel", onClick: () => { reset(); onClose(); } },
        { label: loading ? "Changing…" : "Change password", onClick: () => void handleSubmit() },
      ]}
    >
      <div style={{ display: "flex", flexDirection: "column", gap: 16, textAlign: "left" }}>
        <TextField label="Current password" value={currentPassword} onChange={setCurrentPassword} type="password" leadingIcon="lock" style={{ width: "100%" }} />
        <div>
          <TextField label="New password" value={newPassword} onChange={setNewPassword} type="password" leadingIcon="lock" style={{ width: "100%" }} />
          {newPassword.length > 0 ? (
            <div style={{ marginTop: 8, display: "flex", flexDirection: "column", gap: 4 }}>
              <ProgressIndicator variant="linear" value={(meter.score / 4) * 100} />
              <span className="md-label-small" style={{ color: meter.color }}>{meter.label}</span>
            </div>
          ) : null}
        </div>
        <TextField
          label="Confirm new password"
          value={confirmPassword}
          onChange={setConfirmPassword}
          type="password"
          leadingIcon="lock"
          error={!passwordsMatch}
          supportingText={passwordsMatch ? undefined : "Passwords don't match"}
          style={{ width: "100%" }}
        />
        {error ? <AlertBanner level="warning" title="Couldn't change password" message={error} /> : null}
      </div>
    </Dialog>
  );
}
