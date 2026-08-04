// SPDX-License-Identifier: AGPL-3.0-only

// Package restorebundle is the shared wire format for architecture
// 12.5's office-generated, vessel-imported restore bundle — office's
// httpapi.buildRestoreBundle produces one, JSON-marshals it, encrypts it
// with pkg/backupcrypto against the vessel's own DR public key; vessel's
// httpapi decrypts and unmarshals the same shape back. A dedicated
// package (not pkg/domain, which models core report/event/chat types
// themselves, not a transport envelope around them) — same reasoning as
// pkg/syncproto being separate from pkg/domain.
package restorebundle

import (
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

// Bundle is one vessel's full report history, audit trail, chat, and
// currently-assigned config bundle (architecture 12.5: "all reports,
// config, chat and attachments the office holds for that vessel").
// Attachments are still deliberately excluded — see
// office/restorebundle.BuildBundle's own doc comment for why — a real,
// separately-tracked scope boundary, not a silent gap. ConfigBundle is
// nil if the vessel has no bundle assignment (vessel or group) resolved
// at generation time; a real, reachable state, not an error.
type Bundle struct {
	VesselID     string         `json:"vesselId"`
	VesselName   string         `json:"vesselName"`
	VesselIMO    string         `json:"vesselImo"`
	GeneratedAt  time.Time      `json:"generatedAt"`
	Reports      []BundleReport `json:"reports"`
	ConfigBundle *ConfigBundle  `json:"configBundle,omitempty"`
}

type BundleReport struct {
	ReportID string               `json:"reportId"`
	Versions []*domain.Report     `json:"versions"`
	Events   []domain.Event       `json:"events"`
	Chat     []domain.ChatMessage `json:"chat"`
}

// ConfigBundle is a snapshot of the vessel's resolved config bundle at
// generation time — the exact wire shape vessel/store.PulledConfigBundle
// already applies via the normal PullInbox path (vessel/httpapi/sync.go's
// pullInboxBatch), reused here rather than duplicated: importing a
// restore bundle installs config through the same insert-if-absent path
// an ordinary sync would have, just carried inside this envelope instead
// of PullInboxResponse.
type ConfigBundle struct {
	BundleID    string    `json:"bundleId"`
	VersionNo   int64     `json:"versionNo"`
	ContentJSON []byte    `json:"contentJson"`
	PublishedAt time.Time `json:"publishedAt"`
}
