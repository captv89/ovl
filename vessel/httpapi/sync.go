// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/syncproto"
	"github.com/captv89/ovl/vessel/store"
	ovlsync "github.com/captv89/ovl/vessel/sync"
)

// syncStatusView is GET /api/sync/status's shape and the in-memory
// record RunSyncCycle updates. Ephemeral, not persisted: a restart
// starts this fresh, same as vessel/main.go's nightly backup scheduler
// carrying no history across restarts either.
type syncStatusView struct {
	Enrolled    bool       `json:"enrolled"`
	LastAttempt *time.Time `json:"lastAttempt,omitempty"`
	LastSuccess *time.Time `json:"lastSuccess,omitempty"`
	LastError   string     `json:"lastError,omitempty"`
	OfficeTime  *time.Time `json:"officeTime,omitempty"`
}

// defaultSyncInterval is used before first-run setup completes (no store
// yet to read a configured value from) and matches migration
// 00015_vessel_settings.sql's own seeded default.
const defaultSyncInterval = 5 * time.Minute

// SyncIntervalDuration returns the vessel's currently configured sync
// interval (architecture 11.2's "configurable interval") for
// vessel/main.go's runSyncScheduler to wait on between cycles. Falls
// back to defaultSyncInterval before setup completes or on any store
// error — the scheduler must always have some interval to wait on, and a
// missing/misread setting is never a reason to stop syncing entirely.
func (s *Server) SyncIntervalDuration() time.Duration {
	st := s.storeOrNil()
	if st == nil {
		return defaultSyncInterval
	}
	seconds, err := st.GetSyncIntervalSeconds(context.Background())
	if err != nil {
		return defaultSyncInterval
	}
	return time.Duration(seconds) * time.Second
}

// syncClientOrNil builds a client from the vessel's redeemed credential
// and enrolled office URL, or (false) if the vessel isn't enrolled yet
// (architecture 9.2: enrollment is deferrable, "offline install must be
// possible") or hasn't finished first-run setup at all.
func (s *Server) syncClientOrNil() (*ovlsync.Client, bool) {
	cfg := s.config()
	if cfg == nil || !cfg.Enrollment.Submitted {
		return nil, false
	}
	st := s.storeOrNil()
	if st == nil {
		return nil, false
	}
	cred, err := st.GetSyncCredential(context.Background())
	if err != nil {
		return nil, false
	}
	return ovlsync.NewClient(nil, cfg.Enrollment.OfficeURL, cred.Token), true
}

// RunSyncCycle performs one sync cycle (architecture 11.2): PushOutbox,
// PushAttachments, then PullInbox, then SyncStatus. A vessel that isn't
// enrolled is a silent no-op, not an error — the interval scheduler runs
// unconditionally from boot the same way RunNightlySnapshot does. Each
// phase runs regardless of whether an earlier one failed — they're
// independent RPCs, and a failure in one just means its own work stays
// pending for the next cycle; the first error encountered (in phase
// order) is what's reported as this cycle's LastError.
func (s *Server) RunSyncCycle(ctx context.Context) syncStatusView {
	client, ok := s.syncClientOrNil()
	st := s.storeOrNil()
	if !ok || st == nil {
		s.syncMu.Lock()
		s.syncStat = syncStatusView{Enrolled: false}
		status := s.syncStat
		s.syncMu.Unlock()
		return status
	}

	now := time.Now().UTC()
	s.syncMu.Lock()
	s.syncStat.Enrolled = true
	s.syncStat.LastAttempt = &now
	s.syncMu.Unlock()

	pushErr := s.pushOutboxBatch(ctx, st, client)
	attachErr := s.pushAttachmentsBatch(ctx, st, client)
	appliedRestoreCommandIDs, appliedUserCommandIDs, pullErr := s.pullInboxBatch(ctx, st, client)
	users, rosterErr := buildVesselUserSummaries(ctx, st)
	if rosterErr != nil && pullErr == nil {
		pullErr = rosterErr
	}
	// Report which config bundle this vessel is actually running on
	// (audit 2026-07-22 §3.3) so office's B2 can distinguish "assigned" from
	// "applied." Best-effort: a read failure here must not sink the whole
	// status call, which still carries app version, roster and acks.
	var appliedBundleID string
	var appliedBundleVersion int64
	if cb, ok, err := st.LatestConfigBundle(ctx); err == nil && ok {
		appliedBundleID, appliedBundleVersion = cb.BundleID, cb.VersionNo
	}
	serverTime, statusErr := client.SyncStatus(ctx, s.buildInfo.Version, appliedRestoreCommandIDs, appliedUserCommandIDs, users, appliedBundleID, appliedBundleVersion)

	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	switch {
	case pushErr != nil:
		s.syncStat.LastError = pushErr.Error()
	case attachErr != nil:
		s.syncStat.LastError = attachErr.Error()
	case pullErr != nil:
		s.syncStat.LastError = pullErr.Error()
	case statusErr != nil:
		s.syncStat.LastError = statusErr.Error()
	default:
		s.syncStat.LastError = ""
		s.syncStat.LastSuccess = &now
	}
	if statusErr == nil {
		s.syncStat.OfficeTime = &serverTime
	}
	return s.syncStat
}

// attachmentPushChunkSize is the chunk size this vessel offers attachment
// bytes in over UploadAttachmentChunk — arbitrary but fixed, matching
// office/syncservice's own chunkCount math (any positive value works;
// this just keeps individual RPC payloads modest).
const attachmentPushChunkSize = 256 << 10

// pushAttachmentsBatch sends every attachment not yet confirmed synced
// (architecture 11.2 step 2, "chunked, resumable ... only chunks the
// office confirms missing are sent") using the vessel/sync transport
// vessel/sync/attachments.go already implements and tests — this is the
// automatic caller that package's own doc comment says was "ready for
// whenever attachment capture lands" (Phase 6). Uses a dedicated
// synced_at column rather than the shared outbox table: attachment
// transfer is chunked binary RPC, not the JSON-envelope PushOutbox every
// other outbox kind rides, so the two don't share a queue.
func (s *Server) pushAttachmentsBatch(ctx context.Context, st *store.Store, client *ovlsync.Client) error {
	pending, err := st.ListPendingAttachments(ctx)
	if err != nil {
		return fmt.Errorf("list pending attachments: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}
	astore, err := s.attachmentStore()
	if err != nil {
		return err
	}
	var firstErr error
	for _, a := range pending {
		meta := &syncv1.AttachmentMeta{
			ContentHash: a.ContentHash, ReportId: a.ReportID, VersionNo: int32(a.VersionNo), // #nosec G115 -- report version numbers stay far below 2^31 in practice
			FieldName: a.FieldName, TotalSize: a.SizeBytes, ChunkSize: attachmentPushChunkSize, ContentType: a.ContentType,
			Filename: a.Filename,
		}
		if err := client.UploadAttachment(ctx, astore, meta); err != nil {
			// A single stuck attachment shouldn't block the rest of the
			// batch from even being attempted; it just stays pending
			// (and keeps failing) until whatever's wrong is fixed.
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := st.MarkAttachmentSynced(ctx, a.ID, time.Now().UTC()); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// pullInboxBatch requests everything newer than this vessel's current
// cursors, auto-fetches and applies any restore commands (architecture
// 12.5's DR push path — see applyPulledRestoreCommands' own doc comment
// on why its cursor advance is handled separately from the rest),
// applies any user-management commands (architecture 9.3/12.4's remote
// user administration — see applyPulledUserCommands' own doc comment),
// and applies everything else atomically (vessel/store.ApplyInboxPull)
// — see that method's own doc comment for why that store write and
// cursor advance happen in one transaction. Returns the restore/user
// command ids successfully applied this cycle, for RunSyncCycle to
// report back to office via SyncStatus.
func (s *Server) pullInboxBatch(ctx context.Context, st *store.Store, client *ovlsync.Client) (appliedRestoreCommandIDs, appliedUserCommandIDs []string, err error) {
	current, err := st.GetInboxCursors(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get inbox cursors: %w", err)
	}
	resp, err := client.PullInbox(ctx, &syncv1.SyncCursors{
		ConfigBundleCursor:       current.ConfigBundleCursor,
		SchemaVersionCursor:      current.SchemaVersionCursor,
		RemarkCursor:             current.RemarkCursor,
		ChatCursor:               current.ChatCursor,
		InvalidationNoticeCursor: current.InvalidationNoticeCursor,
		RestoreCommandCursor:     current.RestoreCommandCursor,
		UserCommandCursor:        current.UserCommandCursor,
	})
	if err != nil {
		return nil, nil, err
	}

	schemaVersions := make([]store.PulledSchemaVersion, 0, len(resp.GetSchemaVersions()))
	for _, pv := range resp.GetSchemaVersions() {
		schemaName, err := syncproto.SchemaNameFromKind(pv.GetSchemaKind())
		if err != nil {
			return nil, nil, err
		}
		schemaVersions = append(schemaVersions, store.PulledSchemaVersion{
			SchemaName: schemaName, Version: pv.GetVersion(), Content: pv.GetSchemaJson(), PublishedAt: pv.GetPublishedAt().AsTime(),
		})
	}
	configBundles := make([]store.PulledConfigBundle, 0, len(resp.GetConfigBundles()))
	for _, pb := range resp.GetConfigBundles() {
		configBundles = append(configBundles, store.PulledConfigBundle{
			BundleID: pb.GetBundleId(), VersionNo: pb.GetVersionNo(), Content: pb.GetContentJson(), PublishedAt: pb.GetPublishedAt().AsTime(),
		})
	}
	chatMessages := make([]domain.ChatMessage, 0, len(resp.GetChatMessages()))
	for _, pm := range resp.GetChatMessages() {
		chatMessages = append(chatMessages, syncproto.ChatMessageToDomain(pm, domain.ChatFromOffice))
	}
	invalidationNotices := make([]domain.InvalidationNotice, 0, len(resp.GetInvalidationNotices()))
	for _, pn := range resp.GetInvalidationNotices() {
		invalidationNotices = append(invalidationNotices, syncproto.InvalidationNoticeToDomain(pn))
	}
	remarks := make([]domain.Remark, 0, len(resp.GetRemarks()))
	for _, pr := range resp.GetRemarks() {
		remarks = append(remarks, syncproto.RemarkToDomain(pr))
	}

	appliedRestoreCommandIDs, restoreCommandCursor := s.applyPulledRestoreCommands(ctx, st, client, resp.GetRestoreCommands(), current.RestoreCommandCursor, resp.GetNextCursors().GetRestoreCommandCursor())
	appliedUserCommandIDs, userCommandCursor := applyPulledUserCommands(ctx, st, resp.GetUserCommands(), resp.GetNextCursors().GetUserCommandCursor())

	next := resp.GetNextCursors()
	if err := st.ApplyInboxPull(ctx, schemaVersions, configBundles, chatMessages, invalidationNotices, remarks, store.InboxCursors{
		ConfigBundleCursor:       next.GetConfigBundleCursor(),
		SchemaVersionCursor:      next.GetSchemaVersionCursor(),
		RemarkCursor:             next.GetRemarkCursor(),
		ChatCursor:               next.GetChatCursor(),
		InvalidationNoticeCursor: next.GetInvalidationNoticeCursor(),
		RestoreCommandCursor:     restoreCommandCursor,
		UserCommandCursor:        userCommandCursor,
	}); err != nil {
		return appliedRestoreCommandIDs, appliedUserCommandIDs, err
	}
	return appliedRestoreCommandIDs, appliedUserCommandIDs, nil
}

// applyPulledUserCommands is architecture 9.3/12.4's remote vessel-user-
// administration path, apply half: applies each pulled UserCommand
// independently and advances the cursor past every one of them
// regardless of individual success or failure — deliberately different
// from applyPulledRestoreCommands' all-or-nothing batch model. A restore
// command's own internal apply already isn't atomic (documented
// AppendEvent caveat), so batch-level care mattered less there; a user
// command is a single atomic local mutation (one row created/updated or
// not), and CREATE specifically is not retry-safe (a second attempt
// against an already-created username fails on the UNIQUE constraint,
// which would wedge an all-or-nothing batch forever on its own past
// success). A command that fails here simply never reports as applied —
// office's Users tab keeps showing "queued" for it, the correct signal
// for an Admin to notice and investigate, rather than silently retrying
// an operation that will never succeed.
func applyPulledUserCommands(ctx context.Context, st *store.Store, commands []*syncv1.UserCommand, nextCursor int64) (appliedIDs []string, cursor int64) {
	for _, cmd := range commands {
		if err := applyUserCommand(ctx, st, cmd); err != nil {
			continue
		}
		appliedIDs = append(appliedIDs, cmd.GetCommandId())
	}
	return appliedIDs, nextCursor
}

// applyPulledRestoreCommands is architecture 12.5's DR push path,
// auto-fetch half: for each restore command this cycle's PullInbox
// returned, fetches the encrypted bundle (FetchRestoreBundle RPC),
// decrypts and applies it (vessel/httpapi's decryptRestoreBundle/
// applyRestoreBundle, shared with the Master-facing manual import
// endpoint). Unlike the other PullInbox streams, the restore command
// cursor is deliberately NOT advanced unconditionally alongside the rest
// of this cycle's ApplyInboxPull call: a restore command carries no
// locally-durable row of its own the way a remark or chat message does
// (there is nothing to safely re-derive it from if a later command in
// the same batch fails), so advancing past a command this vessel never
// actually applied would lose it permanently. Instead: the cursor only
// advances to nextCursor once every command in this batch has been
// applied successfully; a failure partway through leaves the cursor
// where it was, so the whole batch (including anything already applied
// this cycle) is retried next cycle — safe because SaveReport/
// InsertChatMessage/ApplyConfigBundle are all idempotent (AppendEvent's
// own dedup limitation, already documented on applyRestoreBundle, is the
// one accepted exception, same as it already is for the manual import
// path). Applied ids are still returned even on a partial failure — the
// caller reports them to office via SyncStatus regardless of this
// vessel's own cursor bookkeeping, since office's record of "did the
// vessel apply this" is independently correct either way.
func (s *Server) applyPulledRestoreCommands(ctx context.Context, st *store.Store, client *ovlsync.Client, commands []*syncv1.RestoreCommand, currentCursor, nextCursor int64) (appliedIDs []string, cursor int64) {
	cursor = currentCursor
	allApplied := true
	for _, rc := range commands {
		ciphertext, err := client.FetchRestoreBundle(ctx, rc.GetCommandId())
		if err != nil {
			allApplied = false
			break
		}
		bundle, err := decryptRestoreBundle(ctx, st, ciphertext)
		if err != nil {
			allApplied = false
			break
		}
		if _, err := applyRestoreBundle(ctx, st, bundle); err != nil {
			allApplied = false
			break
		}
		appliedIDs = append(appliedIDs, rc.GetCommandId())
	}
	if allApplied {
		cursor = nextCursor
	}
	return appliedIDs, cursor
}

// pushOutboxBatch sends every pending outbox item and acks (deletes)
// whichever ones the office accepted. Items the office rejects, or that
// fail to even hydrate/convert locally, are left queued — logged via the
// returned error but not otherwise special-cased; they retry next cycle
// unchanged.
func (s *Server) pushOutboxBatch(ctx context.Context, st *store.Store, client *ovlsync.Client) error {
	pending, err := st.ListOutboxItems(ctx)
	if err != nil {
		return fmt.Errorf("list outbox items: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}

	items := make([]*syncv1.OutboxItem, 0, len(pending))
	for _, item := range pending {
		pbItem, err := buildOutboxItem(ctx, st, item)
		if err != nil {
			// A single malformed local item shouldn't block the rest of
			// the batch from even being attempted; it just stays queued
			// (and keeps failing to build) until whatever's wrong with it
			// is fixed.
			continue
		}
		items = append(items, pbItem)
	}
	if len(items) == 0 {
		return nil
	}

	acks, err := client.PushOutbox(ctx, items)
	if err != nil {
		return err
	}
	byItemID := make(map[string]store.OutboxItem, len(pending))
	for _, item := range pending {
		byItemID[item.ItemID] = item
	}
	now := time.Now().UTC()
	for _, ack := range acks {
		if !ack.GetAccepted() {
			continue
		}
		if err := st.AckOutboxItem(ctx, ack.GetItemId()); err != nil {
			return fmt.Errorf("ack outbox item %s: %w", ack.GetItemId(), err)
		}
		// 18.07.26 manual-test item 2: a reportVersion ack is office
		// confirming it now has this exact submitted version — the point
		// StateSynced (pkg/domain.Report.MarkSynced) exists to record, so
		// the vessel's own UI stops showing "Submitted" forever once
		// office actually has the report.
		item, ok := byItemID[ack.GetItemId()]
		if !ok || item.Kind != store.OutboxItemKindReportVersion {
			continue
		}
		if err := s.markReportSynced(ctx, st, item.ReportID, item.VersionNo, now); err != nil {
			return fmt.Errorf("mark report %s v%d synced: %w", item.ReportID, item.VersionNo, err)
		}
	}
	return nil
}

// markReportSynced loads the report the ack referenced and, only if
// it's still exactly the state MarkSynced expects (StateSubmitted), flips
// it to StateSynced and persists the transition + audit event. A report
// already remarked/invalidated/pushed/synced by the time this runs is
// left alone (MarkSynced's own ok=false) — not an error, just nothing to
// do.
func (s *Server) markReportSynced(ctx context.Context, st *store.Store, reportID string, versionNo int, at time.Time) error {
	report, err := st.GetReport(ctx, reportID, versionNo)
	if err != nil {
		return fmt.Errorf("load report: %w", err)
	}
	event, ok := report.MarkSynced(at)
	if !ok {
		return nil
	}
	if err := st.SaveReport(ctx, report); err != nil {
		return fmt.Errorf("save report: %w", err)
	}
	if _, err := st.AppendEvent(ctx, event); err != nil {
		return fmt.Errorf("append synced event: %w", err)
	}
	return nil
}

// buildOutboxItem hydrates one store.OutboxItem's referenced data
// (report_versions/report_events, looked up fresh rather than
// duplicated in the outbox row itself — see the outbox migration's own
// comment) and converts it to the proto wire type.
func buildOutboxItem(ctx context.Context, st *store.Store, item store.OutboxItem) (*syncv1.OutboxItem, error) {
	switch item.Kind {
	case store.OutboxItemKindReportVersion:
		r, err := st.GetReport(ctx, item.ReportID, item.VersionNo)
		if err != nil {
			return nil, fmt.Errorf("load report %s v%d: %w", item.ReportID, item.VersionNo, err)
		}
		pv, err := syncproto.ReportVersionFromDomain(r)
		if err != nil {
			return nil, err
		}
		return &syncv1.OutboxItem{
			ItemId: item.ItemID, SequenceNo: item.SequenceNo,
			Payload: &syncv1.OutboxItem_ReportVersion{ReportVersion: pv},
		}, nil
	case store.OutboxItemKindReportAuditEvent:
		if item.ReportEventID == nil {
			return nil, fmt.Errorf("reportAuditEvent outbox item %s has no ReportEventID", item.ItemID)
		}
		e, err := st.GetEventByID(ctx, *item.ReportEventID)
		if err != nil {
			return nil, fmt.Errorf("load event %d: %w", *item.ReportEventID, err)
		}
		pe, err := syncproto.ReportAuditEventFromDomain(e)
		if err != nil {
			return nil, err
		}
		return &syncv1.OutboxItem{
			ItemId: item.ItemID, SequenceNo: item.SequenceNo,
			Payload: &syncv1.OutboxItem_AuditEvent{AuditEvent: pe},
		}, nil
	case store.OutboxItemKindChat:
		if item.ChatMessageID == nil {
			return nil, fmt.Errorf("chatMessage outbox item %s has no ChatMessageID", item.ItemID)
		}
		m, err := st.GetChatMessage(ctx, *item.ChatMessageID)
		if err != nil {
			return nil, fmt.Errorf("load chat message %s: %w", *item.ChatMessageID, err)
		}
		return &syncv1.OutboxItem{
			ItemId: item.ItemID, SequenceNo: item.SequenceNo,
			Payload: &syncv1.OutboxItem_ChatMessage{ChatMessage: syncproto.ChatMessageFromDomain(m)},
		}, nil
	default:
		return nil, fmt.Errorf("unknown outbox item kind %q", item.Kind)
	}
}

// handleSyncNow triggers an immediate sync cycle (design handoff A5's
// "Sync now" button) and returns its outcome.
func (s *Server) handleSyncNow(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	status := s.RunSyncCycle(r.Context())
	if !status.Enrolled {
		httpjson.WriteError(w, http.StatusBadRequest, "vessel is not enrolled with an office")
		return
	}
	if status.LastError != "" {
		httpjson.WriteError(w, http.StatusBadGateway, status.LastError)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, status)
}

// handleSyncStatus returns the outcome of the most recent sync cycle
// (manual or scheduled), for a settings/status screen to poll.
func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	s.syncMu.RLock()
	status := s.syncStat
	s.syncMu.RUnlock()
	httpjson.WriteJSON(w, http.StatusOK, status)
}
