// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDecimalDegreesToDMS(t *testing.T) {
	tests := []struct {
		name             string
		value            float64
		wantDeg, wantMin float64
		wantHemi         string
	}{
		{"positive value", 12.5, 12, 30, "N"},
		{"negative value", -45.25, 45, 15, "S"},
		{"zero", 0, 0, 0, "N"},
		{"fractional minutes", 1.10005, 1, 6.003, "N"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deg, min, hemi := decimalDegreesToDMS(tt.value, "N", "S")
			if deg != tt.wantDeg || min != tt.wantMin || hemi != tt.wantHemi {
				t.Errorf("decimalDegreesToDMS(%v) = (%v, %v, %v), want (%v, %v, %v)", tt.value, deg, min, hemi, tt.wantDeg, tt.wantMin, tt.wantHemi)
			}
		})
	}
}

func TestHandleSensorSource_GetSaveRoundtrip(t *testing.T) {
	s, c := newLoggedInTestServer(t)

	t.Run("unconfigured returns a zero view", func(t *testing.T) {
		rec := c.do(http.MethodGet, "/api/settings/sensor-source", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		got := decodeBody[sensorSourceView](t, rec)
		if got.Configured {
			t.Errorf("Configured = true before any save, want false")
		}
	})

	t.Run("non-master is forbidden", func(t *testing.T) {
		c2 := newSecondOfficerClient(t, s)
		rec := c2.do(http.MethodPut, "/api/settings/sensor-source", saveSensorSourceRequest{BaseURL: "http://example.com", APIKey: "secret-key-1234", Enabled: true})
		if rec.Code != http.StatusForbidden {
			t.Errorf("status %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	rec := c.do(http.MethodPut, "/api/settings/sensor-source", saveSensorSourceRequest{BaseURL: "http://sensors.example.com", APIKey: "secret-key-1234", Enabled: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: status %d, body %s", rec.Code, rec.Body)
	}
	saved := decodeBody[sensorSourceView](t, rec)
	if saved.APIKey == "secret-key-1234" {
		t.Error("APIKey round-tripped in full on save response, want masked")
	}
	if saved.APIKey != "••••1234" {
		t.Errorf("APIKey = %q, want masked to last 4 chars", saved.APIKey)
	}

	rec = c.do(http.MethodGet, "/api/settings/sensor-source", nil)
	got := decodeBody[sensorSourceView](t, rec)
	if got.BaseURL != "http://sensors.example.com" || !got.Enabled || !got.Configured {
		t.Errorf("GET after save = %+v, want the saved config back (key masked)", got)
	}
}

func TestHandleFetchSensorData(t *testing.T) {
	sensorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-sensor-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"readings": {
			"latitude": 12.5, "longitude": -45.25,
			"speed_gps_kn": 14.2, "me_consumption_hfo_mt": 24.1
		}}`))
	}))
	defer sensorSrv.Close()

	_, c := newLoggedInTestServer(t)

	t.Run("no source configured is a conflict", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/schemas/log-abstract/fetch-sensor-data", fetchSensorDataRequest{EventTime: time.Now().UTC()})
		if rec.Code != http.StatusConflict {
			t.Errorf("status %d, want %d", rec.Code, http.StatusConflict)
		}
	})

	if rec := c.do(http.MethodPut, "/api/settings/sensor-source", saveSensorSourceRequest{BaseURL: sensorSrv.URL, APIKey: "test-sensor-key", Enabled: true}); rec.Code != http.StatusOK {
		t.Fatalf("save sensor source: status %d, body %s", rec.Code, rec.Body)
	}

	t.Run("unsupported schema has no mapping", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/schemas/bunker-report/fetch-sensor-data", fetchSensorDataRequest{EventTime: time.Now().UTC()})
		if rec.Code != http.StatusNotFound {
			t.Errorf("status %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("fetches and maps readings, including position DMS conversion", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/schemas/log-abstract/fetch-sensor-data", fetchSensorDataRequest{EventTime: time.Now().UTC()})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		got := decodeBody[fetchSensorDataResponse](t, rec)

		if got.Fields["Speed_GPS"] != 14.2 {
			t.Errorf("Fields[Speed_GPS] = %v, want 14.2 (direct mapping)", got.Fields["Speed_GPS"])
		}
		if got.Fields["ME_Consumption_HFO"] != 24.1 {
			t.Errorf("Fields[ME_Consumption_HFO] = %v, want 24.1", got.Fields["ME_Consumption_HFO"])
		}
		if got.Fields["Latitude_Degree"] != 12.0 || got.Fields["Latitude_Minutes"] != 30.0 || got.Fields["Latitude_North_South"] != "N" {
			t.Errorf("latitude fields = deg=%v min=%v hemi=%v, want 12/30/N", got.Fields["Latitude_Degree"], got.Fields["Latitude_Minutes"], got.Fields["Latitude_North_South"])
		}
		if got.Fields["Longitude_Degree"] != 45.0 || got.Fields["Longitude_Minutes"] != 15.0 || got.Fields["Longitude_East_West"] != "W" {
			t.Errorf("longitude fields = deg=%v min=%v hemi=%v, want 45/15/W", got.Fields["Longitude_Degree"], got.Fields["Longitude_Minutes"], got.Fields["Longitude_East_West"])
		}
		// distance_nm/course_deg/etc. were never in the fake sensor's
		// response — must not appear as spurious zero-valued fields.
		if _, ok := got.Fields["Distance"]; ok {
			t.Error("Fields[Distance] present, want absent (not in the sensor's response)")
		}
	})
}

func TestSensorFieldMapFor_LogAbstract_HasAll99NewFields(t *testing.T) {
	m := sensorFieldMapFor("log-abstract")
	wantOVDFields := []string{
		// weather
		"Wind_Dir_Degree", "Wind_Dir", "Wind_Force_Kn", "Wind_Force_Bft",
		"Sea_state_Dir_Degree", "Sea_state_Dir", "Sea_state_Force_Douglas",
		"Period_Of_Wind_Waves", "Swell_Dir_Degree", "Swell_Dir", "Swell_Force",
		"Period_Of_Primary_Swell_Waves", "Current_Dir_Degree", "Current_Dir",
		"Current_Speed", "Temperature_Ambient", "Temperature_Water",
		// cargo draft
		"Draft_Actual_Fore", "Draft_Actual_Aft", "Draft_Recommended_Fore",
		"Draft_Recommended_Aft", "Draft_Ballast_Actual", "Draft_Ballast_Optimum",
		"Draft_Displacement_Actual", "Water_Depth",
		// engine.consumption
		"ME_Consumption_HFO", "ME_Consumption_LFO", "ME_Consumption_MGO", "ME_Consumption_MDO",
		"AE_Consumption_HFO", "AE_Consumption_LFO", "AE_Consumption_MGO", "AE_Consumption_MDO",
		"Boiler_Consumption_HFO", "Boiler_Consumption_LFO", "Boiler_Consumption_MGO", "Boiler_Consumption_MDO",
		"Incinerator_Consumption_O",
		"Cargo_heating_Consumption_HFO", "Cargo_heating_Consumption_LFO",
		"Cargo_heating_Consumption_MGO", "Cargo_heating_Consumption_MDO",
		// engine.performance
		"Discharge_Pump_Work", "Discharge_Pump_SFOC", "Discharge_Pump_Fuel_Type",
		"Shore_Side_Electricity_Reception", "Duration_Shore_Side_Electricity_Reception",
		"Air_Compr_1_Running_Time", "Air_Compr_2_Running_Time", "Scrubber_Running_Hours",
		"Thruster_1_Running_Time", "Thruster_2_Running_Time", "Thruster_3_Running_Time",
		"ME_Barometric_Pressure", "ME_Air_Intake_Temp", "ME_Charge_Air_Coolant_Inlet_Temp",
		"ME_1_Load", "ME_1_Load_percentage", "ME_1_Speed_RPM", "Prop_1_Pitch", "Prop_1_Pitch_Ratio",
		"ME_1_Aux_Blower", "ME_1_Shaft_Gen_Power", "ME_1_Charge_Air_Inlet_Temp",
		"ME_1_Charge_Air_Pressure", "ME_1_Pressure_Drop_Over_Charge_Air_Cooler",
		"ME_1_Pmax", "ME_1_Pcomp", "ME_1_TC_Speed", "ME_1_Exh_Temp_Before_TC", "ME_1_Exh_Temp_After_TC",
		"ME_1_Current_Consumption", "ME_1_SFOC", "ME_1_SFOC_ISO_Corrected",
		"AE_Barometric_Pressure", "AE_Air_Intake_Temp", "AE_Charge_Air_Coolant_Inlet_Temp",
		"AE_1_Load", "AE_1_Charge_Air_Inlet_Temp", "AE_1_Charge_Air_Pressure",
		"AE_1_Pressure_Drop_Over_Charge_Air_Cooler", "AE_1_TC_Speed", "AE_1_Pmax", "AE_1_Pcomp",
		"AE_1_Exh_Temp_Before_TC", "AE_1_Exh_Temp_After_TC", "AE_1_Current_Consumption",
		"AE_1_SFOC", "AE_1_SFOC_ISO_Corrected",
		"Boiler_1_Operation_Mode", "Boiler_1_Feed_Water_Flow", "Boiler_1_Steam_Pressure",
		"Cooling_Water_System_SW_Pumps_In_Service", "Cooling_Water_System_SW_Inlet_Temp",
		"Cooling_Water_System_SW_Outlet_Temp", "Cooling_Water_System_Pressure_Drop_Over_Heat_Exchanger",
		"Cooling_Water_System_Pump_Pressure", "ER_Ventilation_Fans_In_Service", "ER_Ventilation_Waste_Air_Temp",
	}
	got := make(map[string]bool, len(m.Direct))
	for _, ovd := range m.Direct {
		got[ovd] = true
	}
	for _, want := range wantOVDFields {
		if !got[want] {
			t.Errorf("sensorFieldMapFor(\"log-abstract\") is missing OVD field %q", want)
		}
	}
	if len(m.Direct) != 104 {
		t.Errorf("len(Direct) = %d, want 104 (106 sensor fields minus the 2 handled via Position)", len(m.Direct))
	}
}

func TestHandleTestSensorSource(t *testing.T) {
	sensorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-sensor-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"readings": {"latitude": 12.5}}`))
	}))
	defer sensorSrv.Close()

	s, c := newLoggedInTestServer(t)

	t.Run("non-master is forbidden", func(t *testing.T) {
		c2 := newSecondOfficerClient(t, s)
		rec := c2.do(http.MethodPost, "/api/settings/sensor-source/test", testSensorSourceRequest{BaseURL: sensorSrv.URL, APIKey: "test-sensor-key"})
		if rec.Code != http.StatusForbidden {
			t.Errorf("status %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("missing baseUrl is a bad request", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/settings/sensor-source/test", testSensorSourceRequest{APIKey: "test-sensor-key"})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("no apiKey and nothing stored is a bad request", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/settings/sensor-source/test", testSensorSourceRequest{BaseURL: sensorSrv.URL})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("trailing whitespace on baseUrl does not break parsing", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/settings/sensor-source/test", testSensorSourceRequest{BaseURL: sensorSrv.URL + " ", APIKey: "test-sensor-key"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		got := decodeBody[testSensorSourceResponse](t, rec)
		if !got.OK {
			t.Errorf("OK = false, message %q, want true", got.Message)
		}
	})

	t.Run("wrong key reports failure, not an error status", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/settings/sensor-source/test", testSensorSourceRequest{BaseURL: sensorSrv.URL, APIKey: "wrong-key"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		got := decodeBody[testSensorSourceResponse](t, rec)
		if got.OK {
			t.Error("OK = true with a wrong API key, want false")
		}
		if got.Message == "" {
			t.Error("Message is empty on failure, want a reason")
		}
	})

	t.Run("success", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/settings/sensor-source/test", testSensorSourceRequest{BaseURL: sensorSrv.URL, APIKey: "test-sensor-key"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		got := decodeBody[testSensorSourceResponse](t, rec)
		if !got.OK {
			t.Errorf("OK = false, message %q, want true", got.Message)
		}
	})

	t.Run("blank apiKey falls back to the stored key", func(t *testing.T) {
		if rec := c.do(http.MethodPut, "/api/settings/sensor-source", saveSensorSourceRequest{BaseURL: sensorSrv.URL, APIKey: "test-sensor-key", Enabled: true}); rec.Code != http.StatusOK {
			t.Fatalf("save sensor source: status %d, body %s", rec.Code, rec.Body)
		}
		rec := c.do(http.MethodPost, "/api/settings/sensor-source/test", testSensorSourceRequest{BaseURL: sensorSrv.URL})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		got := decodeBody[testSensorSourceResponse](t, rec)
		if !got.OK {
			t.Errorf("OK = false, message %q, want true (should have used the stored apiKey)", got.Message)
		}
	})
}
