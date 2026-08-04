// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"github.com/captv89/ovl/office/auth"
)

func createTestVesselWithGroups(t *testing.T, c *testClient, imo, name string, groups []string) vesselView {
	t.Helper()
	rec := c.do(http.MethodPost, "/api/vessels", createVesselRequest{IMO: imo, Name: name, Type: "Bulk Carrier", Groups: groups})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/vessels: status %d, body %s", rec.Code, rec.Body)
	}
	v := decodeBody[vesselView](t, rec)
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM vessels WHERE id = $1`, v.ID)
	})
	return v
}

// TestHandleRenameVesselGroup_AcrossVessels confirms design handoff
// B10's group rename touches every vessel carrying the tag (and only
// those), preserving each vessel's other tags.
func TestHandleRenameVesselGroup_AcrossVessels(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	inGroup := createTestVesselWithGroups(t, c, "9074729", "MV In Group", []string{"O9-Old", "Kept"})
	notInGroup := createTestVesselWithGroups(t, c, "9319466", "MV Not In Group", []string{"Kept"})

	rec := c.do(http.MethodPost, "/api/vessel-groups/rename", renameVesselGroupRequest{From: "O9-Old", To: "O9-New"})
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
	}
	resp := decodeBody[vesselGroupMutationResponse](t, rec)
	if resp.VesselsUpdated != 1 {
		t.Errorf("VesselsUpdated = %d, want 1", resp.VesselsUpdated)
	}

	renamed := decodeBody[vesselDetailView](t, c.do(http.MethodGet, "/api/vessels/"+inGroup.ID, nil))
	wantGroups := map[string]bool{"O9-New": true, "Kept": true}
	if len(renamed.Vessel.Groups) != len(wantGroups) {
		t.Fatalf("renamed vessel Groups = %+v, want %+v", renamed.Vessel.Groups, wantGroups)
	}
	for _, g := range renamed.Vessel.Groups {
		if !wantGroups[g] {
			t.Errorf("renamed vessel Groups = %+v, unexpected tag %q", renamed.Vessel.Groups, g)
		}
	}

	untouched := decodeBody[vesselDetailView](t, c.do(http.MethodGet, "/api/vessels/"+notInGroup.ID, nil))
	if len(untouched.Vessel.Groups) != 1 || untouched.Vessel.Groups[0] != "Kept" {
		t.Errorf("vessel not in the renamed group Groups = %+v, want unchanged [Kept]", untouched.Vessel.Groups)
	}
}

// TestHandleDeleteVesselGroup_RemovesTagOnly confirms delete removes
// just the one tag, not the vessel's other tags.
func TestHandleDeleteVesselGroup_RemovesTagOnly(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	v := createTestVesselWithGroups(t, c, "9074729", "MV Delete Group", []string{"O9-ToDelete", "Kept"})

	rec := c.do(http.MethodPost, "/api/vessel-groups/delete", deleteVesselGroupRequest{Group: "O9-ToDelete"})
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
	}
	if got := decodeBody[vesselGroupMutationResponse](t, rec).VesselsUpdated; got != 1 {
		t.Errorf("VesselsUpdated = %d, want 1", got)
	}

	after := decodeBody[vesselDetailView](t, c.do(http.MethodGet, "/api/vessels/"+v.ID, nil))
	if len(after.Vessel.Groups) != 1 || after.Vessel.Groups[0] != "Kept" {
		t.Errorf("Groups after delete = %+v, want [Kept]", after.Vessel.Groups)
	}
}

func TestHandleRenameVesselGroup_AdminOnly(t *testing.T) {
	s := newTestServer(t)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	c := newTestClient(t, s)
	loginAs(t, c, viewer, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/vessel-groups/rename", renameVesselGroupRequest{From: "A", To: "B"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("rename as viewer: status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
