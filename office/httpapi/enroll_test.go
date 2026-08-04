// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/synccred"
)

// distinctTestIMO returns a checksum-valid IMO number distinct per
// caller (id in [0, 99]) — same derivation as office/store's own
// distinctIMO helper, duplicated here since it's unexported across
// packages, so this file's tests don't collide with vessels_test.go's
// fixed "9074729" or office/store's own test IDs on the shared Postgres
// instance's vessels.imo UNIQUE constraint.
func distinctTestIMO(t *testing.T, id int) string {
	t.Helper()
	digits := [6]int{9, 0, 7, (id / 10) % 10, id % 10, 2}
	sum := 0
	for i, weight := 0, 7; i < 6; i, weight = i+1, weight-1 {
		sum += digits[i] * weight
	}
	check := sum % 10
	imo := ""
	for _, d := range digits {
		imo += string(rune('0' + d))
	}
	imo += string(rune('0' + check))
	return imo
}

// issueTestEnrollment creates a vessel (as a freshly logged-in Admin) and
// issues its enrollment, returning the vessel id and the one-time
// plaintext code a vessel would present to POST /api/enroll.
func issueTestEnrollment(t *testing.T, s *Server, id int) (vesselID, code string) {
	t.Helper()
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/vessels", createVesselRequest{
		IMO: distinctTestIMO(t, id), Name: "MV Enroll Test", Type: "Bulk Carrier",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/vessels: status %d, body %s", rec.Code, rec.Body)
	}
	created := decodeBody[vesselView](t, rec)
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM vessels WHERE id = $1`, created.ID)
	})

	rec = c.do(http.MethodPost, "/api/vessels/"+created.ID+"/enrollment/issue", issueEnrollmentRequest{})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../enrollment/issue: status %d, body %s", rec.Code, rec.Body)
	}
	issued := decodeBody[issueResultView](t, rec)
	return created.ID, issued.Code
}

func TestHandleRedeemEnrollment(t *testing.T) {
	s := newTestServer(t)
	_, code := issueTestEnrollment(t, s, 70)

	// No session/cookie needed — a fresh, unauthenticated client, since
	// the vessel has no office account, only the code.
	anon := newTestClient(t, s)
	rec := anon.do(http.MethodPost, "/api/enroll", redeemEnrollmentRequest{Code: code})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/enroll: status %d, body %s", rec.Code, rec.Body)
	}
	resp := decodeBody[redeemEnrollmentResponse](t, rec)
	if resp.Credential == "" {
		t.Error("Credential is empty")
	}

	// The same code cannot be redeemed twice.
	rec = anon.do(http.MethodPost, "/api/enroll", redeemEnrollmentRequest{Code: code})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("second POST /api/enroll with the same code: status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestHandleRedeemEnrollment_StoresDRPublicKey covers architecture
// 12.5's DR keypair exchange: the vessel-supplied drPublicKey on the
// redeem request lands on the enrollment row, ready for a future
// restore-bundle to encrypt against.
func TestHandleRedeemEnrollment_StoresDRPublicKey(t *testing.T) {
	s := newTestServer(t)
	vesselID, code := issueTestEnrollment(t, s, 74)

	anon := newTestClient(t, s)
	rec := anon.do(http.MethodPost, "/api/enroll", redeemEnrollmentRequest{Code: code, DRPublicKey: "age1testpublickeyexample"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/enroll: status %d, body %s", rec.Code, rec.Body)
	}

	e, err := s.st.GetEnrollment(context.Background(), vesselID)
	if err != nil {
		t.Fatalf("GetEnrollment: %v", err)
	}
	if e.DRPublicKey != "age1testpublickeyexample" {
		t.Errorf("DRPublicKey = %q, want %q", e.DRPublicKey, "age1testpublickeyexample")
	}
}

func TestHandleRedeemEnrollment_UnknownCode(t *testing.T) {
	s := newTestServer(t)
	anon := newTestClient(t, s)
	rec := anon.do(http.MethodPost, "/api/enroll", redeemEnrollmentRequest{Code: "not-a-real-code"})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleRedeemEnrollment_MissingCode(t *testing.T) {
	s := newTestServer(t)
	anon := newTestClient(t, s)
	rec := anon.do(http.MethodPost, "/api/enroll", redeemEnrollmentRequest{})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleRedeemEnrollment_RevokedCodeIsRejected(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/vessels", createVesselRequest{
		IMO: distinctTestIMO(t, 71), Name: "MV Revoke Test", Type: "Bulk Carrier",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/vessels: status %d, body %s", rec.Code, rec.Body)
	}
	created := decodeBody[vesselView](t, rec)
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM vessels WHERE id = $1`, created.ID)
	})

	rec = c.do(http.MethodPost, "/api/vessels/"+created.ID+"/enrollment/issue", issueEnrollmentRequest{})
	issued := decodeBody[issueResultView](t, rec)

	rec = c.do(http.MethodPost, "/api/vessels/"+created.ID+"/enrollment/revoke", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST .../enrollment/revoke: status %d, body %s", rec.Code, rec.Body)
	}

	anon := newTestClient(t, s)
	rec = anon.do(http.MethodPost, "/api/enroll", redeemEnrollmentRequest{Code: issued.Code})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /api/enroll with a revoked code: status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleRevokeEnrollment_AlsoRevokesCredential(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/vessels", createVesselRequest{
		IMO: distinctTestIMO(t, 72), Name: "MV Revoke After Redeem", Type: "Bulk Carrier",
	})
	created := decodeBody[vesselView](t, rec)
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM vessels WHERE id = $1`, created.ID)
	})
	rec = c.do(http.MethodPost, "/api/vessels/"+created.ID+"/enrollment/issue", issueEnrollmentRequest{})
	issued := decodeBody[issueResultView](t, rec)

	// Redeem for real, so there's an actual credential to revoke.
	anon := newTestClient(t, s)
	rec = anon.do(http.MethodPost, "/api/enroll", redeemEnrollmentRequest{Code: issued.Code})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/enroll: status %d, body %s", rec.Code, rec.Body)
	}
	credential := decodeBody[redeemEnrollmentResponse](t, rec).Credential

	rec = c.do(http.MethodPost, "/api/vessels/"+created.ID+"/enrollment/revoke", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST .../enrollment/revoke: status %d, body %s", rec.Code, rec.Body)
	}

	got, err := s.st.GetVesselCredentialByLookupHash(context.Background(), synccred.LookupHash(credential))
	if err != nil {
		t.Fatalf("GetVesselCredentialByLookupHash: %v", err)
	}
	if got.RevokedAt == nil {
		t.Error("credential RevokedAt is nil after revoking the enrollment, want it set")
	}
}

func TestHandleReissueEnrollment_AlsoRevokesOldCredential(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/vessels", createVesselRequest{
		IMO: distinctTestIMO(t, 73), Name: "MV Reissue After Redeem", Type: "Bulk Carrier",
	})
	created := decodeBody[vesselView](t, rec)
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM vessels WHERE id = $1`, created.ID)
	})
	rec = c.do(http.MethodPost, "/api/vessels/"+created.ID+"/enrollment/issue", issueEnrollmentRequest{})
	issued := decodeBody[issueResultView](t, rec)

	anon := newTestClient(t, s)
	rec = anon.do(http.MethodPost, "/api/enroll", redeemEnrollmentRequest{Code: issued.Code})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/enroll: status %d, body %s", rec.Code, rec.Body)
	}
	oldCredential := decodeBody[redeemEnrollmentResponse](t, rec).Credential

	rec = c.do(http.MethodPost, "/api/vessels/"+created.ID+"/enrollment/reissue", issueEnrollmentRequest{})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST .../enrollment/reissue: status %d, body %s", rec.Code, rec.Body)
	}

	got, err := s.st.GetVesselCredentialByLookupHash(context.Background(), synccred.LookupHash(oldCredential))
	if err != nil {
		t.Fatalf("GetVesselCredentialByLookupHash: %v", err)
	}
	if got.RevokedAt == nil {
		t.Error("old credential RevokedAt is nil after reissuing the enrollment, want it set")
	}
}
