// SPDX-License-Identifier: AGPL-3.0-only

package apikey

import "testing"

func TestMint(t *testing.T) {
	result, err := Mint("Acme Verifier", "admin", nil)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if result.APIKey.Label != "Acme Verifier" {
		t.Errorf("Label = %q, want %q", result.APIKey.Label, "Acme Verifier")
	}
	if result.APIKey.CreatedBy != "admin" {
		t.Errorf("CreatedBy = %q, want %q", result.APIKey.CreatedBy, "admin")
	}
	if result.APIKey.ID == "" {
		t.Error("ID is empty")
	}
	if result.Token == "" {
		t.Error("Token is empty")
	}
	if result.APIKey.TokenHash == result.Token {
		t.Error("TokenHash stores the plaintext token unchanged")
	}
	if result.APIKey.TokenLookupHash != LookupHash(result.Token) {
		t.Error("TokenLookupHash does not match LookupHash(Token)")
	}
	if result.APIKey.RevokedAt != nil {
		t.Error("RevokedAt is set on a freshly minted key, want nil")
	}
	if result.APIKey.GroupID != nil {
		t.Error("GroupID is set when nil was passed in, want nil (unscoped)")
	}

	match, err := result.APIKey.Verify(result.Token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !match {
		t.Error("Verify(the real token) = false, want true")
	}
}

func TestMint_GroupScoped(t *testing.T) {
	group := "north-sea-fleet"
	result, err := Mint("Scoped Customer", "admin", &group)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if result.APIKey.GroupID == nil || *result.APIKey.GroupID != group {
		t.Errorf("GroupID = %v, want %q", result.APIKey.GroupID, group)
	}
}

func TestMint_EmptyLabel(t *testing.T) {
	if _, err := Mint("", "admin", nil); err == nil {
		t.Fatal("Mint(empty label) = nil error, want an error")
	}
}

func TestMint_EmptyCreatedBy(t *testing.T) {
	if _, err := Mint("Acme Verifier", "", nil); err == nil {
		t.Fatal("Mint(empty created-by) = nil error, want an error")
	}
}

func TestAPIKey_Verify_WrongToken(t *testing.T) {
	result, err := Mint("Acme Verifier", "admin", nil)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	match, err := result.APIKey.Verify("not-the-real-token")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if match {
		t.Error("Verify(wrong token) = true, want false")
	}
}

func TestAPIKey_Revoke(t *testing.T) {
	result, err := Mint("Acme Verifier", "admin", nil)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	token := result.Token
	result.APIKey.Revoke()

	if result.APIKey.RevokedAt == nil {
		t.Fatal("RevokedAt is nil after Revoke")
	}
	match, err := result.APIKey.Verify(token)
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
