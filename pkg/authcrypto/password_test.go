// SPDX-License-Identifier: AGPL-3.0-only

package authcrypto

import (
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("HashPassword returned the plaintext password unchanged")
	}

	match, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("VerifyPassword (correct): %v", err)
	}
	if !match {
		t.Error("VerifyPassword(correct password) = false, want true")
	}

	match, err = VerifyPassword("wrong password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword (wrong): %v", err)
	}
	if match {
		t.Error("VerifyPassword(wrong password) = true, want false")
	}
}

func TestHashPassword_TooShort(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("HashPassword with a too-short password: got nil error, want an error")
	}
}

func TestHashPassword_TooLong(t *testing.T) {
	huge := strings.Repeat("a", MaxPasswordLength+1)
	if _, err := HashPassword(huge); err == nil {
		t.Fatal("HashPassword with an over-long password: got nil error, want an error (DoS guard)")
	}
}

func TestRandomToken_IsRandom(t *testing.T) {
	a, err := RandomToken(10)
	if err != nil {
		t.Fatalf("RandomToken: %v", err)
	}
	b, err := RandomToken(10)
	if err != nil {
		t.Fatalf("RandomToken: %v", err)
	}
	if a == b {
		t.Error("RandomToken produced identical output twice in a row")
	}
}

func TestDummyHash(t *testing.T) {
	hash := DummyHash()
	if hash == "" {
		t.Fatal("DummyHash() returned an empty string")
	}
	if hash != DummyHash() {
		t.Error("DummyHash() is not stable across calls")
	}
	match, err := VerifyPassword("anything", hash)
	if err != nil {
		t.Fatalf("VerifyPassword against DummyHash(): %v", err)
	}
	if match {
		t.Error("VerifyPassword matched an arbitrary password against DummyHash()")
	}
}
