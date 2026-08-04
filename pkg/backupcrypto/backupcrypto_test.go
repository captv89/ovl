// SPDX-License-Identifier: AGPL-3.0-only

package backupcrypto

import "testing"

func TestGenerateIdentity(t *testing.T) {
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	if id.PublicKey == "" || id.PrivateKey == "" {
		t.Fatalf("GenerateIdentity() = %+v, want both keys populated", id)
	}
	if id.PublicKey == id.PrivateKey {
		t.Error("PublicKey equals PrivateKey")
	}
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	plaintext := []byte(`{"reports":["r1","r2"]}`)

	ciphertext, err := Encrypt(plaintext, id.PublicKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(ciphertext) == 0 {
		t.Fatal("Encrypt() returned empty ciphertext")
	}

	got, err := Decrypt(ciphertext, id.PrivateKey)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Errorf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestDecrypt_WrongIdentityFails(t *testing.T) {
	id1, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	id2, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	ciphertext, err := Encrypt([]byte("secret"), id1.PublicKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := Decrypt(ciphertext, id2.PrivateKey); err == nil {
		t.Fatal("Decrypt with the wrong identity succeeded, want an error")
	}
}

func TestEncrypt_InvalidRecipient(t *testing.T) {
	if _, err := Encrypt([]byte("x"), "not-a-real-recipient"); err == nil {
		t.Fatal("Encrypt with an invalid recipient string succeeded, want an error")
	}
}
