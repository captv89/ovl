// SPDX-License-Identifier: AGPL-3.0-only

import type { Role } from "./api/client";

// Shared with AppShell.tsx's user chip and administration/UsersTab.tsx's
// role list/checkboxes — kept in its own module (not exported from a
// component file) so Vite's fast-refresh can still treat every .tsx
// file here as component-only.
export const ROLE_LABELS: Record<Role, string> = {
  admin: "Admin",
  configManager: "Config Manager",
  commercialEditor: "Commercial Editor",
  reviewer: "Reviewer",
  viewer: "Viewer",
};
