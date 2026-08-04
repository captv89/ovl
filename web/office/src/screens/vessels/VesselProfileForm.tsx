// SPDX-License-Identifier: AGPL-3.0-only

import { useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Select } from "../../design/components/forms/Select.jsx";
import { Chip } from "../../design/components/surfaces/Chip.jsx";

// Design handoff B2·5's own test feedback #1: "Type is now a dropdown of
// known vessel types... prevents free-text typos." A curated list, not
// an enum this project's schemas define anywhere else — vessel type
// isn't an OVD field, it's office's own profile metadata.
const VESSEL_TYPES = [
  "Bulk Carrier",
  "Container",
  "Oil Tanker",
  "Chemical Tanker",
  "LNG Carrier",
  "LPG Carrier",
  "General Cargo",
  "Ro-Ro",
  "Passenger",
  "Other",
];

// Shared create/edit form for B2's Profile tab. IMO is only editable at
// creation — vessels.Vessel has no setter for it once set (a real IMO
// number is a permanent per-hull identifier), so an existing vessel shows
// it as plain read-only text instead of a TextField.
export function VesselProfileForm({
  initial,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  initial?: { imo: string; name: string; type: string; groups: string[] };
  submitLabel: string;
  onSubmit: (fields: { imo?: string; name: string; type: string; groups: string[] }) => Promise<void>;
  onCancel: () => void;
}) {
  const [imo, setImo] = useState(initial?.imo ?? "");
  const [name, setName] = useState(initial?.name ?? "");
  const [type, setType] = useState(initial?.type ?? "");
  const [groups, setGroups] = useState<string[]>(initial?.groups ?? []);
  const [groupInput, setGroupInput] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const imoEditable = initial === undefined;

  function addGroup() {
    const g = groupInput.trim();
    if (g && !groups.includes(g)) {
      setGroups([...groups, g]);
    }
    setGroupInput("");
  }

  async function handleSubmit() {
    setError(null);
    setSubmitting(true);
    try {
      await onSubmit({ imo: imoEditable ? imo : undefined, name, type, groups });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Something went wrong.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: 480 }}>
      {error ? (
        <div className="md-body-medium" style={{ color: "var(--color-error)" }}>
          {error}
        </div>
      ) : null}

      {imoEditable ? (
        <TextField label="IMO number" value={imo} onChange={setImo} supportingText="7-digit IMO number, permanent once saved" />
      ) : (
        <div>
          <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
            IMO number
          </div>
          <div className="md-body-large">{initial?.imo}</div>
        </div>
      )}

      <TextField label="Vessel name" value={name} onChange={setName} />
      <Select
        label="Type"
        value={type}
        // A pre-existing vessel's type might predate this dropdown (free
        // text from before this change) — keep it selectable rather than
        // silently coercing it to blank/the first option.
        options={type && !VESSEL_TYPES.includes(type) ? [type, ...VESSEL_TYPES] : VESSEL_TYPES}
        onChange={setType}
      />

      <div>
        <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", marginBottom: 8 }}>
          Group tags
        </div>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 8 }}>
          {groups.map((g) => (
            <Chip key={g} label={g} type="input" onDelete={() => setGroups(groups.filter((x) => x !== g))} />
          ))}
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <TextField
            label="Add group tag"
            value={groupInput}
            onChange={setGroupInput}
            style={{ flex: 1 }}
          />
          <Button variant="outlined" onClick={addGroup} disabled={!groupInput.trim()}>
            Add
          </Button>
        </div>
      </div>

      <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
        <Button variant="text" onClick={onCancel} disabled={submitting}>
          Cancel
        </Button>
        <Button variant="filled" onClick={handleSubmit} disabled={submitting || !name.trim() || !type.trim() || (imoEditable && !imo.trim())}>
          {submitLabel}
        </Button>
      </div>
    </div>
  );
}
