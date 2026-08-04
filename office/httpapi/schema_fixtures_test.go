// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/schemaversions"
	"github.com/captv89/ovl/pkg/schema"
)

// createTestUser2 is createTestUser's counterpart for tests that need a
// second distinct user (e.g. a Viewer and a Config Manager in the same
// test) — createTestUser always uses t.Name() as the username, which
// collides on the users.username UNIQUE constraint if called twice in
// one test.
func createTestUser2(t *testing.T, s *Server, roles auth.Roles, password string) *auth.User {
	t.Helper()
	u, err := auth.NewUser(t.Name()+"-2", password, roles)
	if err != nil {
		t.Fatalf("auth.NewUser: %v", err)
	}
	u.MustChangePassword = false
	if err := s.st.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			t.Errorf("cleanup: open raw connection: %v", err)
			return
		}
		defer func() { _ = raw.Close() }()
		if _, err := raw.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, u.ID); err != nil {
			t.Errorf("cleanup: delete test user %s: %v", u.ID, err)
		}
	})
	return u
}

// testSchemaField is a minimal meta-schema-valid field, for building
// fixture schema documents in tests.
func testSchemaField(name string, typ schema.FieldType, schemaMandatory bool) schema.Field {
	return schema.Field{
		Name: name, Label: name, Type: typ, SchemaMandatory: schemaMandatory,
		Relevance: "test", Section: "test", AppliesToEvents: []string{"*"},
	}
}

// testFieldPolicySchemaName gives GET/PUT field-policy tests their own
// synthetic schema name, distinct per test — those endpoints never
// validate schemaName against the meta-schema's fixed enum (unlike
// preview/publish, see testSchemaName), so there is no reason to share a
// real enum value and risk mixing in main.go's real seeded curated data
// (which is exactly what happens once seedCuratedSchemas has run
// against this shared test Postgres, e.g. from a live verification
// pass) — a synthetic name keeps ListSchemaVersions history scoped to
// exactly what each test publishes.
func testFieldPolicySchemaName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-fieldpolicy-%s", t.Name())
}

// buildSchemaJSON marshals a meta-schema-valid document. schemaName must
// be one of the meta-schema's fixed enum values (e.g. "cargo-nomination")
// only when the caller will pass content through the real validator
// (handlePreviewSchemaUpload/handlePublishSchemaVersion); GET/PUT
// field-policy tests should use testFieldPolicySchemaName instead.
// version can be any test-unique string to dodge the schema_versions
// UNIQUE(schema_name, version) constraint in the shared test Postgres.
func buildSchemaJSON(t *testing.T, schemaName, version string, fields ...schema.Field) []byte {
	t.Helper()
	data, err := json.Marshal(schema.Schema{
		SchemaName: schemaName, OvdVersion: "3.13", Version: version, Fields: fields,
	})
	if err != nil {
		t.Fatalf("marshal fixture schema: %v", err)
	}
	return data
}

// publishTestSchemaVersion inserts a schema version directly through the
// store (bypassing the meta-schema validator, matching office/store's
// own schemaversions_test.go convention) and registers cleanup scoped to
// this exact (schemaName, version) row — schemaName is often a real
// meta-schema enum value shared with genuine seeded data (see
// main.go's seedCuratedSchemas), so cleanup must never blanket-delete by
// schema_name alone.
func publishTestSchemaVersion(t *testing.T, s *Server, schemaName, version, source string, content []byte) {
	t.Helper()
	sv, err := schemaversions.Publish(schemaName, version, schemaversions.Source(source), content, "test-fixture")
	if err != nil {
		t.Fatalf("schemaversions.Publish: %v", err)
	}
	if err := s.st.CreateSchemaVersion(context.Background(), sv); err != nil {
		t.Fatalf("CreateSchemaVersion: %v", err)
	}
	registerSchemaVersionCleanup(t, schemaName, version)
}

// registerSchemaVersionCleanup deletes exactly one (schemaName, version)
// row (and any field_policy_assignments for it, across every scope) at
// test end — used both by publishTestSchemaVersion and by tests that
// publish through the real HTTP endpoint instead.
func registerSchemaVersionCleanup(t *testing.T, schemaName, version string) {
	t.Helper()
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			t.Errorf("cleanup: open raw connection: %v", err)
			return
		}
		defer func() { _ = raw.Close() }()
		if _, err := raw.ExecContext(context.Background(), `DELETE FROM schema_versions WHERE schema_name = $1 AND version = $2`, schemaName, version); err != nil {
			t.Errorf("cleanup: delete test schema version %s@%s: %v", schemaName, version, err)
		}
		if _, err := raw.ExecContext(context.Background(), `DELETE FROM field_policy_assignments WHERE schema_name = $1 AND schema_version = $2`, schemaName, version); err != nil {
			t.Errorf("cleanup: delete test field policies for %s@%s: %v", schemaName, version, err)
		}
	})
}
