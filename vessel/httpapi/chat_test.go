// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

func TestHandleChat_PostAndList(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/chat", postChatRequest{Body: "corrected version pushed"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST chat: status %d, body %s", rec.Code, rec.Body)
	}
	posted := decodeBody[chatMessageView](t, rec)
	if posted.Body != "corrected version pushed" || posted.Direction != string(domain.ChatFromVessel) {
		t.Errorf("posted = %+v, want body='corrected version pushed' direction=vessel", posted)
	}

	rec = c.do(http.MethodGet, "/api/reports/"+created.ReportID+"/chat", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET chat: status %d, body %s", rec.Code, rec.Body)
	}
	list := decodeBody[[]chatMessageView](t, rec)
	if len(list) != 1 || list[0].ID != posted.ID {
		t.Errorf("list = %+v, want exactly the posted message", list)
	}
}

func TestHandleChat_Post_RejectsOverCapBody(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	overCap := strings.Repeat("a", domain.MaxChatBodyBytes+1)
	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/chat", postChatRequest{Body: overCap})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST over-cap chat body: status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleChat_Post_EnqueuesOutboxItem(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/chat", postChatRequest{Body: "hello office"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST chat: status %d, body %s", rec.Code, rec.Body)
	}

	items, err := s.storeOrNil().ListOutboxItems(t.Context())
	if err != nil {
		t.Fatalf("ListOutboxItems: %v", err)
	}
	found := false
	for _, item := range items {
		if item.Kind == "chatMessage" && item.ReportID == created.ReportID {
			found = true
		}
	}
	if !found {
		t.Errorf("outbox items = %+v, want a chatMessage item for report %s", items, created.ReportID)
	}
}

func TestHandleChat_NotFound(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodGet, "/api/reports/no-such-report/chat", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET chat for missing report: status %d, want %d", rec.Code, http.StatusNotFound)
	}
}
