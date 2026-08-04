package graphql

import (
	"time"

	"github.com/captv89/ovl/pkg/schema"
)

// Report is hand-modeled (not gqlgen-generated) because the fields(names)
// resolver needs more than the schema exposes: the report's raw JSONB
// field values and its resolved schema, to project on demand per
// whatever names a caller actually asks for. See gqlgen.yml's models.Report
// mapping and resolver.go's Fields method.
type Report struct {
	VesselID   string
	VesselName string
	VesselImo  string
	ReportID   string
	VersionNo  int
	SchemaName string
	EventType  string
	State      string
	EventTime  time.Time

	// RawFields and Schema back the fields(names) resolver — not part of
	// the GraphQL schema itself.
	RawFields map[string]any
	Schema    *schema.Schema
}
