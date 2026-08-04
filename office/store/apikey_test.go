// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/captv89/ovl/office/apikey"
)

func TestStore_CreateAndGetAPIKeyByLookupHash(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	result, err := apikey.Mint("Acme Verifier", "admin", nil)
	if err != nil {
		t.Fatalf("apikey.Mint: %v", err)
	}
	if err := st.CreateAPIKey(ctx, result.APIKey); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	got, err := st.GetAPIKeyByLookupHash(ctx, apikey.LookupHash(result.Token))
	if err != nil {
		t.Fatalf("GetAPIKeyByLookupHash: %v", err)
	}
	if got.ID != result.APIKey.ID {
		t.Errorf("ID = %q, want %q", got.ID, result.APIKey.ID)
	}
	if got.Label != "Acme Verifier" {
		t.Errorf("Label = %q, want %q", got.Label, "Acme Verifier")
	}
	if got.RevokedAt != nil {
		t.Error("RevokedAt is set on a freshly created key, want nil")
	}

	match, err := got.Verify(result.Token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !match {
		t.Error("Verify(the real token) = false after round-tripping through Postgres, want true")
	}
}

func TestStore_GetAPIKeyByLookupHash_NotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.GetAPIKeyByLookupHash(context.Background(), "no-such-hash"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAPIKeyByLookupHash(unknown hash) error = %v, want ErrNotFound", err)
	}
}

func TestStore_RevokeAPIKey(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	result, err := apikey.Mint("Acme Verifier", "admin", nil)
	if err != nil {
		t.Fatalf("apikey.Mint: %v", err)
	}
	if err := st.CreateAPIKey(ctx, result.APIKey); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	if err := st.RevokeAPIKey(ctx, result.APIKey.ID, time.Now().UTC()); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}

	got, err := st.GetAPIKeyByLookupHash(ctx, apikey.LookupHash(result.Token))
	if err != nil {
		t.Fatalf("GetAPIKeyByLookupHash: %v", err)
	}
	if got.RevokedAt == nil {
		t.Fatal("RevokedAt is nil after RevokeAPIKey")
	}
	if match, _ := got.Verify(result.Token); match {
		t.Error("Verify still matches a revoked key's token, want false")
	}
}

func TestStore_TouchAPIKeyLastUsed(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	result, err := apikey.Mint("Acme Verifier", "admin", nil)
	if err != nil {
		t.Fatalf("apikey.Mint: %v", err)
	}
	if err := st.CreateAPIKey(ctx, result.APIKey); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	now := time.Now().UTC()
	if err := st.TouchAPIKeyLastUsed(ctx, result.APIKey.ID, now); err != nil {
		t.Fatalf("TouchAPIKeyLastUsed: %v", err)
	}

	got, err := st.GetAPIKeyByLookupHash(ctx, apikey.LookupHash(result.Token))
	if err != nil {
		t.Fatalf("GetAPIKeyByLookupHash: %v", err)
	}
	if got.LastUsedAt == nil {
		t.Fatal("LastUsedAt is nil after TouchAPIKeyLastUsed")
	}
}

func TestStore_ListAPIKeys(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	first, err := apikey.Mint("First Customer", "admin", nil)
	if err != nil {
		t.Fatalf("apikey.Mint: %v", err)
	}
	if err := st.CreateAPIKey(ctx, first.APIKey); err != nil {
		t.Fatalf("CreateAPIKey (first): %v", err)
	}
	second, err := apikey.Mint("Second Customer", "admin", nil)
	if err != nil {
		t.Fatalf("apikey.Mint: %v", err)
	}
	if err := st.CreateAPIKey(ctx, second.APIKey); err != nil {
		t.Fatalf("CreateAPIKey (second): %v", err)
	}

	list, err := st.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	var sawFirst, sawSecond bool
	for _, k := range list {
		if k.ID == first.APIKey.ID {
			sawFirst = true
		}
		if k.ID == second.APIKey.ID {
			sawSecond = true
		}
	}
	if !sawFirst || !sawSecond {
		t.Errorf("ListAPIKeys did not include both created keys (sawFirst=%v sawSecond=%v)", sawFirst, sawSecond)
	}
}

func TestStore_GetAPIKeyByID(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	result, err := apikey.Mint("Acme Verifier", "admin", nil)
	if err != nil {
		t.Fatalf("apikey.Mint: %v", err)
	}
	if err := st.CreateAPIKey(ctx, result.APIKey); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	got, err := st.GetAPIKeyByID(ctx, result.APIKey.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID: %v", err)
	}
	if got.Label != "Acme Verifier" {
		t.Errorf("Label = %q, want %q", got.Label, "Acme Verifier")
	}

	if _, err := st.GetAPIKeyByID(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAPIKeyByID(unknown id) error = %v, want ErrNotFound", err)
	}
}

func TestStore_DeleteAPIKey(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	result, err := apikey.Mint("Acme Verifier", "admin", nil)
	if err != nil {
		t.Fatalf("apikey.Mint: %v", err)
	}
	if err := st.CreateAPIKey(ctx, result.APIKey); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if err := st.RecordAPIKeyEvent(ctx, result.APIKey.ID, "created", time.Now().UTC()); err != nil {
		t.Fatalf("RecordAPIKeyEvent: %v", err)
	}

	if err := st.DeleteAPIKey(ctx, result.APIKey.ID); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}

	if _, err := st.GetAPIKeyByID(ctx, result.APIKey.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAPIKeyByID after delete: error = %v, want ErrNotFound", err)
	}
	// The event row must go with it (ON DELETE CASCADE) — otherwise a
	// deleted key's history would silently linger, unreachable through
	// any API since ListAPIKeyEvents is only ever called with a live key
	// id from the admin UI.
	events, err := st.ListAPIKeyEvents(ctx, result.APIKey.ID)
	if err != nil {
		t.Fatalf("ListAPIKeyEvents after delete: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("ListAPIKeyEvents after delete = %d events, want 0 (cascade)", len(events))
	}
}

func TestStore_RecordAndListAPIKeyEvents(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	result, err := apikey.Mint("Acme Verifier", "admin", nil)
	if err != nil {
		t.Fatalf("apikey.Mint: %v", err)
	}
	if err := st.CreateAPIKey(ctx, result.APIKey); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	base := time.Now().UTC()
	if err := st.RecordAPIKeyEvent(ctx, result.APIKey.ID, "created", base); err != nil {
		t.Fatalf("RecordAPIKeyEvent(created): %v", err)
	}
	if err := st.RecordAPIKeyEvent(ctx, result.APIKey.ID, "usedGraphQL", base.Add(time.Minute)); err != nil {
		t.Fatalf("RecordAPIKeyEvent(usedGraphQL): %v", err)
	}

	events, err := st.ListAPIKeyEvents(ctx, result.APIKey.ID)
	if err != nil {
		t.Fatalf("ListAPIKeyEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ListAPIKeyEvents = %d events, want 2", len(events))
	}
	// Newest first.
	if events[0].Kind != "usedGraphQL" || events[1].Kind != "created" {
		t.Errorf("events = [%q, %q], want [usedGraphQL, created] (newest first)", events[0].Kind, events[1].Kind)
	}
}
