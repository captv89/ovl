// SPDX-License-Identifier: AGPL-3.0-only

// Package domain defines the report aggregate, its lifecycle states, and
// domain events shared between the vessel and office applications. It is
// deliberately independent of any store (SQLite on the vessel, Postgres
// in the office) and of pkg/schema/pkg/validation's rule configuration —
// callers run the validation engine and pass in the results, and
// persist the returned events; Report only enforces the lifecycle
// invariants themselves (architecture section 8).
package domain
