// SPDX-License-Identifier: AGPL-3.0-only

// Package schemaversions models the office-side schema version registry
// and the upload workflow that publishes new versions (architecture 5.2
// immutability, 5.3 the office schema update workflow; design handoff B5,
// "Configuration: schema versions"). It is the schema-version counterpart
// to office/fieldpolicy and office/compliance: those packages consume a
// (schemaName, schemaVersion) identity but, until this package, nothing
// in office recorded which versions actually exist or what content they
// carry — see office/fieldpolicy's own doc comment, written before this
// package existed.
//
// PrepareUpload/Publish implement 5.3 steps 3-6 (upload, validate, diff,
// publish) as pure functions with no store dependency, matching how
// pkg/schema.DiffSchemas and office/fieldpolicy.Migrate are already
// store-free — office/store wires them to Postgres. Steps 1-2 (download
// the current version, edit it externally) and the actual HTTP/UI layer
// are not built here — same "backend package first, no office HTTP/UI
// yet" deferral used for every other Phase 3 checklist item so far
// (roles, vessels, enrollment, config authoring).
package schemaversions

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/captv89/ovl/pkg/schema"
)

// Source identifies where a schema version's content originated (design
// handoff B5's version list column "source: project curated or company
// edited"; architecture 5.4's two update paths).
type Source string

const (
	SourceProjectCurated Source = "projectCurated"
	SourceCompanyEdited  Source = "companyEdited"
)

var validSources = map[Source]bool{
	SourceProjectCurated: true,
	SourceCompanyEdited:  true,
}

// SchemaVersion is one published, immutable version of a schema document
// (architecture 5.2: "a new version is always a new record... Reports
// permanently reference the schema version they were created under").
// There is no Update method — every field here is set once at Publish and
// never changed again, matching the immutability rule literally.
type SchemaVersion struct {
	ID          string // UUIDv7
	SchemaName  string
	Version     string // e.g. "3.13", "3.13-company-r2" (architecture 5.2)
	Source      Source
	Content     []byte // raw schema JSON, verbatim — same "store the exact bytes" choice as syncproto's ConfigBundle.content_json
	PublishedAt time.Time
	PublishedBy string // office/auth User.ID
}

// UploadPreview is the result of validating and diffing an uploaded
// schema document before publish (design handoff B5: "upload → meta-
// schema validation results ... → diff review screen"). Nothing is
// persisted yet — Publish only runs after the user confirms the preview.
type UploadPreview struct {
	Parsed *schema.Schema

	// Diff is nil when there is no current published version for this
	// schema name to compare against (the first-ever version), matching
	// how design handoff B5's diff review screen has nothing to show in
	// that case.
	Diff *schema.Diff
}

// PrepareUpload validates raw schema document bytes against the
// meta-schema (5.3 step 4: "hard reject on failure with precise errors")
// and, if current is non-nil, computes the mandatory diff against it (5.3
// step 5). current should be the schema name's latest published
// SchemaVersion, or nil if none exists yet.
func PrepareUpload(validator *schema.Validator, current *SchemaVersion, data []byte) (*UploadPreview, error) {
	if err := validator.Validate(data); err != nil {
		return nil, err
	}
	parsed, err := schema.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("schemaversions: parse uploaded schema: %w", err)
	}
	preview := &UploadPreview{Parsed: parsed}
	if current != nil {
		currentParsed, err := schema.Parse(current.Content)
		if err != nil {
			return nil, fmt.Errorf("schemaversions: parse current version %s@%s: %w", current.SchemaName, current.Version, err)
		}
		preview.Diff = schema.DiffSchemas(currentParsed, parsed)
	}
	return preview, nil
}

// Publish creates a new immutable SchemaVersion from a previously
// prepared and user-confirmed upload (5.3 step 6: "user confirms and
// publishes it as a new immutable version"). version is the string the
// user gave it at confirm time (design handoff B5: "confirm and name the
// new version") — it is taken as an explicit parameter rather than read
// from content's own embedded schema.Schema.Version field, matching
// office/fieldpolicy.SchemaFieldPolicy's own choice to treat schema
// identity (name, version) as caller-supplied rather than re-derived from
// parsed content every time.
//
// Uniqueness of (schemaName, version) is enforced at the store layer (a
// UNIQUE constraint), not here — this constructs a valid record, it does
// not check for collisions against anything already published.
func Publish(schemaName, version string, source Source, content []byte, publishedBy string) (*SchemaVersion, error) {
	schemaName = strings.TrimSpace(schemaName)
	if schemaName == "" {
		return nil, errors.New("schemaversions: schema name is required")
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return nil, errors.New("schemaversions: version is required")
	}
	if !validSources[source] {
		return nil, fmt.Errorf("schemaversions: unknown source %q", source)
	}
	if len(content) == 0 {
		return nil, errors.New("schemaversions: content is required")
	}
	publishedBy = strings.TrimSpace(publishedBy)
	if publishedBy == "" {
		return nil, errors.New("schemaversions: publishedBy is required")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("schemaversions: generate id: %w", err)
	}
	return &SchemaVersion{
		ID:          id.String(),
		SchemaName:  schemaName,
		Version:     version,
		Source:      source,
		Content:     content,
		PublishedAt: time.Now().UTC(),
		PublishedBy: publishedBy,
	}, nil
}
