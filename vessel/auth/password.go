// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/captv89/ovl/pkg/authcrypto"
)

// temporaryPasswordAlphabet excludes visually-ambiguous characters
// (0/O, 1/l/I) — this is read off a screen and typed or handed over
// verbally on a ship, not copy-pasted from a password manager.
const temporaryPasswordAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// temporaryPasswordLength comfortably clears authcrypto.MinPasswordLength.
const temporaryPasswordLength = 12

// GenerateTemporaryPassword mints a random password for NewUser/
// ResetPassword to hand out (design handoff A9: "generates a temporary
// one shown once") — the server, not the caller, chooses it, so there is
// never a predictable or fleet-wide-default credential in play.
func GenerateTemporaryPassword() (string, error) {
	out := make([]byte, temporaryPasswordLength)
	max := big.NewInt(int64(len(temporaryPasswordAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("auth: generate temporary password: %w", err)
		}
		out[i] = temporaryPasswordAlphabet[n.Int64()]
	}
	return string(out), nil
}

// hashPassword and VerifyPassword/DummyHash delegate to pkg/authcrypto,
// which office/auth also uses — see that package's doc comment for why
// this logic is shared rather than duplicated per binary.
func hashPassword(password string) (string, error) {
	return authcrypto.HashPassword(password)
}

// VerifyPassword reports whether password matches hash.
func VerifyPassword(password, hash string) (bool, error) {
	return authcrypto.VerifyPassword(password, hash)
}

// DummyHash returns a fixed, valid hash with no known password, for
// timing-safe "unknown user" login responses (see vessel/httpapi's use).
func DummyHash() string {
	return authcrypto.DummyHash()
}
