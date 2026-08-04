// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"
)

func TestStore_SaveAndGetSyncCredential(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if _, err := s.GetSyncCredential(ctx); err != ErrNotFound {
		t.Errorf("GetSyncCredential (never saved) error = %v, want ErrNotFound", err)
	}

	issuedAt := time.Now().UTC().Truncate(time.Second)
	if err := s.SaveSyncCredential(ctx, &SyncCredential{Token: "first-token", IssuedAt: issuedAt}); err != nil {
		t.Fatalf("SaveSyncCredential: %v", err)
	}

	got, err := s.GetSyncCredential(ctx)
	if err != nil {
		t.Fatalf("GetSyncCredential: %v", err)
	}
	if got.Token != "first-token" {
		t.Errorf("Token = %q, want %q", got.Token, "first-token")
	}
	if !got.IssuedAt.Equal(issuedAt) {
		t.Errorf("IssuedAt = %v, want %v", got.IssuedAt, issuedAt)
	}
}

func TestStore_SaveSyncCredential_Supersedes(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.SaveSyncCredential(ctx, &SyncCredential{Token: "first-token", IssuedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("SaveSyncCredential (first): %v", err)
	}
	secondIssuedAt := time.Now().UTC().Truncate(time.Second)
	if err := s.SaveSyncCredential(ctx, &SyncCredential{Token: "second-token", IssuedAt: secondIssuedAt}); err != nil {
		t.Fatalf("SaveSyncCredential (second): %v", err)
	}

	got, err := s.GetSyncCredential(ctx)
	if err != nil {
		t.Fatalf("GetSyncCredential: %v", err)
	}
	if got.Token != "second-token" {
		t.Errorf("Token = %q, want %q (the row should be replaced, not duplicated)", got.Token, "second-token")
	}
	if !got.IssuedAt.Equal(secondIssuedAt) {
		t.Errorf("IssuedAt = %v, want %v", got.IssuedAt, secondIssuedAt)
	}
}
