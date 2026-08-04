// SPDX-License-Identifier: AGPL-3.0-only

// Package backupcrypto wraps filippo.io/age (X25519 + ChaCha20-Poly1305)
// for architecture 12.5's encrypted restore bundles — office generates a
// bundle "encrypted against the vessel's enrollment key," the vessel
// decrypts it locally. age over NaCl/secretbox: modern, audited, a
// standard on-disk format (not a bespoke envelope this project would
// have to maintain itself), and its recipient/identity strings are
// already plain text, so they need no extra encoding to store or
// transmit as JSON.
//
// The keypair itself is generated on the vessel at enrollment time (see
// vessel/sync.Redeem) — the private key never leaves the vessel; only
// the public recipient string is ever sent to office. This is a fresh,
// DR-specific keypair, not a repurposing of the sync bearer credential
// (that's a symmetric secret with no public half to encrypt against,
// and office never retains its plaintext past minting anyway).
package backupcrypto

import (
	"bytes"
	"fmt"
	"io"

	"filippo.io/age"
)

// Identity is one vessel's DR keypair. PublicKey ("age1...") is what
// office encrypts a restore bundle against; PrivateKey
// ("AGE-SECRET-KEY-1...") is what the vessel decrypts it with. Both are
// age's own plain-text string encodings — safe to store/transmit as-is,
// no additional serialization needed.
type Identity struct {
	PublicKey  string
	PrivateKey string
}

// GenerateIdentity creates a fresh X25519 keypair.
func GenerateIdentity() (*Identity, error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("backupcrypto: generate identity: %w", err)
	}
	return &Identity{PublicKey: id.Recipient().String(), PrivateKey: id.String()}, nil
}

// Encrypt encrypts plaintext to recipientPublicKey (an Identity's own
// PublicKey string) — only the matching PrivateKey can decrypt it.
func Encrypt(plaintext []byte, recipientPublicKey string) ([]byte, error) {
	recipient, err := age.ParseX25519Recipient(recipientPublicKey)
	if err != nil {
		return nil, fmt.Errorf("backupcrypto: parse recipient: %w", err)
	}
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return nil, fmt.Errorf("backupcrypto: begin encrypt: %w", err)
	}
	if _, err := w.Write(plaintext); err != nil {
		return nil, fmt.Errorf("backupcrypto: write plaintext: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("backupcrypto: finish encrypt: %w", err)
	}
	return buf.Bytes(), nil
}

// Decrypt reverses Encrypt using identityPrivateKey (an Identity's own
// PrivateKey string).
func Decrypt(ciphertext []byte, identityPrivateKey string) ([]byte, error) {
	identity, err := age.ParseX25519Identity(identityPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("backupcrypto: parse identity: %w", err)
	}
	r, err := age.Decrypt(bytes.NewReader(ciphertext), identity)
	if err != nil {
		return nil, fmt.Errorf("backupcrypto: decrypt: %w", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("backupcrypto: read plaintext: %w", err)
	}
	return out, nil
}
