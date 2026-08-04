// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/vessel/store"
	"github.com/captv89/ovl/vessel/vmsclient"
)

// The VMS (voyage management system) half of the sensor+VMS stub
// expansion (docs/superpowers/specs/2026-08-01-sensor-and-vms-stub-
// expansion-design.md) — mirrors sensors.go's own structure exactly
// (same Master-only config gate, same "any authenticated user can
// trigger a fetch" rule, same "unmapped schema is a real 404, not an
// error" convention) but is a wholly separate wiring: independent
// configure/enable/fail states from the sensor source, independent
// Settings section, independent report-form button (design doc's own
// "separate, not merged" decision).

type vmsSourceView struct {
	BaseURL    string `json:"baseUrl"`
	APIKey     string `json:"apiKey"`
	Enabled    bool   `json:"enabled"`
	Configured bool   `json:"configured"`
}

// handleGetVMSSource returns the vessel's configured VMS source, with
// the API key masked (maskAPIKey, shared with sensors.go). Master-only.
func (s *Server) handleGetVMSSource(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	src, err := st.GetVMSSource(r.Context())
	if err != nil {
		httpjson.WriteJSON(w, http.StatusOK, vmsSourceView{})
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, vmsSourceView{
		BaseURL: src.BaseURL, APIKey: maskAPIKey(src.APIKey), Enabled: src.Enabled, Configured: true,
	})
}

type saveVMSSourceRequest struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	Enabled bool   `json:"enabled"`
}

// handleSaveVMSSource upserts the vessel's VMS source config.
// Master-only, same gate as handleGetVMSSource.
func (s *Server) handleSaveVMSSource(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	var req saveVMSSourceRequest
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
	src := &store.VMSSource{BaseURL: req.BaseURL, APIKey: req.APIKey, Enabled: req.Enabled, UpdatedAt: time.Now().UTC()}
	if err := st.SaveVMSSource(r.Context(), src); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, vmsSourceView{BaseURL: src.BaseURL, APIKey: maskAPIKey(src.APIKey), Enabled: src.Enabled, Configured: true})
}

type testVMSSourceRequest struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
}

type testVMSSourceResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// handleTestVMSSource checks connectivity against a baseUrl/apiKey pair
// without saving it — Settings' "Test connection" action. Master-only,
// same gate as get/save. A blank apiKey falls back to the already-
// stored one, same rule as handleTestSensorSource.
func (s *Server) handleTestVMSSource(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	var req testVMSSourceRequest
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
		if src, err := st.GetVMSSource(r.Context()); err == nil {
			req.APIKey = src.APIKey
		}
	}
	if req.APIKey == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "apiKey is required")
		return
	}

	client := vmsclient.New(req.BaseURL, req.APIKey)
	voyageData, err := client.FetchVoyageData(r.Context(), time.Now().UTC())
	if err != nil {
		httpjson.WriteJSON(w, http.StatusOK, testVMSSourceResponse{OK: false, Message: err.Error()})
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, testVMSSourceResponse{
		OK:      true,
		Message: fmt.Sprintf("Connected — the VMS responded with %d field(s).", len(voyageData)),
	})
}

// vmsFieldMap is Direct-only (unlike sensorFieldMap) — VMS reference
// data has no lat/long position concept to split into a DMS triple.
type vmsFieldMap struct {
	Direct map[string]string
}

// vmsFieldMapFor returns the VMS-key -> schema-field mapping for
// schemaName. Only log-abstract has a curated one, same "local test
// config" pattern sensorFieldMapFor already uses (until Phase 3
// config-bundle authoring supplies a real, company-specific one). Every
// other schema gets a zero-value map, meaning fetch-vms-data has
// nothing to populate — a real, reachable state, not an error.
func vmsFieldMapFor(schemaName string) vmsFieldMap {
	if schemaName != "log-abstract" {
		return vmsFieldMap{}
	}
	return vmsFieldMap{
		Direct: map[string]string{
			// voyage (19)
			"previous_port_unlocode": "Previous_Port_of_Call",
			"next_port_unlocode":     "Next_Port_of_Call",
			"voyage_from_unlocode":   "Voyage_From",
			"voyage_to_unlocode":     "Voyage_To",
			"voyage_type":            "Voyage_Type",
			"voyage_number":          "Voyage_Number",
			"eta":                    "ETA",
			"rta":                    "RTA",
			"speed_order":            "Speed_Order",
			"charter_type":           "Charter_Type",
			"carrier_code":           "Carrier_Code",
			"carrier_name":           "Carrier_Name",
			"service_name":           "Service",
			"voyage_stage":           "Voyage_Stage",
			"voyage_leg":             "Voyage_Leg",
			"voyage_leg_type":        "Voyage_Leg_Type",
			"port_to_port_id":        "Port_To_Port_Id",
			"area_from":              "Area_From",
			"area_to":                "Area_To",

			// cargo (8)
			"cargo_weight_mt":       "Cargo_Mt",
			"deadweight_carried_mt": "Deadweight_Carried",
			"cargo_volume_m3":       "Cargo_M3",
			"passengers":            "Passengers",
			"crew":                  "Crew",
			"containers_full_teu":   "Cargo_Total_Full_TEU",
			"containers_reefer_teu": "Cargo_Reefer_TEU",
			"vehicles_ceu":          "Cargo_CEU",
		},
	}
}

type fetchVMSDataRequest struct {
	EventTime time.Time `json:"eventTime"`
}

type fetchVMSDataResponse struct {
	Fields map[string]any `json:"fields"`
}

// handleFetchVMSData mirrors handleFetchSensorData exactly, one level
// simpler (no window, no position split): fetches the VMS's current
// snapshot at the report's own EventTime and returns the mapped fields
// for the frontend to merge into its own local editing state — nothing
// here touches the store, and every populated field stays a normal,
// editable field the officer can still change before Save draft/Check/
// Submit.
func (s *Server) handleFetchVMSData(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	schemaName := r.PathValue("name")
	fieldMap := vmsFieldMapFor(schemaName)
	if fieldMap.Direct == nil {
		httpjson.WriteError(w, http.StatusNotFound, fmt.Sprintf("%s has no VMS field mapping", schemaName))
		return
	}

	var req fetchVMSDataRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil || req.EventTime.IsZero() {
		httpjson.WriteError(w, http.StatusBadRequest, "eventTime is required")
		return
	}

	src, err := st.GetVMSSource(r.Context())
	if err != nil || !src.Enabled {
		httpjson.WriteError(w, http.StatusConflict, "no VMS source is configured — set one up in Settings")
		return
	}

	client := vmsclient.New(src.BaseURL, src.APIKey)
	voyageData, err := client.FetchVoyageData(r.Context(), req.EventTime)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, fmt.Sprintf("fetch VMS data: %v", err))
		return
	}

	fields := make(map[string]any, len(voyageData))
	for vmsKey, schemaField := range fieldMap.Direct {
		if v, ok := voyageData[vmsKey]; ok {
			if vmsKey == "eta" || vmsKey == "rta" {
				v = normalizeVMSDateTime(v)
			}
			fields[schemaField] = v
		}
	}

	httpjson.WriteJSON(w, http.StatusOK, fetchVMSDataResponse{Fields: fields})
}

// normalizeVMSDateTime converts a VMS eta/rta value — RFC3339 on the wire,
// the correct generic contract format for a third-party VMS integrator, see
// docs/integrations/vms-reference-data-api.md — to this app's own internal
// dateTimeFieldLayout ("2006-01-02 15:04", voyage.go), the canonical format
// every DateTimeField-backed field is stored/edited in on the vessel side
// (Check/Submit's field.format rule and computeVoyageSummary's own ETA
// parse both expect that layout, not RFC3339). Non-string or unparseable
// values pass through unchanged — the officer can still see and correct a
// raw value in the field rather than lose it silently.
func normalizeVMSDateTime(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return v
	}
	return t.UTC().Format(dateTimeFieldLayout)
}
