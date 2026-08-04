// SPDX-License-Identifier: AGPL-3.0-only

package enrollment

import (
	"errors"
	"testing"
)

func TestIssue(t *testing.T) {
	res, err := Issue("vessel-1", "master")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if res.Enrollment.VesselID != "vessel-1" {
		t.Errorf("VesselID = %q, want %q", res.Enrollment.VesselID, "vessel-1")
	}
	if res.Enrollment.State != StateIssued {
		t.Errorf("State = %q, want %q", res.Enrollment.State, StateIssued)
	}
	if res.Code == "" {
		t.Error("Code is empty")
	}
	if res.InitialMasterPassword == "" {
		t.Error("InitialMasterPassword is empty")
	}
	if res.Enrollment.CodeHash == res.Code {
		t.Error("CodeHash stores the plaintext code unchanged")
	}
	if res.Enrollment.InitialMasterPasswordHash == res.InitialMasterPassword {
		t.Error("InitialMasterPasswordHash stores the plaintext password unchanged")
	}
	if res.Enrollment.InitialMasterUsername != "master" {
		t.Errorf("InitialMasterUsername = %q, want %q", res.Enrollment.InitialMasterUsername, "master")
	}
	if res.Enrollment.RevokedAt != nil {
		t.Error("RevokedAt is set on a freshly issued enrollment, want nil")
	}

	match, err := res.Enrollment.VerifyCode(res.Code)
	if err != nil {
		t.Fatalf("VerifyCode: %v", err)
	}
	if !match {
		t.Error("VerifyCode(the real code) = false, want true")
	}
}

func TestIssue_EmptyVesselID(t *testing.T) {
	if _, err := Issue("", "master"); err == nil {
		t.Fatal("Issue(empty vessel id) = nil error, want an error")
	}
}

func TestIssue_DefaultsUsername(t *testing.T) {
	res, err := Issue("vessel-1", "")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if res.Enrollment.InitialMasterUsername != defaultMasterUsername {
		t.Errorf("InitialMasterUsername = %q, want default %q", res.Enrollment.InitialMasterUsername, defaultMasterUsername)
	}
}

func TestEnrollment_VerifyCode_WrongCode(t *testing.T) {
	res, err := Issue("vessel-1", "master")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	match, err := res.Enrollment.VerifyCode("not-the-real-code")
	if err != nil {
		t.Fatalf("VerifyCode: %v", err)
	}
	if match {
		t.Error("VerifyCode(wrong code) = true, want false")
	}
}

func TestEnrollment_Revoke(t *testing.T) {
	res, err := Issue("vessel-1", "master")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	code := res.Code
	res.Enrollment.Revoke()

	if res.Enrollment.State != StateRevoked {
		t.Errorf("State = %q, want %q", res.Enrollment.State, StateRevoked)
	}
	if res.Enrollment.RevokedAt == nil {
		t.Fatal("RevokedAt is nil after Revoke")
	}
	if res.Enrollment.CodeHash != "" {
		t.Error("CodeHash is not cleared after Revoke")
	}
	match, err := res.Enrollment.VerifyCode(code)
	if err != nil {
		t.Fatalf("VerifyCode: %v", err)
	}
	if match {
		t.Error("VerifyCode still matches the old code after Revoke, want false")
	}
}

func TestEnrollment_Reissue(t *testing.T) {
	res, err := Issue("vessel-1", "master")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	oldCode := res.Code
	res.Enrollment.Revoke()

	res2, err := res.Enrollment.Reissue("chief-officer")
	if err != nil {
		t.Fatalf("Reissue: %v", err)
	}
	if res2.Enrollment.State != StateIssued {
		t.Errorf("State after Reissue = %q, want %q", res2.Enrollment.State, StateIssued)
	}
	if res2.Enrollment.RevokedAt != nil {
		t.Error("RevokedAt is still set after Reissue, want nil")
	}
	if res2.Enrollment.InitialMasterUsername != "chief-officer" {
		t.Errorf("InitialMasterUsername = %q, want %q", res2.Enrollment.InitialMasterUsername, "chief-officer")
	}
	if res2.Code == oldCode {
		t.Error("Reissue produced the same code as before, want a fresh one")
	}

	// The old (revoked, then superseded) code must not verify.
	if match, _ := res2.Enrollment.VerifyCode(oldCode); match {
		t.Error("VerifyCode(old code) = true after Reissue, want false")
	}
	// The new code must verify.
	match, err := res2.Enrollment.VerifyCode(res2.Code)
	if err != nil {
		t.Fatalf("VerifyCode: %v", err)
	}
	if !match {
		t.Error("VerifyCode(new code) = false after Reissue, want true")
	}
}

func TestRedeem(t *testing.T) {
	resA, err := Issue("vessel-a", "master")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	resB, err := Issue("vessel-b", "master")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	candidates := []*Enrollment{resA.Enrollment, resB.Enrollment}

	matched, err := Redeem(candidates, resB.Code)
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	if matched.VesselID != "vessel-b" {
		t.Errorf("Redeem matched vessel %q, want %q", matched.VesselID, "vessel-b")
	}
	if matched.State != StateEnrolled {
		t.Errorf("State after Redeem = %q, want %q", matched.State, StateEnrolled)
	}
	if matched.CodeHash != "" {
		t.Error("CodeHash is not cleared after Redeem")
	}
	if resA.Enrollment.State != StateIssued {
		t.Errorf("unrelated candidate's state changed to %q after Redeem, want unchanged %q", resA.Enrollment.State, StateIssued)
	}

	// The same code cannot be redeemed twice.
	if _, err := Redeem(candidates, resB.Code); !errors.Is(err, ErrCodeNotFound) {
		t.Errorf("second Redeem with the same code: err = %v, want ErrCodeNotFound", err)
	}
}

func TestRedeem_UnknownCode(t *testing.T) {
	res, err := Issue("vessel-a", "master")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	candidates := []*Enrollment{res.Enrollment}

	if _, err := Redeem(candidates, "not-a-real-code"); !errors.Is(err, ErrCodeNotFound) {
		t.Errorf("Redeem(unknown code): err = %v, want ErrCodeNotFound", err)
	}
}

func TestRedeem_RevokedCandidateIsSkipped(t *testing.T) {
	res, err := Issue("vessel-a", "master")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	code := res.Code
	res.Enrollment.Revoke()
	candidates := []*Enrollment{res.Enrollment}

	if _, err := Redeem(candidates, code); !errors.Is(err, ErrCodeNotFound) {
		t.Errorf("Redeem(code of a revoked enrollment): err = %v, want ErrCodeNotFound", err)
	}
}
