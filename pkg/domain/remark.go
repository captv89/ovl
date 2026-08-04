// SPDX-License-Identifier: AGPL-3.0-only

package domain

import "time"

// Remark is one reviewer comment against a single field of a specific
// report version (architecture 12.3). A B4 "send remark set" action
// produces several Remarks sharing a created_at/author, which the
// receiving side regroups structurally — the wire message itself
// carries no remark-set id (see sync.proto's Remark message).
type Remark struct {
	ID        string
	ReportID  string
	VersionNo int
	FieldName string
	Body      string
	Author    string
	CreatedAt time.Time
	Resolved  bool
}
