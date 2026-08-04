// SPDX-License-Identifier: AGPL-3.0-only

package schema

import (
	"os"
	"path/filepath"
	"testing"
)

// repoSchemasDir returns the absolute path to the repo's top-level
// schemas/ directory, so tests can validate against the real,
// currently-curated meta-schema and OVD schema documents rather than a
// fixture that can drift from them.
func repoSchemasDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "schemas"))
	if err != nil {
		t.Fatalf("resolve schemas dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "meta-schema.json")); err != nil {
		t.Fatalf("repo schemas dir %s does not contain meta-schema.json: %v", dir, err)
	}
	return dir
}

func newRealValidator(t *testing.T) *Validator {
	t.Helper()
	v, err := NewValidator(os.DirFS(repoSchemasDir(t)), "meta-schema.json")
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	return v
}
