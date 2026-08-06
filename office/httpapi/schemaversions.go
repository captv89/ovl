// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/schemaversions"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/schema"
)

// requireConfigManager is requireAdmin's counterpart for schema-version
// and field-policy authoring (design handoff B5/B6: "Roles: Config
// Manager").
func (s *Server) requireConfigManager(w http.ResponseWriter, r *http.Request) bool {
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return false
	}
	if !user.CanManageConfig() {
		httpjson.WriteError(w, http.StatusForbidden, "only Config Manager may author schema versions and field policy")
		return false
	}
	return true
}

// schemaVersionSummaryView is one row of design handoff B5's version
// list: "version string, source, published date, in-use-by count." The
// in-use-by count is omitted — architecture notes it's genuinely blocked
// on Phase 4 sync existing (no report data anywhere to count against
// yet).
type schemaVersionSummaryView struct {
	SchemaName  string    `json:"schemaName"`
	Version     string    `json:"version"`
	Source      string    `json:"source"`
	PublishedAt time.Time `json:"publishedAt"`
	PublishedBy string    `json:"publishedBy"`
	FieldCount  int       `json:"fieldCount"`
}

func toSchemaVersionSummaryView(sv *schemaversions.SchemaVersion) (schemaVersionSummaryView, error) {
	parsed, err := schema.Parse(sv.Content)
	if err != nil {
		return schemaVersionSummaryView{}, fmt.Errorf("parse schema version %s@%s: %w", sv.SchemaName, sv.Version, err)
	}
	return schemaVersionSummaryView{
		SchemaName: sv.SchemaName, Version: sv.Version, Source: string(sv.Source),
		PublishedAt: sv.PublishedAt, PublishedBy: sv.PublishedBy, FieldCount: len(parsed.Fields),
	}, nil
}

// handleListLatestSchemaVersions serves design handoff B5's top-level
// version list: the latest published version of every known schema.
// Viewable by any authenticated office user (B5: "Others read-only
// list").
func (s *Server) handleListLatestSchemaVersions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	names, err := s.st.ListSchemaNames(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]schemaVersionSummaryView, 0, len(names))
	for _, name := range names {
		sv, err := s.st.LatestSchemaVersion(r.Context(), name)
		if err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		view, err := toSchemaVersionSummaryView(sv)
		if err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, view)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// handleListSchemaVersionHistory serves one schema name's full version
// history, newest first.
func (s *Server) handleListSchemaVersionHistory(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	list, err := s.st.ListSchemaVersions(r.Context(), r.PathValue("name"))
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]schemaVersionSummaryView, len(list))
	for i, sv := range list {
		view, err := toSchemaVersionSummaryView(sv)
		if err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out[i] = view
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// schemaFieldView is one field's shape for B6's field table: "name,
// label, type and unit, OVD relevance (read-only, from the curated
// schema)."
type schemaFieldView struct {
	Name            string   `json:"name"`
	Label           string   `json:"label"`
	Type            string   `json:"type"`
	Unit            *string  `json:"unit,omitempty"`
	SchemaMandatory bool     `json:"schemaMandatory"`
	Relevance       string   `json:"relevance"`
	Section         string   `json:"section"`
	AppliesToEvents []string `json:"appliesToEvents"`
}

// schemaDetailView is one published schema version, parsed.
type schemaDetailView struct {
	SchemaName string            `json:"schemaName"`
	Version    string            `json:"version"`
	Sections   []string          `json:"sections"`
	Fields     []schemaFieldView `json:"fields"`
}

func toSchemaDetailView(sv *schemaversions.SchemaVersion) (schemaDetailView, error) {
	parsed, err := schema.Parse(sv.Content)
	if err != nil {
		return schemaDetailView{}, fmt.Errorf("parse schema version %s@%s: %w", sv.SchemaName, sv.Version, err)
	}
	fields := make([]schemaFieldView, len(parsed.Fields))
	for i, f := range parsed.Fields {
		fields[i] = schemaFieldView{
			Name: f.Name, Label: f.Label, Type: string(f.Type), Unit: f.Unit,
			SchemaMandatory: f.SchemaMandatory, Relevance: f.Relevance, Section: f.Section,
			AppliesToEvents: f.AppliesToEvents,
		}
	}
	return schemaDetailView{SchemaName: sv.SchemaName, Version: sv.Version, Sections: parsed.Sections, Fields: fields}, nil
}

// handleGetSchemaVersion returns one specific published version, parsed
// into its fields — the data B6's field table and B5's diff detail both
// need.
func (s *Server) handleGetSchemaVersion(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	sv, err := s.st.GetSchemaVersion(r.Context(), r.PathValue("name"), r.PathValue("version"))
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "schema version not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, err := toSchemaDetailView(sv)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, view)
}

// handleDownloadSchemaVersion serves the exact, byte-verbatim uploaded
// JSON for one version (design handoff B5: "Download JSON per version")
// — the file an admin downloads to hand-edit for the next OVD revision.
func (s *Server) handleDownloadSchemaVersion(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	name, version := r.PathValue("name"), r.PathValue("version")
	sv, err := s.st.GetSchemaVersion(r.Context(), name, version)
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "schema version not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.json"`, name, version))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(sv.Content) // #nosec G705 -- served as application/json + Content-Disposition: attachment, never inline-rendered by a browser
}

type schemaUploadPreviewRequest struct {
	Content string `json:"content"`
}

// schemaDiffView mirrors schema.Diff for JSON — a category-level grouping
// (added/removed/type/mandatoriness/enum changed), matching design
// handoff B5's diff review screen groups exactly.
type schemaDiffView struct {
	Added                []schemaFieldView `json:"added"`
	Removed              []schemaFieldView `json:"removed"`
	TypeChanged          []string          `json:"typeChanged"`
	MandatorinessChanged []string          `json:"mandatorinessChanged"`
	EnumChanged          []string          `json:"enumChanged"`
}

func toFieldView(f schema.Field) schemaFieldView {
	return schemaFieldView{
		Name: f.Name, Label: f.Label, Type: string(f.Type), Unit: f.Unit,
		SchemaMandatory: f.SchemaMandatory, Relevance: f.Relevance, Section: f.Section,
		AppliesToEvents: f.AppliesToEvents,
	}
}

func toSchemaDiffView(d *schema.Diff) *schemaDiffView {
	if d == nil {
		return nil
	}
	changedNames := func(changes []schema.FieldChange) []string {
		names := make([]string, len(changes))
		for i, c := range changes {
			names[i] = c.Name
		}
		return names
	}
	added := make([]schemaFieldView, len(d.Added))
	for i, f := range d.Added {
		added[i] = toFieldView(f)
	}
	removed := make([]schemaFieldView, len(d.Removed))
	for i, f := range d.Removed {
		removed[i] = toFieldView(f)
	}
	return &schemaDiffView{
		Added: added, Removed: removed,
		TypeChanged:          changedNames(d.TypeChanged),
		MandatorinessChanged: changedNames(d.MandatorinessChanged),
		EnumChanged:          changedNames(d.EnumChanged),
	}
}

type schemaUploadPreviewResponse struct {
	Valid         bool            `json:"valid"`
	Error         string          `json:"error,omitempty"`
	ParsedName    string          `json:"parsedName,omitempty"`
	ParsedVersion string          `json:"parsedVersion,omitempty"`
	Diff          *schemaDiffView `json:"diff,omitempty"`
}

// handlePreviewSchemaUpload runs design handoff B5's upload flow steps
// 3-5 (validate, then diff against the current published version) —
// nothing is persisted here; Publish only runs once the admin confirms
// the diff review screen and names the new version.
func (s *Server) handlePreviewSchemaUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireConfigManager(w, r) {
		return
	}
	var req schemaUploadPreviewRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := r.PathValue("name")
	current, err := s.st.LatestSchemaVersion(r.Context(), name)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		current = nil
	}
	preview, err := schemaversions.PrepareUpload(s.validator, current, []byte(req.Content))
	if err != nil {
		httpjson.WriteJSON(w, http.StatusOK, schemaUploadPreviewResponse{Valid: false, Error: err.Error()})
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, schemaUploadPreviewResponse{
		Valid: true, ParsedName: preview.Parsed.SchemaName, ParsedVersion: preview.Parsed.Version,
		Diff: toSchemaDiffView(preview.Diff),
	})
}

type schemaPublishRequest struct {
	Version string `json:"version"`
	Content string `json:"content"`
}

// handlePublishSchemaVersion runs design handoff B5's step 6: creates a
// new immutable schema version once the admin has confirmed the preview
// and named it. Re-validates rather than trusting the earlier preview
// call, since content is round-tripped through the client between the
// two requests.
func (s *Server) handlePublishSchemaVersion(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	if !user.CanManageConfig() {
		httpjson.WriteError(w, http.StatusForbidden, "only Config Manager may author schema versions and field policy")
		return
	}
	var req schemaPublishRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	content := []byte(req.Content)
	if err := s.validator.Validate(content); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := r.PathValue("name")
	_, err := s.st.LatestSchemaVersion(r.Context(), name)
	source := schemaversions.SourceCompanyEdited
	if errors.Is(err, store.ErrNotFound) {
		source = schemaversions.SourceProjectCurated
	} else if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sv, err := schemaversions.Publish(name, req.Version, source, content, user.ID)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.CreateSchemaVersion(r.Context(), sv); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, err := toSchemaVersionSummaryView(sv)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, view)
}
