// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"testing"

	"github.com/captv89/ovl/pkg/authcrypto"
)

func TestGenerateTemporaryPassword(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		pw, err := GenerateTemporaryPassword()
		if err != nil {
			t.Fatalf("GenerateTemporaryPassword: %v", err)
		}
		if len(pw) != temporaryPasswordLength {
			t.Fatalf("len(pw) = %d, want %d", len(pw), temporaryPasswordLength)
		}
		if err := authcrypto.ValidatePassword(pw); err != nil {
			t.Fatalf("ValidatePassword(%q): %v", pw, err)
		}
		for _, c := range pw {
			if c == '0' || c == 'O' || c == '1' || c == 'l' || c == 'I' {
				t.Errorf("generated password %q contains an excluded ambiguous character %q", pw, c)
			}
		}
		if seen[pw] {
			t.Fatalf("generated the same password twice across 100 draws: %q", pw)
		}
		seen[pw] = true
	}
}
