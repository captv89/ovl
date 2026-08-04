// SPDX-License-Identifier: AGPL-3.0-only

// Package vessels models office-side vessel profile data and free-form
// group tags (architecture 12.4, design handoff B2's Profile tab).
// Enrollment state (B2's separate Enrollment tab: codes, printable
// sheets, revoke/re-issue) is a later Phase 3 checklist item and is
// deliberately not modeled here yet.
package vessels

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Vessel is one vessel's office-side profile (architecture 12.4: "IMO,
// name, type, groups").
type Vessel struct {
	ID   string // UUIDv7
	IMO  string // 7-digit IMO number, immutable once set — see NewVessel
	Name string
	Type string // free-form vessel type (e.g. "Bulk Carrier"); no fixed enum in either handoff doc

	// Groups are free-form tags (fleet, vessel type, fleet manager,
	// architecture 12.4) used for config assignment and dashboard
	// filtering. May be empty — a vessel need not belong to any group.
	Groups []string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewVessel creates a vessel profile. imo must be a valid 7-digit IMO
// number (checksum-verified); it is never changeable afterward — a real
// IMO number is a permanent per-hull identifier, so no SetIMO exists.
func NewVessel(imo, name, vesselType string, groups []string) (*Vessel, error) {
	imo = strings.TrimSpace(imo)
	if err := ValidateIMO(imo); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("vessels: name is required")
	}
	vesselType = strings.TrimSpace(vesselType)
	if vesselType == "" {
		return nil, errors.New("vessels: type is required")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("vessels: generate vessel id: %w", err)
	}
	now := time.Now().UTC()
	return &Vessel{
		ID:        id.String(),
		IMO:       imo,
		Name:      name,
		Type:      vesselType,
		Groups:    normalizeGroups(groups),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// UpdateProfile replaces v's editable profile fields (design handoff B2:
// "Profile: master data, group tags (editable by Admin)"). IMO is not an
// editable field — see NewVessel's doc comment.
func (v *Vessel) UpdateProfile(name, vesselType string, groups []string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("vessels: name is required")
	}
	vesselType = strings.TrimSpace(vesselType)
	if vesselType == "" {
		return errors.New("vessels: type is required")
	}
	v.Name = name
	v.Type = vesselType
	v.Groups = normalizeGroups(groups)
	v.UpdatedAt = time.Now().UTC()
	return nil
}

// normalizeGroups trims whitespace, drops empty tags, and deduplicates —
// group tags are free-form (no fixed catalog to validate against,
// unlike office/auth.Role), so normalization is hygiene only.
func normalizeGroups(groups []string) []string {
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g == "" || slices.Contains(out, g) {
			continue
		}
		out = append(out, g)
	}
	return out
}

// ValidateIMO reports whether imo is a well-formed IMO ship
// identification number: exactly 7 digits, where the 7th is a check
// digit computed from the first six (each multiplied by a descending
// weight 7..2, summed, mod 10). This is IMO's own public numbering
// scheme, not an OVL-specific rule — implemented in full rather than
// just checking "7 digits" so a transposed-digit typo is caught at entry
// time instead of silently stored.
func ValidateIMO(imo string) error {
	if len(imo) != 7 {
		return fmt.Errorf("vessels: IMO number must be 7 digits, got %q", imo)
	}
	var digits [7]int
	for i, r := range imo {
		if r < '0' || r > '9' {
			return fmt.Errorf("vessels: IMO number must be all digits, got %q", imo)
		}
		digits[i] = int(r - '0')
	}
	sum := 0
	for i, weight := 0, 7; i < 6; i, weight = i+1, weight-1 {
		sum += digits[i] * weight
	}
	if checkDigit := sum % 10; checkDigit != digits[6] {
		return fmt.Errorf("vessels: %q fails the IMO check-digit formula (expected check digit %d)", imo, checkDigit)
	}
	return nil
}
