// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	ovlschemas "github.com/captv89/ovl/schemas"

	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/schema"
	"github.com/captv89/ovl/pkg/validation"
	"github.com/captv89/ovl/vessel/store"
)

var (
	schemaValidatorOnce sync.Once
	schemaValidator     *schema.Validator
	schemaValidatorErr  error
)

func getSchemaValidator() (*schema.Validator, error) {
	schemaValidatorOnce.Do(func() {
		schemaValidator, schemaValidatorErr = schema.NewValidator(ovlschemas.FS, "meta-schema.json")
	})
	return schemaValidator, schemaValidatorErr
}

// prefillEntry mirrors architecture 6.4's four prefill classes.
// carryForward and computed are now sourced from the vessel's own last
// submitted report where one exists (see handleGetSchema); only ghost
// (Wind_Force_Kn/Distance in fieldConfigFor) is still fixed demo data,
// pending real sensor/distance-run computation this codebase doesn't
// have inputs for yet.
type prefillEntry struct {
	Class   string `json:"class"` // "carryForward" | "computed" | "ghost" | "none"
	Value   any    `json:"value,omitempty"`
	Formula string `json:"formula,omitempty"` // computed only
}

// robSeriesView mirrors pkg/validation.ROBSeries — the engine stays the
// single source of truth for which series exist per schema
// (validationConfigFor, now delegating to validation.LogAbstractConfig),
// this just lets the frontend recompute the same
// ROB(n) = ROB(n-1) - sum(consumption fields)(n) formula live as a
// responsiveness pre-check (real cascade revalidation still happens
// server-side via runCascade), instead of hardcoding a duplicate series
// list in TypeScript. ConsumptionFields is plural for the same reason
// ROBSeries.ConsumptionFields is — see that type's own doc comment.
type robSeriesView struct {
	Name              string   `json:"name"`
	ROBField          string   `json:"robField"`
	ConsumptionFields []string `json:"consumptionFields"`
}

type schemaConfigResponse struct {
	Schema      *schema.Schema           `json:"schema"`
	FieldPolicy map[string]string        `json:"fieldPolicy"`
	Prefill     map[string]*prefillEntry `json:"prefill"`
	// FieldEvents narrows which voyage event types each field's policy
	// applies to (validation.FieldEvents). This endpoint has no event
	// context of its own — the officer picks the event before the form
	// opens — so the whole map is served and the form applies the gate for
	// the event it was opened with, exactly as it already does for policy
	// state. Absent field ⇒ every event; empty map ⇒ nothing is gated.
	FieldEvents validation.FieldEvents `json:"fieldEvents,omitempty"`
	RobSeries   []robSeriesView        `json:"robSeries,omitempty"`
	// LastReportEventTime is the vessel's last *submitted* report's
	// EventTime for this schema, when one exists — lets the frontend
	// recompute Time_Since_Previous_Report live against whatever Date_UTC/
	// Time_UTC the officer enters on this report, mirroring
	// pkg/validation.EvaluateTimeChain's own formula.
	LastReportEventTime *time.Time `json:"lastReportEventTime,omitempty"`
}

// handleGetSchema serves a curated schema plus the "local test config"
// the Phase 2 checklist calls for: a field policy and prefill class
// assignment. Phase 3 (office config-bundle authoring) doesn't exist
// yet, so both are hardcoded per schema name here rather than read from
// a store — real company-authored data replaces fieldConfigFor's body
// without changing this handler's shape.
func (s *Server) handleGetSchema(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "schema name is required")
		return
	}

	validator, err := getSchemaValidator()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sch, err := schema.Load(ovlschemas.FS, "ovd-3.13/"+name+".json", validator)
	if err != nil {
		httpjson.WriteError(w, http.StatusNotFound, fmt.Sprintf("unknown schema %q", name))
		return
	}

	// Resolve field policy/prefill/validation config from this vessel's
	// applied config bundle (audit 2026-07-22 §1); nil bundle ⇒ the
	// built-in stand-in, same as before any bundle is synced. Decoded once
	// here and reused for validationConfigFor below.
	appliedBundle := s.appliedBundle(r.Context())
	policy, fieldEvents, prefill := fieldConfigFromBundle(appliedBundle, name)

	// IMO is real carryForward data, not fieldConfigFor's demo stand-in:
	// every schema that carries an IMO field should prefill it from this
	// vessel's own enrolled identity (2026-07-14 manual-test feedback: an
	// enrolled vessel shouldn't be asked to retype IMO on every report).
	// Silently skipped if the vessel was never enrolled (offline/deferred
	// path, vessel/store.ErrNotFound) — falls back to a blank field, same
	// as before this change.
	if hasIMOField(sch) {
		if st := s.storeOrNil(); st != nil {
			if identity, err := st.GetVesselIdentity(r.Context()); err == nil {
				prefill["IMO"] = &prefillEntry{Class: "carryForward", Value: identity.IMO}
			}
		}
	}

	// Same idea as IMO above, but sourced from this vessel's own last
	// submitted report of this schema rather than its enrolled identity
	// (2026-07-14 manual-test feedback item 11: "a report created after
	// the previous report is submitted should carry forward values from
	// the previous report"). Every field the last report had a value for
	// carries forward (2026-07-14 p2 manual-test feedback, widening this
	// same session's first, deliberately-curated few-field version:
	// "literally all the fields can be carried forward, allowing the
	// users to edit the changed values and also being a quick reference
	// to what was reported previously" — an explicit, repeated ask, not
	// an assumption). Two exclusions: IMO is skipped since it's already
	// carryForward-prefilled above from the vessel's own enrolled
	// identity, a more authoritative source than a report snapshot; and
	// any field already given a computed prefill below (ROB series,
	// Time_Since_Previous_Report) is left alone, since those need the
	// more specific real-computation treatment, not a plain carried-over
	// value. A fresh voyage with no report history yet leaves every
	// field unprefilled, same as before this change. Every prefilled
	// value stays fully editable (architecture 6.4) — this is a starting
	// point, not a lock.
	// RobSeries describes which fields are ROB continuity tracks for this
	// schema (pkg/validation.Config.ROBSeriesList, the same list runCascade
	// checks against) — populated regardless of report history/store
	// availability, since it's schema config, not report data; only the
	// *prior value* each series carries forward depends on history, below.
	cfg := validationConfigFromBundle(appliedBundle, name)
	robSeries := make([]robSeriesView, len(cfg.ROBSeriesList))
	for i, series := range cfg.ROBSeriesList {
		robSeries[i] = robSeriesView{Name: series.Name, ROBField: series.ROBField, ConsumptionFields: series.ConsumptionFields}
	}

	var lastReportEventTime *time.Time
	if st := s.storeOrNil(); st != nil {
		last := lastSubmittedReport(r.Context(), st, name)

		// Real computed prefill, replacing the fixed-constant demo values
		// fieldConfigFor used to hardcode (see its own comment on why
		// those were fake, and 18.07.26 manual-test items 3/6/8/10: a
		// hardcoded HFO_ROB/Time_Since_Previous_Report on every report
		// meant cascade revalidation (runCascade) was comparing real
		// consumption/elapsed-time entries against a constant that never
		// moved, tripping continuity findings and cascade-invalidating
		// reports that were never actually inconsistent — the root cause,
		// not the symptom. The value set here is only the *base* for the
		// very first render (before the officer has entered any
		// consumption/date-time on this report, expected == prior
		// unchanged); the frontend recomputes it live from RobSeries/
		// LastReportEventTime as those fields change, the same way
		// Wind_Dir etc. already recompute live from a sibling field
		// (derivedFields.ts) — and pkg/validation.EvaluateROBContinuity/
		// EvaluateTimeChain remain the real, authoritative check either
		// way; this is a responsiveness pre-check, not a new rule.
		if last != nil {
			for _, series := range cfg.ROBSeriesList {
				if prior, ok := last.ToValidation().Float(series.ROBField); ok {
					prefill[series.ROBField] = &prefillEntry{
						Class:   "computed",
						Value:   prior,
						Formula: fmt.Sprintf("%.2f previous, minus consumption entered on this report", prior),
					}
				}
			}
			if hasField(sch, "Time_Since_Previous_Report") {
				hours := time.Now().UTC().Sub(last.EventTime).Hours()
				prefill["Time_Since_Previous_Report"] = &prefillEntry{
					Class:   "computed",
					Value:   math.Round(hours*100) / 100,
					Formula: fmt.Sprintf("Elapsed since the previous report at %s UTC", last.EventTime.Format("2006-01-02 15:04")),
				}
			}

			for fieldName, v := range last.Fields {
				if fieldName == "IMO" {
					continue
				}
				existing := prefill[fieldName]
				if existing != nil {
					if existing.Class != "carryForward" {
						continue
					}
				} else if appliedBundle != nil {
					// A bundle is applied and left this field out of its
					// Prefill map entirely: per fieldConfigFromBundle's own
					// contract ("that field simply takes its default") and
					// the office Field Policy editor's own encoding
					// (fieldPolicyLogic.ts effectivePrefill defaults an
					// absent key to "none"; setFieldPrefill/applyBulkPrefill
					// delete the key rather than writing "none"), absence
					// here means the company deliberately wants no prefill
					// for this field, not "carry it forward anyway." Only
					// the no-bundle stand-in path below still defaults every
					// field with report history to carryForward, matching
					// the pre-config-bundle (2026-07-14) behavior it was
					// built for, before this was configurable at all.
					continue
				}
				prefill[fieldName] = &prefillEntry{Class: "carryForward", Value: v}
			}
			et := last.EventTime
			lastReportEventTime = &et
		}
	}

	httpjson.WriteJSON(w, http.StatusOK, schemaConfigResponse{
		Schema: sch, FieldPolicy: policy, FieldEvents: fieldEvents, Prefill: prefill,
		RobSeries: robSeries, LastReportEventTime: lastReportEventTime,
	})
}

func hasField(sch *schema.Schema, name string) bool {
	for _, f := range sch.Fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

func hasIMOField(sch *schema.Schema) bool {
	for _, f := range sch.Fields {
		if f.Name == "IMO" {
			return true
		}
	}
	return false
}

// lastSubmittedReport returns the most recently reported (by EventTime)
// submitted-or-later version of schemaName, or nil if none exists yet —
// a draft/ready report is deliberately excluded, since its fields may
// still be incomplete or wrong and "the previous report" (item 11's own
// wording) reads most naturally as one that was actually finished.
func lastSubmittedReport(ctx context.Context, st *store.Store, schemaName string) *domain.Report {
	reports, err := st.ListLatestReports(ctx, schemaName)
	if err != nil {
		return nil
	}
	// Already ordered by EventTime descending (ListLatestReports' own
	// contract), so the first submitted-or-later match is the most recent.
	for _, rep := range reports {
		if !rep.SubmittedAt.IsZero() {
			return rep
		}
	}
	return nil
}

// fieldConfigFor returns the demo field policy and prefill config for a
// schema. Only log-abstract has one curated by hand (matching fields
// already used elsewhere in the codebase — cmd/ovl-validate's sample
// chain, pkg/validation's rule catalog); every other schema gets empty
// maps, meaning every field defaults to optional (or schemaMandatory,
// which the schema itself always enforces regardless of policy).
func fieldConfigFor(schemaName string) (policy map[string]string, prefill map[string]*prefillEntry) {
	if schemaName != "log-abstract" {
		return map[string]string{}, map[string]*prefillEntry{}
	}
	return map[string]string{
			// Hidden: rarely-used fields a typical company turns off.
			"O_ROB":                   "hidden",
			"O_ROB_type":              "hidden",
			"Cleaning_Event":          "hidden",
			"Boiler_1_Operation_Mode": "hidden",
			// Recommended: soft plausibility inputs, not DNV-mandatory.
			"Wind_Force_Kn":           "recommended",
			"Sea_state_Force_Douglas": "recommended",
			"Temperature_Ambient":     "recommended",
			// Remaining sailing distance is optional by DNV's own schema
			// (unsupportedByDnv) but is what drives Home's voyage-progress
			// bar (2026-07-14 manual-test feedback item 8) — surfaced by
			// default instead of sitting behind "Add N optional fields."
			"Distance_To_Go": "recommended",
			// Company-mandatory: voyage tracking the company requires
			// beyond DNV's own mandatory set.
			"Voyage_Number": "companyMandatory",
			"Voyage_From":   "companyMandatory",
			"Voyage_To":     "companyMandatory",
		}, map[string]*prefillEntry{
			// Voyage_Number/Voyage_From/Voyage_To/Distance_To_Go (and now
			// every other plain field) used to/still carry fixed demo values
			// here — replaced by handleGetSchema's own carryForward block,
			// which sources the vessel's own last submitted report instead
			// of a hardcoded stand-in. HFO_ROB/Time_Since_Previous_Report
			// used to be hardcoded here too (812.4/24.0 on every report,
			// regardless of history) — that was the root cause of 18.07.26
			// manual-test items 3/6/8/10 (fake continuity baseline tripping
			// real cascade revalidation); replaced by handleGetSchema's own
			// real computed-prefill block, sourced from the vessel's last
			// submitted report, same as carryForward. Wind_Force_Kn/Distance
			// remain demo ghost values — not reported as wrong, and real
			// computation for those (a live sensor feed / an actual
			// distance-run calculation) is out of scope here.
			"Wind_Force_Kn": {Class: "ghost", Value: 12.4},
			"Distance":      {Class: "ghost", Value: 187.3},
		}
}

// validationConfigFor returns the plausibility/continuity config
// (pkg/validation.Config) for a schema — validation.LogAbstractConfig()
// for log-abstract (the one schema with a curated ROB series/consumption-
// scheme field list, shared with office/syncservice and cmd/ovl-validate
// rather than duplicated per app, see that function's own doc comment),
// DefaultConfig()'s bare tolerances with no series or fields checked for
// everything else. A stand-in either way until Phase 3 config-bundle
// authoring exists to supply a real, company-specific one.
func validationConfigFor(schemaName string) *validation.Config {
	if schemaName != "log-abstract" {
		return validation.DefaultConfig()
	}
	return validation.LogAbstractConfig()
}
