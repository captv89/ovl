package graphql

// THIS CODE WILL BE UPDATED WITH SCHEMA CHANGES. PREVIOUS IMPLEMENTATION FOR SCHEMA CHANGES WILL BE KEPT IN THE COMMENT SECTION. IMPLEMENTATION FOR UNCHANGED SCHEMA WILL BE KEPT.

import (
	"context"
	"errors"

	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/fieldproject"
	"github.com/captv89/ovl/pkg/schema"
)

// Resolver holds everything the resolvers below need — just the office
// store, the same dependency office/httpapi's own handlers already take.
type Resolver struct {
	Store *store.Store
}

// defaultLimit/maxLimit bound a page when the caller under- or over-
// specifies limit (architecture 13.2: this is the one place external,
// API-key-gated callers touch report data, so it can never be an
// unbounded scan the way office staff's own internal list view could
// safely be).
const (
	defaultLimit = 100
	maxLimit     = 1000
)

// Reports is the resolver for the reports field.
func (r *queryResolver) Reports(ctx context.Context, filter *ReportFilterInput, limit *int, offset *int) (*ReportPage, error) {
	key, ok := APIKeyFromContext(ctx)
	if !ok {
		return nil, errors.New("graphql: no authenticated API key on context")
	}

	sf := store.ReportFilter{}
	if filter != nil {
		sf.VesselID = filter.VesselID
		sf.GroupID = filter.GroupID
		sf.State = filter.State
		sf.EventType = filter.EventType
		sf.SchemaName = filter.SchemaName
		sf.DateFrom = filter.DateFrom
		sf.DateTo = filter.DateTo
	}
	// The key's own group scope always wins over whatever the query
	// argument asked for — a scoped key must never be able to see
	// outside its own vessel group just by omitting or overriding
	// groupId in the request. An unscoped key (GroupID nil) leaves
	// filter.GroupID as the caller specified (including none).
	if key.GroupID != nil {
		sf.GroupID = key.GroupID
	}

	l := defaultLimit
	if limit != nil {
		l = *limit
	}
	if l <= 0 || l > maxLimit {
		l = defaultLimit
	}
	off := 0
	if offset != nil && *offset > 0 {
		off = *offset
	}
	// Fetch one extra row past the page size to know hasNextPage
	// without a separate COUNT query.
	fetchLimit := l + 1
	sf.Limit = &fetchLimit
	sf.Offset = &off

	rows, err := r.Store.ListReports(ctx, sf)
	if err != nil {
		return nil, err
	}
	hasNext := len(rows) > l
	if hasNext {
		rows = rows[:l]
	}

	schemaCache := map[string]*schema.Schema{}
	items := make([]*Report, len(rows))
	for i, row := range rows {
		sch, err := r.schemaFor(ctx, schemaCache, row.SchemaName)
		if err != nil {
			return nil, err
		}
		items[i] = &Report{
			VesselID:   row.VesselID,
			VesselName: row.VesselName,
			VesselImo:  row.VesselIMO,
			ReportID:   row.ReportID,
			VersionNo:  row.VersionNo,
			SchemaName: row.SchemaName,
			EventType:  row.EventType,
			State:      string(row.State),
			EventTime:  row.EventTime,
			RawFields:  row.Fields,
			Schema:     sch,
		}
	}
	return &ReportPage{Items: items, HasNextPage: hasNext}, nil
}

// schemaFor resolves and parses schemaName's latest published schema,
// caching within one Reports call — mirrors office/httpapi's own
// per-request schemaHealthContext cache (reports.go's
// evaluateListRowHealth) rather than re-fetching per row, since a page
// of reports commonly repeats the same handful of schemas.
func (r *Resolver) schemaFor(ctx context.Context, cache map[string]*schema.Schema, schemaName string) (*schema.Schema, error) {
	if sch, ok := cache[schemaName]; ok {
		return sch, nil
	}
	latest, err := r.Store.LatestSchemaVersion(ctx, schemaName)
	if err != nil {
		return nil, err
	}
	sch, err := schema.Parse(latest.Content)
	if err != nil {
		return nil, err
	}
	cache[schemaName] = sch
	return sch, nil
}

// Fields is the resolver for the fields field — the whole point of this
// API (architecture 13.2: "pick whichever data fields they need"), so it
// has to be a real resolver reading obj.RawFields/obj.Schema rather than
// gqlgen's default plain-struct-field binding, which would silently
// ignore the names argument entirely (see gqlgen.yml's own comment on
// this exact footgun).
func (r *reportResolver) Fields(ctx context.Context, obj *Report, names []string) ([]*FieldValue, error) {
	var wanted map[string]bool
	if len(names) > 0 {
		wanted = make(map[string]bool, len(names))
		for _, n := range names {
			wanted[n] = true
		}
	}
	out := make([]*FieldValue, 0, len(obj.Schema.Fields))
	for _, f := range obj.Schema.Fields {
		if wanted != nil && !wanted[f.Name] {
			continue
		}
		val, err := fieldproject.Project(f, obj.RawFields[f.Name])
		if err != nil {
			return nil, err
		}
		fv := &FieldValue{Name: f.Name, Label: f.Label, Type: string(f.Type)}
		switch val.Kind {
		case fieldproject.KindString:
			fv.StringValue = &val.String
		case fieldproject.KindNumber:
			fv.NumberValue = &val.Number
		case fieldproject.KindBool:
			fv.BoolValue = &val.Bool
		}
		out = append(out, fv)
	}
	return out, nil
}

// Query returns QueryResolver implementation.
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

// Report returns ReportResolver implementation.
func (r *Resolver) Report() ReportResolver { return &reportResolver{r} }

type (
	queryResolver  struct{ *Resolver }
	reportResolver struct{ *Resolver }
)
