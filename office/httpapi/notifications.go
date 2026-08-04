// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/enrollment"
)

// notificationLookbackWindow bounds how far back the chat/sync
// projections reach — a notification panel showing every message ever
// sent isn't useful; "recent" is a display concept the wireframe itself
// doesn't pin a number to, so this is a judgment call, not spec text.
const notificationLookbackWindow = 14 * 24 * time.Hour

// notificationCap bounds the combined, sorted result — same reasoning:
// the panel is a recency feed, not a full history browser (design
// handoff B1·N's own "View all notifications" implies a separate full
// history screen for that, out of scope this pass).
const notificationCap = 50

type notificationLinkView struct {
	Section  string `json:"section"` // "vessels" | "reports"
	VesselID string `json:"vesselId,omitempty"`
	ReportID string `json:"reportId,omitempty"`
}

// notificationView is design handoff B1·N's notification row. There is
// no notifications table — see office/store/notifications.go's own doc
// comment — so ID is a deterministic string this handler derives per
// source row, stable across requests so read-state (notification_
// read_state) can track it.
type notificationView struct {
	ID       string                `json:"id"`
	Category string                `json:"category"` // "overdue" | "remark" | "sync"
	Title    string                `json:"title"`
	Message  string                `json:"message"`
	At       time.Time             `json:"at"`
	Read     bool                  `json:"read"`
	Link     *notificationLinkView `json:"link,omitempty"`
}

// handleListNotifications serves design handoff B1·N: a read-only
// projection over overdue vessels (Phase O3's own overdue calculation,
// reused as-is), recent vessel-authored chat messages, and recent
// report-landing activity. Viewable by any authenticated office user —
// each user's own read-state is private to them
// (notification_read_state is keyed by user_id).
func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}

	vesselList, err := s.st.ListVessels(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	lastReports, rules, ok := s.loadOverdueInputs(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()

	// Non-nil for the same reason dashboard.go's overdue slice is — an
	// empty notification feed is the normal steady state, not an edge
	// case, and should marshal to `[]`, not `null`.
	notifications := []notificationView{}
	for _, v := range vesselList {
		_, state, ok := s.enrollmentStateOf(w, r, v.ID)
		if !ok {
			return
		}
		if state != string(enrollment.StateEnrolled) {
			continue
		}
		var lastReportAt *time.Time
		if t, ok := lastReports[v.ID]; ok {
			lastReportAt = &t
		}
		last, overdueHours := overdueStatusFor(v, lastReportAt, rules, now)
		if overdueHours == nil {
			continue
		}
		notifications = append(notifications, notificationView{
			ID:       "overdue:" + v.ID,
			Category: "overdue",
			Title:    fmt.Sprintf("%s is overdue", v.Name),
			Message:  fmt.Sprintf("Last report %s · overdue %.0fh", last.Format("02 Jan 15:04 UTC"), *overdueHours),
			At:       *last,
			Link:     &notificationLinkView{Section: "vessels", VesselID: v.ID},
		})
	}

	since := now.Add(-notificationLookbackWindow)
	chatRows, err := s.st.ListRecentVesselChatMessages(r.Context(), since)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, c := range chatRows {
		body := c.Body
		if len(body) > 140 {
			body = body[:140] + "…"
		}
		eventLabel := c.EventType
		if eventLabel == "" {
			eventLabel = "report"
		}
		notifications = append(notifications, notificationView{
			ID:       "chat:" + c.MessageID,
			Category: "remark",
			Title:    fmt.Sprintf("Reply from %s", c.VesselName),
			Message:  fmt.Sprintf("%s on %s · %s", c.Sender, eventLabel, body),
			At:       c.SentAt,
			Link:     &notificationLinkView{Section: "reports", VesselID: c.VesselID, ReportID: c.ReportID},
		})
	}

	syncRows, err := s.st.ListRecentSyncActivity(r.Context(), since)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, sy := range syncRows {
		plural := "s"
		if sy.Count == 1 {
			plural = ""
		}
		notifications = append(notifications, notificationView{
			ID:       fmt.Sprintf("sync:%s:%s", sy.VesselID, sy.Day),
			Category: "sync",
			Title:    fmt.Sprintf("%d report%s synced · %s", sy.Count, plural, sy.VesselName),
			Message:  sy.Day,
			At:       sy.LastReceivedAt,
			Link:     &notificationLinkView{Section: "reports", VesselID: sy.VesselID},
		})
	}

	sort.Slice(notifications, func(i, j int) bool { return notifications[i].At.After(notifications[j].At) })
	if len(notifications) > notificationCap {
		notifications = notifications[:notificationCap]
	}

	readIDs, err := s.st.ListReadNotificationIDs(r.Context(), user.ID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range notifications {
		notifications[i].Read = readIDs[notifications[i].ID]
	}

	httpjson.WriteJSON(w, http.StatusOK, notifications)
}

type markNotificationsReadRequest struct {
	IDs []string `json:"ids"`
}

// handleMarkNotificationsRead marks the given notification ids read for
// the caller. The frontend's "Mark all read" sends every currently-
// unread id from its last GET /api/notifications response rather than
// this endpoint taking an "all" flag — the notification list is a live
// projection with no server-side concept of "all ids that currently
// exist" independent of computing it, so there is nothing this endpoint
// could resolve "all" against that the client doesn't already have.
func (s *Server) handleMarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	var req markNotificationsReadRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.st.MarkNotificationsRead(r.Context(), user.ID, req.IDs); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"marked": len(req.IDs)})
}
