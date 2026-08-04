// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"fmt"
	"math"
)

// Plausibility rule IDs (architecture 10.1, rule class 2). The
// continuity subset (used by cascade revalidation, architecture 8.3)
// lives in continuity.go.
const (
	RuleTimeBucketSum              = "plausibility.timeBucketSum"
	RuleImpliedSpeed               = "plausibility.impliedSpeed"
	RuleNoDistanceStationary       = "plausibility.noDistanceStationary"
	RuleConsumptionSchemeExclusive = "plausibility.consumptionSchemeExclusivity"
	RulePositionRequired           = "plausibility.positionRequired"
	RulePositionConsistency        = "plausibility.positionConsistency"
)

// timeElapsedFields are the Log Abstract time-bucket fields whose sum
// must equal Time_Since_Previous_Report (architecture 10.1).
var timeElapsedFields = []string{
	"Time_Elapsed_Sailing",
	"Time_Elapsed_Anchoring",
	"Time_Elapsed_DP",
	"Time_Elapsed_Ice",
	"Time_Elapsed_Maneuvering",
	"Time_Elapsed_Waiting",
	"Time_Elapsed_Loading_Unloading",
	"Time_Elapsed_Drifting",
}

// BunkerReportLookup reports whether a submitted Bunker Report exists
// with the given BDN number, for the consumption-scheme-exclusivity
// rule. Vessel/office wiring supplies the real lookup against the
// report store; it is a plain function so the rule stays unit
// testable without a store.
type BunkerReportLookup func(bdnNumber string) bool

// EvaluatePlausibilityRules runs the single-report plausibility checks
// (architecture 10.1, rule class 2) against r. lookup may be nil, in
// which case the BDN-cross-reference part of the consumption-scheme
// check is skipped.
func EvaluatePlausibilityRules(r *Report, cfg *Config, lookup BunkerReportLookup) Findings {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	var findings Findings
	findings = append(findings, evaluateTimeBucketSum(r, cfg)...)
	findings = append(findings, evaluateImpliedSpeed(r, cfg)...)
	findings = append(findings, evaluateNoDistanceStationary(r, cfg)...)
	findings = append(findings, evaluateConsumptionSchemeExclusivity(r, cfg, lookup)...)
	findings = append(findings, evaluatePositionRequired(r, cfg)...)
	return findings
}

func evaluateTimeBucketSum(r *Report, cfg *Config) Findings {
	tsp, tspOk := r.Float("Time_Since_Previous_Report")
	if !tspOk {
		return nil
	}
	sum := 0.0
	anyBucket := false
	for _, name := range timeElapsedFields {
		if v, ok := r.Float(name); ok {
			sum += v
			anyBucket = true
		}
	}
	if !anyBucket {
		return nil
	}
	if math.Abs(sum-tsp) > cfg.TimeBucketToleranceHours {
		return Findings{{
			RuleID:   RuleTimeBucketSum,
			Severity: cfg.severity(RuleTimeBucketSum, SeverityWarning),
			Field:    "Time_Since_Previous_Report",
			Message:  fmt.Sprintf("time elapsed buckets sum to %.2fh, but Time_Since_Previous_Report is %.2fh", sum, tsp),
		}}
	}
	return nil
}

func evaluateImpliedSpeed(r *Report, cfg *Config) Findings {
	distance, distOk := r.Float("Distance")
	sailingHours, sailOk := r.Float("Time_Elapsed_Sailing")
	if !distOk || !sailOk || sailingHours <= 0 {
		return nil
	}
	speed := distance / sailingHours
	if speed < cfg.ImpliedSpeedMinKn || speed > cfg.ImpliedSpeedMaxKn {
		return Findings{{
			RuleID:   RuleImpliedSpeed,
			Severity: cfg.severity(RuleImpliedSpeed, SeverityWarning),
			Field:    "Distance",
			Message:  fmt.Sprintf("implied speed %.1f kn is outside the plausible range %.0f-%.0f kn", speed, cfg.ImpliedSpeedMinKn, cfg.ImpliedSpeedMaxKn),
		}}
	}
	return nil
}

// evaluateNoDistanceStationary checks "no distance greater than 0 while
// moored or at anchor for the full period" (architecture 10.1). At
// anchor is detected from Time_Elapsed_Anchoring covering the whole
// period; moored is approximated from Mode == "InPort", the closest
// concrete signal available in the curated Log Abstract schema (the OVD
// data has no dedicated "moored" time bucket distinct from anchoring).
func evaluateNoDistanceStationary(r *Report, cfg *Config) Findings {
	distance, distOk := r.Float("Distance")
	if !distOk || distance <= 0 {
		return nil
	}
	stationary := false
	if tsp, tspOk := r.Float("Time_Since_Previous_Report"); tspOk {
		if anchoring, ok := r.Float("Time_Elapsed_Anchoring"); ok && tsp > 0 && anchoring >= tsp-cfg.TimeBucketToleranceHours {
			stationary = true
		}
	}
	if mode, ok := r.String("Mode"); ok && mode == "InPort" {
		stationary = true
	}
	if stationary {
		return Findings{{
			RuleID:   RuleNoDistanceStationary,
			Severity: cfg.severity(RuleNoDistanceStationary, SeverityWarning),
			Field:    "Distance",
			Message:  "distance greater than 0 reported while moored or at anchor for the full period",
		}}
	}
	return nil
}

// evaluateConsumptionSchemeExclusivity is a hard OVD rule (architecture
// 10.1): fuel-type-based and BDN-based consumption must not both be
// reported on the same event, and BDN-based consumption requires a
// matching submitted Bunker Report. Its severity is always
// SeverityError and cannot be relaxed by config (architecture 10.2).
func evaluateConsumptionSchemeExclusivity(r *Report, cfg *Config, lookup BunkerReportLookup) Findings {
	var findings Findings
	fuelTypeReported := false
	for _, name := range cfg.FuelTypeConsumptionFields {
		if !r.IsEmpty(name) {
			fuelTypeReported = true
			break
		}
	}
	var bdnFields []string
	for _, name := range cfg.BDNMarkerFields {
		if !r.IsEmpty(name) {
			bdnFields = append(bdnFields, name)
		}
	}
	bdnReported := len(bdnFields) > 0

	if fuelTypeReported && bdnReported {
		findings = append(findings, Finding{
			RuleID:   RuleConsumptionSchemeExclusive,
			Severity: SeverityError,
			Message:  "both fuel-type-based and BDN-based consumption are reported on the same event",
		})
	}
	if lookup != nil {
		for _, field := range bdnFields {
			bdn, _ := r.String(field)
			if bdn != "" && !lookup(bdn) {
				findings = append(findings, Finding{
					RuleID:   RuleConsumptionSchemeExclusive,
					Severity: SeverityError,
					Field:    field,
					Message:  fmt.Sprintf("no submitted Bunker Report found for BDN number %q", bdn),
				})
			}
		}
	}
	return findings
}

// evaluatePositionRequired checks that position is present when at sea
// and moving, and that reported lat/long components are internally
// consistent (architecture 10.1). "At sea and moving" is approximated
// from Mode == "AtSea" or "Sailing".
func evaluatePositionRequired(r *Report, cfg *Config) Findings {
	var findings Findings
	mode, _ := r.String("Mode")
	atSeaMoving := mode == "AtSea" || mode == "Sailing"

	latDeg, latDegOk := r.Float("Latitude_Degree")
	latNS, latNSOk := r.String("Latitude_North_South")
	lonDeg, lonDegOk := r.Float("Longitude_Degree")
	lonEW, lonEWOk := r.String("Longitude_East_West")

	if atSeaMoving && (!latDegOk || !lonDegOk || !latNSOk || !lonEWOk) {
		findings = append(findings, Finding{
			RuleID:   RulePositionRequired,
			Severity: cfg.severity(RulePositionRequired, SeverityWarning),
			Message:  "position is required when at sea and moving",
		})
	}

	if latDegOk && (latDeg < 0 || latDeg > 90) {
		findings = append(findings, positionConsistencyFinding(cfg, "Latitude_Degree", "latitude degrees must be between 0 and 90"))
	}
	if v, ok := r.Float("Latitude_Minutes"); ok && (v < 0 || v >= 60) {
		findings = append(findings, positionConsistencyFinding(cfg, "Latitude_Minutes", "latitude minutes must be between 0 and 60"))
	}
	if latNSOk && latNS != "N" && latNS != "S" {
		findings = append(findings, positionConsistencyFinding(cfg, "Latitude_North_South", `latitude hemisphere must be "N" or "S"`))
	}
	if lonDegOk && (lonDeg < 0 || lonDeg > 180) {
		findings = append(findings, positionConsistencyFinding(cfg, "Longitude_Degree", "longitude degrees must be between 0 and 180"))
	}
	if v, ok := r.Float("Longitude_Minutes"); ok && (v < 0 || v >= 60) {
		findings = append(findings, positionConsistencyFinding(cfg, "Longitude_Minutes", "longitude minutes must be between 0 and 60"))
	}
	if lonEWOk && lonEW != "E" && lonEW != "W" {
		findings = append(findings, positionConsistencyFinding(cfg, "Longitude_East_West", `longitude hemisphere must be "E" or "W"`))
	}
	return findings
}

func positionConsistencyFinding(cfg *Config, field, message string) Finding {
	return Finding{
		RuleID:   RulePositionConsistency,
		Severity: cfg.severity(RulePositionConsistency, SeverityError),
		Field:    field,
		Message:  message,
	}
}
