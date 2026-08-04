// SPDX-License-Identifier: AGPL-3.0-only

// Package validation implements the field, plausibility, and continuity
// rule engine, including cascade revalidation across a vessel's report
// chain. It runs identically on the vessel and the office; both sides
// must compute the same report health from the same data.
//
// See handoff/OVL_Architecture_Handoff.md sections 8.3 and 10.
package validation
