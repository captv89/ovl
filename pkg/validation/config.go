// SPDX-License-Identifier: AGPL-3.0-only

package validation

// ROBSeries names one remaining-on-board continuity track: a ROB field,
// every consumption field it is reduced by, and a name used to look up
// how much was bunkered since the previous report (architecture 8.3:
// ROB(n) = ROB(n-1) - sum(consumption fields)(n) + bunkered(n)).
// ConsumptionFields is plural because a single ROB tank is drawn down by
// multiple separate consumers on a real vessel (Main Engine, Auxiliary
// Engine, Boiler, and sometimes more) — see LogAbstractConfig's own doc
// comment for why a single field was wrong.
//
// The real Log Abstract schema has on the order of fifteen ROB fields
// (per fuel type, cylinder oil, fresh water, sludge...); LogAbstractConfig
// covers the ten fuel-type ones (the ones with a well-defined "reduced by
// consumption, topped up by bunkering" formula this package's generic
// check already models). pkg/validation's own engine stays schema-
// agnostic — EvaluateROBContinuity just iterates whatever ROBSeriesList a
// Config carries — but a Config is only useful once populated with real
// series, and LogAbstractConfig is that single, shared, curated source
// (see its own doc comment for why this one exception to "the engine
// doesn't know about specific schemas" is justified).
type ROBSeries struct {
	Name              string
	ROBField          string
	ConsumptionFields []string
}

// AlternatingGroup is a set of OVD event types that must alternate
// between two stages across a vessel's report chain — the "event
// ordering sanity" continuity rule (architecture 10.1): Arrival cannot
// follow Arrival without a Departure, EndOfShifting requires a prior
// BeginOfShifting, and similar pairs. Stage0 and Stage1 list the event
// type spellings (see schemas/ovd-3.13/enums/event-types.json) that
// count as each stage; an event whose EventType is not in either stage
// is simply ignored by this group.
type AlternatingGroup struct {
	Name   string
	Stage0 []string
	Stage1 []string
}

// DefaultEventGroups is derived from the fixed-spelling event types
// (schemas/ovd-3.13/enums/event-types.json) and the pairs the
// architecture doc names explicitly in 10.1.
func DefaultEventGroups() []AlternatingGroup {
	return []AlternatingGroup{
		{Name: "arrivalDeparture", Stage0: []string{"Arrival", "ArrivalSTS"}, Stage1: []string{"Departure", "DepartureSTS"}},
		{Name: "seaPassage", Stage0: []string{"BOSP", "BeginOfSeaPassage", "FAOP", "FullAheadOnPassage"}, Stage1: []string{"EOSP", "EndOfSeaPassage"}},
		{Name: "shifting", Stage0: []string{"BeginOfShifting"}, Stage1: []string{"EndOfShifting"}},
		{Name: "canalPassage", Stage0: []string{"Begin canal passage"}, Stage1: []string{"End canal passage"}},
		{Name: "anchoringDrifting", Stage0: []string{"Begin Anchoring/Drifting"}, Stage1: []string{"End Anchoring/Drifting"}},
		{Name: "fuelChangeOver", Stage0: []string{"Begin fuel change over"}, Stage1: []string{"End fuel change over"}},
		{Name: "deviation", Stage0: []string{"Begin of deviation"}, Stage1: []string{"End of deviation"}},
		{Name: "specialArea", Stage0: []string{"Entering special area"}, Stage1: []string{"Leaving special area"}},
		{Name: "offhire", Stage0: []string{"Beginofoffhire"}, Stage1: []string{"Endofoffhire"}},
	}
}

// Config holds the tunable inputs to the plausibility and continuity
// rules: tolerances, vessel-plausible bounds, and the per-vessel/company
// data that architecture 10.2 says belongs in the config bundle
// (severity per rule) or is computed from other submitted data (ROB
// series, bunkered amounts, consumption-scheme field lists). None of
// this is wired to a real config bundle store yet (Phase 3); the zero
// value plus DefaultConfig gives reasonable, documented defaults for
// unit testing and early wiring.
type Config struct {
	// TimeBucketToleranceHours bounds how far the sum of Time_Elapsed_*
	// fields may drift from Time_Since_Previous_Report. Not specified
	// numerically by the architecture doc (only "within tolerance");
	// this default is a provisional placeholder pending real-world
	// tuning once Phase 3 config authoring exists.
	TimeBucketToleranceHours float64

	// ImpliedSpeedMinKn / ImpliedSpeedMaxKn are the vessel-plausible
	// speed bounds; architecture 10.1 gives the default explicitly:
	// 0 to 30 kn.
	ImpliedSpeedMinKn float64
	ImpliedSpeedMaxKn float64

	// TimeChainToleranceHours bounds how far Time_Since_Previous_Report
	// may drift from the actual timestamp delta to the previous report
	// (architecture 8.3). Provisional default, as above.
	TimeChainToleranceHours float64

	// ROBToleranceMt bounds ROB continuity drift (architecture 8.3).
	// Provisional default, as above.
	ROBToleranceMt float64

	// ROBSeriesList are the ROB continuity tracks to check. Empty by
	// default; callers supply the series relevant to their schema/company
	// configuration.
	ROBSeriesList []ROBSeries

	// BunkeredAmounts maps an ROBSeries.Name to how much was bunkered in
	// that series since the previous report, as computed from submitted
	// Bunker Reports. Series absent from the map are treated as 0
	// bunkered.
	BunkeredAmounts map[string]float64

	// FuelTypeConsumptionFields and BDNMarkerFields are the field names
	// that indicate, respectively, the fuel-type-based and BDN-based
	// consumption reporting schemes (architecture 10.1: "consumption
	// scheme exclusivity"). Empty by default; callers supply the field
	// names relevant to their schema.
	FuelTypeConsumptionFields []string
	BDNMarkerFields           []string

	// EventGroups are the alternating begin/end event pairs used by the
	// event ordering sanity check. Defaults to DefaultEventGroups().
	EventGroups []AlternatingGroup

	// Severities overrides the default severity for a rule ID. Schema
	// mandatoriness and rules architecture 10.1 calls out as "hard OVD
	// rules" (currently: consumption scheme exclusivity) cannot be
	// downgraded from SeverityError, per architecture 10.2.
	Severities map[string]Severity
}

// DefaultConfig returns a Config with the doc-specified default (implied
// speed bounds) and reasonable provisional defaults for the tolerances
// the doc leaves unspecified. It carries no ROB series, consumption
// field lists, or bunkered amounts — those are schema/company-specific
// and must be supplied by the caller.
func DefaultConfig() *Config {
	return &Config{
		TimeBucketToleranceHours: 0.1,
		ImpliedSpeedMinKn:        0,
		ImpliedSpeedMaxKn:        30,
		TimeChainToleranceHours:  0.1,
		ROBToleranceMt:           0.5,
		EventGroups:              DefaultEventGroups(),
		Severities:               map[string]Severity{},
	}
}

// logAbstractFuelTypes describes every remaining-on-board (ROB) track in
// the curated log-abstract schema (schemas/ovd-3.13/log-abstract.json's
// "rob" section) under the fuel-type consumption scheme: the suffix used
// by the matching "engine.consumption" section fields (ME_Consumption_
// <suffix>, etc.), the actual ROB field name (irregular for Methanol/
// Ethanol/Other — "Methanol_ROB"/"Ethanol_ROB"/"O_ROB", not "M_ROB" etc.),
// and whether the schema defines Inert Gas Generator / Cargo Heating
// consumer fields for it (only true for HFO/LFO/MGO/MDO — the schema has
// no Inert_gas_Consumption_LNG or similar for the other six).
var logAbstractFuelTypes = []struct {
	suffix         string
	robField       string
	extraConsumers bool
}{
	{"HFO", "HFO_ROB", true},
	{"LFO", "LFO_ROB", true},
	{"MGO", "MGO_ROB", true},
	{"MDO", "MDO_ROB", true},
	{"LPGP", "LPGP_ROB", false},
	{"LPGB", "LPGB_ROB", false},
	{"LNG", "LNG_ROB", false},
	{"M", "Methanol_ROB", false},
	{"E", "Ethanol_ROB", false},
	{"O", "O_ROB", false},
}

// LogAbstractConfig is the one real, hand-curated Config this codebase
// currently maintains, for the one schema (log-abstract) with a curated
// ROB continuity chain — the single shared source vessel/httpapi,
// office/syncservice, and cmd/ovl-validate all call, replacing what used
// to be three independent hand-copies of the same list (schemas.go,
// cascade.go, main.go), all of which only tracked a single series (HFO,
// checked against ME_Consumption_HFO alone).
//
// That was a real correctness bug, not just an incompleteness: a real
// vessel's HFO tank is also drawn down by its Auxiliary Engines, Boiler,
// Inert Gas Generator, and Cargo Heating plant — all present as separate
// "engine.consumption" fields in the same curated schema — so checking
// only ME_Consumption_HFO made ROB continuity silently too lenient
// (AE/Boiler/etc. consumption was never subtracted, so a real ROB drop
// explained by them looked like unexplained drift, or worse, a genuine
// drift could hide behind unaccounted consumption). Flagged directly in
// 18.07.26 manual-test triage ("why limit the auto calculation to just
// these 2 fields... you need to have a peripheral vision"). Fixed by
// summing every consumer category's field for each fuel type
// (ROBSeries.ConsumptionFields, plural) and covering all ten fuel types
// the schema curates a ROB field for, not just HFO.
//
// The three-way hand-duplication itself was also worth closing: office/
// syncservice/cascade.go's own prior doc comment reasoned duplication was
// "deliberate... the two apps' schemas.go/cascade.go don't otherwise
// import each other" — true, but beside the point, since both already
// import pkg/validation regardless; putting the one shared definition
// here needs no new cross-app import, only what both already have.
//
// Deliberately scoped to the fuel-type consumption scheme only, not the
// BDN-linked one (ME_Consumption/_BDN_2../3/4, IGG_Consumption,
// GCU_Consumption, DPP_Consumption, Incinerator_Consumption) — real-world
// OVD reporting practice treats BDN-based consumption as the uncommon
// path (see PROJECT.md's domain-knowledge note), so fuel-type is the one
// worth getting exactly right first. A report using the BDN scheme
// instead simply leaves these fields empty; EvaluateROBContinuity already
// no-ops any series whose ROB field is missing from either report being
// compared, so this isn't a silent gap for BDN-scheme reports — it's an
// explicit, documented scope boundary, same treatment as the deprecated
// sulphur-content ROB fields (HFO_HS_ROB etc., "deprecated since
// interface version 3.3" per the schema's own field descriptions) and the
// non-fuel ROB fields (Fresh_Water_ROB, Sludge_ROB, cylinder/system oil)
// — those need a genuinely different replenishment model (production vs.
// consumption, or "topped up" rather than "bunkered"), not a bug fix of
// this list, and weren't asked for.
func LogAbstractConfig() *Config {
	cfg := DefaultConfig()
	series := make([]ROBSeries, 0, len(logAbstractFuelTypes))
	var fuelFields []string
	for _, ft := range logAbstractFuelTypes {
		fields := []string{
			"ME_Consumption_" + ft.suffix,
			"AE_Consumption_" + ft.suffix,
			"Boiler_Consumption_" + ft.suffix,
		}
		if ft.extraConsumers {
			fields = append(fields, "Inert_gas_Consumption_"+ft.suffix, "Cargo_heating_Consumption_"+ft.suffix)
		}
		series = append(series, ROBSeries{Name: ft.suffix, ROBField: ft.robField, ConsumptionFields: fields})
		fuelFields = append(fuelFields, fields...)
	}
	cfg.ROBSeriesList = series
	cfg.FuelTypeConsumptionFields = fuelFields
	return cfg
}

// severity returns the effective severity for ruleID: the config
// override if present, otherwise deflt. Hard-rule callers pass
// SeverityError as deflt and ignore overrides entirely (see
// evaluateConsumptionSchemeExclusivity).
func (c *Config) severity(ruleID string, deflt Severity) Severity {
	if c == nil {
		return deflt
	}
	if s, ok := c.Severities[ruleID]; ok {
		return s
	}
	return deflt
}
