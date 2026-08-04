// SPDX-License-Identifier: AGPL-3.0-only

package domain

import "testing"

func TestNewChatMessage(t *testing.T) {
	m, err := NewChatMessage("report-1", "master", "corrected version pushed", ChatFromVessel)
	if err != nil {
		t.Fatalf("NewChatMessage: %v", err)
	}
	if m.ID == "" {
		t.Error("ID is empty")
	}
	if m.ReportID != "report-1" || m.Sender != "master" || m.Body != "corrected version pushed" || m.Direction != ChatFromVessel {
		t.Errorf("m = %+v, want matching fields", m)
	}
	if m.SentAt.IsZero() {
		t.Error("SentAt is zero")
	}
}

func TestNewChatMessage_RejectsOverCapBody(t *testing.T) {
	body := make([]byte, MaxChatBodyBytes+1)
	for i := range body {
		body[i] = 'a'
	}
	if _, err := NewChatMessage("report-1", "master", string(body), ChatFromVessel); err == nil {
		t.Fatal("NewChatMessage with over-cap body: got nil error, want an error")
	}
}

func TestNewChatMessage_AllowsExactlyMaxCapBody(t *testing.T) {
	body := make([]byte, MaxChatBodyBytes)
	for i := range body {
		body[i] = 'a'
	}
	if _, err := NewChatMessage("report-1", "master", string(body), ChatFromVessel); err != nil {
		t.Errorf("NewChatMessage at exactly the cap: %v, want success", err)
	}
}
