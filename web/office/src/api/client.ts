// SPDX-License-Identifier: AGPL-3.0-only

// Typed client for office/httpapi's JSON API. Shapes here must match
// office/httpapi's Go structs exactly (server.go's userView, setup.go's
// setupStatusResponse).

// Mirrors office/auth.Role's string values.
export type Role = "admin" | "configManager" | "commercialEditor" | "reviewer" | "viewer";

export interface UserView {
  id: string;
  username: string;
  roles: Role[];
  mustChangePassword: boolean;
  active: boolean;
  createdAt: string;
}

export interface CreateUserResponse {
  user: UserView;
  temporaryPassword: string;
}

export interface VesselGroupMutationResponse {
  vesselsUpdated: number;
}

// Mirrors office/httpapi's apiKeyView (architecture 13.1's data-API
// credential — API-key + GraphQL/CSV access for external customers,
// separate from office staff's own session-cookie login).
export interface ApiKeyView {
  id: string;
  label: string;
  groupId: string | null;
  createdBy: string;
  createdAt: string;
  revokedAt: string | null;
  lastUsedAt: string | null;
}

export interface CreateApiKeyResponse {
  apiKey: ApiKeyView;
  token: string;
}

// Mirrors office/httpapi's apiKeyEventView — one row of a key's
// activity log (2026-07-25 redesign's API Access log panel).
export interface ApiKeyEventView {
  kind: "created" | "revoked" | "usedGraphQL" | "usedCSV";
  at: string;
}

export interface SystemView {
  version: string;
  databaseReachable: boolean;
  attachmentStoreBytes: number;
  attachmentStoreCount: number;
}

export interface SetupStatus {
  hasAnyUser: boolean;
}

export interface FindingView {
  ruleId: string;
  severity: "error" | "warning" | "info";
  field?: string;
  message: string;
}

export interface CreateCommercialReportResponse {
  report?: ReportView;
  findings: FindingView[];
}

export interface VesselPositionView {
  vesselId: string;
  vesselName: string;
  vesselImo: string;
  groups: string[];
  lat: number;
  lon: number;
  status: "ok" | "remarked" | "overdue";
  asOf: string;
}

export interface NotificationLinkView {
  section: "vessels" | "reports";
  vesselId?: string;
  reportId?: string;
}

export interface NotificationView {
  id: string;
  category: "overdue" | "remark" | "sync";
  title: string;
  message: string;
  at: string;
  read: boolean;
  link?: NotificationLinkView;
}

export interface DashboardOverdueVesselView {
  vesselId: string;
  vesselName: string;
  vesselImo: string;
  groups: string[];
  lastReportAt: string;
  overdueHours: number;
}

export interface DashboardDataQualityPointView {
  date: string;
  errors: number;
  warnings: number;
}

// Mirrors office/httpapi's dashboardOperationsRow (architecture 16's
// "operations overview: simple consumption and distance view per vessel
// and group over a selectable period"). Replaces the removed "OVD sync
// status" widget's slot — that one surfaced the now-cancelled Veracity
// push integration's health and has no equivalent in the pull-based data
// API that replaced it.
export interface DashboardOperationsRow {
  vesselId: string;
  vesselName: string;
  vesselImo: string;
  totalDistanceNm: number;
  totalConsumptionMt: number;
  reportCount: number;
}

export interface DashboardView {
  overdueVessels: DashboardOverdueVesselView[];
  overdueVesselCount: number;
  enrolledVesselCount: number;
  compliancePercent: number;
  reportsNeedingReview: number;
  dataQualityTrend: DashboardDataQualityPointView[];
  operationsOverview: DashboardOperationsRow[];
  operationsPeriodDays: number;
}

// Mirrors office/enrollment.State's string values, plus the synthetic
// "notIssued" office/httpapi reports when no enrollment row exists yet.
export type EnrollmentState = "notIssued" | "issued" | "enrolled" | "revoked";

export interface VesselView {
  id: string;
  imo: string;
  name: string;
  type: string;
  groups: string[];
  enrollmentState: EnrollmentState;
  createdAt: string;
  updatedAt: string;
  /** Absent if this vessel has never had a report land office-side. */
  lastReportAt?: string;
  /** Set only once the vessel has exceeded its effective cadence's max gap. */
  overdueHours?: number;
  /** Absent if this vessel has never completed a SyncStatus check-in. */
  lastSyncAt?: string;
  /** Absent alongside lastSyncAt when the vessel has never synced. */
  appVersion?: string;
  /** The config bundle the vessel reports it is actually running on (§3.3).
   * Compare against bundleAssignment.bundleId to tell whether the ship is on
   * its assigned config or hasn't pulled it yet. Absent until the vessel has
   * applied any bundle. */
  appliedBundleId?: string;
  appliedBundleVersion?: number;
}

export interface EnrollmentView {
  state: EnrollmentState;
  initialMasterUsername?: string;
  issuedAt?: string;
  revokedAt?: string;
  /** Whether the vessel has (re-)redeemed since the DR keypair exchange
   * landed (architecture 12.5) — a restore bundle can only be generated
   * once this is true. */
  hasDRKey: boolean;
}

export interface BundleAssignmentView {
  bundleId: string;
  bundleLabel: string;
  publishedAt: string;
  scopeType: "fleet" | "vessel" | "group";
  scopeKey?: string;
}

// Mirrors office/httpapi's restoreCommandView — one office-issued DR
// push's status (design handoff B2's DR tab, architecture 12.5/11.2's DR
// push path).
export interface RestoreCommandView {
  id: string;
  reason: string;
  issuedBy: string;
  issuedAt: string;
  fetchedAt?: string;
  appliedAt?: string;
}

// Mirrors office/httpapi's vesselUserView — a read-only mirror of one
// vessel-local account (architecture 9.3/12.4's remote user
// administration, 2026-07-21). Reported by the vessel on its own sync
// check-ins; never a source of truth. No password data.
export interface VesselUserView {
  username: string;
  role: string;
  active: boolean;
  canSubmit: boolean;
  updatedAt: string;
}

// Mirrors office/httpapi's userCommandView — one queued remote user-
// management action's status, same queued/fetched/applied shape as
// RestoreCommandView.
export interface UserCommandView {
  id: string;
  action: string;
  username: string;
  role?: string;
  issuedBy: string;
  issuedAt: string;
  fetchedAt?: string;
  appliedAt?: string;
}

// Mirrors office/httpapi's queuedUserCommandResponse — TemporaryPassword
// is set only for create/reset-password, revealed exactly once here.
export interface QueuedUserCommandResponse {
  command: UserCommandView;
  temporaryPassword?: string;
}

export interface VesselDetailView {
  vessel: VesselView;
  enrollment: EnrollmentView | null;
  bundleAssignment: BundleAssignmentView | null;
  restoreCommands: RestoreCommandView[];
  vesselUsers: VesselUserView[];
  userCommands: UserCommandView[];
}

export interface IssueResultView {
  enrollment: EnrollmentView;
  code: string;
  initialMasterPassword: string;
}

export interface SchemaVersionSummary {
  schemaName: string;
  version: string;
  source: "projectCurated" | "companyEdited";
  publishedAt: string;
  publishedBy: string;
  fieldCount: number;
}

export interface SchemaField {
  name: string;
  label: string;
  type: string;
  unit?: string;
  schemaMandatory: boolean;
  relevance: string;
  section: string;
  appliesToEvents: string[];
}

export interface SchemaDetail {
  schemaName: string;
  version: string;
  sections: string[];
  fields: SchemaField[];
}

export interface SchemaDiff {
  added: SchemaField[];
  removed: SchemaField[];
  typeChanged: string[];
  mandatorinessChanged: string[];
  enumChanged: string[];
}

export interface SchemaUploadPreview {
  valid: boolean;
  error?: string;
  parsedName?: string;
  parsedVersion?: string;
  diff?: SchemaDiff | null;
}

export interface FieldPolicyMigration {
  fromVersion: string;
  newFields: string[];
  removedFields: string[];
}

export type ScopeType = "fleet" | "group" | "vessel";

export interface Scope {
  type: ScopeType;
  key?: string;
}

export interface ProfileAssignmentView {
  scope: Scope;
  profiles: string[];
  updatedAt: string;
}

export interface CadenceRuleView {
  scope: Scope;
  minReportIntervalHours: number;
  maxGapHours: number;
  updatedAt: string;
}

export interface RuleCatalog {
  overridable: string[];
  hard: string[];
}

export interface RuleSeverityAssignmentView {
  scope: Scope;
  severities: Record<string, string>;
  updatedAt: string;
}

export interface ConfigBundleSummary {
  id: string;
  label: string;
  schemaVersionCount: number;
  fieldPolicyRows: number;
  regulatoryProfileRows: number;
  cadenceRuleRows: number;
  ruleSeverityRows: number;
  publishedAt: string;
  publishedBy: string;
}

// Mirrors office/httpapi's vesselConfigView — one row of the fleet-wide
// "Vessel configs" tab (2026-07-25 redesign): every vessel's assigned
// bundle compared against what it last reported actually running.
export interface VesselConfigView {
  vesselId: string;
  vesselName: string;
  imo: string;
  status: "synced" | "outOfDate" | "pendingSync" | "unassigned";
  assignedBundleLabel?: string;
  activeSince?: string;
}


export interface FieldPolicyView {
  schemaName: string;
  version: string;
  scope: Scope;
  fields: SchemaField[];
  policy: Record<string, string>;
  prefill: Record<string, string>;
  /**
   * Narrows which voyage event types each field's policy applies to. A field
   * absent from the map applies to every event (the default every
   * unconfigured row shows); a field listed here is hidden on any report
   * whose event type is not in its list.
   */
  events: Record<string, string[]>;
  /**
   * The curated event-type vocabulary the applies-to-events control offers.
   * Empty for a schema with no event concept of its own (bunker-report,
   * edn-report, commercial-period, cargo-nomination), where the control is
   * hidden entirely.
   */
  eventTypes: string[];
  migration?: FieldPolicyMigration | null;
}

// One scope's summary row in the "current assignments" list — the
// field-policy equivalent of ProfileAssignmentView/CadenceRuleView/
// RuleSeverityAssignmentView, except versioned (field policy is
// authored per schema version as well as per scope).
export interface FieldPolicyAssignmentView {
  scope: Scope;
  version: string;
  policyCount: number;
  prefillCount: number;
  updatedAt: string;
}

// Mirrors pkg/domain.State's string values (design handoff section 2's
// 9-state report-lifecycle vocabulary, minus "pushed"/Phase 6's not-yet-
// modeled "push failed" — see office/store.ReportFilter's own doc
// comment on why PushFailedOnly isn't in ReportFilter below either).
export type ReportState = "draft" | "ready" | "submitted" | "synced" | "pushed" | "remarked" | "invalidated";

export interface ReportView {
  reportId: string;
  versionNo: number;
  schemaName: string;
  eventType: string;
  eventTime: string;
  fields: Record<string, unknown>;
  state: ReportState;
  submittedAt?: string;
}

// One row of design handoff B3's fleet-wide reports explorer.
export interface HealthView {
  errors: number;
  warnings: number;
}

export interface ReportListItemView {
  vesselId: string;
  vesselName: string;
  vesselImo: string;
  reportId: string;
  versionNo: number;
  schemaName: string;
  eventType: string;
  state: ReportState;
  eventTime: string;
  submittedAt?: string;
  reviewed: boolean;
  hasRemarks: boolean;
  health: HealthView;
}

export interface ReportDetailView {
  vesselId: string;
  vesselName: string;
  vesselImo: string;
  latest: ReportView;
  versions: number[];
}

// Mirrors pkg/domain.EventType's string values (architecture 14's audit
// trail), same vocabulary web/vessel's own client.ts already defines.
export type DomainEventType =
  | "created"
  | "section_saved"
  | "health_check_result"
  | "submitted"
  | "synced"
  | "remarked"
  | "correction_started"
  | "resubmitted"
  | "invalidated"
  | "restore_applied"
  | "finding_acknowledged";

// Mirrors office/httpapi's eventView. origin ("vessel" | "office") is an
// office-only concept — see office/store.ReportAuditEventRow's own doc
// comment on why it isn't part of the shared domain event shape.
export interface ReportEvent {
  versionNo: number;
  type: DomainEventType;
  at: string;
  actor?: string;
  detail?: Record<string, unknown>;
  origin: "vessel" | "office";
}

// Mirrors office/store.ReportFilter's query-parameter shape (design
// handoff B3's filter bar). Every field optional.
export interface ReportFilter {
  vesselId?: string;
  group?: string;
  state?: string;
  eventType?: string;
  schema?: string;
  dateFrom?: string;
  dateTo?: string;
  hasRemarks?: boolean;
  invalidatedOnly?: boolean;
}

// Mirrors pkg/domain.MaxChatBodyBytes. Duplicated here (Go and TS can't
// share a constant directly) — if the Go value ever changes, update
// this too. Same value as web/vessel's own client.ts.
export const MAX_CHAT_BODY_BYTES = 4096;

// Mirrors pkg/domain.ChatDirection's string values.
export type ChatDirection = "vessel" | "office";

// Mirrors office/httpapi's chatMessageView (design handoff B4/A8's chat
// wall).
export interface ChatMessageView {
  id: string;
  reportId: string;
  sender: string;
  body: string;
  sentAt: string;
  direction: ChatDirection;
}

// Mirrors office/httpapi's remarkView (design handoff B4's Remark mode,
// A7's Remarks tab).
export interface RemarkView {
  id: string;
  reportId: string;
  versionNo: number;
  fieldName: string;
  body: string;
  author: string;
  createdAt: string;
  resolved: boolean;
}

export interface RemarkFieldInput {
  fieldName: string;
  body: string;
}

// Mirrors office/httpapi's reportAttachmentView (architecture 15's
// "inline preview on vessel and office" — Phase 6).
export interface AttachmentView {
  id: string;
  reportId: string;
  versionNo: number;
  fieldName: string;
  filename: string;
  contentType: string;
  sizeBytes: number;
  receivedAt: string;
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: "same-origin",
  });
  if (res.status === 204) {
    return undefined as T;
  }
  const data: unknown = await res.json().catch(() => null);
  if (!res.ok) {
    const message =
      data && typeof data === "object" && "error" in data
        ? String((data as { error: unknown }).error)
        : `request failed with status ${res.status}`;
    throw new ApiError(res.status, message);
  }
  return data as T;
}

// B8's create-commercial-report endpoint deliberately returns 422 (not
// 2xx) with a real createCommercialReportResponse body — {findings, no
// report} — for a failed health check (see office/httpapi/commercial.go's
// own doc comment: "either lands as Submitted... or is rejected with
// the findings for the editor to fix"). That is a normal outcome for
// this one endpoint, not a request failure, so it can't go through
// request<T>()'s generic "any non-2xx throws" contract the way every
// other endpoint does.
async function postAllowingHealthCheckFailure<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    credentials: "same-origin",
  });
  const data: unknown = await res.json().catch(() => null);
  if (!res.ok && res.status !== 422) {
    const message =
      data && typeof data === "object" && "error" in data
        ? String((data as { error: unknown }).error)
        : `request failed with status ${res.status}`;
    throw new ApiError(res.status, message);
  }
  return data as T;
}

export const api = {
  getSetupStatus: () => request<SetupStatus>("GET", "/api/setup/status"),
  getDashboard: (group?: string | null, opsDays?: number) => {
    const params = new URLSearchParams();
    if (group) params.set("group", group);
    if (opsDays) params.set("opsDays", String(opsDays));
    const qs = params.toString();
    return request<DashboardView>("GET", `/api/dashboard${qs ? `?${qs}` : ""}`);
  },
  listVesselPositions: (group?: string | null) =>
    request<VesselPositionView[]>("GET", `/api/vessels/positions${group ? `?group=${encodeURIComponent(group)}` : ""}`),
  listNotifications: () => request<NotificationView[]>("GET", "/api/notifications"),
  markNotificationsRead: (ids: string[]) => request<{ marked: number }>("POST", "/api/notifications/mark-read", { ids }),
  setupAdmin: (username: string, password: string) =>
    request<UserView>("POST", "/api/setup/admin", { username, password }),
  login: (username: string, password: string) =>
    request<UserView>("POST", "/api/auth/login", { username, password }),
  logout: () => request<void>("POST", "/api/auth/logout"),
  me: () => request<UserView>("GET", "/api/auth/me"),
  changePassword: (currentPassword: string, newPassword: string) =>
    request<UserView>("POST", "/api/auth/change-password", { currentPassword, newPassword }),

  listVessels: () => request<VesselView[]>("GET", "/api/vessels"),
  createVessel: (imo: string, name: string, type: string, groups: string[]) =>
    request<VesselView>("POST", "/api/vessels", { imo, name, type, groups }),
  getVessel: (id: string) => request<VesselDetailView>("GET", `/api/vessels/${id}`),
  updateVessel: (id: string, name: string, type: string, groups: string[]) =>
    request<VesselView>("PUT", `/api/vessels/${id}`, { name, type, groups }),
  issueEnrollment: (vesselId: string, initialMasterUsername: string) =>
    request<IssueResultView>("POST", `/api/vessels/${vesselId}/enrollment/issue`, { initialMasterUsername }),
  reissueEnrollment: (vesselId: string, initialMasterUsername: string) =>
    request<IssueResultView>("POST", `/api/vessels/${vesselId}/enrollment/reissue`, { initialMasterUsername }),
  revokeEnrollment: (vesselId: string) =>
    request<EnrollmentView>("POST", `/api/vessels/${vesselId}/enrollment/revoke`),
  restoreBundleUrl: (vesselId: string) => `/api/vessels/${vesselId}/restore-bundle`,
  pushRestoreBundle: (vesselId: string, reason: string) =>
    request<RestoreCommandView>("POST", `/api/vessels/${vesselId}/restore-bundle/push`, reason ? { reason } : undefined),

  createVesselUser: (vesselId: string, username: string, role: string) =>
    request<QueuedUserCommandResponse>("POST", `/api/vessels/${vesselId}/users`, { username, role }),
  resetVesselUserPassword: (vesselId: string, username: string) =>
    request<QueuedUserCommandResponse>("POST", `/api/vessels/${vesselId}/users/${encodeURIComponent(username)}/reset-password`),
  setVesselUserRole: (vesselId: string, username: string, role: string) =>
    request<QueuedUserCommandResponse>("PUT", `/api/vessels/${vesselId}/users/${encodeURIComponent(username)}/role`, { role }),
  setVesselUserCanSubmit: (vesselId: string, username: string, canSubmit: boolean) =>
    request<QueuedUserCommandResponse>("PUT", `/api/vessels/${vesselId}/users/${encodeURIComponent(username)}/can-submit`, { canSubmit }),
  deactivateVesselUser: (vesselId: string, username: string) =>
    request<QueuedUserCommandResponse>("POST", `/api/vessels/${vesselId}/users/${encodeURIComponent(username)}/deactivate`),
  reactivateVesselUser: (vesselId: string, username: string) =>
    request<QueuedUserCommandResponse>("POST", `/api/vessels/${vesselId}/users/${encodeURIComponent(username)}/reactivate`),

  listLatestSchemaVersions: () => request<SchemaVersionSummary[]>("GET", "/api/schema-versions"),
  listSchemaVersionHistory: (name: string) => request<SchemaVersionSummary[]>("GET", `/api/schema-versions/${name}/versions`),
  getSchemaVersion: (name: string, version: string) =>
    request<SchemaDetail>("GET", `/api/schema-versions/${name}/versions/${version}`),
  downloadSchemaVersionUrl: (name: string, version: string) => `/api/schema-versions/${name}/versions/${version}/download`,
  previewSchemaUpload: (name: string, content: string) =>
    request<SchemaUploadPreview>("POST", `/api/schema-versions/${name}/preview`, { content }),
  publishSchemaVersion: (name: string, version: string, content: string) =>
    request<SchemaVersionSummary>("POST", `/api/schema-versions/${name}/publish`, { version, content }),

  getFieldPolicy: (name: string, scope: Scope = { type: "fleet" }) => {
    const params = new URLSearchParams({ scopeType: scope.type });
    if (scope.key) params.set("scopeKey", scope.key);
    return request<FieldPolicyView>("GET", `/api/field-policies/${name}?${params.toString()}`);
  },
  saveFieldPolicy: (
    name: string,
    scope: Scope,
    policy: Record<string, string>,
    prefill: Record<string, string>,
    events: Record<string, string[]>,
  ) => request<FieldPolicyView>("PUT", `/api/field-policies/${name}`, { scope, policy, prefill, events }),
  listFieldPolicyAssignments: (name: string) =>
    request<FieldPolicyAssignmentView[]>("GET", `/api/field-policies/${name}/assignments`),

  listProfileAssignments: () => request<ProfileAssignmentView[]>("GET", "/api/compliance/profiles"),
  saveProfileAssignment: (scope: Scope, profiles: string[]) =>
    request<ProfileAssignmentView>("PUT", "/api/compliance/profiles", { scope, profiles }),
  listCadenceRules: () => request<CadenceRuleView[]>("GET", "/api/compliance/cadence"),
  saveCadenceRule: (scope: Scope, minReportIntervalHours: number, maxGapHours: number) =>
    request<CadenceRuleView>("PUT", "/api/compliance/cadence", { scope, minReportIntervalHours, maxGapHours }),
  listOverridableRules: () => request<RuleCatalog>("GET", "/api/compliance/rules"),
  listRuleSeverityAssignments: () => request<RuleSeverityAssignmentView[]>("GET", "/api/compliance/rule-severities"),
  saveRuleSeverityAssignment: (scope: Scope, severities: Record<string, string>) =>
    request<RuleSeverityAssignmentView>("PUT", "/api/compliance/rule-severities", { scope, severities }),

  previewConfigBundle: () => request<ConfigBundleSummary>("GET", "/api/config-bundles/preview"),
  publishConfigBundle: (label: string) => request<ConfigBundleSummary>("POST", "/api/config-bundles/publish", { label }),
  listConfigBundles: () => request<ConfigBundleSummary[]>("GET", "/api/config-bundles"),
  listBundleAssignments: () => request<BundleAssignmentView[]>("GET", "/api/config-bundles/assignments"),
  saveBundleAssignment: (scope: Scope, bundleId: string) =>
    request<BundleAssignmentView>("PUT", "/api/config-bundles/assignments", { scope, bundleId }),
  listVesselConfigs: () => request<VesselConfigView[]>("GET", "/api/vessel-configs"),

  createCommercialReport: (schemaName: string, vesselId: string, fields: Record<string, unknown>) =>
    postAllowingHealthCheckFailure<CreateCommercialReportResponse>(`/api/commercial/${schemaName}/reports`, { vesselId, fields }),

  listReports: (filter?: ReportFilter) => {
    const params = new URLSearchParams();
    if (filter) {
      for (const [key, value] of Object.entries(filter)) {
        if (value !== undefined && value !== "") params.set(key, String(value));
      }
    }
    const qs = params.toString();
    return request<ReportListItemView[]>("GET", `/api/reports${qs ? `?${qs}` : ""}`);
  },
  markReviewed: (items: { vesselId: string; reportId: string }[]) =>
    request<{ marked: number }>("POST", "/api/reports/mark-reviewed", { items }),
  getReport: (vesselId: string, reportId: string) =>
    request<ReportDetailView>("GET", `/api/reports/${vesselId}/${reportId}`),
  listReportEvents: (vesselId: string, reportId: string) =>
    request<ReportEvent[]>("GET", `/api/reports/${vesselId}/${reportId}/events`),
  listReportVersions: (vesselId: string, reportId: string) =>
    request<ReportView[]>("GET", `/api/reports/${vesselId}/${reportId}/versions`),
  listChat: (vesselId: string, reportId: string) =>
    request<ChatMessageView[]>("GET", `/api/reports/${vesselId}/${reportId}/chat`),
  postChat: (vesselId: string, reportId: string, body: string) =>
    request<ChatMessageView>("POST", `/api/reports/${vesselId}/${reportId}/chat`, { body }),
  listRemarks: (vesselId: string, reportId: string) =>
    request<RemarkView[]>("GET", `/api/reports/${vesselId}/${reportId}/remarks`),
  createRemarkSet: (vesselId: string, reportId: string, remarks: RemarkFieldInput[]) =>
    request<RemarkView[]>("POST", `/api/reports/${vesselId}/${reportId}/remarks`, { remarks }),
  setRemarkResolved: (id: string, resolved: boolean) =>
    request<{ resolved: boolean }>("PATCH", `/api/remarks/${id}`, { resolved }),
  listAttachments: (vesselId: string, reportId: string) =>
    request<AttachmentView[]>("GET", `/api/reports/${vesselId}/${reportId}/attachments`),
  /** Not fetched through request<T> — used directly as an <img src>/<a href> so the browser streams the response itself. */
  attachmentDownloadUrl: (vesselId: string, reportId: string, attachmentId: string) =>
    `/api/reports/${vesselId}/${reportId}/attachments/${attachmentId}`,

  getSystem: () => request<SystemView>("GET", "/api/system"),

  listUsers: () => request<UserView[]>("GET", "/api/users"),
  createUser: (username: string, roles: Role[]) =>
    request<CreateUserResponse>("POST", "/api/users", { username, roles }),
  updateUserRoles: (id: string, roles: Role[]) =>
    request<UserView>("PUT", `/api/users/${id}/roles`, { roles }),
  deactivateUser: (id: string) => request<UserView>("POST", `/api/users/${id}/deactivate`),
  reactivateUser: (id: string) => request<UserView>("POST", `/api/users/${id}/reactivate`),
  resetUserPassword: (id: string) => request<CreateUserResponse>("POST", `/api/users/${id}/reset-password`),

  listApiKeys: () => request<ApiKeyView[]>("GET", "/api/api-keys"),
  createApiKey: (label: string, groupId?: string | null) =>
    request<CreateApiKeyResponse>("POST", "/api/api-keys", { label, groupId: groupId ?? null }),
  revokeApiKey: (id: string) => request<{ revoked: boolean }>("POST", `/api/api-keys/${id}/revoke`),
  deleteApiKey: (id: string) => request<{ deleted: boolean }>("DELETE", `/api/api-keys/${id}`),
  listApiKeyEvents: (id: string) => request<ApiKeyEventView[]>("GET", `/api/api-keys/${id}/events`),

  renameVesselGroup: (from: string, to: string) =>
    request<VesselGroupMutationResponse>("POST", "/api/vessel-groups/rename", { from, to }),
  deleteVesselGroup: (group: string) =>
    request<VesselGroupMutationResponse>("POST", "/api/vessel-groups/delete", { group }),
};
