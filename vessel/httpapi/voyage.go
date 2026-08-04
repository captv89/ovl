// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/validation"
)

// dateTimeFieldLayout matches pkg/validation's own dateTimeLayout (which in
// turn matches the OVD 3.13 interface description's "yyyy-mm-dd hh:mm"
// format) and DateTimeField's "datetime" mode contract
// (web/vessel/src/design/components/forms/DateTimeField.d.ts): always UTC,
// no timezone suffix stored. Was briefly "2006-01-02T15:04" (a "T" separator
// borrowed from the native datetime-local input convention) until the
// 2026-08-02 ETA-field bug fix found that pkg/validation's field.format
// rule never accepted "T" — every dateTime-typed field (ETA/RTA,
// Period_Start/End) failed validation regardless of entry method.
const dateTimeFieldLayout = "2006-01-02 15:04"

// voyagePositionView is the vessel's position as of the latest report,
// reconstructed from the same Degree/Minutes/Hemisphere triple the officer
// entered (pkg/schema's position section) rather than a lossy round-trip
// through a single decimal value.
type voyagePositionView struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Text      string  `json:"text"`
}

// voyageSummaryView is a read-model for design handoff A3's landing
// dashboard, not a new stored entity: everything in it is derived, on every
// request, from fields already captured on submitted/draft reports (OVD
// schema's voyage/position/distanceAndSpeed sections). This keeps it
// impossible to drift from what the vessel has actually reported — there is
// no second place this data could be edited or fall out of sync. Scoped to
// log-abstract, the only curated schema carrying these fields (same scoping
// as domain.Report.NaturalKey).
type voyageSummaryView struct {
	IMO          string     `json:"imo,omitempty"`
	VoyageNumber string     `json:"voyageNumber,omitempty"`
	FromPort     string     `json:"fromPort,omitempty"`
	ToPort       string     `json:"toPort,omitempty"`
	ETA          *time.Time `json:"eta,omitempty"`
	DepartedAt   *time.Time `json:"departedAt,omitempty"`
	LastReportAt time.Time  `json:"lastReportAt"`

	Position *voyagePositionView `json:"position,omitempty"`

	// Nil, not zero, when the underlying fields were never entered — an
	// honest "not available" the same way SyncStatusFooter is, rather than
	// a fabricated 0.
	DistanceSailedNm    *float64 `json:"distanceSailedNm,omitempty"`
	DistanceRemainingNm *float64 `json:"distanceRemainingNm,omitempty"`
	ProgressPercent     *float64 `json:"progressPercent,omitempty"`
}

// handleGetCurrentVoyage returns the derived voyage summary for the
// vessel's most recent log-abstract report, or null if none exist yet.
func (s *Server) handleGetCurrentVoyage(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	reports, err := st.ListLatestReports(r.Context(), "log-abstract")
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, computeVoyageSummary(reports))
}

// computeVoyageSummary derives the current-voyage view from reports (latest
// version of each report ID, any order). Identity/ETA/position come from
// the single most recent report by EventTime; progress comes from summing
// Distance ("distance over ground" for that reporting period, OVD
// distanceAndSpeed section) across every report sharing the latest report's
// Voyage_Number, against the latest report's own Distance_To_Go.
func computeVoyageSummary(reports []*domain.Report) *voyageSummaryView {
	if len(reports) == 0 {
		return nil
	}
	sorted := make([]*domain.Report, len(reports))
	copy(sorted, reports)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].EventTime.Before(sorted[j].EventTime) })
	latest := sorted[len(sorted)-1]
	lv := latest.ToValidation()

	view := &voyageSummaryView{LastReportAt: latest.EventTime}
	if voyageNumber, ok := lv.String("Voyage_Number"); ok {
		view.VoyageNumber = voyageNumber
	}
	if fromPort, ok := lv.String("Voyage_From"); ok {
		view.FromPort = fromPort
	}
	if toPort, ok := lv.String("Voyage_To"); ok {
		view.ToPort = toPort
	}
	if imo, ok := lv.Float("IMO"); ok {
		view.IMO = strconv.FormatFloat(imo, 'f', 0, 64)
	}
	if etaStr, ok := lv.String("ETA"); ok && etaStr != "" {
		if t, err := time.Parse(dateTimeFieldLayout, etaStr); err == nil {
			view.ETA = &t
		}
	}
	view.Position = readPosition(lv)

	if view.VoyageNumber == "" {
		return view
	}

	// Every report sharing this voyage number, still in EventTime order —
	// architecture 10.1 already treats Voyage_Number as the voyage-
	// continuity key for cross-report plausibility, so reusing it here
	// rather than inventing a second grouping concept.
	var sameVoyage []*domain.Report
	for _, r := range sorted {
		if vn, ok := r.ToValidation().String("Voyage_Number"); ok && vn == view.VoyageNumber {
			sameVoyage = append(sameVoyage, r)
		}
	}
	if len(sameVoyage) == 0 {
		return view
	}
	departedAt := sameVoyage[0].EventTime
	view.DepartedAt = &departedAt

	var sailed float64
	haveSailed := false
	for _, r := range sameVoyage {
		if d, ok := r.ToValidation().Float("Distance"); ok {
			sailed += d
			haveSailed = true
		}
	}
	if !haveSailed {
		return view
	}
	view.DistanceSailedNm = &sailed

	remaining, ok := lv.Float("Distance_To_Go")
	if !ok {
		return view
	}
	view.DistanceRemainingNm = &remaining
	if total := sailed + remaining; total > 0 {
		pct := sailed / total * 100
		if pct > 100 {
			pct = 100
		}
		view.ProgressPercent = &pct
	}
	return view
}

// readPosition reconstructs decimal + human-readable position from the
// Degree/Minutes/Hemisphere triple (pkg/schema position section) the
// officer actually entered — nil if either axis is incomplete.
func readPosition(lv *validation.Report) *voyagePositionView {
	latDeg, latDegOk := lv.Float("Latitude_Degree")
	latMin, latMinOk := lv.Float("Latitude_Minutes")
	latHemi, latHemiOk := lv.String("Latitude_North_South")
	lonDeg, lonDegOk := lv.Float("Longitude_Degree")
	lonMin, lonMinOk := lv.Float("Longitude_Minutes")
	lonHemi, lonHemiOk := lv.String("Longitude_East_West")
	if !latDegOk || !latMinOk || !latHemiOk || !lonDegOk || !lonMinOk || !lonHemiOk {
		return nil
	}
	lat := latDeg + latMin/60
	if strings.EqualFold(latHemi, "S") {
		lat = -lat
	}
	lon := lonDeg + lonMin/60
	if strings.EqualFold(lonHemi, "W") {
		lon = -lon
	}
	text := fmt.Sprintf("%02.0f°%05.2f′%s %03.0f°%05.2f′%s", latDeg, latMin, strings.ToUpper(latHemi), lonDeg, lonMin, strings.ToUpper(lonHemi))
	return &voyagePositionView{Latitude: lat, Longitude: lon, Text: text}
}
