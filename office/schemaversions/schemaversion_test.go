// SPDX-License-Identifier: AGPL-3.0-only

package schemaversions

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/schema"
)

const validLogAbstractV1 = `{
	"schemaName": "log-abstract",
	"ovdVersion": "3.13",
	"version": "3.13",
	"fields": [
		{
			"name": "IMO",
			"label": "IMO Number",
			"type": "wholeNumber",
			"schemaMandatory": true,
			"relevance": "mandatory for MRV&DCS",
			"section": "header",
			"appliesToEvents": ["*"]
		}
	]
}`

// validLogAbstractV2 adds one field and changes IMO's type, so diffing it
// against validLogAbstractV1 exercises both the Added and TypeChanged
// categories pkg/schema.DiffSchemas tracks.
const validLogAbstractV2 = `{
	"schemaName": "log-abstract",
	"ovdVersion": "3.13",
	"version": "3.13-company-r1",
	"fields": [
		{
			"name": "IMO",
			"label": "IMO Number",
			"type": "text",
			"schemaMandatory": true,
			"relevance": "mandatory for MRV&DCS",
			"section": "header",
			"appliesToEvents": ["*"]
		},
		{
			"name": "Vessel_Name",
			"label": "Vessel Name",
			"type": "text",
			"schemaMandatory": false,
			"relevance": "x",
			"section": "header",
			"appliesToEvents": ["*"]
		}
	]
}`

// realValidator compiles the repo's actual meta-schema, matching how
// pkg/schema's own tests validate against the real thing rather than a
// synthetic stand-in — office upload validation must behave identically
// to CI's schema validation, not just some hand-rolled subset of it.
func realValidator(t *testing.T) *schema.Validator {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "schemas"))
	if err != nil {
		t.Fatalf("resolve schemas dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "meta-schema.json")); err != nil {
		t.Fatalf("repo schemas dir %s does not contain meta-schema.json: %v", dir, err)
	}
	v, err := schema.NewValidator(os.DirFS(dir), "meta-schema.json")
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	return v
}

func TestPublish(t *testing.T) {
	tests := []struct {
		name        string
		schemaName  string
		version     string
		source      Source
		content     []byte
		publishedBy string
		wantErr     bool
	}{
		{
			name:        "valid",
			schemaName:  "log-abstract",
			version:     "3.13",
			source:      SourceProjectCurated,
			content:     []byte(validLogAbstractV1),
			publishedBy: "user-1",
		},
		{name: "missing schema name", version: "3.13", source: SourceProjectCurated, content: []byte(validLogAbstractV1), publishedBy: "user-1", wantErr: true},
		{name: "missing version", schemaName: "log-abstract", source: SourceProjectCurated, content: []byte(validLogAbstractV1), publishedBy: "user-1", wantErr: true},
		{name: "unknown source", schemaName: "log-abstract", version: "3.13", source: Source("bogus"), content: []byte(validLogAbstractV1), publishedBy: "user-1", wantErr: true},
		{name: "missing content", schemaName: "log-abstract", version: "3.13", source: SourceProjectCurated, publishedBy: "user-1", wantErr: true},
		{name: "missing publishedBy", schemaName: "log-abstract", version: "3.13", source: SourceProjectCurated, content: []byte(validLogAbstractV1), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sv, err := Publish(tt.schemaName, tt.version, tt.source, tt.content, tt.publishedBy)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Publish() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if sv.ID == "" {
				t.Error("ID is empty")
			}
			if sv.PublishedAt.IsZero() {
				t.Error("PublishedAt is zero")
			}
			if sv.PublishedAt.Location() != time.UTC {
				t.Errorf("PublishedAt location = %v, want UTC", sv.PublishedAt.Location())
			}
		})
	}
}

func TestPublish_TrimsWhitespace(t *testing.T) {
	sv, err := Publish("  log-abstract  ", "  3.13  ", SourceProjectCurated, []byte(validLogAbstractV1), "  user-1  ")
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if sv.SchemaName != "log-abstract" || sv.Version != "3.13" || sv.PublishedBy != "user-1" {
		t.Errorf("Publish did not trim whitespace: %+v", sv)
	}
}

func TestPrepareUpload_ValidatesAgainstMetaSchema(t *testing.T) {
	v := realValidator(t)

	if _, err := PrepareUpload(v, nil, []byte(validLogAbstractV1)); err != nil {
		t.Fatalf("PrepareUpload(valid): %v", err)
	}

	invalid := `{"schemaName":"log-abstract","ovdVersion":"3.13","version":"3.13","fields":[{"name":"X","label":"X","type":"currency","schemaMandatory":false,"relevance":"x","section":"header","appliesToEvents":["*"]}]}`
	if _, err := PrepareUpload(v, nil, []byte(invalid)); err == nil {
		t.Fatal("PrepareUpload(invalid): got nil error, want a meta-schema validation error")
	}
}

func TestPrepareUpload_NoCurrentVersion(t *testing.T) {
	v := realValidator(t)

	preview, err := PrepareUpload(v, nil, []byte(validLogAbstractV1))
	if err != nil {
		t.Fatalf("PrepareUpload: %v", err)
	}
	if preview.Diff != nil {
		t.Errorf("Diff = %+v, want nil when there is no current version", preview.Diff)
	}
	if preview.Parsed.SchemaName != "log-abstract" {
		t.Errorf("Parsed.SchemaName = %q, want log-abstract", preview.Parsed.SchemaName)
	}
}

func TestPrepareUpload_DiffsAgainstCurrentVersion(t *testing.T) {
	v := realValidator(t)

	current, err := Publish("log-abstract", "3.13", SourceProjectCurated, []byte(validLogAbstractV1), "user-1")
	if err != nil {
		t.Fatalf("Publish(current): %v", err)
	}

	preview, err := PrepareUpload(v, current, []byte(validLogAbstractV2))
	if err != nil {
		t.Fatalf("PrepareUpload: %v", err)
	}
	if preview.Diff == nil {
		t.Fatal("Diff is nil, want a diff against the current version")
	}
	if len(preview.Diff.Added) != 1 || preview.Diff.Added[0].Name != "Vessel_Name" {
		t.Errorf("Diff.Added = %+v, want [Vessel_Name]", preview.Diff.Added)
	}
	if len(preview.Diff.TypeChanged) != 1 || preview.Diff.TypeChanged[0].Name != "IMO" {
		t.Errorf("Diff.TypeChanged = %+v, want [IMO]", preview.Diff.TypeChanged)
	}
	if preview.Diff.Empty() {
		t.Error("Diff.Empty() = true, want false")
	}
}

func TestPrepareUpload_CurrentVersionUnparsable(t *testing.T) {
	v := realValidator(t)
	current := &SchemaVersion{SchemaName: "log-abstract", Version: "3.13", Content: []byte("not json")}
	if _, err := PrepareUpload(v, current, []byte(validLogAbstractV1)); err == nil {
		t.Fatal("PrepareUpload with an unparsable current version: got nil error, want an error")
	}
}

func TestPrepareUpload_MalformedUpload(t *testing.T) {
	v := realValidator(t)
	if _, err := PrepareUpload(v, nil, []byte("not json")); err == nil {
		t.Fatal("PrepareUpload(malformed JSON): got nil error, want an error")
	}
}
