// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/enrollment"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/office/vessels"
	"github.com/captv89/ovl/pkg/fieldproject"
	"github.com/captv89/ovl/pkg/schema"
)

// dashboardOverdueVesselView is one row of design handoff B1's "Overdue
// vessels" widget table.
type dashboardOverdueVesselView struct {
	VesselID     string    `json:"vesselId"`
	VesselName   string    `json:"vesselName"`
	VesselIMO    string    `json:"vesselImo"`
	Groups       []string  `json:"groups"`
	LastReportAt time.Time `json:"lastReportAt"`
	OverdueHours float64   `json:"overdueHours"`
}

// dashboardDataQualityPointView is one day's bar in B1's "Data quality"
// sparkline — errors + warnings summed across every report whose latest
// version's event_time falls on that day.
type dashboardDataQualityPointView struct {
	Date     string `json:"date"` // "2026-07-15"
	Errors   int    `json:"errors"`
	Warnings int    `json:"warnings"`
}

// dashboardOperationsRow is one vessel's row in B1/architecture 16's
// "operations overview" widget: simple consumption and distance over the
// caller-selected period (dashboardOperationsDefaultDays, ?opsDays=).
// Scoped to log-abstract (the only curated schema carrying Distance/
// consumption fields) — TotalConsumptionMt sums every decimal field
// whose unit is "mt" and name mentions "Consumption" (ME/AE/Boiler/
// inert-gas across every fuel type), schema-driven rather than a
// hardcoded field list, so it stays correct if the schema ever adds
// another consumer or fuel type.
type dashboardOperationsRow struct {
	VesselID           string  `json:"vesselId"`
	VesselName         string  `json:"vesselName"`
	VesselIMO          string  `json:"vesselImo"`
	TotalDistanceNM    float64 `json:"totalDistanceNm"`
	TotalConsumptionMt float64 `json:"totalConsumptionMt"`
	ReportCount        int     `json:"reportCount"`
}

// dashboardView is B1's whole-screen payload. Reporting compliance %,
// the data-quality trend, and the operations overview are all computed
// live from current data, not a stored historical time series (none
// exists) — see this file's own handler comment for exactly what each
// measures. The "OVD sync status" widget architecture 16 originally
// specified alongside operations overview is gone — it existed to
// surface the now-cancelled Veracity push integration's health (DNV
// declined API access, see architecture handoff section 13's own note)
// and has no equivalent in the pull-based data API that replaced it;
// operations overview below takes its slot rather than leaving it
// empty.
type dashboardView struct {
	// OverdueVessels is capped at dashboardOverdueVesselCap (worst
	// first); OverdueVesselCount is the true total so the KPI tile never
	// undercounts once a fleet has more overdue vessels than the table
	// shows.
	OverdueVessels       []dashboardOverdueVesselView    `json:"overdueVessels"`
	OverdueVesselCount   int                             `json:"overdueVesselCount"`
	EnrolledVesselCount  int                             `json:"enrolledVesselCount"`
	CompliancePercent    float64                         `json:"compliancePercent"`
	ReportsNeedingReview int                             `json:"reportsNeedingReview"`
	DataQualityTrend     []dashboardDataQualityPointView `json:"dataQualityTrend"`
	OperationsOverview   []dashboardOperationsRow        `json:"operationsOverview"`
	OperationsPeriodDays int                             `json:"operationsPeriodDays"`
}

const dashboardOverdueVesselCap = 10
const dashboardTrendDays = 7
const dashboardOperationsDefaultDays = 30
const dashboardOperationsMaxDays = 365

// handleGetDashboard serves design handoff B1's Dashboard, scoped by an
// optional ?group= query param (the office-wide global group filter,
// Office UI rework Phase O1). Viewable by any authenticated office user
// — the dashboard is a read-only rollup, no different from the screens
// its own widgets link into.
func (s *Server) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
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

	lastReports, rules, ok := s.loadOverdueInputs(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()

	// Initialized non-nil, not just declared: a nil slice marshals to
	// JSON null, and the frontend calls .length on this unconditionally
	// (a fleet with zero currently-overdue vessels is the normal case,
	// not an edge case — this crashed the whole Dashboard the first time
	// a live test happened to have none, caught during Phase O7's
	// verification).
	overdue := []dashboardOverdueVesselView{}
	enrolledCount, compliantCount := 0, 0
	for _, v := range vesselList {
		_, state, ok := s.enrollmentStateOf(w, r, v.ID)
		if !ok {
			return
		}
		if state != string(enrollment.StateEnrolled) {
			continue
		}
		enrolledCount++

		var lastReportAt *time.Time
		if t, ok := lastReports[v.ID]; ok {
			lastReportAt = &t
		}
		last, overdueHours := overdueStatusFor(v, lastReportAt, rules, now)
		if overdueHours == nil {
			// Not overdue, or has never reported yet (a grace period, not
			// a violation — see overdueStatusFor's own doc comment).
			compliantCount++
			continue
		}
		overdue = append(overdue, dashboardOverdueVesselView{
			VesselID: v.ID, VesselName: v.Name, VesselIMO: v.IMO, Groups: v.Groups,
			LastReportAt: *last, OverdueHours: *overdueHours,
		})
	}
	sort.Slice(overdue, func(i, j int) bool { return overdue[i].OverdueHours > overdue[j].OverdueHours })
	overdueVesselCount := len(overdue)
	if len(overdue) > dashboardOverdueVesselCap {
		overdue = overdue[:dashboardOverdueVesselCap]
	}

	compliancePercent := 100.0
	if enrolledCount > 0 {
		compliancePercent = float64(compliantCount) / float64(enrolledCount) * 100
	}

	notYetReviewed := false
	remarked := "remarked"
	filter := reportFilterForGroup(group)
	filter.State = &remarked
	filter.Reviewed = &notYetReviewed
	needingReviewRows, err := s.st.ListReports(r.Context(), filter)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	trendFrom := now.AddDate(0, 0, -dashboardTrendDays+1)
	trendFilter := reportFilterForGroup(group)
	trendFilter.DateFrom = &trendFrom
	trendRows, err := s.st.ListReports(r.Context(), trendFilter)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ev := newHealthEvaluator(s.st)
	byDay := make(map[string]dashboardDataQualityPointView)
	for _, row := range trendRows {
		health := ev.row(r.Context(), row)
		day := row.EventTime.UTC().Format("2006-01-02")
		point := byDay[day]
		point.Date = day
		point.Errors += health.Errors
		point.Warnings += health.Warnings
		byDay[day] = point
	}
	trend := make([]dashboardDataQualityPointView, 0, dashboardTrendDays)
	for i := 0; i < dashboardTrendDays; i++ {
		day := trendFrom.AddDate(0, 0, i).Format("2006-01-02")
		if point, ok := byDay[day]; ok {
			trend = append(trend, point)
		} else {
			trend = append(trend, dashboardDataQualityPointView{Date: day})
		}
	}

	opsDays := dashboardOperationsDefaultDays
	if raw := r.URL.Query().Get("opsDays"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= dashboardOperationsMaxDays {
			opsDays = n
		}
	}
	operations, err := s.dashboardOperationsOverview(r.Context(), group, now, opsDays)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpjson.WriteJSON(w, http.StatusOK, dashboardView{
		OverdueVessels:       overdue,
		OverdueVesselCount:   overdueVesselCount,
		EnrolledVesselCount:  enrolledCount,
		CompliancePercent:    compliancePercent,
		ReportsNeedingReview: len(needingReviewRows),
		DataQualityTrend:     trend,
		OperationsOverview:   operations,
		OperationsPeriodDays: opsDays,
	})
}

// dashboardOperationsOverview computes architecture 16's "operations
// overview: simple consumption and distance view per vessel and group
// over a selectable period" — scoped to log-abstract (the only curated
// schema carrying Distance/consumption fields), summing the schema's own
// Distance field and every decimal field whose unit is "mt" and name
// mentions "Consumption", per vessel, sorted highest-consumption first
// (same "worst/most first" convention the overdue-vessels widget uses).
func (s *Server) dashboardOperationsOverview(ctx context.Context, group string, now time.Time, days int) ([]dashboardOperationsRow, error) {
	logAbstract := "log-abstract"
	from := now.AddDate(0, 0, -days)
	filter := reportFilterForGroup(group)
	filter.SchemaName = &logAbstract
	filter.DateFrom = &from
	rows, err := s.st.ListReports(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []dashboardOperationsRow{}, nil
	}

	// No published log-abstract schema version is a real, reachable
	// state (a fresh install before Phase 1's curated-schema seeding has
	// ever run, or a test fixture) — degrade to report counts only
	// rather than failing the whole dashboard, mirroring
	// loadSchemaHealthContext's own "missing schema" resilience instead
	// of treating this differently from that established pattern.
	var sch *schema.Schema
	if latest, err := s.st.LatestSchemaVersion(ctx, logAbstract); err == nil {
		if parsed, err := schema.Parse(latest.Content); err == nil {
			sch = parsed
		}
	}
	var consumptionFields []schema.Field
	var distanceField schema.Field
	var hasDistance bool
	if sch != nil {
		for _, f := range sch.Fields {
			if f.Type == schema.FieldTypeDecimal && f.Unit != nil && *f.Unit == "mt" && strings.Contains(f.Name, "Consumption") {
				consumptionFields = append(consumptionFields, f)
			}
		}
		distanceField, hasDistance = sch.FieldByName("Distance")
	}

	byVessel := make(map[string]*dashboardOperationsRow)
	var order []string
	for _, row := range rows {
		agg, ok := byVessel[row.VesselID]
		if !ok {
			agg = &dashboardOperationsRow{VesselID: row.VesselID, VesselName: row.VesselName, VesselIMO: row.VesselIMO}
			byVessel[row.VesselID] = agg
			order = append(order, row.VesselID)
		}
		agg.ReportCount++
		if hasDistance {
			if v, err := fieldproject.Project(distanceField, row.Fields[distanceField.Name]); err == nil && v.Kind == fieldproject.KindNumber {
				agg.TotalDistanceNM += v.Number
			}
		}
		for _, f := range consumptionFields {
			if v, err := fieldproject.Project(f, row.Fields[f.Name]); err == nil && v.Kind == fieldproject.KindNumber {
				agg.TotalConsumptionMt += v.Number
			}
		}
	}

	out := make([]dashboardOperationsRow, 0, len(order))
	for _, id := range order {
		out = append(out, *byVessel[id])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalConsumptionMt > out[j].TotalConsumptionMt })
	return out, nil
}

func reportFilterForGroup(group string) store.ReportFilter {
	var filter store.ReportFilter
	if group != "" {
		filter.GroupID = &group
	}
	return filter
}
