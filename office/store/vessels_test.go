// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/captv89/ovl/office/vessels"
)

// distinctIMO returns a checksum-valid IMO number distinct per caller
// (id in [0, 99]), so parallel/repeated runs against the same shared
// Postgres don't collide on the vessels.imo UNIQUE constraint. Built by
// varying two digits of a known-valid IMO and re-deriving the check
// digit, rather than hardcoding a small fixed pool of real-looking
// numbers.
func distinctIMO(t *testing.T, id int) string {
	t.Helper()
	digits := [6]int{9, 0, 7, (id / 10) % 10, id % 10, 2}
	sum := 0
	for i, weight := 0, 7; i < 6; i, weight = i+1, weight-1 {
		sum += digits[i] * weight
	}
	check := sum % 10
	imo := ""
	for _, d := range digits {
		imo += string(rune('0' + d))
	}
	imo += string(rune('0' + check))
	if err := vessels.ValidateIMO(imo); err != nil {
		t.Fatalf("distinctIMO(%d) produced an invalid IMO %q: %v", id, imo, err)
	}
	return imo
}

func newTestVessel(t *testing.T, first int, groups []string) *vessels.Vessel {
	t.Helper()
	v, err := vessels.NewVessel(distinctIMO(t, first), t.Name(), "Bulk Carrier", groups)
	if err != nil {
		t.Fatalf("vessels.NewVessel: %v", err)
	}
	return v
}

func deleteTestVessel(t *testing.T, st *Store, id string) {
	t.Helper()
	if _, err := st.db.ExecContext(context.Background(), `DELETE FROM vessels WHERE id = $1`, id); err != nil {
		t.Errorf("cleanup: delete test vessel %s: %v", id, err)
	}
}

func TestStore_CreateAndGetVessel(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	v := newTestVessel(t, 1, []string{"Fleet A", "Pacific"})
	if err := st.CreateVessel(ctx, v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}
	t.Cleanup(func() { deleteTestVessel(t, st, v.ID) })

	got, err := st.GetVessel(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetVessel: %v", err)
	}
	if got.IMO != v.IMO {
		t.Errorf("IMO = %q, want %q", got.IMO, v.IMO)
	}
	if got.Name != v.Name || got.Type != v.Type {
		t.Errorf("Name/Type = %q/%q, want %q/%q", got.Name, got.Type, v.Name, v.Type)
	}
	if len(got.Groups) != 2 {
		t.Errorf("Groups = %v, want 2 entries", got.Groups)
	}

	byIMO, err := st.GetVesselByIMO(ctx, v.IMO)
	if err != nil {
		t.Fatalf("GetVesselByIMO: %v", err)
	}
	if byIMO.ID != v.ID {
		t.Errorf("GetVesselByIMO returned ID %q, want %q", byIMO.ID, v.ID)
	}
}

func TestStore_GetVessel_NotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.GetVessel(context.Background(), "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetVessel(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_UpdateVessel(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	v := newTestVessel(t, 2, []string{"Fleet A"})
	if err := st.CreateVessel(ctx, v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}
	t.Cleanup(func() { deleteTestVessel(t, st, v.ID) })

	if err := v.UpdateProfile("MV Renamed", "Container", []string{"Fleet B"}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if err := st.UpdateVessel(ctx, v); err != nil {
		t.Fatalf("UpdateVessel: %v", err)
	}

	got, err := st.GetVessel(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetVessel: %v", err)
	}
	if got.Name != "MV Renamed" || got.Type != "Container" {
		t.Errorf("Name/Type = %q/%q, want MV Renamed/Container", got.Name, got.Type)
	}
	if len(got.Groups) != 1 || got.Groups[0] != "Fleet B" {
		t.Errorf("Groups = %v, want [Fleet B]", got.Groups)
	}
	if got.IMO != v.IMO {
		t.Errorf("IMO changed to %q, want unchanged %q", got.IMO, v.IMO)
	}
}

func TestStore_UpdateVessel_NotFound(t *testing.T) {
	st := openTestStore(t)
	v := newTestVessel(t, 3, nil)
	v.ID = "00000000-0000-0000-0000-000000000000"
	if err := st.UpdateVessel(context.Background(), v); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateVessel(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_ListVessels(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	v1 := newTestVessel(t, 4, nil)
	v1.Name += "-1"
	v2 := newTestVessel(t, 5, nil)
	v2.Name += "-2"
	for _, v := range []*vessels.Vessel{v1, v2} {
		if err := st.CreateVessel(ctx, v); err != nil {
			t.Fatalf("CreateVessel(%s): %v", v.Name, err)
		}
		t.Cleanup(func(id string) func() { return func() { deleteTestVessel(t, st, id) } }(v.ID))
	}

	list, err := st.ListVessels(ctx)
	if err != nil {
		t.Fatalf("ListVessels: %v", err)
	}
	var foundV1, foundV2 bool
	for _, v := range list {
		if v.ID == v1.ID {
			foundV1 = true
		}
		if v.ID == v2.ID {
			foundV2 = true
		}
	}
	if !foundV1 || !foundV2 {
		t.Errorf("ListVessels() missing created vessels: foundV1=%v foundV2=%v", foundV1, foundV2)
	}
}

func TestStore_ListVesselsByGroup(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	inGroup := newTestVessel(t, 6, []string{"Fleet A", "Pacific"})
	notInGroup := newTestVessel(t, 7, []string{"Fleet B"})
	for _, v := range []*vessels.Vessel{inGroup, notInGroup} {
		if err := st.CreateVessel(ctx, v); err != nil {
			t.Fatalf("CreateVessel(%s): %v", v.Name, err)
		}
		t.Cleanup(func(id string) func() { return func() { deleteTestVessel(t, st, id) } }(v.ID))
	}

	got, err := st.ListVesselsByGroup(ctx, "Fleet A")
	if err != nil {
		t.Fatalf("ListVesselsByGroup: %v", err)
	}
	var foundIn, foundNotIn bool
	for _, v := range got {
		if v.ID == inGroup.ID {
			foundIn = true
		}
		if v.ID == notInGroup.ID {
			foundNotIn = true
		}
	}
	if !foundIn {
		t.Error("ListVesselsByGroup(\"Fleet A\") missing the vessel tagged with that group")
	}
	if foundNotIn {
		t.Error("ListVesselsByGroup(\"Fleet A\") returned a vessel not tagged with that group")
	}
}
