// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/captv89/ovl/office/schemaversions"
)

// distinctSchemaName gives each test its own schema_name value, so
// parallel/repeated runs against the same shared Postgres don't collide
// on the schema_versions UNIQUE(schema_name, version) constraint — same
// reasoning as vessels_test.go's distinctIMO.
func distinctSchemaName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("log-abstract-test-%s", t.Name())
}

func deleteTestSchemaVersions(t *testing.T, st *Store, schemaName string) {
	t.Helper()
	if _, err := st.db.ExecContext(context.Background(), `DELETE FROM schema_versions WHERE schema_name = $1`, schemaName); err != nil {
		t.Errorf("cleanup: delete test schema versions for %s: %v", schemaName, err)
	}
}

func TestStore_CreateAndGetSchemaVersion(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	schemaName := distinctSchemaName(t)
	t.Cleanup(func() { deleteTestSchemaVersions(t, st, schemaName) })

	sv, err := schemaversions.Publish(schemaName, "3.13", schemaversions.SourceProjectCurated, []byte(`{"schemaName":"log-abstract"}`), "user-1")
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := st.CreateSchemaVersion(ctx, sv); err != nil {
		t.Fatalf("CreateSchemaVersion: %v", err)
	}

	got, err := st.GetSchemaVersion(ctx, schemaName, "3.13")
	if err != nil {
		t.Fatalf("GetSchemaVersion: %v", err)
	}
	if got.ID != sv.ID {
		t.Errorf("ID = %q, want %q", got.ID, sv.ID)
	}
	if got.Source != schemaversions.SourceProjectCurated {
		t.Errorf("Source = %q, want %q", got.Source, schemaversions.SourceProjectCurated)
	}
	if string(got.Content) != `{"schemaName":"log-abstract"}` {
		t.Errorf("Content = %q, want the exact bytes published", got.Content)
	}
	if got.PublishedBy != "user-1" {
		t.Errorf("PublishedBy = %q, want user-1", got.PublishedBy)
	}
	if !got.PublishedAt.Equal(sv.PublishedAt) {
		t.Errorf("PublishedAt = %v, want %v", got.PublishedAt, sv.PublishedAt)
	}
}

func TestStore_GetSchemaVersion_NotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.GetSchemaVersion(context.Background(), "does-not-exist", "1.0"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSchemaVersion(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_CreateSchemaVersion_DuplicateVersion(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	schemaName := distinctSchemaName(t)
	t.Cleanup(func() { deleteTestSchemaVersions(t, st, schemaName) })

	sv1, err := schemaversions.Publish(schemaName, "3.13", schemaversions.SourceProjectCurated, []byte(`{}`), "user-1")
	if err != nil {
		t.Fatalf("Publish (first): %v", err)
	}
	if err := st.CreateSchemaVersion(ctx, sv1); err != nil {
		t.Fatalf("CreateSchemaVersion (first): %v", err)
	}

	sv2, err := schemaversions.Publish(schemaName, "3.13", schemaversions.SourceCompanyEdited, []byte(`{}`), "user-2")
	if err != nil {
		t.Fatalf("Publish (second): %v", err)
	}
	if err := st.CreateSchemaVersion(ctx, sv2); err == nil {
		t.Fatal("CreateSchemaVersion with a duplicate (schema_name, version): got nil error, want a UNIQUE constraint violation")
	}
}

func TestStore_LatestSchemaVersion(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	schemaName := distinctSchemaName(t)
	t.Cleanup(func() { deleteTestSchemaVersions(t, st, schemaName) })

	older, err := schemaversions.Publish(schemaName, "3.13", schemaversions.SourceProjectCurated, []byte(`{}`), "user-1")
	if err != nil {
		t.Fatalf("Publish (older): %v", err)
	}
	if err := st.CreateSchemaVersion(ctx, older); err != nil {
		t.Fatalf("CreateSchemaVersion (older): %v", err)
	}

	newer, err := schemaversions.Publish(schemaName, "3.13-company-r1", schemaversions.SourceCompanyEdited, []byte(`{}`), "user-2")
	if err != nil {
		t.Fatalf("Publish (newer): %v", err)
	}
	newer.PublishedAt = older.PublishedAt.Add(time.Second) // guarantee strict, Postgres-visible ordering (TIMESTAMPTZ is microsecond-precision; both Publish calls can land in the same instant, or even the same microsecond)
	if err := st.CreateSchemaVersion(ctx, newer); err != nil {
		t.Fatalf("CreateSchemaVersion (newer): %v", err)
	}

	got, err := st.LatestSchemaVersion(ctx, schemaName)
	if err != nil {
		t.Fatalf("LatestSchemaVersion: %v", err)
	}
	if got.ID != newer.ID {
		t.Errorf("LatestSchemaVersion returned %q, want the newer version %q", got.ID, newer.ID)
	}
}

func TestStore_LatestSchemaVersion_NotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.LatestSchemaVersion(context.Background(), "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Errorf("LatestSchemaVersion(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_ListSchemaVersions(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	schemaName := distinctSchemaName(t)
	t.Cleanup(func() { deleteTestSchemaVersions(t, st, schemaName) })

	v1, err := schemaversions.Publish(schemaName, "3.13", schemaversions.SourceProjectCurated, []byte(`{}`), "user-1")
	if err != nil {
		t.Fatalf("Publish (v1): %v", err)
	}
	if err := st.CreateSchemaVersion(ctx, v1); err != nil {
		t.Fatalf("CreateSchemaVersion (v1): %v", err)
	}
	v2, err := schemaversions.Publish(schemaName, "3.13-company-r1", schemaversions.SourceCompanyEdited, []byte(`{}`), "user-2")
	if err != nil {
		t.Fatalf("Publish (v2): %v", err)
	}
	v2.PublishedAt = v1.PublishedAt.Add(time.Second)
	if err := st.CreateSchemaVersion(ctx, v2); err != nil {
		t.Fatalf("CreateSchemaVersion (v2): %v", err)
	}

	// A different schema name's version must not leak into this
	// schema's list.
	other, err := schemaversions.Publish(schemaName+"-other", "1.0", schemaversions.SourceProjectCurated, []byte(`{}`), "user-1")
	if err != nil {
		t.Fatalf("Publish (other): %v", err)
	}
	if err := st.CreateSchemaVersion(ctx, other); err != nil {
		t.Fatalf("CreateSchemaVersion (other): %v", err)
	}
	t.Cleanup(func() { deleteTestSchemaVersions(t, st, schemaName+"-other") })

	list, err := st.ListSchemaVersions(ctx, schemaName)
	if err != nil {
		t.Fatalf("ListSchemaVersions: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListSchemaVersions returned %d versions, want 2", len(list))
	}
	if list[0].ID != v2.ID || list[1].ID != v1.ID {
		t.Errorf("ListSchemaVersions order = [%q, %q], want newest-first [%q, %q]", list[0].ID, list[1].ID, v2.ID, v1.ID)
	}
}

func TestStore_ListSchemaVersionsSince(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	schemaName := distinctSchemaName(t)
	t.Cleanup(func() { deleteTestSchemaVersions(t, st, schemaName) })

	v1, err := schemaversions.Publish(schemaName, "3.13", schemaversions.SourceProjectCurated, []byte(`{}`), "user-1")
	if err != nil {
		t.Fatalf("Publish (v1): %v", err)
	}
	if err := st.CreateSchemaVersion(ctx, v1); err != nil {
		t.Fatalf("CreateSchemaVersion (v1): %v", err)
	}

	// Nothing before this vessel's baseline cursor (0) should be
	// invisible once we know v1's real cursor — establish the baseline
	// first.
	beforeV2, err := st.ListSchemaVersionsSince(ctx, 0)
	if err != nil {
		t.Fatalf("ListSchemaVersionsSince(0): %v", err)
	}
	var v1Cursor int64
	found := false
	for _, item := range beforeV2 {
		if item.Version.ID == v1.ID {
			v1Cursor = item.Cursor
			found = true
		}
	}
	if !found {
		t.Fatalf("v1 not present in ListSchemaVersionsSince(0)")
	}

	v2, err := schemaversions.Publish(schemaName, "3.13-company-r1", schemaversions.SourceCompanyEdited, []byte(`{}`), "user-2")
	if err != nil {
		t.Fatalf("Publish (v2): %v", err)
	}
	if err := st.CreateSchemaVersion(ctx, v2); err != nil {
		t.Fatalf("CreateSchemaVersion (v2): %v", err)
	}

	sinceV1, err := st.ListSchemaVersionsSince(ctx, v1Cursor)
	if err != nil {
		t.Fatalf("ListSchemaVersionsSince(v1Cursor): %v", err)
	}
	sawV1, sawV2 := false, false
	for _, item := range sinceV1 {
		if item.Version.ID == v1.ID {
			sawV1 = true
		}
		if item.Version.ID == v2.ID {
			sawV2 = true
		}
	}
	if sawV1 {
		t.Error("ListSchemaVersionsSince(v1Cursor) still includes v1, want only versions after it")
	}
	if !sawV2 {
		t.Error("ListSchemaVersionsSince(v1Cursor) does not include v2")
	}
}
