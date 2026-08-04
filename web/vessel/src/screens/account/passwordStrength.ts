// SPDX-License-Identifier: AGPL-3.0-only

// Shared by MasterStep (first-run wizard) and ChangePasswordDialog — a
// small local heuristic, not a full policy. vessel/auth's own
// MinPasswordLength (8, pkg/authcrypto) is the actual enforced floor;
// this only guides the user toward a stronger choice before they submit.
export const MIN_PASSWORD_LENGTH = 8;

export interface PasswordStrength {
  score: number;
  label: string;
  color: string;
}

export function passwordStrength(password: string): PasswordStrength {
  if (password.length === 0) {
    return { score: 0, label: "", color: "var(--color-outline-variant)" };
  }
  let score = 0;
  if (password.length >= MIN_PASSWORD_LENGTH) score++;
  if (password.length >= 14) score++;
  if (/[A-Z]/.test(password) && /[a-z]/.test(password)) score++;
  if (/[0-9]/.test(password) || /[^A-Za-z0-9]/.test(password)) score++;

  const levels = [
    { label: "Too short", color: "var(--color-error)" },
    { label: "Weak", color: "var(--color-status-caution)" },
    { label: "Fair", color: "var(--color-status-caution)" },
    { label: "Good", color: "var(--color-status-underway)" },
    { label: "Strong", color: "var(--color-status-underway)" },
  ];
  const level = levels[Math.min(score, levels.length - 1)];
  return { score, label: level.label, color: level.color };
}
