// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

func mustParseUTC(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01-02T15:04", s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}

func TestComputeVoyageSummary_NoReports(t *testing.T) {
	if got := computeVoyageSummary(nil); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestComputeVoyageSummary_NoVoyageNumberYet(t *testing.T) {
	r := &domain.Report{
		EventTime: mustParseUTC(t, "2026-07-08T06:00"),
		Fields:    map[string]any{"IMO": 9481001.0},
	}
	got := computeVoyageSummary([]*domain.Report{r})
	if got == nil {
		t.Fatal("expected non-nil summary")
	}
	if got.VoyageNumber != "" || got.DistanceSailedNm != nil || got.ProgressPercent != nil {
		t.Fatalf("expected an honest empty voyage view, got %+v", got)
	}
	if got.IMO != "9481001" {
		t.Fatalf("IMO = %q, want 9481001", got.IMO)
	}
}

func TestComputeVoyageSummary_IdentityAndETA(t *testing.T) {
	r := &domain.Report{
		EventTime: mustParseUTC(t, "2026-07-02T06:12"),
		Fields: map[string]any{
			"IMO":           9481001.0,
			"Voyage_Number": "V-2418",
			"Voyage_From":   "SGSIN",
			"Voyage_To":     "AEFJR",
			"ETA":           "2026-07-04 09:00",
		},
	}
	got := computeVoyageSummary([]*domain.Report{r})
	if got.VoyageNumber != "V-2418" || got.FromPort != "SGSIN" || got.ToPort != "AEFJR" {
		t.Fatalf("unexpected identity: %+v", got)
	}
	if got.ETA == nil || !got.ETA.Equal(mustParseUTC(t, "2026-07-04T09:00")) {
		t.Fatalf("ETA = %v, want 2026-07-04T09:00", got.ETA)
	}
	// A lone report is its own departure — sailed distance stays nil
	// (no Distance field entered yet), not fabricated as 0.
	if got.DepartedAt == nil || !got.DepartedAt.Equal(r.EventTime) {
		t.Fatalf("DepartedAt = %v, want %v", got.DepartedAt, r.EventTime)
	}
	if got.DistanceSailedNm != nil {
		t.Fatalf("expected nil DistanceSailedNm, got %v", *got.DistanceSailedNm)
	}
}

func TestComputeVoyageSummary_ProgressAcrossVoyage(t *testing.T) {
	departure := &domain.Report{
		EventTime: mustParseUTC(t, "2026-07-02T06:12"),
		Fields: map[string]any{
			"Voyage_Number": "V-2418",
			"Voyage_From":   "SGSIN",
			"Voyage_To":     "AEFJR",
			"Distance":      12.0,
		},
	}
	noon := &domain.Report{
		EventTime: mustParseUTC(t, "2026-07-03T12:00"),
		Fields: map[string]any{
			"Voyage_Number":  "V-2418",
			"Voyage_From":    "SGSIN",
			"Voyage_To":      "AEFJR",
			"Distance":       288.0,
			"Distance_To_Go": 700.0,
		},
	}
	// A report from a *previous* voyage must not bleed into this one's totals.
	priorVoyage := &domain.Report{
		EventTime: mustParseUTC(t, "2026-06-30T08:05"),
		Fields:    map[string]any{"Voyage_Number": "V-2417", "Distance": 9999.0},
	}

	got := computeVoyageSummary([]*domain.Report{noon, departure, priorVoyage})
	if got.VoyageNumber != "V-2418" {
		t.Fatalf("VoyageNumber = %q, want V-2418", got.VoyageNumber)
	}
	if got.DepartedAt == nil || !got.DepartedAt.Equal(departure.EventTime) {
		t.Fatalf("DepartedAt = %v, want %v", got.DepartedAt, departure.EventTime)
	}
	if got.DistanceSailedNm == nil || *got.DistanceSailedNm != 300.0 {
		t.Fatalf("DistanceSailedNm = %v, want 300 (12+288)", got.DistanceSailedNm)
	}
	if got.DistanceRemainingNm == nil || *got.DistanceRemainingNm != 700.0 {
		t.Fatalf("DistanceRemainingNm = %v, want 700", got.DistanceRemainingNm)
	}
	wantPct := 300.0 / 1000.0 * 100
	if got.ProgressPercent == nil || *got.ProgressPercent != wantPct {
		t.Fatalf("ProgressPercent = %v, want %v", got.ProgressPercent, wantPct)
	}
}

func TestComputeVoyageSummary_Position(t *testing.T) {
	r := &domain.Report{
		EventTime: mustParseUTC(t, "2026-07-02T12:00"),
		Fields: map[string]any{
			"Voyage_Number":        "V-2418",
			"Latitude_Degree":      1.0,
			"Latitude_Minutes":     14.2,
			"Latitude_North_South": "N",
			"Longitude_Degree":     104.0,
			"Longitude_Minutes":    2.8,
			"Longitude_East_West":  "E",
		},
	}
	got := computeVoyageSummary([]*domain.Report{r})
	if got.Position == nil {
		t.Fatal("expected a position")
	}
	wantLat := 1.0 + 14.2/60
	wantLon := 104.0 + 2.8/60
	if got.Position.Latitude != wantLat || got.Position.Longitude != wantLon {
		t.Fatalf("Position = %+v, want lat %v lon %v", got.Position, wantLat, wantLon)
	}
}

func TestComputeVoyageSummary_PositionIncomplete(t *testing.T) {
	r := &domain.Report{
		EventTime: mustParseUTC(t, "2026-07-02T12:00"),
		Fields:    map[string]any{"Latitude_Degree": 1.0, "Latitude_Minutes": 14.2},
	}
	if got := computeVoyageSummary([]*domain.Report{r}); got.Position != nil {
		t.Fatalf("expected nil position with an incomplete triple, got %+v", got.Position)
	}
}

func TestComputeVoyageSummary_SouthAndWestHemispheresNegate(t *testing.T) {
	r := &domain.Report{
		EventTime: mustParseUTC(t, "2026-07-02T12:00"),
		Fields: map[string]any{
			"Latitude_Degree": 1.0, "Latitude_Minutes": 14.2, "Latitude_North_South": "S",
			"Longitude_Degree": 104.0, "Longitude_Minutes": 2.8, "Longitude_East_West": "W",
		},
	}
	got := computeVoyageSummary([]*domain.Report{r})
	if got.Position.Latitude >= 0 || got.Position.Longitude >= 0 {
		t.Fatalf("expected negative lat/lon for S/W, got %+v", got.Position)
	}
}
