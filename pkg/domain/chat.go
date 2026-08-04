// SPDX-License-Identifier: AGPL-3.0-only

package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ChatDirection is which side authored a ChatMessage. The wire message
// itself carries no direction field (see sync.proto's ChatMessage) — it
// is inferred structurally: a message landed via PushOutbox is
// ChatFromVessel, one authored through office HTTP is ChatFromOffice.
type ChatDirection string

const (
	ChatFromVessel ChatDirection = "vessel"
	ChatFromOffice ChatDirection = "office"
)

// MaxChatBodyBytes caps a single chat message's body (architecture 12.3:
// "text-only, size-capped"), enforced on every write path on both sides.
const MaxChatBodyBytes = 4096

// ChatMessage is one entry in a report's chat wall (architecture 12.3,
// design A8) — per-report, text-only, both directions.
type ChatMessage struct {
	ID        string
	ReportID  string
	Sender    string
	Body      string
	SentAt    time.Time
	Direction ChatDirection
}

// NewChatMessage creates a chat message, minting a UUIDv7 id and
// stamping SentAt at creation time. The single place both vessel and
// office HTTP handlers enforce MaxChatBodyBytes, so the size cap can't
// drift between the two write paths.
func NewChatMessage(reportID, sender, body string, direction ChatDirection) (ChatMessage, error) {
	if len(body) > MaxChatBodyBytes {
		return ChatMessage{}, fmt.Errorf("chat message body of %d bytes exceeds the %d byte limit", len(body), MaxChatBodyBytes)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return ChatMessage{}, fmt.Errorf("generate chat message id: %w", err)
	}
	return ChatMessage{
		ID: id.String(), ReportID: reportID, Sender: sender, Body: body,
		SentAt: time.Now().UTC(), Direction: direction,
	}, nil
}
