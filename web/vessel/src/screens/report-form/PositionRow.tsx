// SPDX-License-Identifier: AGPL-3.0-only

import { PositionField } from "../../design/components/maritime/PositionField.jsx";
import type { Finding, SchemaField, FieldPolicyState } from "../../api/client";
import { fieldInfoTip, isRequired } from "./fieldPolicy";
import { pickFinding, policyOutlineFor, type FieldValue } from "./FieldRow";
import { HighlightableField } from "./HighlightableField";

interface PositionRowProps {
  axis: "lat" | "lon";
  label: string;
  degreeField: SchemaField;
  minutesField: SchemaField;
  hemisphereField: SchemaField;
  state: FieldPolicyState;
  values: Record<string, FieldValue>;
  onChange: (name: string, value: FieldValue, markOverridden: boolean) => void;
  highlightField?: string | null;
  highlightNonce?: number | null;
  remarkedFieldNames?: Record<string, string>;
  findings?: Finding[];
}

function asString(v: FieldValue | undefined): string {
  return typeof v === "string" ? v : "";
}

export function PositionRow({ axis, label, degreeField, minutesField, hemisphereField, state, values, onChange, highlightField = null, highlightNonce = null, remarkedFieldNames, findings = [] }: PositionRowProps) {
  const fieldNames = [degreeField.name, minutesField.name, hemisphereField.name];
  const finding = pickFinding(findings, fieldNames);
  // Always on — see FieldRow's own doc comment on why this isn't gated on
  // a Check having run.
  const outline = policyOutlineFor(state, degreeField.relevance);
  return (
    <HighlightableField
      fieldNames={fieldNames}
      highlightField={highlightField}
      highlightNonce={highlightNonce}
      remarkedFieldNames={remarkedFieldNames}
      outline={outline}
      style={{ gridColumn: "span 2" }}
    >
      <PositionField
        axis={axis}
        label={label}
        required={isRequired(state)}
        degree={asString(values[degreeField.name])}
        minutes={asString(values[minutesField.name])}
        hemisphere={asString(values[hemisphereField.name])}
        onChangeDegree={(v) => onChange(degreeField.name, v, false)}
        onChangeMinutes={(v) => onChange(minutesField.name, v, false)}
        onChangeHemisphere={(v) => onChange(hemisphereField.name, v, false)}
        error={finding?.severity === "error"}
        warning={finding?.severity === "warning"}
        supportingText={finding?.message}
        infoTip={fieldInfoTip(degreeField)}
        policyOutline={outline}
      />
    </HighlightableField>
  );
}
