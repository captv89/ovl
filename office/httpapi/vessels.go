// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/configbundle"
	"github.com/captv89/ovl/office/enrollment"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/office/vessels"
)

// requireAdmin is authenticatedUser's counterpart for vessel/enrollment
// mutations (design handoff B2: "Create, edit, enroll, revoke: Admin").
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return nil, false
	}
	if !user.CanManageUsers() {
		httpjson.WriteError(w, http.StatusForbidden, "only Admin may manage vessels and enrollment")
		return nil, false
	}
	return user, true
}

// vesselView is the JSON shape for one vessel, with the enrollment state
// folded in (design handoff B2's list: "name, IMO, type, groups,
// enrollment state, last sync, last report, overdue indicator"). Last
// report/overdue and last sync are now all real (Office UI rework Phase
// O3, and the vessel-health follow-up) — the vessel_sync_status table's
// own last-contact timestamp/app version, recorded by every successful
// SyncStatus call (office/syncservice.Server.SyncStatus).
type vesselView struct {
	ID              string     `json:"id"`
	IMO             string     `json:"imo"`
	Name            string     `json:"name"`
	Type            string     `json:"type"`
	Groups          []string   `json:"groups"`
	EnrollmentState string     `json:"enrollmentState"` // "notIssued" | enrollment.State
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	LastReportAt    *time.Time `json:"lastReportAt,omitempty"`
	// OverdueHours is set only once the vessel has actually exceeded its
	// effective cadence's max gap — a vessel with a recent report but no
	// violation has no overdue badge, matching B1/B2's "overdue" concept
	// (a compliance state), not a raw "hours since last report" readout.
	OverdueHours *float64 `json:"overdueHours,omitempty"`
	// LastSyncAt/AppVersion are absent if this vessel has never completed
	// a SyncStatus call — "never synced" is a real, reachable state (a
	// freshly enrolled vessel), not an error. Distinct from LastReportAt:
	// a vessel can sync (check in, pull config/commands) without having
	// any report to push that cycle.
	LastSyncAt *time.Time `json:"lastSyncAt,omitempty"`
	AppVersion string     `json:"appVersion,omitempty"`
	// AppliedBundleID/Version report which config bundle the vessel itself
	// says it is running (audit 2026-07-22 §3.3) — compared against the
	// assigned bundle (bundleAssignmentView) to tell whether the ship is on
	// its assigned config or hasn't pulled it yet. Empty/0 until the vessel
	// has applied any bundle.
	AppliedBundleID      string `json:"appliedBundleId,omitempty"`
	AppliedBundleVersion int64  `json:"appliedBundleVersion,omitempty"`
}

func toVesselView(v *vessels.Vessel, enrollState string, lastReportAt *time.Time, overdueHours *float64, sync *store.VesselSyncStatus) vesselView {
	groups := v.Groups
	if groups == nil {
		groups = []string{}
	}
	view := vesselView{
		ID: v.ID, IMO: v.IMO, Name: v.Name, Type: v.Type, Groups: groups,
		EnrollmentState: enrollState, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
		LastReportAt: lastReportAt, OverdueHours: overdueHours,
	}
	if sync != nil {
		view.LastSyncAt = &sync.LastSeenAt
		view.AppVersion = sync.AppVersion
		view.AppliedBundleID = sync.AppliedBundleID
		view.AppliedBundleVersion = sync.AppliedBundleVersion
	}
	return view
}

// syncStatusFor returns vesselID's last-seen sync status, or nil if it
// has never completed one — same "absence is a real state, not an
// error" convention overdueStatusFor's lastReportAt already follows.
func (s *Server) syncStatusFor(ctx context.Context, vesselID string) *store.VesselSyncStatus {
	status, err := s.st.GetVesselSyncStatus(ctx, vesselID)
	if err != nil {
		return nil
	}
	return status
}

// overdueStatusFor resolves whether v is overdue given lastReportAt (nil
// if it has never reported — never counted as overdue by this metric;
// enrollment state already distinguishes "not issued"/"never reported"
// from a real cadence violation) and every stored cadence rule. Returns
// the raw elapsed hours since the last report, not "elapsed minus the
// threshold" — design handoff B1's "Overdue by" column reads as time
// since the last report (a vessel last heard from 2 days 8 hours ago
// shows "2d 8h"), the threshold only decides *whether* to show it.
func overdueStatusFor(v *vessels.Vessel, lastReportAt *time.Time, rules []*compliance.CadenceRule, now time.Time) (last *time.Time, overdueHours *float64) {
	if lastReportAt == nil {
		return nil, nil
	}
	cadence := compliance.EffectiveCadence(rules, v.ID, v.Groups)
	elapsed := now.Sub(*lastReportAt).Hours()
	if elapsed <= cadence.MaxGapHours {
		return lastReportAt, nil
	}
	return lastReportAt, &elapsed
}

// enrollmentView is the JSON shape for one vessel's enrollment record.
// CodeHash/InitialMasterPasswordHash never appear — those secrets exist
// only in the one-time issueResultView response. HasDRKey exposes only
// whether e.DRPublicKey is set, never the key itself (not a secret, but
// no reason to ship it to a screen that never uses it) — B2's DR tab
// needs this to tell "vessel hasn't (re-)redeemed since the DR keypair
// exchange landed" apart from "restore bundle is ready to generate"
// without trial-and-error against handleGenerateRestoreBundle's 409.
type enrollmentView struct {
	State                 string     `json:"state"`
	InitialMasterUsername string     `json:"initialMasterUsername,omitempty"`
	IssuedAt              *time.Time `json:"issuedAt,omitempty"`
	RevokedAt             *time.Time `json:"revokedAt,omitempty"`
	HasDRKey              bool       `json:"hasDRKey"`
}

func toEnrollmentView(e *enrollment.Enrollment) *enrollmentView {
	if e == nil {
		return nil
	}
	v := &enrollmentView{State: string(e.State), InitialMasterUsername: e.InitialMasterUsername, HasDRKey: e.DRPublicKey != ""}
	if !e.IssuedAt.IsZero() {
		v.IssuedAt = &e.IssuedAt
	}
	v.RevokedAt = e.RevokedAt
	return v
}

// bundleAssignmentView is the "which bundle applies" preview shown on
// B2's Config tab — display purposes only. Resolution itself now defers
// to office/configbundle.Resolve, the authoritative version PullInbox
// also uses (see office/syncservice), rather than a separately
// maintained copy of the same vessel-vs-group precedence logic.
type bundleAssignmentView struct {
	BundleID    string    `json:"bundleId"`
	BundleLabel string    `json:"bundleLabel"`
	PublishedAt time.Time `json:"publishedAt"`
	ScopeType   string    `json:"scopeType"`
	ScopeKey    string    `json:"scopeKey,omitempty"`
}

func (s *Server) resolveBundleAssignment(w http.ResponseWriter, r *http.Request, v *vessels.Vessel) (*bundleAssignmentView, bool) {
	assignments, err := s.st.ListBundleAssignments(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	match := configbundle.Resolve(assignments, v.ID, v.Groups)
	if match == nil {
		return nil, true
	}
	bundle, err := s.st.GetConfigBundle(r.Context(), match.BundleID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, true
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	return &bundleAssignmentView{
		BundleID: bundle.ID, BundleLabel: bundle.Label, PublishedAt: bundle.PublishedAt,
		ScopeType: string(match.Scope.Type), ScopeKey: match.Scope.Key,
	}, true
}

// enrollmentStateOf loads vesselID's enrollment, returning "notIssued"
// (not an error) when none has ever been issued — GetEnrollment's own
// doc comment establishes that absence-of-row convention.
func (s *Server) enrollmentStateOf(w http.ResponseWriter, r *http.Request, vesselID string) (*enrollment.Enrollment, string, bool) {
	e, err := s.st.GetEnrollment(r.Context(), vesselID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, "notIssued", true
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil, "", false
	}
	return e, string(e.State), true
}

// handleListVessels serves design handoff B2's fleet list. Viewable by
// any authenticated office user.
func (s *Server) handleListVessels(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	list, err := s.st.ListVessels(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	lastReports, rules, ok := s.loadOverdueInputs(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	out := make([]vesselView, len(list))
	for i, v := range list {
		_, state, ok := s.enrollmentStateOf(w, r, v.ID)
		if !ok {
			return
		}
		var lastReportAt *time.Time
		if t, ok := lastReports[v.ID]; ok {
			lastReportAt = &t
		}
		last, overdue := overdueStatusFor(v, lastReportAt, rules, now)
		out[i] = toVesselView(v, state, last, overdue, s.syncStatusFor(r.Context(), v.ID))
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// loadOverdueInputs fetches the two pieces overdueStatusFor needs, once
// per request rather than once per vessel — see LastReportEventTimeByVessel's
// own doc comment on why this must stay a single aggregate query.
func (s *Server) loadOverdueInputs(w http.ResponseWriter, r *http.Request) (map[string]time.Time, []*compliance.CadenceRule, bool) {
	lastReports, err := s.st.LastReportEventTimeByVessel(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil, nil, false
	}
	rules, err := s.st.ListCadenceRules(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil, nil, false
	}
	return lastReports, rules, true
}

type createVesselRequest struct {
	IMO    string   `json:"imo"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Groups []string `json:"groups"`
}

// handleCreateVessel creates a new vessel profile. Admin-only (B2).
func (s *Server) handleCreateVessel(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var req createVesselRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	v, err := vessels.NewVessel(req.IMO, req.Name, req.Type, req.Groups)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.CreateVessel(r.Context(), v); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toVesselView(v, "notIssued", nil, nil, nil))
}

// vesselDetailView is B2's vessel detail response, covering the Profile,
// Enrollment, Config, and DR-status tabs in one fetch. Reports
// (api.listReports with a vesselId filter) is real but fetched separately
// by its own tab client-side. RestoreCommands is the DR tab's push-status
// history (queued/fetched/applied, architecture 12.5/11.2's DR push
// path) — folded in here rather than a separate endpoint since it's
// small and the DR tab already needs a vessel fetch anyway; downloading
// the bundle itself (GET .../restore-bundle) stays its own request, same
// as before, since that's a large binary payload, not JSON.
type vesselDetailView struct {
	Vessel           vesselView            `json:"vessel"`
	Enrollment       *enrollmentView       `json:"enrollment"`
	BundleAssignment *bundleAssignmentView `json:"bundleAssignment"`
	RestoreCommands  []restoreCommandView  `json:"restoreCommands"`
	// VesselUsers/UserCommands are architecture 9.3/12.4's remote user
	// administration (2026-07-21) — B2's Users tab. VesselUsers is the
	// read-only roster mirror (office/store.ReplaceVesselUsers, updated
	// on the vessel's own sync check-ins); UserCommands is the same
	// queued/fetched/applied status history RestoreCommands already
	// established the shape for.
	VesselUsers  []vesselUserView  `json:"vesselUsers"`
	UserCommands []userCommandView `json:"userCommands"`
}

// restoreCommandView is one office-issued DR push's status (design
// handoff B2's DR tab) — mirrors office/store.RestoreCommand.
type restoreCommandView struct {
	ID        string     `json:"id"`
	Reason    string     `json:"reason"`
	IssuedBy  string     `json:"issuedBy"`
	IssuedAt  time.Time  `json:"issuedAt"`
	FetchedAt *time.Time `json:"fetchedAt,omitempty"`
	AppliedAt *time.Time `json:"appliedAt,omitempty"`
}

func toRestoreCommandView(c store.RestoreCommand) restoreCommandView {
	return restoreCommandView{
		ID: c.ID, Reason: c.Reason, IssuedBy: c.IssuedBy, IssuedAt: c.IssuedAt,
		FetchedAt: c.FetchedAt, AppliedAt: c.AppliedAt,
	}
}

func (s *Server) handleGetVessel(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	v, err := s.st.GetVessel(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "vessel not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	e, state, ok := s.enrollmentStateOf(w, r, v.ID)
	if !ok {
		return
	}
	bundle, ok := s.resolveBundleAssignment(w, r, v)
	if !ok {
		return
	}
	lastReports, rules, ok := s.loadOverdueInputs(w, r)
	if !ok {
		return
	}
	var lastReportAt *time.Time
	if t, ok := lastReports[v.ID]; ok {
		lastReportAt = &t
	}
	last, overdue := overdueStatusFor(v, lastReportAt, rules, time.Now().UTC())

	restoreCommands, err := s.st.ListRestoreCommandsForVessel(r.Context(), v.ID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	restoreCommandViews := make([]restoreCommandView, len(restoreCommands))
	for i, c := range restoreCommands {
		restoreCommandViews[i] = toRestoreCommandView(c)
	}

	vesselUsers, err := s.st.ListVesselUsers(r.Context(), v.ID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	vesselUserViews := make([]vesselUserView, len(vesselUsers))
	for i, u := range vesselUsers {
		vesselUserViews[i] = toVesselUserView(u)
	}
	userCommands, err := s.st.ListUserCommandsForVessel(r.Context(), v.ID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	userCommandViews := make([]userCommandView, len(userCommands))
	for i, c := range userCommands {
		userCommandViews[i] = toUserCommandView(c)
	}

	httpjson.WriteJSON(w, http.StatusOK, vesselDetailView{
		Vessel: toVesselView(v, state, last, overdue, s.syncStatusFor(r.Context(), v.ID)), Enrollment: toEnrollmentView(e), BundleAssignment: bundle,
		RestoreCommands: restoreCommandViews, VesselUsers: vesselUserViews, UserCommands: userCommandViews,
	})
}

type updateVesselRequest struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Groups []string `json:"groups"`
}

// handleUpdateVessel edits profile fields (never IMO — vessels.Vessel
// has no setter for it). Admin-only (B2).
func (s *Server) handleUpdateVessel(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	v, err := s.st.GetVessel(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "vessel not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req updateVesselRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := v.UpdateProfile(req.Name, req.Type, req.Groups); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.UpdateVessel(r.Context(), v); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, state, ok := s.enrollmentStateOf(w, r, v.ID)
	if !ok {
		return
	}
	lastReports, rules, ok := s.loadOverdueInputs(w, r)
	if !ok {
		return
	}
	var lastReportAt *time.Time
	if t, ok := lastReports[v.ID]; ok {
		lastReportAt = &t
	}
	last, overdue := overdueStatusFor(v, lastReportAt, rules, time.Now().UTC())
	httpjson.WriteJSON(w, http.StatusOK, toVesselView(v, state, last, overdue, s.syncStatusFor(r.Context(), v.ID)))
}

// issueResultView carries the one-time plaintext enrollment secrets for
// the printable sheet (architecture 12.4) — the only response shape that
// ever includes them; they cannot be recovered from any GET afterward.
type issueResultView struct {
	Enrollment            enrollmentView `json:"enrollment"`
	Code                  string         `json:"code"`
	InitialMasterPassword string         `json:"initialMasterPassword"`
}

type issueEnrollmentRequest struct {
	InitialMasterUsername string `json:"initialMasterUsername"`
}

// handleIssueEnrollment issues a fresh enrollment for a vessel that has
// never had one. Admin-only (B2).
func (s *Server) handleIssueEnrollment(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	vesselID := r.PathValue("id")
	if _, err := s.st.GetVessel(r.Context(), vesselID); errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "vessel not found")
		return
	} else if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req issueEnrollmentRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := enrollment.Issue(vesselID, req.InitialMasterUsername)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.UpsertEnrollment(r.Context(), result.Enrollment); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, issueResultView{
		Enrollment: *toEnrollmentView(result.Enrollment), Code: result.Code, InitialMasterPassword: result.InitialMasterPassword,
	})
}

// handleReissueEnrollment regenerates the code/credentials for a vessel
// that already has an enrollment record (any state — issued, enrolled,
// or revoked). Admin-only (B2).
func (s *Server) handleReissueEnrollment(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	vesselID := r.PathValue("id")
	e, err := s.st.GetEnrollment(r.Context(), vesselID)
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "vessel has no enrollment to re-issue; issue one first")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req issueEnrollmentRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := e.Reissue(req.InitialMasterUsername)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.UpsertEnrollment(r.Context(), result.Enrollment); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// A fresh code implies the previous credential (if the vessel had
	// already redeemed one) should stop working too — otherwise it would
	// silently keep syncing even though a brand new enrollment just
	// superseded it. See RevokeVesselCredential's own doc comment.
	if err := s.st.RevokeVesselCredential(r.Context(), vesselID, time.Now().UTC()); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, issueResultView{
		Enrollment: *toEnrollmentView(result.Enrollment), Code: result.Code, InitialMasterPassword: result.InitialMasterPassword,
	})
}

// handleRevokeEnrollment invalidates a vessel's current enrollment
// credential. Admin-only (B2).
func (s *Server) handleRevokeEnrollment(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	vesselID := r.PathValue("id")
	e, err := s.st.GetEnrollment(r.Context(), vesselID)
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "vessel has no enrollment to revoke")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	e.Revoke()
	if err := s.st.UpsertEnrollment(r.Context(), e); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Revoking the enrollment must also cut off any sync credential
	// already redeemed from it — otherwise a vessel that enrolled before
	// the revoke keeps syncing indefinitely despite architecture 11.1's
	// "revocable in the office" promise. See RevokeVesselCredential's own
	// doc comment.
	if err := s.st.RevokeVesselCredential(r.Context(), vesselID, time.Now().UTC()); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toEnrollmentView(e))
}
