// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/vessel/sensorclient"
	"github.com/captv89/ovl/vessel/store"
)

// 18.07.26 manual-test items 4/9: the vessel-side half of "vessel pulls
// from an onboard sensor REST service" (the decided architecture — see
// vessel/sensorclient's own doc comment on why pull, not an ingestion
// endpoint). This file wires the Master-configured sensor source
// (vessel/store.SensorSource) and the log-abstract field mapping that
// turns a sensor service's own field vocabulary into curated OVD field
// names — see sensorFieldMapFor's own doc comment on why only
// log-abstract has one.

type sensorSourceView struct {
	BaseURL string `json:"baseUrl"`
	// APIKey is masked on GET (last 4 characters only) — this is a
	// long-lived credential the vessel holds, not a one-time reveal
	// token like an office-issued API key, but it still shouldn't round-
	// trip back to the browser in full on every settings page load.
	// Changing it means entering a full new value on PUT.
	APIKey     string `json:"apiKey"`
	Enabled    bool   `json:"enabled"`
	Configured bool   `json:"configured"`
}

func maskAPIKey(key string) string {
	if len(key) <= 4 {
		return "••••"
	}
	return "••••" + key[len(key)-4:]
}

// handleGetSensorSource returns the vessel's configured sensor source,
// with the API key masked. Master-only (Settings, same gate as backup/
// restore) — this is vessel-wide config, not a per-user preference.
func (s *Server) handleGetSensorSource(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	src, err := st.GetSensorSource(r.Context())
	if err != nil {
		httpjson.WriteJSON(w, http.StatusOK, sensorSourceView{})
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, sensorSourceView{
		BaseURL: src.BaseURL, APIKey: maskAPIKey(src.APIKey), Enabled: src.Enabled, Configured: true,
	})
}

type saveSensorSourceRequest struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	Enabled bool   `json:"enabled"`
}

// handleSaveSensorSource upserts the vessel's sensor source config.
// Master-only, same gate as handleGetSensorSource.
func (s *Server) handleSaveSensorSource(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	var req saveSensorSourceRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.BaseURL == "" || req.APIKey == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "baseUrl and apiKey are required")
		return
	}
	src := &store.SensorSource{BaseURL: req.BaseURL, APIKey: req.APIKey, Enabled: req.Enabled, UpdatedAt: time.Now().UTC()}
	if err := st.SaveSensorSource(r.Context(), src); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, sensorSourceView{BaseURL: src.BaseURL, APIKey: maskAPIKey(src.APIKey), Enabled: src.Enabled, Configured: true})
}

type testSensorSourceRequest struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
}

type testSensorSourceResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// testSensorSourceWindow is a short, fixed probe window — a "Test
// connection" click cares about reachability/auth, not any particular
// reading, so there's no reason to anchor it to a report's EventTime the
// way handleFetchSensorData's real fetchWindowStart does.
const testSensorSourceWindow = 5 * time.Minute

// handleTestSensorSource checks connectivity against a baseUrl/apiKey
// pair without saving it — Settings' "Test connection" action, so a
// Master can catch a typo'd URL or wrong key before committing it (and
// before it makes a Fetch sensor data click on a real report fail).
// Master-only, same gate as get/save. A blank apiKey falls back to the
// already-stored one, matching the masked-apiKey display's implication
// elsewhere on this screen that leaving it blank means "the current
// one" — the UI has no way to show the real key back to retype.
func (s *Server) handleTestSensorSource(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	var req testSensorSourceRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.BaseURL == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "baseUrl is required")
		return
	}
	if req.APIKey == "" {
		if src, err := st.GetSensorSource(r.Context()); err == nil {
			req.APIKey = src.APIKey
		}
	}
	if req.APIKey == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "apiKey is required")
		return
	}

	client := sensorclient.New(req.BaseURL, req.APIKey)
	now := time.Now().UTC()
	readings, err := client.FetchReadings(r.Context(), now.Add(-testSensorSourceWindow), now)
	if err != nil {
		httpjson.WriteJSON(w, http.StatusOK, testSensorSourceResponse{OK: false, Message: err.Error()})
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, testSensorSourceResponse{
		OK:      true,
		Message: fmt.Sprintf("Connected — the sensor service responded with %d reading(s).", len(readings)),
	})
}

// sensorFieldMap describes how one schema's fields are populated from a
// sensor service's own reading vocabulary. Direct entries are a plain
// 1:1 substitution (same value, same unit). Position is handled
// separately (positionFields, non-nil only for schemas with a lat/long
// triple) since a sensor naturally reports signed decimal degrees, not
// the curated schema's degree/minutes/hemisphere split — see
// decimalDegreesToDMS.
type sensorFieldMap struct {
	Direct   map[string]string
	Position *positionFieldNames
}

type positionFieldNames struct {
	LatDegree, LatMinutes, LatHemisphere string
	LonDegree, LonMinutes, LonHemisphere string
}

// sensorFieldMapFor returns the sensor-key -> schema-field mapping for
// schemaName. Only log-abstract has a curated one — same "local test
// config" stand-in pattern fieldConfigFor/validationConfigFor already
// use for this one schema, until Phase 3 config-bundle authoring exists
// to supply a real, company-specific one (a company's actual onboard
// sensor system will use its own field vocabulary, not this stub's).
// Every other schema gets a zero-value map (Direct nil, Position nil),
// meaning fetch-sensor-data has nothing to populate — a real, reachable
// state (Bunker/EDN reports have no cadence/sensor-feed concept), not an
// error.
func sensorFieldMapFor(schemaName string) sensorFieldMap {
	if schemaName != "log-abstract" {
		return sensorFieldMap{}
	}
	return sensorFieldMap{
		Direct: map[string]string{
			// distanceAndSpeed (unchanged from the original 7-field stub)
			"speed_gps_kn":           "Speed_GPS",
			"speed_through_water_kn": "Speed_Through_Water",
			"course_deg":             "Course",
			"true_heading_deg":       "True_Heading",
			"distance_nm":            "Distance",

			// weather (17)
			"wind_dir_deg":            "Wind_Dir_Degree",
			"wind_dir_sector":         "Wind_Dir",
			"wind_speed_kn":           "Wind_Force_Kn",
			"wind_force_bft":          "Wind_Force_Bft",
			"sea_state_dir_deg":       "Sea_state_Dir_Degree",
			"sea_state_dir_sector":    "Sea_state_Dir",
			"sea_state_force_douglas": "Sea_state_Force_Douglas",
			"wave_period_s":           "Period_Of_Wind_Waves",
			"swell_dir_deg":           "Swell_Dir_Degree",
			"swell_dir_sector":        "Swell_Dir",
			"swell_height_m":          "Swell_Force",
			"swell_period_s":          "Period_Of_Primary_Swell_Waves",
			"current_dir_deg":         "Current_Dir_Degree",
			"current_dir_sector":      "Current_Dir",
			"current_speed_kn":        "Current_Speed",
			"air_temp_c":              "Temperature_Ambient",
			"sea_temp_c":              "Temperature_Water",

			// cargo draft/water depth (8)
			"draft_fore_m":             "Draft_Actual_Fore",
			"draft_aft_m":              "Draft_Actual_Aft",
			"draft_fore_recommended_m": "Draft_Recommended_Fore",
			"draft_aft_recommended_m":  "Draft_Recommended_Aft",
			"ballast_actual_mt":        "Draft_Ballast_Actual",
			"ballast_optimum_mt":       "Draft_Ballast_Optimum",
			"displacement_actual_mt":   "Draft_Displacement_Actual",
			"water_depth_m":            "Water_Depth",

			// engine.consumption (17) — me_consumption_hfo_mt supersedes
			// the original hfo_consumption_mt key (cmd/ovl-sensor-stub
			// renamed it in the same pass).
			"me_consumption_hfo_mt":            "ME_Consumption_HFO",
			"me_consumption_lfo_mt":            "ME_Consumption_LFO",
			"me_consumption_mgo_mt":            "ME_Consumption_MGO",
			"me_consumption_mdo_mt":            "ME_Consumption_MDO",
			"ae_consumption_hfo_mt":            "AE_Consumption_HFO",
			"ae_consumption_lfo_mt":            "AE_Consumption_LFO",
			"ae_consumption_mgo_mt":            "AE_Consumption_MGO",
			"ae_consumption_mdo_mt":            "AE_Consumption_MDO",
			"boiler_consumption_hfo_mt":        "Boiler_Consumption_HFO",
			"boiler_consumption_lfo_mt":        "Boiler_Consumption_LFO",
			"boiler_consumption_mgo_mt":        "Boiler_Consumption_MGO",
			"boiler_consumption_mdo_mt":        "Boiler_Consumption_MDO",
			"incinerator_consumption_other_mt": "Incinerator_Consumption_O",
			"cargo_heating_consumption_hfo_mt": "Cargo_heating_Consumption_HFO",
			"cargo_heating_consumption_lfo_mt": "Cargo_heating_Consumption_LFO",
			"cargo_heating_consumption_mgo_mt": "Cargo_heating_Consumption_MGO",
			"cargo_heating_consumption_mdo_mt": "Cargo_heating_Consumption_MDO",

			// engine.performance (57)
			"discharge_pump_work_kwh":                 "Discharge_Pump_Work",
			"discharge_pump_sfoc_g_per_kwh":           "Discharge_Pump_SFOC",
			"discharge_pump_fuel_type":                "Discharge_Pump_Fuel_Type",
			"shore_power_kwh":                         "Shore_Side_Electricity_Reception",
			"shore_power_duration_h":                  "Duration_Shore_Side_Electricity_Reception",
			"air_compressor_1_running_h":              "Air_Compr_1_Running_Time",
			"air_compressor_2_running_h":              "Air_Compr_2_Running_Time",
			"scrubber_running_h":                      "Scrubber_Running_Hours",
			"bow_thruster_running_h":                  "Thruster_1_Running_Time",
			"stern_thruster_running_h":                "Thruster_2_Running_Time",
			"thruster_3_running_h":                    "Thruster_3_Running_Time",
			"me_barometric_pressure_bar":              "ME_Barometric_Pressure",
			"me_air_intake_temp_c":                    "ME_Air_Intake_Temp",
			"me_charge_air_coolant_inlet_temp_c":      "ME_Charge_Air_Coolant_Inlet_Temp",
			"me1_load_kw":                             "ME_1_Load",
			"me1_load_pct":                            "ME_1_Load_percentage",
			"me1_speed_rpm":                           "ME_1_Speed_RPM",
			"prop_1_pitch_m":                          "Prop_1_Pitch",
			"prop_1_pitch_ratio":                      "Prop_1_Pitch_Ratio",
			"me1_aux_blower":                          "ME_1_Aux_Blower",
			"me1_shaft_gen_power_kw":                  "ME_1_Shaft_Gen_Power",
			"me1_charge_air_inlet_temp_c":             "ME_1_Charge_Air_Inlet_Temp",
			"me1_charge_air_pressure_bar":             "ME_1_Charge_Air_Pressure",
			"me1_charge_air_cooler_pressure_drop_bar": "ME_1_Pressure_Drop_Over_Charge_Air_Cooler",
			"me1_pmax_bar":                            "ME_1_Pmax",
			"me1_pcomp_bar":                           "ME_1_Pcomp",
			"me1_tc_speed_rpm":                        "ME_1_TC_Speed",
			"me1_exh_temp_before_tc_c":                "ME_1_Exh_Temp_Before_TC",
			"me1_exh_temp_after_tc_c":                 "ME_1_Exh_Temp_After_TC",
			"me1_fuel_meter_mt_per_h":                 "ME_1_Current_Consumption",
			"me1_sfoc_g_per_kwh":                      "ME_1_SFOC",
			"me1_sfoc_iso_g_per_kwh":                  "ME_1_SFOC_ISO_Corrected",
			"ae_barometric_pressure_bar":              "AE_Barometric_Pressure",
			"ae_air_intake_temp_c":                    "AE_Air_Intake_Temp",
			"ae_charge_air_coolant_inlet_temp_c":      "AE_Charge_Air_Coolant_Inlet_Temp",
			"ae1_load_kw":                             "AE_1_Load",
			"ae1_charge_air_inlet_temp_c":             "AE_1_Charge_Air_Inlet_Temp",
			"ae1_charge_air_pressure_bar":             "AE_1_Charge_Air_Pressure",
			"ae1_charge_air_cooler_pressure_drop_bar": "AE_1_Pressure_Drop_Over_Charge_Air_Cooler",
			"ae1_tc_speed_rpm":                        "AE_1_TC_Speed",
			"ae1_pmax_bar":                            "AE_1_Pmax",
			"ae1_pcomp_bar":                           "AE_1_Pcomp",
			"ae1_exh_temp_before_tc_c":                "AE_1_Exh_Temp_Before_TC",
			"ae1_exh_temp_after_tc_c":                 "AE_1_Exh_Temp_After_TC",
			"ae1_fuel_meter_mt_per_h":                 "AE_1_Current_Consumption",
			"ae1_sfoc_g_per_kwh":                      "AE_1_SFOC",
			"ae1_sfoc_iso_g_per_kwh":                  "AE_1_SFOC_ISO_Corrected",
			// Boiler_1_Operation_Mode is hidden by fieldConfigFor
			// (schemas.go) on this vessel's local demo field policy —
			// stubbed/mapped anyway because the design doc's "a field
			// hidden fleet-wide is not stubbed" scope rule is checked
			// against the office Postgres policy, not this separate
			// vessel-local demo one. A real, known, low-harm gap
			// between the two policy sources (hidden fields are
			// skipped by validation regardless), not an oversight.
			"boiler1_operation_mode":                   "Boiler_1_Operation_Mode",
			"boiler1_feed_water_flow_m3_min":           "Boiler_1_Feed_Water_Flow",
			"boiler1_steam_pressure_bar":               "Boiler_1_Steam_Pressure",
			"cooling_sw_pumps_in_service":              "Cooling_Water_System_SW_Pumps_In_Service",
			"cooling_sw_inlet_temp_c":                  "Cooling_Water_System_SW_Inlet_Temp",
			"cooling_sw_outlet_temp_c":                 "Cooling_Water_System_SW_Outlet_Temp",
			"cooling_heat_exchanger_pressure_drop_bar": "Cooling_Water_System_Pressure_Drop_Over_Heat_Exchanger",
			"cooling_pump_pressure_bar":                "Cooling_Water_System_Pump_Pressure",
			"er_ventilation_fans_in_service":           "ER_Ventilation_Fans_In_Service",
			"er_ventilation_waste_air_temp_c":          "ER_Ventilation_Waste_Air_Temp",
		},
		Position: &positionFieldNames{
			LatDegree: "Latitude_Degree", LatMinutes: "Latitude_Minutes", LatHemisphere: "Latitude_North_South",
			LonDegree: "Longitude_Degree", LonMinutes: "Longitude_Minutes", LonHemisphere: "Longitude_East_West",
		},
	}
}

// decimalDegreesToDMS splits a signed decimal-degree value (positive =
// posLabel, e.g. "N"/"E"; negative = negLabel, e.g. "S"/"W") into the
// curated schema's own degree/minutes/hemisphere triple.
func decimalDegreesToDMS(value float64, posLabel, negLabel string) (degree float64, minutes float64, hemisphere string) {
	hemisphere = posLabel
	if value < 0 {
		hemisphere = negLabel
		value = -value
	}
	degree = math.Trunc(value)
	minutes = (value - degree) * 60
	return degree, math.Round(minutes*1000) / 1000, hemisphere
}

type fetchSensorDataRequest struct {
	EventTime time.Time `json:"eventTime"`
}

type fetchSensorDataResponse struct {
	Fields map[string]any `json:"fields"`
}

// handleFetchSensorData is design decision 4's officer-facing action:
// "once the report is open, the user makes the entry of date and time of
// the event and ... a button ... to fetch sensor data ... auto populate
// relevant fields." Any authenticated user may trigger it (same access
// as opening the report form itself) — this is a read-only query against
// an external service, not a mutation to the report. Returns the mapped
// fields for the frontend to merge into its own local editing state
// (ensureReportCreated's own pattern: nothing here touches the store),
// so the officer sees them land and can still edit or reject any of
// them before Save draft/Check/Submit — same "starting point, not a
// lock" rule every other prefill class already follows. Downstream
// computed fields (HFO_ROB, Time_Since_Previous_Report, Wind_Dir, ...)
// then update automatically via ReportForm.tsx's existing live-derive
// effect once these values land in `values` — no separate wiring needed
// for that half of the request.
func (s *Server) handleFetchSensorData(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	schemaName := r.PathValue("name")
	fieldMap := sensorFieldMapFor(schemaName)
	if fieldMap.Direct == nil && fieldMap.Position == nil {
		httpjson.WriteError(w, http.StatusNotFound, fmt.Sprintf("%s has no sensor field mapping", schemaName))
		return
	}

	var req fetchSensorDataRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil || req.EventTime.IsZero() {
		httpjson.WriteError(w, http.StatusBadRequest, "eventTime is required")
		return
	}

	src, err := st.GetSensorSource(r.Context())
	if err != nil || !src.Enabled {
		httpjson.WriteError(w, http.StatusConflict, "no sensor source is configured — set one up in Settings")
		return
	}

	from := fetchWindowStart(r.Context(), st, schemaName, req.EventTime)
	client := sensorclient.New(src.BaseURL, src.APIKey)
	readings, err := client.FetchReadings(r.Context(), from, req.EventTime)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, fmt.Sprintf("fetch sensor data: %v", err))
		return
	}

	fields := make(map[string]any, len(readings))
	for sensorKey, schemaField := range fieldMap.Direct {
		if v, ok := readings[sensorKey]; ok {
			fields[schemaField] = v
		}
	}
	if fieldMap.Position != nil {
		if lat, ok := readings["latitude"].(float64); ok {
			deg, min, hemi := decimalDegreesToDMS(lat, "N", "S")
			fields[fieldMap.Position.LatDegree] = deg
			fields[fieldMap.Position.LatMinutes] = min
			fields[fieldMap.Position.LatHemisphere] = hemi
		}
		if lon, ok := readings["longitude"].(float64); ok {
			deg, min, hemi := decimalDegreesToDMS(lon, "E", "W")
			fields[fieldMap.Position.LonDegree] = deg
			fields[fieldMap.Position.LonMinutes] = min
			fields[fieldMap.Position.LonHemisphere] = hemi
		}
	}

	httpjson.WriteJSON(w, http.StatusOK, fetchSensorDataResponse{Fields: fields})
}

// fetchWindowStart is the query window's lower bound: the vessel's last
// submitted report's own EventTime for this schema, matching how every
// other cross-report computation in this codebase anchors to (see
// handleGetSchema's own lastSubmittedReport call). Falls back to 24
// hours before the requested event time when there's no prior submitted
// report to anchor to (a real, reachable state — a vessel's very first
// report) — matches homeData.ts's own MAX_GAP_HOURS default cadence
// rather than inventing a different number.
func fetchWindowStart(ctx context.Context, st *store.Store, schemaName string, eventTime time.Time) time.Time {
	if last := lastSubmittedReport(ctx, st, schemaName); last != nil {
		return last.EventTime
	}
	return eventTime.Add(-24 * time.Hour)
}
