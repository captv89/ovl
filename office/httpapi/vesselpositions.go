// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/enrollment"
	"github.com/captv89/ovl/office/vessels"
)

// vesselPositionView is design handoff B2·M's fleet map marker. Status
// mirrors Tideline's fixed bridge-alert marker convention (green
// underway/OK, amber caution, red alert — see the design system's own
// readme) rather than a vessel navigational state this project has no
// data for: red means overdue (Phase O3's own calculation), amber means
// the vessel has at least one remarked report still awaiting review,
// green ("ok") otherwise.
type vesselPositionView struct {
	VesselID   string    `json:"vesselId"`
	VesselName string    `json:"vesselName"`
	VesselIMO  string    `json:"vesselImo"`
	Groups     []string  `json:"groups"`
	Lat        float64   `json:"lat"`
	Lon        float64   `json:"lon"`
	Status     string    `json:"status"` // "ok" | "remarked" | "overdue"
	AsOf       time.Time `json:"asOf"`
}

// parseLogAbstractPosition extracts a plottable coordinate from a Log
// Abstract report's field values (architecture's own compound degree +
// decimal-minute + hemisphere representation — see schemas/ovd-3.13/
// log-abstract.json's Latitude_Degree/Latitude_Minutes/
// Latitude_North_South trio and its Longitude counterpart). Returns
// ok=false if any of the six sub-fields is missing or unparseable —
// most reports won't have all six filled (Position is not schema-
// mandatory), so a vessel simply doesn't appear on the map until one
// does, rather than plotting a wrong/default coordinate.
func parseLogAbstractPosition(fields map[string]any) (lat, lon float64, ok bool) {
	latDeg, ok1 := asFloat(fields["Latitude_Degree"])
	latMin, ok2 := asFloat(fields["Latitude_Minutes"])
	latHemi, ok3 := asHemisphere(fields["Latitude_North_South"], 'N', 'S')
	lonDeg, ok4 := asFloat(fields["Longitude_Degree"])
	lonMin, ok5 := asFloat(fields["Longitude_Minutes"])
	lonHemi, ok6 := asHemisphere(fields["Longitude_East_West"], 'E', 'W')
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
		return 0, 0, false
	}
	lat = (latDeg + latMin/60) * latHemi
	lon = (lonDeg + lonMin/60) * lonHemi
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, false
	}
	return lat, lon, true
}

func asFloat(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

// asHemisphere reads a single-letter hemisphere indicator and returns
// +1/-1 to multiply the coordinate by. positive/negative are the two
// letters that mean +1 ('N' or 'E') and -1 ('S' or 'W') respectively —
// matched case-insensitively since the field is free text (no enumRef),
// not a validated enum.
func asHemisphere(v any, positive, negative byte) (float64, bool) {
	s, ok := v.(string)
	if !ok || len(s) == 0 {
		return 0, false
	}
	switch s[0] | 0x20 { // lowercase
	case positive | 0x20:
		return 1, true
	case negative | 0x20:
		return -1, true
	default:
		return 0, false
	}
}

// handleListVesselPositions serves design handoff B2·M's fleet map,
// optionally scoped by ?group= (Phase O1's global filter, same as the
// Vessels list and Dashboard). Viewable by any authenticated office
// user — a read-only view of the same data the Vessels list already
// shows, just plotted instead of tabulated.
func (s *Server) handleListVesselPositions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	group := r.URL.Query().Get("group")

	var vesselList []*vessels.Vessel
	var err error
	if group != "" {
		vesselList, err = s.st.ListVesselsByGroup(r.Context(), group)
	} else {
		vesselList, err = s.st.ListVessels(r.Context())
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	positions, err := s.st.LatestLogAbstractFieldsByVessel(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	lastReports, rules, ok := s.loadOverdueInputs(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()

	remarkedNeedingReview, err := s.vesselsWithUnreviewedRemarks(r.Context(), group)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Non-nil so a fleet with zero plottable positions marshals to `[]`,
	// not `null` — see dashboard.go's own comment on the exact same
	// class of bug this phase's live verification caught.
	out := []vesselPositionView{}
	for _, v := range vesselList {
		_, state, ok := s.enrollmentStateOf(w, r, v.ID)
		if !ok {
			return
		}
		if state != string(enrollment.StateEnrolled) {
			continue
		}
		row, hasPosition := positions[v.ID]
		if !hasPosition {
			continue
		}
		lat, lon, parsed := parseLogAbstractPosition(row.Fields)
		if !parsed {
			continue
		}

		var lastReportAt *time.Time
		if t, ok := lastReports[v.ID]; ok {
			lastReportAt = &t
		}
		_, overdueHours := overdueStatusFor(v, lastReportAt, rules, now)

		status := "ok"
		switch {
		case overdueHours != nil:
			status = "overdue"
		case remarkedNeedingReview[v.ID]:
			status = "remarked"
		}

		out = append(out, vesselPositionView{
			VesselID: v.ID, VesselName: v.Name, VesselIMO: v.IMO, Groups: v.Groups,
			Lat: lat, Lon: lon, Status: status, AsOf: row.EventTime,
		})
	}

	httpjson.WriteJSON(w, http.StatusOK, out)
}

// vesselsWithUnreviewedRemarks returns the set of vessel ids that have
// at least one remarked, not-yet-reviewed report — the same query shape
// Phase O5's dashboard "reports needing review" KPI uses, reduced to a
// per-vessel set instead of a count.
func (s *Server) vesselsWithUnreviewedRemarks(ctx context.Context, group string) (map[string]bool, error) {
	notYetReviewed := false
	remarked := "remarked"
	filter := reportFilterForGroup(group)
	filter.State = &remarked
	filter.Reviewed = &notYetReviewed
	rows, err := s.st.ListReports(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		out[row.VesselID] = true
	}
	return out, nil
}
