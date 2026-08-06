// SPDX-License-Identifier: AGPL-3.0-only

// Package authcrypto is the shared password-hashing primitive for both
// ovl-vessel's local accounts (architecture 9.3) and ovl-office's local
// stand-in accounts (architecture 12.2, pending real OIDC integration).
// Extracted from vessel/auth rather than duplicated when office/auth
// needed the identical logic: password hashing is security-sensitive
// code where two copies risk silently drifting apart, unlike the
// generic per-binary infrastructure (e.g. each app's own SPA-serving
// handler) that's deliberately kept separate elsewhere in this repo.
package authcrypto

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"sync"

	"github.com/alexedwards/argon2id"
)

// MinPasswordLength is a correctness floor (reject empty/trivial
// passwords), not a full complexity policy — neither handoff doc
// specifies one, and inventing rules (character classes, etc.) beyond
// preventing an obviously-unsafe password isn't this package's call to
// make.
const MinPasswordLength = 8

// MaxPasswordLength bounds the cost of hashing: argon2id's work scales
// with input size, so an unbounded password length is a cheap DoS lever
// against whatever eventually calls this (any LAN- or internet-reachable
// login endpoint). 256 bytes is far beyond any realistic real password.
const MaxPasswordLength = 256

// hashParams are deliberately stronger than argon2id's own DefaultParams
// (whose doc comment says "development/testing purposes only"): OWASP's
// current Argon2id guidance for interactive login is met by t=3 at 64
// MiB, which a modest machine can still compute in well under a second.
var hashParams = &argon2id.Params{
	Memory:      64 * 1024, // 64 MiB
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// ValidatePassword reports whether password satisfies the length bounds
// above.
func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("authcrypto: password must be at least %d characters", MinPasswordLength)
	}
	if len(password) > MaxPasswordLength {
		return fmt.Errorf("authcrypto: password must be at most %d characters", MaxPasswordLength)
	}
	return nil
}

// HashPassword validates and argon2id-hashes password, returning the
// encoded PHC-format hash string to store.
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := argon2id.CreateHash(password, hashParams)
	if err != nil {
		return "", fmt.Errorf("authcrypto: hash password: %w", err)
	}
	return hash, nil
}

// VerifyPassword reports whether password matches hash, using a
// constant-time comparison (argon2id.ComparePasswordAndHash).
func VerifyPassword(password, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, fmt.Errorf("authcrypto: verify password: %w", err)
	}
	return match, nil
}

var (
	dummyHashOnce sync.Once
	dummyHash     string
)

// DummyHash returns a fixed, valid argon2id hash with no known
// password. A login handler should VerifyPassword against this when a
// username lookup fails, so a "no such user" response costs the same
// wall-clock time as a "wrong password" response — otherwise the
// hashing-vs-not-hashing timing difference lets an attacker enumerate
// valid usernames. Computed once, lazily, on first use (not at package
// init) so importing this package doesn't pay an argon2id hash's cost
// unless a login endpoint actually exists and is used.
func DummyHash() string {
	dummyHashOnce.Do(func() {
		hash, err := argon2id.CreateHash("this-is-not-a-real-account-do-not-use", hashParams)
		if err != nil {
			panic(fmt.Errorf("authcrypto: compute dummy hash: %w", err))
		}
		dummyHash = hash
	})
	return dummyHash
}

// RandomToken returns a cryptographically random, printable token derived
// from numBytes of entropy: base32-encoded (unambiguous uppercase
// alphabet, no padding), grouped into 4-character blocks separated by
// hyphens for readability on a printed sheet and easy manual re-typing.
// Shared by every one-time-secret/bearer-token generator in this project
// (enrollment codes, initial Master passwords, sync credentials) so the
// entropy source and formatting can't silently drift between them.
func RandomToken(numBytes int) (string, error) {
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("authcrypto: read random bytes: %w", err)
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	var grouped strings.Builder
	for i, r := range encoded {
		if i > 0 && i%4 == 0 {
			grouped.WriteByte('-')
		}
		grouped.WriteRune(r)
	}
	return grouped.String(), nil
}
