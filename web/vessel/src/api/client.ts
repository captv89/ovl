// SPDX-License-Identifier: AGPL-3.0-only

// Typed client for vessel/httpapi's JSON API. Shapes here must match
// vessel/httpapi's Go structs exactly (server.go's userView,
// setup.go's setupStatusResponse, vessel/bootstrap's Config/Enrollment).

export type Mode = "standalone" | "server";

export interface Enrollment {
  officeURL: string;
  code: string;
  submitted: boolean;
}

export interface SetupStatus {
  configured: boolean;
  mode: Mode | "";
  dataDir: string;
  defaultDataDir: string;
  enrollment: Enrollment;
  hasMaster: boolean;
}

export type Role =
  | "master"
  | "chiefOfficer"
  | "secondOfficer"
  | "thirdOfficer"
  | "chiefEngineer"
  | "secondEngineer";

export interface UserView {
  id: string;
  username: string;
  role: Role;
  canSubmit: boolean;
  mustChangePassword: boolean;
}

// Mirrors pkg/schema.FieldType exactly.
export type FieldType = "text" | "wholeNumber" | "decimal" | "date" | "time" | "dateTime" | "boolean" | "enum";

// Mirrors pkg/schema.Field exactly.
export interface SchemaField {
  name: string;
  label: string;
  type: FieldType;
  unit?: string;
  maxLength?: number;
  enumRef?: string;
  schemaMandatory: boolean;
  mandatoryNote?: string;
  relevance: string;
  section: string;
  appliesToEvents: string[];
  description?: string;
  unsupportedByDnv?: boolean;
}

// Mirrors pkg/schema.Schema exactly.
export interface Schema {
  schemaName: string;
  ovdVersion: string;
  version: string;
  sections?: string[];
  fields: SchemaField[];
}

// Mirrors pkg/validation.FieldPolicyState's string values.
export type FieldPolicyState = "hidden" | "optional" | "recommended" | "companyMandatory" | "schemaMandatory";

export type PrefillClass = "carryForward" | "computed" | "ghost" | "none";

// Mirrors vessel/httpapi's prefillEntry.
export interface PrefillEntry {
  class: PrefillClass;
  value?: string | number | boolean;
  formula?: string;
}

// Mirrors vessel/httpapi's robSeriesView — which fields are ROB
// continuity tracks for this schema (pkg/validation.Config.ROBSeriesList,
// the same list runCascade checks against), so the form can recompute the
// same ROB(n) = ROB(n-1) - sum(consumption fields)(n) formula live as the
// officer enters consumption, rather than duplicating a hardcoded series
// list. consumptionFields is plural — a real ROB tank is drawn down by
// more than one consumer (Main Engine, Auxiliary Engine, Boiler, ...),
// see pkg/validation.ROBSeries's own doc comment.
export interface RobSeries {
  name: string;
  robField: string;
  consumptionFields: string[];
}

// Mirrors vessel/httpapi's schemaConfigResponse.
export interface SchemaConfig {
  schema: Schema;
  fieldPolicy: Record<string, FieldPolicyState>;
  /**
   * Narrows which voyage event types each field's policy applies to
   * (validation.FieldEvents). A field absent from the map applies to every
   * event; a field listed here is hidden on any report whose event type is
   * not in its list. Absent entirely when the company has narrowed nothing.
   */
  fieldEvents?: Record<string, string[]>;
  prefill: Record<string, PrefillEntry>;
  robSeries?: RobSeries[];
  /** The vessel's last submitted report's EventTime for this schema, if one exists — lets Time_Since_Previous_Report recompute live. */
  lastReportEventTime?: string;
}

// Mirrors vessel/httpapi's sensorSourceView (18.07.26 manual-test items
// 4/9's vessel-pulls-from-a-sensor-service architecture). apiKey is
// always masked (last 4 characters) when it comes back from the server —
// see that type's own doc comment on why.
export interface SensorSourceView {
  baseUrl: string;
  apiKey: string;
  enabled: boolean;
  configured: boolean;
}

// Mirrors vessel/httpapi's testSensorSourceResponse — Settings' "Test
// connection" action. Always a 200 with ok/message; ApiError is reserved
// for request-shape problems (missing baseUrl, no apiKey to test with).
export interface SensorSourceTestResult {
  ok: boolean;
  message: string;
}

// Mirrors vessel/httpapi's fetchSensorDataResponse — a flat schema-field
// -> value map, merged into ReportForm's own `values` the same way any
// other field change would be, not a distinct "sensor" concept the form
// needs to track separately.
export interface FetchSensorDataResult {
  fields: Record<string, string | number | boolean>;
}

// Mirrors vessel/httpapi's vmsSourceView — the VMS (voyage management
// system) half of the sensor+VMS stub expansion. Independent
// configure/enable/fail state from SensorSourceView, not merged with it
// — see that design doc's own "separate, not merged" decision.
export interface VMSSourceView {
  baseUrl: string;
  apiKey: string;
  enabled: boolean;
  configured: boolean;
}

// Mirrors vessel/httpapi's testVMSSourceResponse.
export interface VMSSourceTestResult {
  ok: boolean;
  message: string;
}

// Mirrors vessel/httpapi's fetchVMSDataResponse — a flat schema-field ->
// value map, merged into ReportForm's own `values` the same way
// FetchSensorDataResult is.
export interface FetchVMSDataResult {
  fields: Record<string, string | number | boolean>;
}

// Mirrors pkg/domain.State's string values.
export type ReportState = "draft" | "ready" | "submitted" | "synced" | "pushed" | "remarked" | "invalidated";

// Mirrors vessel/httpapi's reportView.
export interface ReportView {
  reportId: string;
  versionNo: number;
  schemaName: string;
  eventType: string;
  eventTime: string;
  fields: Record<string, unknown>;
  state: ReportState;
  invalidatedFrom?: ReportState;
  invalidatedRules?: string[];
  createdAt: string;
  createdBy: string;
  updatedAt: string;
  submittedAt?: string;
  submittedBy?: string;
}

// Mirrors vessel/httpapi's voyagePositionView.
export interface VoyagePosition {
  latitude: number;
  longitude: number;
  text: string;
}

// Mirrors vessel/httpapi's voyageSummaryView — a read-model derived from
// the latest log-abstract reports (Voyage_Number/From/To/ETA/position/
// distance fields), not a separately stored entity. Null when the vessel
// has no log-abstract reports yet.
export interface VoyageSummary {
  imo?: string;
  voyageNumber?: string;
  fromPort?: string;
  toPort?: string;
  eta?: string;
  departedAt?: string;
  lastReportAt: string;
  position?: VoyagePosition;
  distanceSailedNm?: number;
  distanceRemainingNm?: number;
  progressPercent?: number;
}

// Mirrors vessel/httpapi's syncStatusView — ephemeral, not persisted (a
// vessel restart starts this fresh), reflecting the outcome of the most
// recent sync cycle, manual or scheduled.
export interface SyncStatus {
  enrolled: boolean;
  lastAttempt?: string;
  lastSuccess?: string;
  lastError?: string;
  officeTime?: string;
}

// Mirrors vessel/httpapi's syncSettingsView.
export interface SyncSettings {
  syncIntervalSeconds: number;
}

// Mirrors vessel/httpapi's vesselConfigView — the applied config bundle's
// operational settings the SPA needs directly (cadence ceiling, enabled
// regulatory profiles). bundleId/bundleVersionNo are empty until a bundle
// has been synced.
export interface VesselConfig {
  maxGapHours: number;
  regulatoryProfiles: string[];
  bundleId?: string;
  bundleVersionNo?: number;
}

// Mirrors pkg/validation.Severity's string values.
export type Severity = "error" | "warning" | "info";

// Mirrors vessel/httpapi's findingView.
export interface Finding {
  ruleId: string;
  severity: Severity;
  field?: string;
  message: string;
}

// Mirrors pkg/validation.RegulatoryProfile's string values.
export type RegulatoryProfile = "mrv" | "dcs" | "cii" | "voyageVerification";

// Mirrors vessel/httpapi's profileReadinessView.
export interface ProfileReadiness {
  profile: RegulatoryProfile;
  ready: boolean;
  missingFields: string[];
}

// Mirrors vessel/httpapi's continuityImpactView.
export interface ContinuityImpact {
  reportId: string;
  eventType: string;
  eventTime: string;
  invalidatedRules: string[];
}

// Mirrors vessel/httpapi's checkResponse.
export interface CheckResult {
  report: ReportView;
  findings: Finding[];
  regulatoryReadiness: ProfileReadiness[];
  continuityImpact: ContinuityImpact[];
}

// Mirrors vessel/httpapi's validateResponse.
export interface ValidateResult {
  findings: Finding[];
}

// Mirrors vessel/httpapi's eventTypeView.
export interface EventType {
  code: string;
  remark?: string;
}

// Mirrors vessel/httpapi's eventSuggestionView.
export interface EventSuggestion {
  after: string;
  suggest: string[];
}

// Mirrors vessel/httpapi's enumValuesView.
export interface EnumValues {
  values: string[];
}

// Mirrors pkg/domain.EventType's string values (architecture 14).
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

// Mirrors vessel/httpapi's eventView.
export interface ReportEvent {
  versionNo: number;
  type: DomainEventType;
  at: string;
  actor?: string;
  detail?: Record<string, unknown>;
}

// Mirrors pkg/domain.MaxChatBodyBytes. Duplicated here (Go and TS can't
// share a constant directly) — if the Go value ever changes, update
// this too.
export const MAX_CHAT_BODY_BYTES = 4096;

// Mirrors pkg/domain.ChatDirection's string values.
export type ChatDirection = "vessel" | "office";

// Mirrors vessel/httpapi's chatMessageView (design handoff A8's
// per-report chat wall).
export interface ChatMessageView {
  id: string;
  reportId: string;
  sender: string;
  body: string;
  sentAt: string;
  direction: ChatDirection;
}

// Mirrors vessel/httpapi's invalidationNoticeView (design handoff A7's
// notices strip, Phase 5 Slice S4).
export interface InvalidationNoticeView {
  versionNo: number;
  brokenRules: string[];
  computedAt: string;
}

// Mirrors vessel/httpapi's remarkView (design handoff A7's Remarks tab).
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

// Mirrors vessel/httpapi's attachmentView (design handoff A5·B/A7's
// Attachments section, architecture 15: Bunker/EDN report attachments).
export interface AttachmentView {
  id: string;
  reportId: string;
  versionNo: number;
  fieldName: string;
  filename: string;
  contentType: string;
  sizeBytes: number;
  uploadedAt: string;
  uploadedBy: string;
  synced: boolean;
}

// Mirrors vessel/httpapi's backupInfo (architecture 9.6: local DR).
export interface Backup {
  id: string;
  createdAt: string;
}

// Mirrors vessel/httpapi's adminUserView (design handoff A9). Never
// includes a password hash.
export interface AdminUserView extends UserView {
  active: boolean;
  createdAt: string;
  updatedAt: string;
}

// Mirrors vessel/httpapi's createUserResponse.
export interface CreateUserResult {
  user: AdminUserView;
  temporaryPassword: string;
}

// Mirrors vessel/httpapi's resetPasswordResponse.
export interface ResetPasswordResult {
  temporaryPassword: string;
}

// Mirrors vessel/httpapi's schemaInfo (design handoff A10 diagnostics).
export interface SchemaInfo {
  schemaName: string;
  version: string;
  ovdVersion: string;
}

// Mirrors vessel/httpapi's systemInfoResponse.
export interface SystemInfo {
  version: string;
  commit: string;
  date: string;
  schemas: SchemaInfo[];
}

// Mirrors vessel/httpapi's lockView (architecture 9.5: section soft locks).
export interface SectionLock {
  section: string;
  userId: string;
  username: string;
  role: Role;
  acquiredAt: string;
  expiresAt: string;
}

// Mirrors vessel/httpapi's lockConflictResponse — the 409 body shape from
// both POST .../locks/{section} and PATCH .../reports/{id} when the
// target section is held by someone else. Carried on ApiError.body so
// callers can read `.lock` off a caught conflict.
export interface LockConflict {
  error: string;
  lock: SectionLock;
}

export class ApiError extends Error {
  status: number;
  /** The full decoded error response body, if any — lock-conflict callers read `.lock` off this. */
  body: unknown;
  constructor(status: number, message: string, body?: unknown) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

async function request<T>(method: string, path: string, body?: unknown, keepalive?: boolean): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: "same-origin",
    keepalive,
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
    throw new ApiError(res.status, message, data);
  }
  return data as T;
}

// uploadAttachmentFile posts a multipart/form-data body — separate from
// request<T> since that helper only ever sends JSON. The browser sets
// the multipart Content-Type (with boundary) itself when the body is a
// FormData and no Content-Type header is supplied, so none is set here.
async function uploadAttachmentFile(reportId: string, file: Blob, filename: string, fieldName?: string): Promise<AttachmentView> {
  const form = new FormData();
  form.set("file", file, filename);
  if (fieldName) form.set("fieldName", fieldName);
  const res = await fetch(`/api/reports/${encodeURIComponent(reportId)}/attachments`, {
    method: "POST",
    body: form,
    credentials: "same-origin",
  });
  const data: unknown = await res.json().catch(() => null);
  if (!res.ok) {
    const message =
      data && typeof data === "object" && "error" in data
        ? String((data as { error: unknown }).error)
        : `request failed with status ${res.status}`;
    throw new ApiError(res.status, message, data);
  }
  return data as AttachmentView;
}

export const api = {
  getSetupStatus: () => request<SetupStatus>("GET", "/api/setup/status"),
  setupMode: (mode: Mode, dataDir: string) =>
    request<SetupStatus>("POST", "/api/setup/mode", { mode, dataDir }),
  setupEnrollment: (input: { officeURL?: string; code?: string; skip: boolean }) =>
    request<SetupStatus>("POST", "/api/setup/enrollment", input),
  setupMaster: (username: string, password: string) =>
    request<UserView>("POST", "/api/setup/master", { username, password }),
  login: (username: string, password: string) =>
    request<UserView>("POST", "/api/auth/login", { username, password }),
  logout: () => request<void>("POST", "/api/auth/logout"),
  me: () => request<UserView>("GET", "/api/auth/me"),
  changePassword: (currentPassword: string, newPassword: string) =>
    request<UserView>("POST", "/api/auth/change-password", { currentPassword, newPassword }),
  getSchema: (name: string) => request<SchemaConfig>("GET", `/api/schemas/${encodeURIComponent(name)}`),
  listEventTypes: () => request<EventType[]>("GET", "/api/event-types"),
  listEventSuggestions: () => request<EventSuggestion[]>("GET", "/api/event-suggestions"),
  getEnum: (name: string) => request<EnumValues>("GET", `/api/enums/${encodeURIComponent(name)}`),
  createReport: (input: { schemaName: string; eventType: string; eventTime: string; fields: Record<string, unknown> }) =>
    request<ReportView>("POST", "/api/reports", input),
  getReport: (id: string) => request<ReportView>("GET", `/api/reports/${encodeURIComponent(id)}`),
  /** Draft/ready only — a submitted-or-later report 409s (vessel/httpapi's loadEditableReport). */
  deleteReport: (id: string) => request<{ deleted: boolean }>("DELETE", `/api/reports/${encodeURIComponent(id)}`),
  listReportEvents: (id: string) => request<ReportEvent[]>("GET", `/api/reports/${encodeURIComponent(id)}/events`),
  listReportVersions: (id: string) => request<ReportView[]>("GET", `/api/reports/${encodeURIComponent(id)}/versions`),
  getChat: (reportId: string) => request<ChatMessageView[]>("GET", `/api/reports/${encodeURIComponent(reportId)}/chat`),
  postChat: (reportId: string, body: string) =>
    request<ChatMessageView>("POST", `/api/reports/${encodeURIComponent(reportId)}/chat`, { body }),
  listInvalidationNotices: (reportId: string) =>
    request<InvalidationNoticeView[]>("GET", `/api/reports/${encodeURIComponent(reportId)}/invalidation-notices`),
  listRemarks: (reportId: string) =>
    request<RemarkView[]>("GET", `/api/reports/${encodeURIComponent(reportId)}/remarks`),
  listReports: (schemaName: string) => request<ReportView[]>("GET", `/api/reports?schema=${encodeURIComponent(schemaName)}`),
  saveSection: (id: string, section: string, changes: Record<string, unknown>) =>
    request<ReportView>("PATCH", `/api/reports/${encodeURIComponent(id)}`, { section, changes }),
  validateReport: (id: string, fields: Record<string, unknown>) =>
    request<ValidateResult>("POST", `/api/reports/${encodeURIComponent(id)}/validate`, { fields }),
  checkReport: (id: string) => request<CheckResult>("POST", `/api/reports/${encodeURIComponent(id)}/check`),
  acknowledgeFinding: (id: string, body: { ruleId: string; field?: string; message: string; acknowledged: boolean }) =>
    request<{ acknowledged: boolean }>("POST", `/api/reports/${encodeURIComponent(id)}/findings/acknowledge`, body),
  submitReport: (id: string) => request<ReportView>("POST", `/api/reports/${encodeURIComponent(id)}/submit`),
  startCorrection: (id: string) => request<ReportView>("POST", `/api/reports/${encodeURIComponent(id)}/correction`),
  getCurrentVoyage: () => request<VoyageSummary | null>("GET", "/api/voyage/current"),
  getSensorSource: () => request<SensorSourceView>("GET", "/api/settings/sensor-source"),
  saveSensorSource: (input: { baseUrl: string; apiKey: string; enabled: boolean }) =>
    request<SensorSourceView>("PUT", "/api/settings/sensor-source", input),
  testSensorSource: (input: { baseUrl: string; apiKey: string }) =>
    request<SensorSourceTestResult>("POST", "/api/settings/sensor-source/test", input),
  fetchSensorData: (schemaName: string, eventTime: string) =>
    request<FetchSensorDataResult>("POST", `/api/schemas/${encodeURIComponent(schemaName)}/fetch-sensor-data`, { eventTime }),
  getVMSSource: () => request<VMSSourceView>("GET", "/api/settings/vms-source"),
  saveVMSSource: (input: { baseUrl: string; apiKey: string; enabled: boolean }) =>
    request<VMSSourceView>("PUT", "/api/settings/vms-source", input),
  testVMSSource: (input: { baseUrl: string; apiKey: string }) =>
    request<VMSSourceTestResult>("POST", "/api/settings/vms-source/test", input),
  fetchVMSData: (schemaName: string, eventTime: string) =>
    request<FetchVMSDataResult>("POST", `/api/schemas/${encodeURIComponent(schemaName)}/fetch-vms-data`, { eventTime }),
  getSyncStatus: () => request<SyncStatus>("GET", "/api/sync/status"),
  /** Throws ApiError(400) if not enrolled, ApiError(502) with the failure message if the cycle ran but failed. */
  syncNow: () => request<SyncStatus>("POST", "/api/sync/now"),
  getVesselConfig: () => request<VesselConfig>("GET", "/api/vessel-config"),
  getSyncSettings: () => request<SyncSettings>("GET", "/api/settings/sync"),
  saveSyncSettings: (syncIntervalSeconds: number) =>
    request<SyncSettings>("PUT", "/api/settings/sync", { syncIntervalSeconds }),
  listBackups: () => request<Backup[]>("GET", "/api/admin/backups"),
  snapshotNow: () => request<Backup>("POST", "/api/admin/backups"),
  restoreBackup: (id: string) => request<{ restored: boolean }>("POST", `/api/admin/backups/${encodeURIComponent(id)}/restore`, { confirm: true }),
  getSystemInfo: () => request<SystemInfo>("GET", "/api/system/info"),
  listUsers: () => request<AdminUserView[]>("GET", "/api/admin/users"),
  createUser: (username: string, role: Role) =>
    request<CreateUserResult>("POST", "/api/admin/users", { username, role }),
  resetUserPassword: (id: string) =>
    request<ResetPasswordResult>("POST", `/api/admin/users/${encodeURIComponent(id)}/reset-password`),
  updateUser: (id: string, patch: { canSubmit?: boolean; active?: boolean }) =>
    request<AdminUserView>("PATCH", `/api/admin/users/${encodeURIComponent(id)}`, patch),
  // Also serves as the renewal call for a lock the caller already holds
  // (architecture 9.5) — acquiring your own live lock just extends it.
  acquireLock: (reportId: string, section: string) =>
    request<SectionLock>("POST", `/api/reports/${encodeURIComponent(reportId)}/locks/${encodeURIComponent(section)}`),
  // beacon uses fetch's keepalive so the release request can outlive the
  // page (tab close/navigate-away, see ReportForm's pagehide handler).
  releaseLock: (reportId: string, section: string, beacon = false) =>
    request<void>("DELETE", `/api/reports/${encodeURIComponent(reportId)}/locks/${encodeURIComponent(section)}`, undefined, beacon),
  forceReleaseLock: (reportId: string, section: string) =>
    request<void>("POST", `/api/reports/${encodeURIComponent(reportId)}/locks/${encodeURIComponent(section)}/force-release`),
  listLocks: (reportId: string) =>
    request<SectionLock[]>("GET", `/api/reports/${encodeURIComponent(reportId)}/locks`),
  listAttachments: (reportId: string) =>
    request<AttachmentView[]>("GET", `/api/reports/${encodeURIComponent(reportId)}/attachments`),
  uploadAttachment: (reportId: string, file: Blob, filename: string, fieldName?: string) =>
    uploadAttachmentFile(reportId, file, filename, fieldName),
  deleteAttachment: (reportId: string, attachmentId: string) =>
    request<{ deleted: boolean }>("DELETE", `/api/reports/${encodeURIComponent(reportId)}/attachments/${encodeURIComponent(attachmentId)}`),
  /** Not fetched through request<T> — used directly as an <img src>/<a href> so the browser streams the response itself. */
  attachmentDownloadUrl: (reportId: string, attachmentId: string) =>
    `/api/reports/${encodeURIComponent(reportId)}/attachments/${encodeURIComponent(attachmentId)}`,
};
