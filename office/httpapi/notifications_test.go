// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/pkg/domain"
)

// TestHandleListNotifications_AndMarkRead exercises all three
// notification categories end to end (overdue, remark/chat, sync),
// confirms the combined feed sorts newest-first, and confirms marking
// one notification read persists and comes back Read=true on the next
// fetch while an untouched one stays unread.
func TestHandleListNotifications_AndMarkRead(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	rule, err := compliance.NewCadenceRule(compliance.FleetScope(), compliance.DefaultMinReportIntervalHours, 12)
	if err != nil {
		t.Fatalf("NewCadenceRule: %v", err)
	}
	if err := s.st.SaveCadenceRule(context.Background(), rule); err != nil {
		t.Fatalf("SaveCadenceRule: %v", err)
	}

	overdueVessel := createTestVesselForReports(t, s, 90)
	enrollTestVessel(t, s, overdueVessel.ID)
	chatVessel := createTestVesselForReports(t, s, 91)
	enrollTestVessel(t, s, chatVessel.ID)

	now := time.Now().UTC()
	landTestReport(t, s, overdueVessel.ID, "report-overdue", 1, now.Add(-30*time.Hour), domain.StateSubmitted)
	landTestReport(t, s, chatVessel.ID, "report-chat", 1, now.Add(-1*time.Hour), domain.StateSubmitted)

	msg, err := domain.NewChatMessage("report-chat", "a.chen", "Re-checked the log.", domain.ChatFromVessel)
	if err != nil {
		t.Fatalf("NewChatMessage: %v", err)
	}
	if err := s.st.InsertChatMessage(context.Background(), chatVessel.ID, msg); err != nil {
		t.Fatalf("InsertChatMessage: %v", err)
	}

	rec := c.do(http.MethodGet, "/api/notifications", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/notifications: status %d, body %s", rec.Code, rec.Body)
	}
	list := decodeBody[[]notificationView](t, rec)

	var overdueNotif, chatNotif, syncNotif *notificationView
	for i := range list {
		switch {
		case list[i].ID == "overdue:"+overdueVessel.ID:
			overdueNotif = &list[i]
		case list[i].Category == "remark" && list[i].Link != nil && list[i].Link.VesselID == chatVessel.ID:
			chatNotif = &list[i]
		case list[i].Category == "sync" && list[i].Link != nil && list[i].Link.VesselID == chatVessel.ID:
			syncNotif = &list[i]
		}
	}
	if overdueNotif == nil {
		t.Fatalf("no overdue notification found in %+v", list)
	}
	if overdueNotif.Read {
		t.Error("overdue notification Read = true before marking, want false")
	}
	if chatNotif == nil {
		t.Fatalf("no remark/chat notification found in %+v", list)
	}
	if syncNotif == nil {
		t.Fatalf("no sync notification found in %+v", list)
	}

	// Sorted newest-first.
	for i := 1; i < len(list); i++ {
		if list[i-1].At.Before(list[i].At) {
			t.Errorf("notifications not sorted newest-first: %s (%s) before %s (%s)",
				list[i-1].ID, list[i-1].At, list[i].ID, list[i].At)
		}
	}

	// Mark just the overdue notification read.
	rec = c.do(http.MethodPost, "/api/notifications/mark-read", markNotificationsReadRequest{IDs: []string{overdueNotif.ID}})
	if rec.Code != http.StatusOK {
		t.Fatalf("mark-read: status %d, body %s", rec.Code, rec.Body)
	}

	rec = c.do(http.MethodGet, "/api/notifications", nil)
	list2 := decodeBody[[]notificationView](t, rec)
	var overdueAfter, chatAfter *notificationView
	for i := range list2 {
		if list2[i].ID == overdueNotif.ID {
			overdueAfter = &list2[i]
		}
		if list2[i].ID == chatNotif.ID {
			chatAfter = &list2[i]
		}
	}
	if overdueAfter == nil || !overdueAfter.Read {
		t.Errorf("overdue notification after mark-read = %+v, want Read=true", overdueAfter)
	}
	if chatAfter == nil || chatAfter.Read {
		t.Errorf("chat notification after marking only the overdue one = %+v, want Read=false (untouched)", chatAfter)
	}
}

func TestHandleListNotifications_RequiresAuth(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodGet, "/api/notifications", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
