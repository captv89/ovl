// SPDX-License-Identifier: AGPL-3.0-only

package synccred

import "testing"

func TestMint(t *testing.T) {
	result, err := Mint("vessel-1")
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if result.Credential.VesselID != "vessel-1" {
		t.Errorf("VesselID = %q, want %q", result.Credential.VesselID, "vessel-1")
	}
	if result.Token == "" {
		t.Error("Token is empty")
	}
	if result.Credential.TokenHash == result.Token {
		t.Error("TokenHash stores the plaintext token unchanged")
	}
	if result.Credential.TokenLookupHash != LookupHash(result.Token) {
		t.Error("TokenLookupHash does not match LookupHash(Token)")
	}
	if result.Credential.RevokedAt != nil {
		t.Error("RevokedAt is set on a freshly minted credential, want nil")
	}

	match, err := result.Credential.Verify(result.Token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !match {
		t.Error("Verify(the real token) = false, want true")
	}
}

func TestMint_EmptyVesselID(t *testing.T) {
	if _, err := Mint(""); err == nil {
		t.Fatal("Mint(empty vessel id) = nil error, want an error")
	}
}

func TestCredential_Verify_WrongToken(t *testing.T) {
	result, err := Mint("vessel-1")
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	match, err := result.Credential.Verify("not-the-real-token")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if match {
		t.Error("Verify(wrong token) = true, want false")
	}
}

func TestCredential_Revoke(t *testing.T) {
	result, err := Mint("vessel-1")
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	token := result.Token
	result.Credential.Revoke()

	if result.Credential.RevokedAt == nil {
		t.Fatal("RevokedAt is nil after Revoke")
	}
	match, err := result.Credential.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if match {
		t.Error("Verify still matches the token after Revoke, want false")
	}
}

func TestLookupHash_Deterministic(t *testing.T) {
	first := LookupHash("some-token")
	second := LookupHash("some-token")
	if first != second {
		t.Error("LookupHash is not deterministic for the same input")
	}
	if first == LookupHash("some-other-token") {
		t.Error("LookupHash produced the same digest for two different inputs")
	}
}
