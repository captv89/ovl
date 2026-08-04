// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/schema"
)

func TestHandleCreateRemarkSet_TransitionsReportToRemarked(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	reviewer := createTestUser(t, s, auth.Roles{auth.RoleReviewer}, "correct horse battery staple")
	loginAs(t, c, reviewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 68)
	landTestReport(t, s, v.ID, "report-remark-1", 1, time.Now().UTC(), domain.StateSubmitted)

	rec := c.do(http.MethodPost, "/api/reports/"+v.ID+"/report-remark-1/remarks", createRemarkSetRequest{
		Remarks: []remarkFieldInput{
			{FieldName: "Cargo_Mt", Body: "please double-check this figure"},
			{FieldName: "HFO_ROB", Body: "looks off vs. BDN"},
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST remarks: status %d, body %s", rec.Code, rec.Body)
	}
	created := decodeBody[[]remarkView](t, rec)
	if len(created) != 2 {
		t.Fatalf("len(created) = %d, want 2", len(created))
	}
	if created[0].Author != reviewer.Username {
		t.Errorf("Author = %q, want %q", created[0].Author, reviewer.Username)
	}

	rec = c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-remark-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET report detail: status %d, body %s", rec.Code, rec.Body)
	}
	detail := decodeBody[reportDetailView](t, rec)
	if detail.Latest.State != domain.StateRemarked {
		t.Errorf("report state = %q, want %q", detail.Latest.State, domain.StateRemarked)
	}

	rec = c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-remark-1/events", nil)
	events := decodeBody[[]eventView](t, rec)
	found := false
	for _, e := range events {
		if e.Type == domain.EventRemarked {
			found = true
		}
	}
	if !found {
		t.Errorf("events = %+v, want a remarked event", events)
	}
}

func TestHandleCreateRemarkSet_PostsLinkingChatMessage(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	reviewer := createTestUser(t, s, auth.Roles{auth.RoleReviewer}, "correct horse battery staple")
	loginAs(t, c, reviewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 70)
	landTestReport(t, s, v.ID, "report-remark-chat-1", 1, time.Now().UTC(), domain.StateSubmitted)

	rec := c.do(http.MethodPost, "/api/reports/"+v.ID+"/report-remark-chat-1/remarks", createRemarkSetRequest{
		Remarks: []remarkFieldInput{{FieldName: "Cargo_Mt", Body: "please double-check this figure"}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST remarks: status %d, body %s", rec.Code, rec.Body)
	}

	rec = c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-remark-chat-1/chat", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET chat: status %d, body %s", rec.Code, rec.Body)
	}
	messages := decodeBody[[]chatMessageView](t, rec)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	if messages[0].Direction != string(domain.ChatFromOffice) {
		t.Errorf("Direction = %q, want %q", messages[0].Direction, domain.ChatFromOffice)
	}
	if messages[0].Sender != reviewer.Username {
		t.Errorf("Sender = %q, want %q", messages[0].Sender, reviewer.Username)
	}
	// 18.07.26 manual-test item 13: the chat summary must show the
	// schema's real field label, not the raw "Cargo_Mt" key — computed
	// via fieldLabels (the same helper the production code path uses)
	// rather than hardcoded, since this shared test Postgres's published
	// "log-abstract" version (and therefore its exact label text) isn't
	// this test's own fixture to pin down.
	label := fieldLabels(context.Background(), s.st, "log-abstract", []string{"Cargo_Mt"})[0]
	wantBody := fmt.Sprintf("Flagged: %s\nplease double-check this figure", label)
	if messages[0].Body != wantBody {
		t.Errorf("Body = %q, want %q", messages[0].Body, wantBody)
	}
}

func TestHandleCreateRemarkSet_ChatSummaryTruncatesFieldList(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	reviewer := createTestUser(t, s, auth.Roles{auth.RoleReviewer}, "correct horse battery staple")
	loginAs(t, c, reviewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 71)
	landTestReport(t, s, v.ID, "report-remark-chat-2", 1, time.Now().UTC(), domain.StateSubmitted)

	c.do(http.MethodPost, "/api/reports/"+v.ID+"/report-remark-chat-2/remarks", createRemarkSetRequest{
		Remarks: []remarkFieldInput{
			{FieldName: "Cargo_Mt", Body: "a"},
			{FieldName: "HFO_ROB", Body: "b"},
			{FieldName: "Draft_Fwd", Body: "c"},
			{FieldName: "Draft_Aft", Body: "d"},
		},
	})

	rec := c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-remark-chat-2/chat", nil)
	messages := decodeBody[[]chatMessageView](t, rec)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	labels := fieldLabels(context.Background(), s.st, "log-abstract", []string{"Cargo_Mt", "HFO_ROB", "Draft_Fwd"})
	wantBody := fmt.Sprintf("Flagged: %s, %s, %s (+1 more)", labels[0], labels[1], labels[2])
	if messages[0].Body != wantBody {
		t.Errorf("Body = %q, want %q", messages[0].Body, wantBody)
	}
}

// TestHandleCreateRemarkSet_ChatSummaryUsesFieldLabelsNotRawKeys is the
// hermetic proof for 18.07.26 manual-test item 13, independent of
// whatever "log-abstract" happens to look like in the shared test
// Postgres (unlike the two tests above): a synthetic schema with a field
// whose label is deliberately different from its raw key, published just
// for this test, so the expected chat text is a literal, not derived
// from the same fieldLabels call the production code path uses.
func TestHandleCreateRemarkSet_ChatSummaryUsesFieldLabelsNotRawKeys(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	reviewer := createTestUser(t, s, auth.Roles{auth.RoleReviewer}, "correct horse battery staple")
	loginAs(t, c, reviewer, "correct horse battery staple")

	schemaName := testFieldPolicySchemaName(t)
	content := buildSchemaJSON(t, schemaName, "1",
		schema.Field{Name: "raw_key_1", Label: "Human Friendly Label", Type: schema.FieldTypeDecimal, Relevance: "test", Section: "test", AppliesToEvents: []string{"*"}},
	)
	publishTestSchemaVersion(t, s, schemaName, "1", "projectCurated", content)

	v := createTestVesselForReports(t, s, 72)
	r := &domain.Report{
		ReportID: "report-remark-label-1", VersionNo: 1, SchemaName: schemaName, EventType: "Departure",
		EventTime: time.Now().UTC(), Fields: map[string]any{"raw_key_1": 1.0}, State: domain.StateSubmitted,
	}
	if err := s.st.UpsertReportVersion(context.Background(), v.ID, r, "1", time.Now().UTC()); err != nil {
		t.Fatalf("UpsertReportVersion: %v", err)
	}

	rec := c.do(http.MethodPost, "/api/reports/"+v.ID+"/report-remark-label-1/remarks", createRemarkSetRequest{
		Remarks: []remarkFieldInput{{FieldName: "raw_key_1", Body: "check this"}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST remarks: status %d, body %s", rec.Code, rec.Body)
	}

	rec = c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-remark-label-1/chat", nil)
	messages := decodeBody[[]chatMessageView](t, rec)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	wantBody := "Flagged: Human Friendly Label\ncheck this"
	if messages[0].Body != wantBody {
		t.Errorf("Body = %q, want %q (raw key %q must not appear)", messages[0].Body, wantBody, "raw_key_1")
	}
}

func TestHandleCreateRemarkSet_RequiresReviewer(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 69)
	landTestReport(t, s, v.ID, "report-remark-2", 1, time.Now().UTC(), domain.StateSubmitted)

	rec := c.do(http.MethodPost, "/api/reports/"+v.ID+"/report-remark-2/remarks", createRemarkSetRequest{
		Remarks: []remarkFieldInput{{FieldName: "Cargo_Mt", Body: "check"}},
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("POST remarks as viewer: status %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleListRemarks(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	reviewer := createTestUser(t, s, auth.Roles{auth.RoleReviewer}, "correct horse battery staple")
	loginAs(t, c, reviewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 73)
	landTestReport(t, s, v.ID, "report-remark-3", 1, time.Now().UTC(), domain.StateSubmitted)

	c.do(http.MethodPost, "/api/reports/"+v.ID+"/report-remark-3/remarks", createRemarkSetRequest{
		Remarks: []remarkFieldInput{{FieldName: "Cargo_Mt", Body: "check"}},
	})

	rec := c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-remark-3/remarks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET remarks: status %d, body %s", rec.Code, rec.Body)
	}
	remarks := decodeBody[[]remarkView](t, rec)
	if len(remarks) != 1 || remarks[0].FieldName != "Cargo_Mt" {
		t.Errorf("remarks = %+v, want one Cargo_Mt remark", remarks)
	}
}

func TestHandleSetRemarkResolved(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	reviewer := createTestUser(t, s, auth.Roles{auth.RoleReviewer}, "correct horse battery staple")
	loginAs(t, c, reviewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 74)
	landTestReport(t, s, v.ID, "report-remark-4", 1, time.Now().UTC(), domain.StateSubmitted)

	rec := c.do(http.MethodPost, "/api/reports/"+v.ID+"/report-remark-4/remarks", createRemarkSetRequest{
		Remarks: []remarkFieldInput{{FieldName: "Cargo_Mt", Body: "check"}},
	})
	created := decodeBody[[]remarkView](t, rec)

	rec = c.do(http.MethodPatch, "/api/remarks/"+created[0].ID, setRemarkResolvedRequest{Resolved: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH remark: status %d, body %s", rec.Code, rec.Body)
	}

	rec = c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-remark-4/remarks", nil)
	remarks := decodeBody[[]remarkView](t, rec)
	if len(remarks) != 1 || !remarks[0].Resolved {
		t.Errorf("remarks = %+v, want Resolved=true", remarks)
	}
}

func TestHandleSetRemarkResolved_RequiresReviewer(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	rec := c.do(http.MethodPatch, "/api/remarks/some-id", setRemarkResolvedRequest{Resolved: true})
	if rec.Code != http.StatusForbidden {
		t.Errorf("PATCH remark as viewer: status %d, want %d", rec.Code, http.StatusForbidden)
	}
}
