// SPDX-License-Identifier: AGPL-3.0-only

package syncproto

import (
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/captv89/ovl/pkg/domain"
	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"
)

// schemaKindByName and its inverse are the wire mapping between
// pkg/schema's string schema names (domain.Report.SchemaName, e.g.
// "log-abstract") and the proto's ReportSchemaKind enum (architecture
// handoff section 7) — kept here, not in pkg/schema or pkg/domain,
// since neither of those packages otherwise knows the sync wire format
// exists.
var schemaKindByName = map[string]syncv1.ReportSchemaKind{
	"log-abstract":      syncv1.ReportSchemaKind_REPORT_SCHEMA_KIND_LOG_ABSTRACT,
	"bunker-report":     syncv1.ReportSchemaKind_REPORT_SCHEMA_KIND_BUNKER_REPORT,
	"edn-report":        syncv1.ReportSchemaKind_REPORT_SCHEMA_KIND_EDN_REPORT,
	"commercial-period": syncv1.ReportSchemaKind_REPORT_SCHEMA_KIND_COMMERCIAL_PERIOD,
	"cargo-nomination":  syncv1.ReportSchemaKind_REPORT_SCHEMA_KIND_CARGO_NOMINATION,
}

var schemaNameByKind = func() map[syncv1.ReportSchemaKind]string {
	out := make(map[syncv1.ReportSchemaKind]string, len(schemaKindByName))
	for name, kind := range schemaKindByName {
		out[kind] = name
	}
	return out
}()

// SchemaKindFromName converts a pkg/schema schema name to its proto
// ReportSchemaKind. Returns an error for any name outside the five
// curated OVD schemas (architecture handoff section 7) — there is no
// "unspecified" fallback here, since a report always has a real schema.
func SchemaKindFromName(name string) (syncv1.ReportSchemaKind, error) {
	kind, ok := schemaKindByName[name]
	if !ok {
		return syncv1.ReportSchemaKind_REPORT_SCHEMA_KIND_UNSPECIFIED, fmt.Errorf("syncproto: unknown schema name %q", name)
	}
	return kind, nil
}

// SchemaNameFromKind is SchemaKindFromName's inverse.
func SchemaNameFromKind(kind syncv1.ReportSchemaKind) (string, error) {
	name, ok := schemaNameByKind[kind]
	if !ok {
		return "", fmt.Errorf("syncproto: unknown schema kind %v", kind)
	}
	return name, nil
}

// User command action string values (architecture 9.3/12.4's remote
// vessel-user-administration, 2026-07-21). office/store.UserCommand.Action
// stores one of these; vessel/httpapi switches on the same values when
// applying a pulled command — kept here, not in either app's own
// package, so the vocabulary can't drift between the two sides the way a
// value defined in only one of them could.
const (
	UserCommandActionCreate        = "create"
	UserCommandActionResetPassword = "resetPassword"
	UserCommandActionSetRole       = "setRole"
	UserCommandActionSetActive     = "setActive"
	UserCommandActionSetCanSubmit  = "setCanSubmit"
)

var userCommandActionByString = map[string]syncv1.UserCommandAction{
	UserCommandActionCreate:        syncv1.UserCommandAction_USER_COMMAND_ACTION_CREATE,
	UserCommandActionResetPassword: syncv1.UserCommandAction_USER_COMMAND_ACTION_RESET_PASSWORD,
	UserCommandActionSetRole:       syncv1.UserCommandAction_USER_COMMAND_ACTION_SET_ROLE,
	UserCommandActionSetActive:     syncv1.UserCommandAction_USER_COMMAND_ACTION_SET_ACTIVE,
	UserCommandActionSetCanSubmit:  syncv1.UserCommandAction_USER_COMMAND_ACTION_SET_CAN_SUBMIT,
}

var userCommandActionByProto = func() map[syncv1.UserCommandAction]string {
	out := make(map[syncv1.UserCommandAction]string, len(userCommandActionByString))
	for name, action := range userCommandActionByString {
		out[action] = name
	}
	return out
}()

// UserCommandActionFromString converts office/store's stored action
// string to the proto enum.
func UserCommandActionFromString(action string) (syncv1.UserCommandAction, error) {
	pb, ok := userCommandActionByString[action]
	if !ok {
		return syncv1.UserCommandAction_USER_COMMAND_ACTION_UNSPECIFIED, fmt.Errorf("syncproto: unknown user command action %q", action)
	}
	return pb, nil
}

// UserCommandActionToString is UserCommandActionFromString's inverse.
func UserCommandActionToString(pb syncv1.UserCommandAction) (string, error) {
	action, ok := userCommandActionByProto[pb]
	if !ok {
		return "", fmt.Errorf("syncproto: unknown user command action %v", pb)
	}
	return action, nil
}

// VesselRoleMaster and the five crew roles are vessel/auth.Role's own
// string values (architecture 9.3), duplicated here rather than
// imported — office never imports vessel's own packages (or vice versa;
// see vessel/sync/client_test.go's own doc comment on why), so a
// UserCommand's role string needs a validator on office's side that
// doesn't require pulling in vessel/auth. Keep in sync with
// vessel/auth/role.go's DefaultRoles by hand; there are exactly six and
// they haven't changed since Phase 2.
const (
	VesselRoleMaster         = "master"
	VesselRoleChiefOfficer   = "chiefOfficer"
	VesselRoleSecondOfficer  = "secondOfficer"
	VesselRoleThirdOfficer   = "thirdOfficer"
	VesselRoleChiefEngineer  = "chiefEngineer"
	VesselRoleSecondEngineer = "secondEngineer"
)

var vesselRoles = map[string]bool{
	VesselRoleMaster: true, VesselRoleChiefOfficer: true, VesselRoleSecondOfficer: true,
	VesselRoleThirdOfficer: true, VesselRoleChiefEngineer: true, VesselRoleSecondEngineer: true,
}

// IsValidVesselRole reports whether role is one of the six fixed vessel
// roles.
func IsValidVesselRole(role string) bool {
	return vesselRoles[role]
}

// lifecycleStateByDomain and its inverse map pkg/domain.State 1:1 onto
// the proto's ReportLifecycleState (architecture 8.1) — the two enums
// were authored to match exactly (see pkg/domain/state.go's own doc
// comment), so this is a pure lookup, never a lossy projection.
var lifecycleStateByDomain = map[domain.State]syncv1.ReportLifecycleState{
	domain.StateDraft:       syncv1.ReportLifecycleState_REPORT_LIFECYCLE_STATE_DRAFT,
	domain.StateReady:       syncv1.ReportLifecycleState_REPORT_LIFECYCLE_STATE_READY,
	domain.StateSubmitted:   syncv1.ReportLifecycleState_REPORT_LIFECYCLE_STATE_SUBMITTED,
	domain.StateSynced:      syncv1.ReportLifecycleState_REPORT_LIFECYCLE_STATE_SYNCED,
	domain.StatePushed:      syncv1.ReportLifecycleState_REPORT_LIFECYCLE_STATE_PUSHED,
	domain.StateRemarked:    syncv1.ReportLifecycleState_REPORT_LIFECYCLE_STATE_REMARKED,
	domain.StateInvalidated: syncv1.ReportLifecycleState_REPORT_LIFECYCLE_STATE_INVALIDATED,
}

var domainByLifecycleState = func() map[syncv1.ReportLifecycleState]domain.State {
	out := make(map[syncv1.ReportLifecycleState]domain.State, len(lifecycleStateByDomain))
	for s, k := range lifecycleStateByDomain {
		out[k] = s
	}
	return out
}()

// auditEventTypeByDomain and its inverse map pkg/domain.EventType 1:1
// onto the proto's AuditEventType (architecture 14) — same "authored to
// match" relationship as the lifecycle states above.
var auditEventTypeByDomain = map[domain.EventType]syncv1.AuditEventType{
	domain.EventCreated:             syncv1.AuditEventType_AUDIT_EVENT_TYPE_CREATED,
	domain.EventSectionSaved:        syncv1.AuditEventType_AUDIT_EVENT_TYPE_SECTION_SAVED,
	domain.EventHealthCheckResult:   syncv1.AuditEventType_AUDIT_EVENT_TYPE_HEALTH_CHECK_RESULT,
	domain.EventSubmitted:           syncv1.AuditEventType_AUDIT_EVENT_TYPE_SUBMITTED,
	domain.EventSynced:              syncv1.AuditEventType_AUDIT_EVENT_TYPE_SYNCED,
	domain.EventRemarked:            syncv1.AuditEventType_AUDIT_EVENT_TYPE_REMARKED,
	domain.EventCorrectionStarted:   syncv1.AuditEventType_AUDIT_EVENT_TYPE_CORRECTION_STARTED,
	domain.EventResubmitted:         syncv1.AuditEventType_AUDIT_EVENT_TYPE_RESUBMITTED,
	domain.EventInvalidated:         syncv1.AuditEventType_AUDIT_EVENT_TYPE_INVALIDATED,
	domain.EventRestoreApplied:      syncv1.AuditEventType_AUDIT_EVENT_TYPE_RESTORE_APPLIED,
	domain.EventFindingAcknowledged: syncv1.AuditEventType_AUDIT_EVENT_TYPE_FINDING_ACKNOWLEDGED,
}

var domainByAuditEventType = func() map[syncv1.AuditEventType]domain.EventType {
	out := make(map[syncv1.AuditEventType]domain.EventType, len(auditEventTypeByDomain))
	for e, t := range auditEventTypeByDomain {
		out[t] = e
	}
	return out
}()

// ReportVersionFromDomain converts r to its proto wire type
// (PushOutbox's payload). Fails if r.SchemaName isn't one of the five
// curated schemas or r.Fields contains a value structpb can't represent
// (architecture 7's fields are all JSON-primitive-typed, so this should
// only ever happen for a malformed report).
func ReportVersionFromDomain(r *domain.Report) (*syncv1.ReportVersion, error) {
	kind, err := SchemaKindFromName(r.SchemaName)
	if err != nil {
		return nil, err
	}
	state, ok := lifecycleStateByDomain[r.State]
	if !ok {
		return nil, fmt.Errorf("syncproto: unknown report state %q", r.State)
	}
	fields, err := structpb.NewStruct(r.Fields)
	if err != nil {
		return nil, fmt.Errorf("syncproto: convert fields for report %s v%d: %w", r.ReportID, r.VersionNo, err)
	}
	out := &syncv1.ReportVersion{
		ReportId:      r.ReportID,
		VersionNo:     int32(r.VersionNo), // #nosec G115 -- report version numbers stay far below 2^31 in practice
		SchemaKind:    kind,
		SchemaVersion: "", // resolved from the config bundle/schema registry, not carried on Report yet — Step 4's territory
		EventType:     r.EventType,
		State:         state,
		EventTime:     timestamppb.New(r.EventTime),
		Fields:        fields,
	}
	if !r.SubmittedAt.IsZero() {
		out.SubmittedAt = timestamppb.New(r.SubmittedAt)
	}
	return out, nil
}

// ReportVersionToDomain is ReportVersionFromDomain's inverse, for the
// office side to reconstruct enough of a domain.Report to persist
// (office/store's report_versions table, Step 3's landing surface).
// CreatedAt/CreatedBy/UpdatedAt/SubmittedBy are not carried by the proto
// message (see sync.proto's own ReportVersion fields) and are left zero;
// the office's own row records when *it* received the item instead.
func ReportVersionToDomain(pv *syncv1.ReportVersion) (*domain.Report, error) {
	schemaName, err := SchemaNameFromKind(pv.GetSchemaKind())
	if err != nil {
		return nil, err
	}
	state, ok := domainByLifecycleState[pv.GetState()]
	if !ok {
		return nil, fmt.Errorf("syncproto: unknown lifecycle state %v", pv.GetState())
	}
	r := &domain.Report{
		ReportID:   pv.GetReportId(),
		VersionNo:  int(pv.GetVersionNo()),
		SchemaName: schemaName,
		EventType:  pv.GetEventType(),
		EventTime:  pv.GetEventTime().AsTime(),
		Fields:     pv.GetFields().AsMap(),
		State:      state,
	}
	if pv.GetSubmittedAt() != nil {
		r.SubmittedAt = pv.GetSubmittedAt().AsTime()
	}
	return r, nil
}

// ReportAuditEventFromDomain converts e to its proto wire type. detail
// must contain only structpb-representable values (bool, string,
// float64, nil, map[string]any, []any) — true for every event type this
// project currently enqueues to the outbox (Step 3: only EventSubmitted,
// whose Detail is empty). EventSectionSaved's Detail carries
// []domain.FieldChange, which is NOT yet structpb-representable; nothing
// enqueues that event type today (see vessel/httpapi.handleSubmitReport's
// own doc comment on outbox scope), so this is a known, deliberately
// unaddressed gap rather than a silent bug — convert that Detail to
// plain maps here first if a later step starts syncing section_saved
// events.
func ReportAuditEventFromDomain(e domain.Event) (*syncv1.ReportAuditEvent, error) {
	eventType, ok := auditEventTypeByDomain[e.Type]
	if !ok {
		return nil, fmt.Errorf("syncproto: unknown event type %q", e.Type)
	}
	detail, err := structpb.NewStruct(e.Detail)
	if err != nil {
		return nil, fmt.Errorf("syncproto: convert detail for %s event on report %s v%d: %w", e.Type, e.ReportID, e.VersionNo, err)
	}
	return &syncv1.ReportAuditEvent{
		ReportId:   e.ReportID,
		VersionNo:  int32(e.VersionNo), // #nosec G115 -- report version numbers stay far below 2^31 in practice
		EventType:  eventType,
		Actor:      e.Actor,
		OccurredAt: timestamppb.New(e.At),
		Detail:     detail,
	}, nil
}

// ReportAuditEventToDomain is ReportAuditEventFromDomain's inverse.
func ReportAuditEventToDomain(pe *syncv1.ReportAuditEvent) (domain.Event, error) {
	eventType, ok := domainByAuditEventType[pe.GetEventType()]
	if !ok {
		return domain.Event{}, fmt.Errorf("syncproto: unknown audit event type %v", pe.GetEventType())
	}
	return domain.Event{
		ReportID:  pe.GetReportId(),
		VersionNo: int(pe.GetVersionNo()),
		Type:      eventType,
		At:        pe.GetOccurredAt().AsTime(),
		Actor:     pe.GetActor(),
		Detail:    pe.GetDetail().AsMap(),
	}, nil
}

// RemarkFromDomain converts r to its proto wire type. Unlike
// ReportVersion/ReportAuditEvent, Remark has no enum field to look up,
// so this conversion cannot fail.
func RemarkFromDomain(r domain.Remark) *syncv1.Remark {
	return &syncv1.Remark{
		Id:        r.ID,
		ReportId:  r.ReportID,
		VersionNo: int32(r.VersionNo), // #nosec G115 -- report version numbers stay far below 2^31 in practice
		FieldName: r.FieldName,
		Body:      r.Body,
		Author:    r.Author,
		CreatedAt: timestamppb.New(r.CreatedAt),
		Resolved:  r.Resolved,
	}
}

// RemarkToDomain is RemarkFromDomain's inverse.
func RemarkToDomain(pr *syncv1.Remark) domain.Remark {
	return domain.Remark{
		ID:        pr.GetId(),
		ReportID:  pr.GetReportId(),
		VersionNo: int(pr.GetVersionNo()),
		FieldName: pr.GetFieldName(),
		Body:      pr.GetBody(),
		Author:    pr.GetAuthor(),
		CreatedAt: pr.GetCreatedAt().AsTime(),
		Resolved:  pr.GetResolved(),
	}
}

// ChatMessageFromDomain converts m to its proto wire type. m.Direction
// is not carried — the wire ChatMessage message has no direction field
// (see domain.ChatMessage's own doc comment); it is structural on both
// send and receive, not a serialized property.
func ChatMessageFromDomain(m domain.ChatMessage) *syncv1.ChatMessage {
	return &syncv1.ChatMessage{
		Id:       m.ID,
		ReportId: m.ReportID,
		Sender:   m.Sender,
		Body:     m.Body,
		SentAt:   timestamppb.New(m.SentAt),
	}
}

// ChatMessageToDomain is ChatMessageFromDomain's inverse. direction must
// be supplied by the caller (ChatFromVessel for an item landed via
// PushOutbox, ChatFromOffice for one returned by PullInbox) since the
// wire message itself carries no direction field.
func ChatMessageToDomain(pm *syncv1.ChatMessage, direction domain.ChatDirection) domain.ChatMessage {
	return domain.ChatMessage{
		ID:        pm.GetId(),
		ReportID:  pm.GetReportId(),
		Sender:    pm.GetSender(),
		Body:      pm.GetBody(),
		SentAt:    pm.GetSentAt().AsTime(),
		Direction: direction,
	}
}

// InvalidationNoticeFromDomain converts n to its proto wire type. Like
// Remark, this has no enum field to look up and cannot fail.
func InvalidationNoticeFromDomain(n domain.InvalidationNotice) *syncv1.InvalidationNotice {
	return &syncv1.InvalidationNotice{
		ReportId:    n.ReportID,
		VersionNo:   int32(n.VersionNo), // #nosec G115 -- report version numbers stay far below 2^31 in practice
		BrokenRules: n.BrokenRules,
		ComputedAt:  timestamppb.New(n.ComputedAt),
	}
}

// InvalidationNoticeToDomain is InvalidationNoticeFromDomain's inverse.
func InvalidationNoticeToDomain(pn *syncv1.InvalidationNotice) domain.InvalidationNotice {
	return domain.InvalidationNotice{
		ReportID:    pn.GetReportId(),
		VersionNo:   int(pn.GetVersionNo()),
		BrokenRules: pn.GetBrokenRules(),
		ComputedAt:  pn.GetComputedAt().AsTime(),
	}
}
