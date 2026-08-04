// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
)

func TestStore_SaveAndGetVesselIdentity(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if _, err := s.GetVesselIdentity(ctx); err != ErrNotFound {
		t.Errorf("GetVesselIdentity (never saved) error = %v, want ErrNotFound", err)
	}

	if err := s.SaveVesselIdentity(ctx, &VesselIdentity{Name: "MV Testship", IMO: "9074729"}); err != nil {
		t.Fatalf("SaveVesselIdentity: %v", err)
	}

	got, err := s.GetVesselIdentity(ctx)
	if err != nil {
		t.Fatalf("GetVesselIdentity: %v", err)
	}
	if got.Name != "MV Testship" || got.IMO != "9074729" {
		t.Errorf("GetVesselIdentity = %+v, want {Name: MV Testship, IMO: 9074729}", got)
	}
}

func TestStore_SaveVesselIdentity_Supersedes(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.SaveVesselIdentity(ctx, &VesselIdentity{Name: "MV First", IMO: "9074729"}); err != nil {
		t.Fatalf("SaveVesselIdentity (first): %v", err)
	}
	if err := s.SaveVesselIdentity(ctx, &VesselIdentity{Name: "MV Second", IMO: "9319466"}); err != nil {
		t.Fatalf("SaveVesselIdentity (second): %v", err)
	}

	got, err := s.GetVesselIdentity(ctx)
	if err != nil {
		t.Fatalf("GetVesselIdentity: %v", err)
	}
	if got.Name != "MV Second" || got.IMO != "9319466" {
		t.Errorf("GetVesselIdentity = %+v, want {Name: MV Second, IMO: 9319466} (the row should be replaced, not duplicated)", got)
	}
}
