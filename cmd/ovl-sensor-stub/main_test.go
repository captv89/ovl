// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func approxEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

func TestSeed_PositionAt(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC) // 48h voyage
	s := seed{StartLat: 0, StartLon: 0, EndLat: 10, EndLon: 20, VoyageStart: start, VoyageEnd: end}

	t.Run("at voyage start", func(t *testing.T) {
		lat, lon := s.positionAt(start)
		if lat != 0 || lon != 0 {
			t.Errorf("positionAt(start) = (%v, %v), want (0, 0)", lat, lon)
		}
	})

	t.Run("at voyage end", func(t *testing.T) {
		lat, lon := s.positionAt(end)
		if lat != 10 || lon != 20 {
			t.Errorf("positionAt(end) = (%v, %v), want (10, 20)", lat, lon)
		}
	})

	t.Run("halfway through", func(t *testing.T) {
		lat, lon := s.positionAt(start.Add(24 * time.Hour))
		if !approxEqual(lat, 5, 0.001) || !approxEqual(lon, 10, 0.001) {
			t.Errorf("positionAt(midpoint) = (%v, %v), want ~(5, 10)", lat, lon)
		}
	})

	t.Run("clamps before voyage start", func(t *testing.T) {
		lat, lon := s.positionAt(start.Add(-24 * time.Hour))
		if lat != 0 || lon != 0 {
			t.Errorf("positionAt(before start) = (%v, %v), want clamped to start (0, 0)", lat, lon)
		}
	})

	t.Run("clamps after voyage end", func(t *testing.T) {
		lat, lon := s.positionAt(end.Add(24 * time.Hour))
		if lat != 10 || lon != 20 {
			t.Errorf("positionAt(after end) = (%v, %v), want clamped to end (10, 20)", lat, lon)
		}
	})
}

func TestHaversineNM(t *testing.T) {
	// Same point: zero distance.
	if d := haversineNM(1.29, 103.85, 1.29, 103.85); d != 0 {
		t.Errorf("haversineNM(same point) = %v, want 0", d)
	}
	// One degree of latitude is ~60 nautical miles by definition (a
	// nautical mile is defined as one minute of arc).
	if d := haversineNM(0, 0, 1, 0); !approxEqual(d, 60, 0.5) {
		t.Errorf("haversineNM(1 degree latitude) = %v, want ~60nm", d)
	}
}

func TestInitialBearingDeg(t *testing.T) {
	if b := initialBearingDeg(0, 0, 0, 0); b != 0 {
		t.Errorf("initialBearingDeg(same point) = %v, want 0", b)
	}
	// Due north.
	if b := initialBearingDeg(0, 0, 1, 0); !approxEqual(b, 0, 0.1) {
		t.Errorf("initialBearingDeg(due north) = %v, want ~0", b)
	}
	// Due east.
	if b := initialBearingDeg(0, 0, 0, 1); !approxEqual(b, 90, 0.1) {
		t.Errorf("initialBearingDeg(due east) = %v, want ~90", b)
	}
}

func TestHandleReadings(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	s := seed{StartLat: 0, StartLon: 0, EndLat: 10, EndLon: 20, VoyageStart: start, VoyageEnd: end, APIKey: "test-key"}
	handler := handleReadings(s)

	doRequest := func(t *testing.T, auth, from, to string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/readings?from="+from+"&to="+to, nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		handler(w, req)
		return w
	}

	t.Run("missing bearer key is unauthorized", func(t *testing.T) {
		rec := doRequest(t, "", "2026-07-01T00:00:00Z", "2026-07-01T12:00:00Z")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("wrong bearer key is unauthorized", func(t *testing.T) {
		rec := doRequest(t, "Bearer wrong-key", "2026-07-01T00:00:00Z", "2026-07-01T12:00:00Z")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("missing time params is a bad request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readings", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("returns logically consistent position/speed/distance for the window", func(t *testing.T) {
		// [start, start+24h] spans exactly the first half of the voyage:
		// position at `to` should be the interpolated midpoint (5, 10),
		// distance/speed should reflect the great-circle distance covered
		// over those 24 hours, deterministically (not randomized).
		rec := doRequest(t, "Bearer test-key", "2026-07-01T00:00:00Z", "2026-07-02T00:00:00Z")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		var body struct {
			Readings map[string]any `json:"readings"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		lat, _ := body.Readings["latitude"].(float64)
		lon, _ := body.Readings["longitude"].(float64)
		if !approxEqual(lat, 5, 0.01) || !approxEqual(lon, 10, 0.01) {
			t.Errorf("latitude/longitude = %v/%v, want ~5/~10", lat, lon)
		}
		wantDistance := haversineNM(0, 0, 5, 10)
		distanceNM, _ := body.Readings["distance_nm"].(float64)
		if !approxEqual(distanceNM, wantDistance, 0.5) {
			t.Errorf("distance_nm = %v, want ~%v (haversine from start to midpoint)", distanceNM, wantDistance)
		}
		wantSpeed := wantDistance / 24
		speedGPS, _ := body.Readings["speed_gps_kn"].(float64)
		if !approxEqual(speedGPS, wantSpeed, 0.5) {
			t.Errorf("speed_gps_kn = %v, want ~%v (distance/24h)", speedGPS, wantSpeed)
		}
		// Randomized-but-plausible fields must still be present and
		// within their documented bounds.
		hfo, _ := body.Readings["me_consumption_hfo_mt"].(float64)
		if hfo < 0 || hfo > 24*1.5 {
			t.Errorf("me_consumption_hfo_mt = %v, want within [0, 36] for a 24h window", hfo)
		}
		wind, _ := body.Readings["wind_speed_kn"].(float64)
		if wind < 0 || wind > 25 {
			t.Errorf("wind_speed_kn = %v, want within [0, 25]", wind)
		}
	})

	t.Run("weather fields are present and internally consistent", func(t *testing.T) {
		rec := doRequest(t, "Bearer test-key", "2026-07-01T00:00:00Z", "2026-07-02T00:00:00Z")
		var body struct {
			Readings map[string]any `json:"readings"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		wantKeys := []string{
			"wind_dir_deg", "wind_dir_sector", "wind_speed_kn", "wind_force_bft",
			"sea_state_dir_deg", "sea_state_dir_sector", "sea_state_force_douglas",
			"wave_period_s", "swell_dir_deg", "swell_dir_sector", "swell_height_m",
			"swell_period_s", "current_dir_deg", "current_dir_sector", "current_speed_kn",
			"air_temp_c", "sea_temp_c",
		}
		for _, k := range wantKeys {
			if _, ok := body.Readings[k]; !ok {
				t.Errorf("missing weather field %q", k)
			}
		}
		windDeg, _ := body.Readings["wind_dir_deg"].(float64)
		windSector, _ := body.Readings["wind_dir_sector"].(float64)
		if wantSector := float64(degreeToSector(windDeg)); windSector != wantSector {
			t.Errorf("wind_dir_sector = %v, want %v (derived from wind_dir_deg=%v)", windSector, wantSector, windDeg)
		}
		windKn, _ := body.Readings["wind_speed_kn"].(float64)
		bft, _ := body.Readings["wind_force_bft"].(float64)
		if wantBft := float64(windSpeedToBeaufort(windKn)); bft != wantBft {
			t.Errorf("wind_force_bft = %v, want %v (derived from wind_speed_kn=%v)", bft, wantBft, windKn)
		}

		// wholeNumber-typed direction fields (design doc weather table)
		// must come back integer-valued, not with round1's one-decimal
		// place — a fractional value here fails pkg/validation's
		// field.format check and blocks MarkReady/Submit.
		wholeNumberFields := []string{"wind_dir_deg", "sea_state_dir_deg", "swell_dir_deg", "current_dir_deg"}
		for _, k := range wholeNumberFields {
			v, ok := body.Readings[k].(float64)
			if !ok {
				t.Errorf("missing or non-numeric wholeNumber-typed field %q", k)
				continue
			}
			if v != math.Trunc(v) {
				t.Errorf("%s = %v, want a whole number (wholeNumber-typed field)", k, v)
			}
		}
	})

	t.Run("same request is deterministic (not re-randomized per call)", func(t *testing.T) {
		rec1 := doRequest(t, "Bearer test-key", "2026-07-01T00:00:00Z", "2026-07-02T00:00:00Z")
		rec2 := doRequest(t, "Bearer test-key", "2026-07-01T00:00:00Z", "2026-07-02T00:00:00Z")
		if rec1.Body.String() != rec2.Body.String() {
			t.Errorf("identical requests returned different bodies:\n%s\nvs\n%s", rec1.Body, rec2.Body)
		}
	})

	t.Run("draft fields are present with aft greater than fore", func(t *testing.T) {
		rec := doRequest(t, "Bearer test-key", "2026-07-01T00:00:00Z", "2026-07-02T00:00:00Z")
		var body struct {
			Readings map[string]any `json:"readings"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		wantKeys := []string{
			"draft_fore_m", "draft_aft_m", "draft_fore_recommended_m", "draft_aft_recommended_m",
			"ballast_actual_mt", "ballast_optimum_mt", "displacement_actual_mt", "water_depth_m",
		}
		for _, k := range wantKeys {
			if _, ok := body.Readings[k]; !ok {
				t.Errorf("missing draft field %q", k)
			}
		}
		fore, _ := body.Readings["draft_fore_m"].(float64)
		aft, _ := body.Readings["draft_aft_m"].(float64)
		if aft <= fore {
			t.Errorf("draft_aft_m = %v, want > draft_fore_m = %v (trim)", aft, fore)
		}
	})

	t.Run("consumption fields are present and non-negative", func(t *testing.T) {
		rec := doRequest(t, "Bearer test-key", "2026-07-01T00:00:00Z", "2026-07-02T00:00:00Z")
		var body struct {
			Readings map[string]any `json:"readings"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		for _, c := range consumptionRates {
			v, ok := body.Readings[c.key].(float64)
			if !ok {
				t.Errorf("missing consumption field %q", c.key)
				continue
			}
			if v < 0 {
				t.Errorf("%s = %v, want >= 0", c.key, v)
			}
		}
	})

	t.Run("performance fields are present with correct types and load_pct derived from load_kw", func(t *testing.T) {
		rec := doRequest(t, "Bearer test-key", "2026-07-01T00:00:00Z", "2026-07-02T00:00:00Z")
		var body struct {
			Readings map[string]any `json:"readings"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		for _, sr := range performanceSteadyRanges {
			if _, ok := body.Readings[sr.key].(float64); !ok {
				t.Errorf("missing or non-numeric steady field %q", sr.key)
			}
		}
		// wholeNumber-typed steady fields (design doc field vocabulary
		// table) must come back integer-valued, not just float64 — a
		// fractional value here would fail RuleFieldFormat once these
		// fields are wired into a schema.
		for _, sr := range performanceWholeSteadyRanges {
			v, ok := body.Readings[sr.key].(float64)
			if !ok {
				t.Errorf("missing or non-numeric whole-number steady field %q", sr.key)
				continue
			}
			if v != math.Trunc(v) {
				t.Errorf("%s = %v, want a whole number (wholeNumber-typed field)", sr.key, v)
			}
		}
		for _, cr := range performanceCumulativeRanges {
			v, ok := body.Readings[cr.key].(float64)
			if !ok {
				t.Errorf("missing or non-numeric cumulative field %q", cr.key)
				continue
			}
			if v < 0 {
				t.Errorf("%s = %v, want >= 0", cr.key, v)
			}
		}
		if _, ok := body.Readings["me1_aux_blower"].(bool); !ok {
			t.Error("me1_aux_blower is not a boolean")
		}
		if _, ok := body.Readings["boiler1_operation_mode"].(bool); !ok {
			t.Error("boiler1_operation_mode is not a boolean")
		}
		fuelType, ok := body.Readings["discharge_pump_fuel_type"].(string)
		if !ok {
			t.Fatal("discharge_pump_fuel_type is not a string")
		}
		if fuelType != "HFO" && fuelType != "MGO" && fuelType != "MDO" {
			t.Errorf("discharge_pump_fuel_type = %q, want one of HFO/MGO/MDO", fuelType)
		}

		loadKW, _ := body.Readings["me1_load_kw"].(float64)
		loadPct, _ := body.Readings["me1_load_pct"].(float64)
		wantPct := round1(loadKW / ratedMEPowerKW * 100)
		if loadPct != wantPct {
			t.Errorf("me1_load_pct = %v, want %v (derived from me1_load_kw=%v)", loadPct, wantPct, loadKW)
		}
	})
}

func TestDegreeToSector(t *testing.T) {
	tests := []struct {
		deg  float64
		want int
	}{
		{0, 1}, {45, 2}, {90, 3}, {135, 4}, {180, 5}, {225, 6}, {270, 7}, {315, 8}, {359, 1},
	}
	for _, tt := range tests {
		if got := degreeToSector(tt.deg); got != tt.want {
			t.Errorf("degreeToSector(%v) = %v, want %v", tt.deg, got, tt.want)
		}
	}
}

func TestWindSpeedToBeaufort(t *testing.T) {
	tests := []struct {
		kn   float64
		want int
	}{
		{0, 0}, {0.5, 0}, {3, 1}, {10, 3}, {25, 6}, {40, 8}, {70, 12},
	}
	for _, tt := range tests {
		if got := windSpeedToBeaufort(tt.kn); got != tt.want {
			t.Errorf("windSpeedToBeaufort(%v) = %v, want %v", tt.kn, got, tt.want)
		}
	}
}

func TestHandleVoyageData(t *testing.T) {
	s := seed{
		APIKey:              "test-key",
		VoyageNumber:        "V.TEST-1",
		VoyageFrom:          "SGSIN",
		VoyageTo:            "HKHKG",
		PreviousPort:        "SGSIN",
		NextPort:            "HKHKG",
		VoyageType:          "One way",
		CharterType:         "TC",
		CarrierCode:         "MAEU",
		CarrierName:         "Example Carrier Line",
		ServiceName:         "Asia Feeder",
		VoyageStage:         "Laden",
		VoyageLeg:           "1",
		VoyageLegType:       "Loaded",
		PortToPortID:        "SGSIN-HKHKG",
		AreaFrom:            "Singapore Strait",
		AreaTo:              "South China Sea",
		SpeedOrder:          "12.5 kn",
		ETA:                 time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC),
		RTA:                 time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC),
		CargoWeightMT:       45000,
		DeadweightCarriedMT: 47000,
		CargoVolumeM3:       52000,
		Passengers:          0,
		Crew:                21,
		ContainersFullTEU:   0,
		ContainersReeferTEU: 0,
		VehiclesCEU:         0,
	}
	handler := handleVoyageData(s)

	doRequest := func(t *testing.T, auth, at string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/voyage-data?at="+at, nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		handler(w, req)
		return w
	}

	t.Run("missing bearer key is unauthorized", func(t *testing.T) {
		rec := doRequest(t, "", "2026-07-05T12:00:00Z")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("missing at param is a bad request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/voyage-data", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("returns all 27 VMS fields from the seed", func(t *testing.T) {
		rec := doRequest(t, "Bearer test-key", "2026-07-05T12:00:00Z")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		var body struct {
			VoyageData map[string]any `json:"voyageData"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		wantKeys := []string{
			"previous_port_unlocode", "next_port_unlocode", "voyage_from_unlocode", "voyage_to_unlocode",
			"voyage_type", "voyage_number", "eta", "rta", "speed_order", "charter_type",
			"carrier_code", "carrier_name", "service_name", "voyage_stage", "voyage_leg",
			"voyage_leg_type", "port_to_port_id", "area_from", "area_to",
			"cargo_weight_mt", "deadweight_carried_mt", "cargo_volume_m3", "passengers",
			"crew", "containers_full_teu", "containers_reefer_teu", "vehicles_ceu",
		}
		if len(wantKeys) != 27 {
			t.Fatalf("test bug: wantKeys has %d entries, want 27", len(wantKeys))
		}
		for _, k := range wantKeys {
			if _, ok := body.VoyageData[k]; !ok {
				t.Errorf("missing VMS field %q", k)
			}
		}
		if got := body.VoyageData["voyage_number"]; got != "V.TEST-1" {
			t.Errorf("voyage_number = %v, want V.TEST-1", got)
		}
		if got := body.VoyageData["crew"]; got != float64(21) {
			t.Errorf("crew = %v, want 21", got)
		}
	})

	t.Run("different at values against the same seed return identical bodies (not randomized)", func(t *testing.T) {
		rec1 := doRequest(t, "Bearer test-key", "2026-07-05T12:00:00Z")
		rec2 := doRequest(t, "Bearer test-key", "2026-07-06T18:00:00Z")
		if rec1.Body.String() != rec2.Body.String() {
			t.Errorf("bodies differ for different `at` values against the same seed — VMS data must not vary with `at`:\n%s\nvs\n%s", rec1.Body, rec2.Body)
		}
	})
}
