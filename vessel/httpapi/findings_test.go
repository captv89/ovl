// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"testing"
	"time"
)

func TestHandleAcknowledgeFinding_AppendsEventAndEnqueues(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/findings/acknowledge", acknowledgeFindingRequest{
		RuleID:       "field.required",
		Field:        "Period_Id",
		Message:      "Period Id is recommended but empty",
		Acknowledged: true,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("acknowledge: status %d, body %s", rec.Code, rec.Body)
	}

	eventsRec := c.do(http.MethodGet, "/api/reports/"+created.ReportID+"/events", nil)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("GET events: status %d", eventsRec.Code)
	}
	events := decodeBody[[]eventView](t, eventsRec)
	var found *eventView
	for i := range events {
		if events[i].Type == "finding_acknowledged" {
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("no finding_acknowledged event in %+v", events)
	}
	if found.Actor != "master" {
		t.Errorf("Actor = %q, want master", found.Actor)
	}
	if found.Detail["ruleId"] != "field.required" || found.Detail["field"] != "Period_Id" || found.Detail["acknowledged"] != true {
		t.Errorf("Detail = %+v, want ruleId/field/acknowledged preserved", found.Detail)
	}

	st := s.storeOrNil()
	items, err := st.ListOutboxItems(t.Context())
	if err != nil {
		t.Fatalf("ListOutboxItems: %v", err)
	}
	var enqueued bool
	for _, item := range items {
		if item.Kind == "reportAuditEvent" && item.ReportID == created.ReportID {
			enqueued = true
		}
	}
	if !enqueued {
		t.Errorf("no reportAuditEvent outbox item for %s in %+v", created.ReportID, items)
	}
}

func TestHandleAcknowledgeFinding_RequiresRuleID(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/findings/acknowledge", acknowledgeFindingRequest{
		Message:      "missing rule id",
		Acknowledged: true,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAcknowledgeFinding_RejectsOnceSubmitted(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil); rec.Code != http.StatusOK {
		t.Fatalf("check: status %d", rec.Code)
	}
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d", rec.Code)
	}

	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/findings/acknowledge", acknowledgeFindingRequest{
		RuleID:       "field.required",
		Message:      "irrelevant now",
		Acknowledged: true,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}
