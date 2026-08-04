// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/captv89/ovl/office/enrollment"
	"github.com/captv89/ovl/office/vessels"
)

// createTestVessel creates and registers cleanup for a vessel to hang an
// enrollment test off of (enrollments.vessel_id is FK-constrained to
// vessels.id).
func createTestVessel(t *testing.T, st *Store, first int) *vessels.Vessel {
	t.Helper()
	v := newTestVessel(t, first, nil)
	if err := st.CreateVessel(context.Background(), v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}
	t.Cleanup(func() { deleteTestVessel(t, st, v.ID) })
	return v
}

func TestStore_UpsertAndGetEnrollment(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 20)

	res, err := enrollment.Issue(v.ID, "master")
	if err != nil {
		t.Fatalf("enrollment.Issue: %v", err)
	}
	if err := st.UpsertEnrollment(ctx, res.Enrollment); err != nil {
		t.Fatalf("UpsertEnrollment: %v", err)
	}

	got, err := st.GetEnrollment(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetEnrollment: %v", err)
	}
	if got.State != enrollment.StateIssued {
		t.Errorf("State = %q, want %q", got.State, enrollment.StateIssued)
	}
	if got.InitialMasterUsername != "master" {
		t.Errorf("InitialMasterUsername = %q, want %q", got.InitialMasterUsername, "master")
	}
	if got.CodeHash == "" {
		t.Error("CodeHash round-tripped empty")
	}
	if got.RevokedAt != nil {
		t.Error("RevokedAt is set on a freshly issued enrollment, want nil")
	}

	match, err := got.VerifyCode(res.Code)
	if err != nil {
		t.Fatalf("VerifyCode: %v", err)
	}
	if !match {
		t.Error("VerifyCode(the real code) = false after round-tripping through Postgres, want true")
	}
}

func TestStore_GetEnrollment_NotFound(t *testing.T) {
	st := openTestStore(t)
	v := createTestVessel(t, st, 21)
	if _, err := st.GetEnrollment(context.Background(), v.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetEnrollment(never issued) error = %v, want ErrNotFound", err)
	}
}

func TestStore_UpsertEnrollment_Reissue(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 22)

	res, err := enrollment.Issue(v.ID, "master")
	if err != nil {
		t.Fatalf("enrollment.Issue: %v", err)
	}
	if err := st.UpsertEnrollment(ctx, res.Enrollment); err != nil {
		t.Fatalf("UpsertEnrollment: %v", err)
	}

	res.Enrollment.Revoke()
	if err := st.UpsertEnrollment(ctx, res.Enrollment); err != nil {
		t.Fatalf("UpsertEnrollment (revoke): %v", err)
	}
	revoked, err := st.GetEnrollment(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetEnrollment: %v", err)
	}
	if revoked.State != enrollment.StateRevoked {
		t.Errorf("State = %q, want %q", revoked.State, enrollment.StateRevoked)
	}
	if revoked.RevokedAt == nil {
		t.Error("RevokedAt is nil after a revoke round-tripped through Postgres")
	}

	res2, err := revoked.Reissue("chief-officer")
	if err != nil {
		t.Fatalf("Reissue: %v", err)
	}
	if err := st.UpsertEnrollment(ctx, res2.Enrollment); err != nil {
		t.Fatalf("UpsertEnrollment (reissue): %v", err)
	}
	reissued, err := st.GetEnrollment(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetEnrollment: %v", err)
	}
	if reissued.State != enrollment.StateIssued {
		t.Errorf("State = %q, want %q", reissued.State, enrollment.StateIssued)
	}
	if reissued.RevokedAt != nil {
		t.Error("RevokedAt is still set after a reissue round-tripped through Postgres, want nil")
	}
	if reissued.InitialMasterUsername != "chief-officer" {
		t.Errorf("InitialMasterUsername = %q, want %q", reissued.InitialMasterUsername, "chief-officer")
	}
	if match, _ := reissued.VerifyCode(res.Code); match {
		t.Error("VerifyCode(original, superseded code) = true, want false")
	}
	match, err := reissued.VerifyCode(res2.Code)
	if err != nil {
		t.Fatalf("VerifyCode: %v", err)
	}
	if !match {
		t.Error("VerifyCode(the reissued code) = false, want true")
	}
}

func TestStore_Enrollment_CascadeDeletesWithVessel(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := newTestVessel(t, 23, nil)
	if err := st.CreateVessel(ctx, v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}

	res, err := enrollment.Issue(v.ID, "master")
	if err != nil {
		t.Fatalf("enrollment.Issue: %v", err)
	}
	if err := st.UpsertEnrollment(ctx, res.Enrollment); err != nil {
		t.Fatalf("UpsertEnrollment: %v", err)
	}

	deleteTestVessel(t, st, v.ID)

	if _, err := st.GetEnrollment(ctx, v.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetEnrollment after vessel deletion error = %v, want ErrNotFound (cascade)", err)
	}
}
