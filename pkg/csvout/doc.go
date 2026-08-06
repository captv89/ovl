// SPDX-License-Identifier: AGPL-3.0-only

// Package csvout generates OVD-formatted CSV exports per schema, standard
// column order and formatting per schema version.
//
// Originally written to match what a since-cancelled Veracity API push
// integration would have produced, so a manual CSV download and an API
// submission would be content-identical (DNV declined API access, so
// that push integration was removed — this package was never
// Veracity-specific itself and needed no changes). Now one of two data-
// egress paths external customers use: a bulk/compliance-style download,
// alongside the API-key-gated GraphQL endpoint for flexible per-field,
// per-time-period queries.
package csvout
