// SPDX-License-Identifier: AGPL-3.0-only

// Command ovl-sensor-stub is a stand-in for two independent, vessel-
// initiated-pull REST contracts: a real onboard sensor/IAS data
// service (GET /readings?from=&to=, see vessel/sensorclient's own doc
// comment for the full contract) and a real VMS (voyage management
// system) reference-data service (GET /voyage-data?at=, see
// vessel/vmsclient's own doc comment). Both are documented
// independently for third-party integrators in docs/integrations/ —
// nothing about sharing this one process is part of either contract.
//
// Sensor readings for a queried [from, to]: position/speed/distance/
// course are computed by linear-interpolating between seed positions
// (deterministic, not randomized); weather/draft/consumption/
// performance fields are randomized within plausible, internally-
// consistent bounds per query (same treatment as the original wind/
// fuel randomization). VMS voyage/cargo data is not randomized at
// all — a real VMS returns a stable voyage plan on repeated polls —
// and comes entirely from CLI seed flags, fixed for the process's
// lifetime.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"os"
	"time"
)

type seed struct {
	StartLat, StartLon float64
	EndLat, EndLon     float64
	VoyageStart        time.Time
	VoyageEnd          time.Time
	APIKey             string

	// VMS (voyage management system) reference data — deterministic,
	// not derived from the position/time seed above. See this file's
	// own doc comment update below and docs/integrations/
	// vms-reference-data-api.md for the full contract.
	VoyageNumber        string
	VoyageFrom          string
	VoyageTo            string
	PreviousPort        string
	NextPort            string
	VoyageType          string
	CharterType         string
	CarrierCode         string
	CarrierName         string
	ServiceName         string
	VoyageStage         string
	VoyageLeg           string
	VoyageLegType       string
	PortToPortID        string
	AreaFrom            string
	AreaTo              string
	SpeedOrder          string
	ETA                 time.Time
	RTA                 time.Time
	CargoWeightMT       float64
	DeadweightCarriedMT float64
	CargoVolumeM3       float64
	Passengers          int
	Crew                int
	ContainersFullTEU   int
	ContainersReeferTEU int
	VehiclesCEU         float64
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ovl-sensor-stub:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("ovl-sensor-stub", flag.ContinueOnError)
	port := fs.Int("port", 8422, "port to listen on")
	apiKey := fs.String("api-key", "stub-sensor-key", "bearer API key callers must present")
	startLat := fs.Float64("start-lat", 1.29, "seed start latitude, decimal degrees (positive = N)")
	startLon := fs.Float64("start-lon", 103.85, "seed start longitude, decimal degrees (positive = E)")
	endLat := fs.Float64("end-lat", 22.31, "seed end latitude, decimal degrees")
	endLon := fs.Float64("end-lon", 114.17, "seed end longitude, decimal degrees")
	voyageStartStr := fs.String("voyage-start", "", "voyage start, RFC3339 (default: 7 days ago)")
	voyageEndStr := fs.String("voyage-end", "", "voyage end, RFC3339 (default: 7 days from now)")
	voyageNumber := fs.String("voyage-number", "V.2026-014", "VMS: voyage number")
	voyageFrom := fs.String("voyage-from-port", "SGSIN", "VMS: voyage-from UN/LOCODE")
	voyageTo := fs.String("voyage-to-port", "HKHKG", "VMS: voyage-to UN/LOCODE")
	previousPort := fs.String("previous-port", "SGSIN", "VMS: previous port UN/LOCODE")
	nextPort := fs.String("next-port", "HKHKG", "VMS: next port UN/LOCODE")
	voyageType := fs.String("voyage-type", "One way", "VMS: voyage type (One way/Round trip/STS/Idle)")
	charterType := fs.String("charter-type", "TC", "VMS: charter type code (TC/VC/CVC/COA)")
	carrierCode := fs.String("carrier-code", "MAEU", "VMS: carrier code")
	carrierName := fs.String("carrier-name", "Example Carrier Line", "VMS: carrier name")
	serviceName := fs.String("service-name", "Asia Feeder", "VMS: service name")
	voyageStage := fs.String("voyage-stage", "Laden", "VMS: voyage stage")
	voyageLeg := fs.String("voyage-leg", "1", "VMS: voyage leg")
	voyageLegType := fs.String("voyage-leg-type", "Loaded", "VMS: voyage leg type")
	portToPortID := fs.String("port-to-port-id", "SGSIN-HKHKG", "VMS: port-to-port id")
	areaFrom := fs.String("area-from", "Singapore Strait", "VMS: area from")
	areaTo := fs.String("area-to", "South China Sea", "VMS: area to")
	speedOrder := fs.String("speed-order", "12.5 kn", "VMS: speed order")
	etaStr := fs.String("eta", "", "VMS: ETA, RFC3339 (default: voyage end)")
	rtaStr := fs.String("rta", "", "VMS: RTA, RFC3339 (default: same as ETA)")
	cargoWeightMT := fs.Float64("cargo-weight-mt", 45000, "VMS: cargo weight, mt")
	deadweightCarriedMT := fs.Float64("deadweight-carried-mt", 47000, "VMS: deadweight carried, mt")
	cargoVolumeM3 := fs.Float64("cargo-volume-m3", 52000, "VMS: cargo volume, m3")
	passengers := fs.Int("passengers", 0, "VMS: passengers")
	crew := fs.Int("crew", 21, "VMS: crew")
	containersFullTEU := fs.Int("containers-full-teu", 0, "VMS: full container TEU")
	containersReeferTEU := fs.Int("containers-reefer-teu", 0, "VMS: reefer container TEU")
	vehiclesCEU := fs.Float64("vehicles-ceu", 0, "VMS: vehicles, CEU")
	if err := fs.Parse(args); err != nil {
		return err
	}

	now := time.Now().UTC()
	voyageStart := now.Add(-7 * 24 * time.Hour)
	if *voyageStartStr != "" {
		t, err := time.Parse(time.RFC3339, *voyageStartStr)
		if err != nil {
			return fmt.Errorf("parse -voyage-start: %w", err)
		}
		voyageStart = t
	}
	voyageEnd := now.Add(7 * 24 * time.Hour)
	if *voyageEndStr != "" {
		t, err := time.Parse(time.RFC3339, *voyageEndStr)
		if err != nil {
			return fmt.Errorf("parse -voyage-end: %w", err)
		}
		voyageEnd = t
	}

	eta := voyageEnd
	if *etaStr != "" {
		t, err := time.Parse(time.RFC3339, *etaStr)
		if err != nil {
			return fmt.Errorf("parse -eta: %w", err)
		}
		eta = t
	}
	rta := eta
	if *rtaStr != "" {
		t, err := time.Parse(time.RFC3339, *rtaStr)
		if err != nil {
			return fmt.Errorf("parse -rta: %w", err)
		}
		rta = t
	}

	s := seed{
		StartLat: *startLat, StartLon: *startLon,
		EndLat: *endLat, EndLon: *endLon,
		VoyageStart: voyageStart, VoyageEnd: voyageEnd,
		APIKey: *apiKey,

		VoyageNumber: *voyageNumber, VoyageFrom: *voyageFrom, VoyageTo: *voyageTo,
		PreviousPort: *previousPort, NextPort: *nextPort,
		VoyageType: *voyageType, CharterType: *charterType,
		CarrierCode: *carrierCode, CarrierName: *carrierName, ServiceName: *serviceName,
		VoyageStage: *voyageStage, VoyageLeg: *voyageLeg, VoyageLegType: *voyageLegType,
		PortToPortID: *portToPortID, AreaFrom: *areaFrom, AreaTo: *areaTo,
		SpeedOrder: *speedOrder, ETA: eta, RTA: rta,
		CargoWeightMT: *cargoWeightMT, DeadweightCarriedMT: *deadweightCarriedMT, CargoVolumeM3: *cargoVolumeM3,
		Passengers: *passengers, Crew: *crew,
		ContainersFullTEU: *containersFullTEU, ContainersReeferTEU: *containersReeferTEU,
		VehiclesCEU: *vehiclesCEU,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /readings", handleReadings(s))
	mux.HandleFunc("GET /voyage-data", handleVoyageData(s))

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	slog.Info("ovl-sensor-stub listening", "addr", addr, "voyageStart", voyageStart, "voyageEnd", voyageEnd)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	return srv.ListenAndServe()
}

func handleReadings(s seed) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.APIKey {
			http.Error(w, "invalid or missing bearer API key", http.StatusUnauthorized)
			return
		}
		from, err := parseTimeParam(r, "from")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		to, err := parseTimeParam(r, "to")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		fromLat, fromLon := s.positionAt(from)
		toLat, toLon := s.positionAt(to)
		distance := haversineNM(fromLat, fromLon, toLat, toLon)
		elapsedHours := to.Sub(from).Hours()
		speed := 0.0
		if elapsedHours > 0 {
			speed = distance / elapsedHours
		}
		course := initialBearingDeg(fromLat, fromLon, toLat, toLon)

		//nolint:gosec // math/rand is fine here — this is a demo/test data
		// stub, not a security-sensitive value.
		rnd := rand.New(rand.NewSource(to.UnixNano())) // #nosec G404 -- same reasoning, gosec's own directive syntax (nolint above is golangci-lint's)
		readings := map[string]any{
			"latitude":               round3(toLat),
			"longitude":              round3(toLon),
			"speed_gps_kn":           round1(speed),
			"speed_through_water_kn": round1(speed * (0.97 + rnd.Float64()*0.03)), // a little current/slip, always <= GPS speed
			"course_deg":             round1(course),
			"true_heading_deg":       round1(math.Mod(course+rnd.Float64()*4-2+360, 360)), // small heading/course offset (leeway/set)
			"distance_nm":            round1(distance),
		}
		addWeatherReadings(readings, rnd)
		addDraftReadings(readings, rnd)
		addConsumptionReadings(readings, rnd, elapsedHours)
		addPerformanceReadings(readings, rnd, elapsedHours)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"readings": readings})
	}
}

// handleVoyageData serves the VMS (voyage management system) reference-
// data contract: GET /voyage-data?at=<RFC3339>, bearer auth,
// {"voyageData": {...}}. Unlike /readings, this is a snapshot at one
// instant, not a [from, to] window — voyage-plan and cargo-manifest
// data isn't a rate — and the response is entirely deterministic
// (seeded once at process start via CLI flags), matching a real VMS
// returning the same voyage plan on repeated polls (design doc,
// "Stub generation logic").
func handleVoyageData(s seed) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.APIKey {
			http.Error(w, "invalid or missing bearer API key", http.StatusUnauthorized)
			return
		}
		if _, err := parseTimeParam(r, "at"); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		voyageData := map[string]any{
			"previous_port_unlocode": s.PreviousPort,
			"next_port_unlocode":     s.NextPort,
			"voyage_from_unlocode":   s.VoyageFrom,
			"voyage_to_unlocode":     s.VoyageTo,
			"voyage_type":            s.VoyageType,
			"voyage_number":          s.VoyageNumber,
			"eta":                    s.ETA.UTC().Format(time.RFC3339),
			"rta":                    s.RTA.UTC().Format(time.RFC3339),
			"speed_order":            s.SpeedOrder,
			"charter_type":           s.CharterType,
			"carrier_code":           s.CarrierCode,
			"carrier_name":           s.CarrierName,
			"service_name":           s.ServiceName,
			"voyage_stage":           s.VoyageStage,
			"voyage_leg":             s.VoyageLeg,
			"voyage_leg_type":        s.VoyageLegType,
			"port_to_port_id":        s.PortToPortID,
			"area_from":              s.AreaFrom,
			"area_to":                s.AreaTo,

			"cargo_weight_mt":       s.CargoWeightMT,
			"deadweight_carried_mt": s.DeadweightCarriedMT,
			"cargo_volume_m3":       s.CargoVolumeM3,
			"passengers":            s.Passengers,
			"crew":                  s.Crew,
			"containers_full_teu":   s.ContainersFullTEU,
			"containers_reefer_teu": s.ContainersReeferTEU,
			"vehicles_ceu":          s.VehiclesCEU,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"voyageData": voyageData})
	}
}

func parseTimeParam(r *http.Request, name string) (time.Time, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return time.Time{}, fmt.Errorf("missing required query parameter %q", name)
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %q: %w", name, err)
	}
	return t, nil
}

// positionAt linearly interpolates between the seed's start/end
// position by t's fraction of the voyage window, clamped to [0,1] —
// before voyage start the vessel is "still at" the start position,
// after voyage end it's "arrived and staying at" the end position.
// Straight-line interpolation, not great-circle — a deliberate
// simplification for a stub (see this file's own doc comment); good
// enough to produce a monotonically progressing, internally consistent
// track, not meant to be navigationally accurate over long distances.
func (s seed) positionAt(t time.Time) (lat, lon float64) {
	total := s.VoyageEnd.Sub(s.VoyageStart)
	if total <= 0 {
		return s.EndLat, s.EndLon
	}
	frac := t.Sub(s.VoyageStart).Seconds() / total.Seconds()
	frac = math.Max(0, math.Min(1, frac))
	return s.StartLat + (s.EndLat-s.StartLat)*frac, s.StartLon + (s.EndLon-s.StartLon)*frac
}

const earthRadiusNM = 3440.065

func toRad(deg float64) float64 { return deg * math.Pi / 180 }
func toDeg(rad float64) float64 { return rad * 180 / math.Pi }

// haversineNM is the great-circle distance between two points, in
// nautical miles — used for the *distance covered* readout even though
// positionAt itself interpolates linearly; over the short windows a
// report-to-report gap actually spans, the two agree closely enough,
// and haversine is the standard "distance between two lat/lons" formula
// regardless of how those two points were derived.
func haversineNM(lat1, lon1, lat2, lon2 float64) float64 {
	φ1, φ2 := toRad(lat1), toRad(lat2)
	Δφ := toRad(lat2 - lat1)
	Δλ := toRad(lon2 - lon1)
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusNM * c
}

// initialBearingDeg is the great-circle initial bearing from (lat1,lon1)
// to (lat2,lon2), in degrees [0, 360). Returns 0 for two identical
// points (no direction of travel to report).
func initialBearingDeg(lat1, lon1, lat2, lon2 float64) float64 {
	if lat1 == lat2 && lon1 == lon2 {
		return 0
	}
	φ1, φ2 := toRad(lat1), toRad(lat2)
	Δλ := toRad(lon2 - lon1)
	y := math.Sin(Δλ) * math.Cos(φ2)
	x := math.Cos(φ1)*math.Sin(φ2) - math.Sin(φ1)*math.Cos(φ2)*math.Cos(Δλ)
	θ := math.Atan2(y, x)
	return math.Mod(toDeg(θ)+360, 360)
}

// randRange returns a uniform random float64 in [lo, hi) — the shared
// building block every plausible-but-unrelated-to-the-seed field below
// uses, so each field's bounds are visible at its own call site instead
// of buried in one shared magic-number table.
func randRange(rnd *rand.Rand, lo, hi float64) float64 {
	return lo + rnd.Float64()*(hi-lo)
}

// degreeToSector maps a compass degree [0,360) onto the curated OVD
// schema's shared 8-point sector vocabulary (1=N, 2=NE, 3=E, 4=SE, 5=S,
// 6=SW, 7=W, 8=NW) used by Wind_Dir/Sea_state_Dir/Swell_Dir/Current_Dir
// alike — kept derived from the paired *_Degree field, never
// independently randomized, so a response is never internally
// contradictory (e.g. degree=10 but sector=SW).
func degreeToSector(deg float64) int {
	return int(math.Mod(deg+22.5, 360)/45) + 1
}

// windSpeedToBeaufort derives the Beaufort force from wind speed in
// knots (standard Beaufort scale thresholds), so Wind_Force_Bft is never
// independently randomized against Wind_Force_Kn.
func windSpeedToBeaufort(kn float64) int {
	thresholds := []float64{1, 4, 7, 11, 17, 22, 28, 34, 41, 48, 56, 64}
	for i, t := range thresholds {
		if kn < t {
			return i
		}
	}
	return 12
}

// addWeatherReadings fills the 17-field weather group. Directions are
// randomized once per call and their sectors derived (never
// independently randomized); Beaufort force is derived from wind speed.
// wind_speed_kn keeps its pre-existing key name and bounds (see
// handleReadings' own doc comment) — now formally grouped here.
func addWeatherReadings(readings map[string]any, rnd *rand.Rand) {
	windDirDeg := randRange(rnd, 0, 360)
	windSpeedKn := randRange(rnd, 0, 25)
	seaStateDirDeg := randRange(rnd, 0, 360)
	swellDirDeg := randRange(rnd, 0, 360)
	currentDirDeg := randRange(rnd, 0, 360)

	readings["wind_dir_deg"] = math.Round(windDirDeg)
	readings["wind_dir_sector"] = float64(degreeToSector(windDirDeg))
	readings["wind_speed_kn"] = round1(windSpeedKn)
	readings["wind_force_bft"] = float64(windSpeedToBeaufort(windSpeedKn))
	readings["sea_state_dir_deg"] = math.Round(seaStateDirDeg)
	readings["sea_state_dir_sector"] = float64(degreeToSector(seaStateDirDeg))
	readings["sea_state_force_douglas"] = math.Round(randRange(rnd, 0, 6))
	readings["wave_period_s"] = round1(randRange(rnd, 3, 12))
	readings["swell_dir_deg"] = math.Round(swellDirDeg)
	readings["swell_dir_sector"] = float64(degreeToSector(swellDirDeg))
	readings["swell_height_m"] = round1(randRange(rnd, 0, 4))
	readings["swell_period_s"] = round1(randRange(rnd, 4, 14))
	readings["current_dir_deg"] = math.Round(currentDirDeg)
	readings["current_dir_sector"] = float64(degreeToSector(currentDirDeg))
	readings["current_speed_kn"] = round1(randRange(rnd, 0, 3))
	readings["air_temp_c"] = round1(randRange(rnd, 15, 35))
	readings["sea_temp_c"] = round1(randRange(rnd, 15, 32))
}

// addDraftReadings fills the 8-field cargo-draft/water-depth group.
// Plausible laden-vessel figures with small per-query noise; aft draft
// is derived from fore draft plus a trim margin rather than
// independently randomized — a real vessel's draft reading is never
// aft < fore by an implausible amount, and independently randomizing
// both would occasionally produce a physically nonsensical trim.
func addDraftReadings(readings map[string]any, rnd *rand.Rand) {
	draftFore := randRange(rnd, 7, 13)
	trim := randRange(rnd, 0.2, 1.0)
	draftAft := draftFore + trim
	ballastActual := randRange(rnd, 800, 2500)
	displacementActual := randRange(rnd, 25000, 65000)

	readings["draft_fore_m"] = round1(draftFore)
	readings["draft_aft_m"] = round1(draftAft)
	readings["draft_fore_recommended_m"] = round1(draftFore + randRange(rnd, -0.15, 0.15))
	readings["draft_aft_recommended_m"] = round1(draftAft + randRange(rnd, -0.15, 0.15))
	readings["ballast_actual_mt"] = round1(ballastActual)
	readings["ballast_optimum_mt"] = round1(ballastActual + randRange(rnd, -100, 100))
	readings["displacement_actual_mt"] = round1(displacementActual)
	readings["water_depth_m"] = round1(randRange(rnd, 20, 150))
}

// consumptionRate is one fuel/consumer pair's plausible burn-rate bounds
// in mt/hour — the same "rate x elapsed hours" shape as the original
// hfo_consumption_mt formula (me_consumption_hfo_mt below reuses its
// exact 0.8-1.2 mt/h bounds, since it supersedes that field).
type consumptionRate struct {
	key    string
	loRate float64
	hiRate float64
}

var consumptionRates = []consumptionRate{
	{"me_consumption_hfo_mt", 0.8, 1.2},
	{"me_consumption_lfo_mt", 0.05, 0.15},
	{"me_consumption_mgo_mt", 0.05, 0.15},
	{"me_consumption_mdo_mt", 0.05, 0.15},
	{"ae_consumption_hfo_mt", 0.05, 0.15},
	{"ae_consumption_lfo_mt", 0.03, 0.08},
	{"ae_consumption_mgo_mt", 0.05, 0.12},
	{"ae_consumption_mdo_mt", 0.05, 0.12},
	{"boiler_consumption_hfo_mt", 0.02, 0.08},
	{"boiler_consumption_lfo_mt", 0.01, 0.05},
	{"boiler_consumption_mgo_mt", 0.01, 0.05},
	{"boiler_consumption_mdo_mt", 0.01, 0.05},
	{"incinerator_consumption_other_mt", 0.005, 0.02},
	{"cargo_heating_consumption_hfo_mt", 0.01, 0.05},
	{"cargo_heating_consumption_lfo_mt", 0.005, 0.03},
	{"cargo_heating_consumption_mgo_mt", 0.005, 0.03},
	{"cargo_heating_consumption_mdo_mt", 0.005, 0.03},
}

// addConsumptionReadings fills the 17-field engine.consumption group,
// each a distinct plausible burn-rate multiplied by the window's
// elapsed hours — same shape as the pre-existing hfo_consumption_mt
// formula this supersedes.
func addConsumptionReadings(readings map[string]any, rnd *rand.Rand, elapsedHours float64) {
	elapsed := math.Max(0, elapsedHours)
	for _, c := range consumptionRates {
		readings[c.key] = round1(elapsed * randRange(rnd, c.loRate, c.hiRate))
	}
}

// ratedMEPowerKW is the assumed main-engine rated power used to derive
// me1_load_pct from me1_load_kw — a fixed assumption, not itself
// randomized, so the two fields never disagree (design doc: "correlated
// fields kept consistent, e.g. me1_load_pct derived from me1_load_kw
// against a fixed rated-power assumption, not separately randomized").
const ratedMEPowerKW = 8000.0

// steadyRange is one engine.performance field's plausible steady-state
// bounds — independently randomized per call, no cross-field
// correlation implied.
type steadyRange struct {
	key    string
	lo, hi float64
}

var performanceSteadyRanges = []steadyRange{
	{"discharge_pump_sfoc_g_per_kwh", 180, 230},
	{"me_barometric_pressure_bar", 0.95, 1.05},
	{"me_air_intake_temp_c", 25, 40},
	{"me_charge_air_coolant_inlet_temp_c", 25, 36},
	{"me1_load_kw", 4000, 7200},
	{"me1_speed_rpm", 80, 110},
	{"prop_1_pitch_m", 5.5, 7.0},
	{"prop_1_pitch_ratio", 0.6, 1.0},
	{"me1_charge_air_inlet_temp_c", 35, 55},
	{"me1_charge_air_pressure_bar", 2.0, 3.5},
	{"me1_charge_air_cooler_pressure_drop_bar", 0.05, 0.25},
	{"me1_pmax_bar", 120, 160},
	{"me1_pcomp_bar", 90, 130},
	{"me1_tc_speed_rpm", 8000, 14000},
	{"me1_exh_temp_before_tc_c", 320, 420},
	{"me1_exh_temp_after_tc_c", 220, 320},
	{"me1_fuel_meter_mt_per_h", 0.6, 1.4},
	{"me1_sfoc_g_per_kwh", 165, 190},
	{"me1_sfoc_iso_g_per_kwh", 165, 190},
	{"ae_barometric_pressure_bar", 0.95, 1.05},
	{"ae_air_intake_temp_c", 25, 40},
	{"ae_charge_air_coolant_inlet_temp_c", 25, 36},
	{"ae1_load_kw", 200, 900},
	{"ae1_charge_air_inlet_temp_c", 35, 55},
	{"ae1_charge_air_pressure_bar", 1.5, 2.5},
	{"ae1_charge_air_cooler_pressure_drop_bar", 0.03, 0.15},
	{"ae1_tc_speed_rpm", 15000, 25000},
	{"ae1_pmax_bar", 90, 130},
	{"ae1_pcomp_bar", 60, 100},
	{"ae1_exh_temp_before_tc_c", 350, 450},
	{"ae1_exh_temp_after_tc_c", 250, 350},
	{"ae1_fuel_meter_mt_per_h", 0.05, 0.15},
	{"ae1_sfoc_g_per_kwh", 200, 230},
	{"ae1_sfoc_iso_g_per_kwh", 200, 230},
	{"boiler1_feed_water_flow_m3_min", 0.5, 3.0},
	{"boiler1_steam_pressure_bar", 6, 9},
	{"cooling_sw_inlet_temp_c", 20, 32},
	{"cooling_sw_outlet_temp_c", 30, 42},
	{"cooling_heat_exchanger_pressure_drop_bar", 0.2, 0.6},
	{"cooling_pump_pressure_bar", 1.5, 3.0},
	{"er_ventilation_waste_air_temp_c", 30, 45},
}

// performanceWholeSteadyRanges are the engine.performance steady-state
// fields the OVD schema types as wholeNumber (design doc field
// vocabulary table) rather than decimal — rounded to an integer via
// math.Round, same idiom as sea_state_force_douglas/wind_force_bft in
// addWeatherReadings, not round1's one-decimal rounding used for the
// decimal-typed fields above. A fractional value here would fail
// pkg/validation/fieldrules.go's RuleFieldFormat once these fields are
// wired into a schema.
var performanceWholeSteadyRanges = []steadyRange{
	{"me1_shaft_gen_power_kw", 0, 500},
	{"cooling_sw_pumps_in_service", 1, 3},
	{"er_ventilation_fans_in_service", 2, 6},
}

// cumulativeRange is one engine.performance field that accumulates with
// elapsed time in the window (running hours, cumulative energy) rather
// than being an instantaneous steady-state reading — loRate/hiRate are
// a duty-cycle multiplier applied to elapsedHours (design doc:
// "running-hour fields accumulate with elapsed time in the window").
type cumulativeRange struct {
	key            string
	loRate, hiRate float64
}

var performanceCumulativeRanges = []cumulativeRange{
	{"discharge_pump_work_kwh", 50, 150},
	{"shore_power_kwh", 0, 300},
	{"shore_power_duration_h", 0, 1},
	{"air_compressor_1_running_h", 0, 0.6},
	{"air_compressor_2_running_h", 0, 0.6},
	{"scrubber_running_h", 0, 0.9},
	{"bow_thruster_running_h", 0, 0.1},
	{"stern_thruster_running_h", 0, 0.1},
	{"thruster_3_running_h", 0, 0.05},
}

// dischargePumpFuelTypes are the plausible fuel codes for this bulk/
// tanker-profile fleet's cargo discharge pump, from
// schemas/ovd-3.13/enums/fuel-types.json's own `code` values — the
// sensor endpoint represents this one enum-typed field as its string
// code (see this plan's Global Constraints on the number/bool/string
// wire type).
var dischargePumpFuelTypes = []string{"HFO", "MGO", "MDO"}

// addPerformanceReadings fills the 57-field engine.performance group:
// 44 independently-randomized steady-state fields, 9 cumulative
// (running-hour/energy) fields, 2 booleans, 1 enum-as-string, and
// me1_load_pct derived from me1_load_kw (never independently
// randomized against it).
func addPerformanceReadings(readings map[string]any, rnd *rand.Rand, elapsedHours float64) {
	elapsed := math.Max(0, elapsedHours)
	for _, sr := range performanceSteadyRanges {
		readings[sr.key] = round1(randRange(rnd, sr.lo, sr.hi))
	}
	for _, sr := range performanceWholeSteadyRanges {
		readings[sr.key] = math.Round(randRange(rnd, sr.lo, sr.hi))
	}
	for _, cr := range performanceCumulativeRanges {
		readings[cr.key] = round1(elapsed * randRange(rnd, cr.loRate, cr.hiRate))
	}
	meLoadKW, _ := readings["me1_load_kw"].(float64)
	readings["me1_load_pct"] = round1(meLoadKW / ratedMEPowerKW * 100)
	readings["me1_aux_blower"] = rnd.Float64() < 0.5
	readings["boiler1_operation_mode"] = rnd.Float64() < 0.7
	readings["discharge_pump_fuel_type"] = dischargePumpFuelTypes[rnd.Intn(len(dischargePumpFuelTypes))]
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
