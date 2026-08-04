// SPDX-License-Identifier: AGPL-3.0-only

package schema

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestLoad(t *testing.T) {
	v := newRealValidator(t)
	fsys := fstest.MapFS{
		"log-abstract.json": &fstest.MapFile{Data: []byte(validMinimalSchema)},
		"invalid.json":      &fstest.MapFile{Data: []byte(`{"ovdVersion":"3.13"}`)},
	}

	s, err := Load(fsys, "log-abstract.json", v)
	if err != nil {
		t.Fatalf("Load(valid): %v", err)
	}
	if s.SchemaName != "log-abstract" {
		t.Errorf("SchemaName = %q, want log-abstract", s.SchemaName)
	}

	if _, err := Load(fsys, "invalid.json", v); err == nil {
		t.Fatal("Load(invalid): got nil error, want a validation error")
	}

	if _, err := Load(fsys, "missing.json", v); err == nil {
		t.Fatal("Load(missing file): got nil error, want an error")
	}
}

func TestLoad_NoValidator(t *testing.T) {
	fsys := fstest.MapFS{"s.json": &fstest.MapFile{Data: []byte(validMinimalSchema)}}
	if _, err := Load(fsys, "s.json", nil); err != nil {
		t.Fatalf("Load with nil validator: %v", err)
	}
}

// TestLoad_RealCuratedSchemas validates every currently-curated OVD 3.13
// schema document against the real meta-schema, formalizing the check
// that must eventually run in CI (architecture 5.2: "every schema file in
// the repo is validated").
func TestLoad_RealCuratedSchemas(t *testing.T) {
	v := newRealValidator(t)
	dir := filepath.Join(repoSchemasDir(t), "ovd-3.13")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	fsys := os.DirFS(dir)
	found := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		found++
		t.Run(e.Name(), func(t *testing.T) {
			s, err := Load(fsys, e.Name(), v)
			if err != nil {
				t.Fatalf("Load(%s): %v", e.Name(), err)
			}
			if len(s.Fields) == 0 {
				t.Errorf("%s: schema has no fields", e.Name())
			}
		})
	}
	if found != 5 {
		t.Errorf("found %d schema JSON files in %s, want 5", found, dir)
	}
}
