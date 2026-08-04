// SPDX-License-Identifier: AGPL-3.0-only

import type { Role } from "../../api/client";

export const ROLE_LABELS: Record<Role, string> = {
  master: "Master",
  chiefOfficer: "Chief Officer",
  secondOfficer: "Second Officer",
  thirdOfficer: "Third Officer",
  chiefEngineer: "Chief Engineer",
  secondEngineer: "Second Engineer",
};
