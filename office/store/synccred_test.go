// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/captv89/ovl/office/enrollment"
	"github.com/captv89/ovl/office/synccred"
)

func TestStore_UpsertAndGetVesselCredentialByLookupHash(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 30)

	result, err := synccred.Mint(v.ID)
	if err != nil {
		t.Fatalf("synccred.Mint: %v", err)
	}
	if err := st.UpsertVesselCredential(ctx, result.Credential); err != nil {
		t.Fatalf("UpsertVesselCredential: %v", err)
	}

	got, err := st.GetVesselCredentialByLookupHash(ctx, synccred.LookupHash(result.Token))
	if err != nil {
		t.Fatalf("GetVesselCredentialByLookupHash: %v", err)
	}
	if got.VesselID != v.ID {
		t.Errorf("VesselID = %q, want %q", got.VesselID, v.ID)
	}
	if got.RevokedAt != nil {
		t.Error("RevokedAt is set on a freshly minted credential, want nil")
	}

	match, err := got.Verify(result.Token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !match {
		t.Error("Verify(the real token) = false after round-tripping through Postgres, want true")
	}
}

func TestStore_GetVesselCredentialByLookupHash_NotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.GetVesselCredentialByLookupHash(context.Background(), "no-such-hash"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetVesselCredentialByLookupHash(unknown hash) error = %v, want ErrNotFound", err)
	}
}

func TestStore_UpsertVesselCredential_Supersedes(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 31)

	first, err := synccred.Mint(v.ID)
	if err != nil {
		t.Fatalf("synccred.Mint: %v", err)
	}
	if err := st.UpsertVesselCredential(ctx, first.Credential); err != nil {
		t.Fatalf("UpsertVesselCredential (first): %v", err)
	}

	second, err := synccred.Mint(v.ID)
	if err != nil {
		t.Fatalf("synccred.Mint: %v", err)
	}
	if err := st.UpsertVesselCredential(ctx, second.Credential); err != nil {
		t.Fatalf("UpsertVesselCredential (second): %v", err)
	}

	// The first token's lookup hash must no longer resolve to anything —
	// the row was replaced in place, not duplicated.
	if _, err := st.GetVesselCredentialByLookupHash(ctx, synccred.LookupHash(first.Token)); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetVesselCredentialByLookupHash(superseded token) error = %v, want ErrNotFound", err)
	}

	got, err := st.GetVesselCredentialByLookupHash(ctx, synccred.LookupHash(second.Token))
	if err != nil {
		t.Fatalf("GetVesselCredentialByLookupHash (second): %v", err)
	}
	if match, _ := got.Verify(second.Token); !match {
		t.Error("Verify(the current token) = false, want true")
	}
}

func TestStore_Enrollment_ListIssuedEnrollments(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	vIssued := createTestVessel(t, st, 32)
	vEnrolled := createTestVessel(t, st, 33)

	issuedRes, err := enrollment.Issue(vIssued.ID, "master")
	if err != nil {
		t.Fatalf("enrollment.Issue: %v", err)
	}
	if err := st.UpsertEnrollment(ctx, issuedRes.Enrollment); err != nil {
		t.Fatalf("UpsertEnrollment (issued): %v", err)
	}

	enrolledRes, err := enrollment.Issue(vEnrolled.ID, "master")
	if err != nil {
		t.Fatalf("enrollment.Issue: %v", err)
	}
	matched, err := enrollment.Redeem([]*enrollment.Enrollment{enrolledRes.Enrollment}, enrolledRes.Code)
	if err != nil {
		t.Fatalf("enrollment.Redeem: %v", err)
	}
	if err := st.UpsertEnrollment(ctx, matched); err != nil {
		t.Fatalf("UpsertEnrollment (enrolled): %v", err)
	}

	candidates, err := st.ListIssuedEnrollments(ctx)
	if err != nil {
		t.Fatalf("ListIssuedEnrollments: %v", err)
	}
	var sawIssued, sawEnrolled bool
	for _, c := range candidates {
		if c.VesselID == vIssued.ID {
			sawIssued = true
		}
		if c.VesselID == vEnrolled.ID {
			sawEnrolled = true
		}
	}
	if !sawIssued {
		t.Error("ListIssuedEnrollments did not include the still-issued enrollment")
	}
	if sawEnrolled {
		t.Error("ListIssuedEnrollments included an already-enrolled enrollment, want only StateIssued rows")
	}
}
