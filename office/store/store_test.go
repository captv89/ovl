// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"os"
	"testing"
)

// testDSN returns the Postgres connection string for integration tests,
// skipping the test if none is configured. Real infra, not a mock —
// matches vessel/store's own precedent of testing against a real SQLite
// file rather than an in-memory fake. See deploy/office/docker-compose.yml
// for a local instance to point OVL_TEST_DATABASE_URL at.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("OVL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("OVL_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	return dsn
}

func TestOpen_RunsMigrationsAndPings(t *testing.T) {
	dsn := testDSN(t)

	st, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	if err := st.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestOpen_IsIdempotent(t *testing.T) {
	dsn := testDSN(t)

	for i := range 2 {
		st, err := Open(dsn)
		if err != nil {
			t.Fatalf("Open (pass %d): %v", i, err)
		}
		if err := st.Close(); err != nil {
			t.Fatalf("Close (pass %d): %v", i, err)
		}
	}
}
