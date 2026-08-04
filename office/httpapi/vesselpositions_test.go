// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/pkg/domain"
)

func TestParseLogAbstractPosition(t *testing.T) {
	tests := []struct {
		name    string
		fields  map[string]any
		wantOK  bool
		wantLat float64
		wantLon float64
	}{
		{
			name: "12N 04.0, 043E 55.0 -> NE quadrant",
			fields: map[string]any{
				"Latitude_Degree": 12.0, "Latitude_Minutes": 4.0, "Latitude_North_South": "N",
				"Longitude_Degree": 43.0, "Longitude_Minutes": 55.0, "Longitude_East_West": "E",
			},
			wantOK: true, wantLat: 12 + 4.0/60, wantLon: 43 + 55.0/60,
		},
		{
			name: "S and W hemispheres negate",
			fields: map[string]any{
				"Latitude_Degree": 33.0, "Latitude_Minutes": 30.0, "Latitude_North_South": "S",
				"Longitude_Degree": 70.0, "Longitude_Minutes": 15.0, "Longitude_East_West": "W",
			},
			wantOK: true, wantLat: -(33 + 30.0/60), wantLon: -(70 + 15.0/60),
		},
		{
			name:   "missing fields",
			fields: map[string]any{"Latitude_Degree": 12.0},
			wantOK: false,
		},
		{
			name: "unrecognized hemisphere letter",
			fields: map[string]any{
				"Latitude_Degree": 12.0, "Latitude_Minutes": 4.0, "Latitude_North_South": "X",
				"Longitude_Degree": 43.0, "Longitude_Minutes": 55.0, "Longitude_East_West": "E",
			},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lat, lon, ok := parseLogAbstractPosition(tt.fields)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if diff := lat - tt.wantLat; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("lat = %v, want %v", lat, tt.wantLat)
			}
			if diff := lon - tt.wantLon; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("lon = %v, want %v", lon, tt.wantLon)
			}
		})
	}
}

// TestHandleListVesselPositions exercises the endpoint end to end: an
// overdue vessel with a real position shows status "overdue"; an
// on-time vessel with a remarked-unreviewed report shows "remarked"; a
// vessel with no Position fields on its latest Log Abstract report is
// excluded entirely rather than plotted at a wrong coordinate.
func TestHandleListVesselPositions(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	rule, err := compliance.NewCadenceRule(compliance.FleetScope(), compliance.DefaultMinReportIntervalHours, 12)
	if err != nil {
		t.Fatalf("NewCadenceRule: %v", err)
	}
	if err := s.st.SaveCadenceRule(context.Background(), rule); err != nil {
		t.Fatalf("SaveCadenceRule: %v", err)
	}

	overdueVessel := createTestVesselForReports(t, s, 95)
	enrollTestVessel(t, s, overdueVessel.ID)
	remarkedVessel := createTestVesselForReports(t, s, 96)
	enrollTestVessel(t, s, remarkedVessel.ID)
	noPositionVessel := createTestVesselForReports(t, s, 97)
	enrollTestVessel(t, s, noPositionVessel.ID)

	now := time.Now().UTC()
	positionFields := map[string]any{
		"IMO": float64(9074729),
		"Latitude_Degree": 12.0, "Latitude_Minutes": 4.0, "Latitude_North_South": "N",
		"Longitude_Degree": 43.0, "Longitude_Minutes": 55.0, "Longitude_East_West": "E",
	}

	overdueReport := &domain.Report{
		ReportID: "report-overdue-pos", VersionNo: 1, SchemaName: "log-abstract", EventType: "Departure",
		EventTime: now.Add(-30 * time.Hour), State: domain.StateSubmitted, Fields: positionFields,
	}
	if err := s.st.UpsertReportVersion(context.Background(), overdueVessel.ID, overdueReport, "3.13", now.Add(-30*time.Hour)); err != nil {
		t.Fatalf("UpsertReportVersion (overdue): %v", err)
	}

	remarkedReport := &domain.Report{
		ReportID: "report-remarked-pos", VersionNo: 1, SchemaName: "log-abstract", EventType: "Noon at Sea",
		EventTime: now.Add(-1 * time.Hour), State: domain.StateRemarked, Fields: positionFields,
	}
	if err := s.st.UpsertReportVersion(context.Background(), remarkedVessel.ID, remarkedReport, "3.13", now.Add(-1*time.Hour)); err != nil {
		t.Fatalf("UpsertReportVersion (remarked): %v", err)
	}

	landTestReport(t, s, noPositionVessel.ID, "report-no-pos", 1, now.Add(-1*time.Hour), domain.StateSubmitted)

	rec := c.do(http.MethodGet, "/api/vessels/positions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/vessels/positions: status %d, body %s", rec.Code, rec.Body)
	}
	list := decodeBody[[]vesselPositionView](t, rec)

	byID := make(map[string]vesselPositionView, len(list))
	for _, v := range list {
		byID[v.VesselID] = v
	}

	overdue, ok := byID[overdueVessel.ID]
	if !ok {
		t.Fatalf("overdue vessel missing from %+v", list)
	}
	if overdue.Status != "overdue" {
		t.Errorf("overdue vessel Status = %q, want %q", overdue.Status, "overdue")
	}
	if overdue.Lat < 12 || overdue.Lat > 12.1 {
		t.Errorf("overdue vessel Lat = %v, want ~12.07", overdue.Lat)
	}

	remarked, ok := byID[remarkedVessel.ID]
	if !ok {
		t.Fatalf("remarked vessel missing from %+v", list)
	}
	if remarked.Status != "remarked" {
		t.Errorf("remarked vessel Status = %q, want %q", remarked.Status, "remarked")
	}

	if _, present := byID[noPositionVessel.ID]; present {
		t.Errorf("vessel with no Position fields on its latest report should be excluded, found %+v", byID[noPositionVessel.ID])
	}
}
