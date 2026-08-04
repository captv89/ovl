// SPDX-License-Identifier: AGPL-3.0-only

import { Select } from "../../design/components/forms/Select.jsx";
import type { Scope, VesselView } from "../../api/client";

// Shared scope picker for B7's profiles/cadence/rule-severities panels —
// architecture 6.2/6.3 and design handoff B7 all describe the identical
// "assignable fleet-wide, per group, or per vessel" shape.
export function ScopeSelector({
  scope,
  onChange,
  vessels,
  allowFleet = true,
}: {
  scope: Scope;
  onChange: (scope: Scope) => void;
  vessels: VesselView[];
  allowFleet?: boolean;
}) {
  const groups = [...new Set(vessels.flatMap((v) => v.groups))].sort();
  const typeOptions = allowFleet ? ["fleet", "group", "vessel"] : ["group", "vessel"];

  return (
    <div style={{ display: "flex", gap: 12, alignItems: "flex-end" }}>
      <Select
        label="Scope"
        value={scope.type}
        options={typeOptions}
        onChange={(t) => onChange(t === "fleet" ? { type: "fleet" } : { type: t as Scope["type"], key: "" })}
      />
      {scope.type === "group" ? (
        <Select label="Group" value={scope.key ?? ""} options={groups} onChange={(key) => onChange({ type: "group", key })} />
      ) : null}
      {scope.type === "vessel" ? (
        <Select
          label="Vessel"
          value={vessels.find((v) => v.id === scope.key)?.name ?? ""}
          options={vessels.map((v) => v.name)}
          onChange={(name) => {
            const v = vessels.find((v) => v.name === name);
            if (v) onChange({ type: "vessel", key: v.id });
          }}
        />
      ) : null}
    </div>
  );
}
