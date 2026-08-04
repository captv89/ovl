// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"io/fs"
	"net/http"
	"os"

	"github.com/captv89/ovl/internal/httpjson"
)

// systemView is design handoff B10's System tab. Real values only — no
// job-queue/background-worker health, since River isn't wired yet (see
// PROJECT.md's Phase 4-6 status); a fake "queue: healthy" reading would
// be actively misleading once a real queue does exist and this screen
// hasn't been revisited to show it honestly.
type systemView struct {
	Version              string `json:"version"`
	DatabaseReachable    bool   `json:"databaseReachable"`
	AttachmentStoreBytes int64  `json:"attachmentStoreBytes"`
	AttachmentStoreCount int    `json:"attachmentStoreCount"`
}

// handleGetSystem serves design handoff B10's System tab. Any
// authenticated user may view it (read-only, no admin-only action
// here) — same viewable-by-anyone default the Dashboard already uses.
func (s *Server) handleGetSystem(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	reachable := s.st.Ping(r.Context()) == nil
	totalBytes, count := attachmentStoreUsage(s.attachments.BaseDir)
	httpjson.WriteJSON(w, http.StatusOK, systemView{
		Version:              s.version,
		DatabaseReachable:    reachable,
		AttachmentStoreBytes: totalBytes,
		AttachmentStoreCount: count,
	})
}

// attachmentStoreUsage walks the content-addressed attachment store
// directory (pkg/attachmentstore's own BaseDir/<2 hex chars>/<hash>
// layout) and totals real file sizes on disk — not a stored counter,
// since nothing maintains one; office's attachment volume is small
// enough (Phase 6, EDN/Bunker inline previews) that a walk on each
// System tab load is cheap. Missing directory (attachments never
// received) reads as zero, not an error.
func attachmentStoreUsage(baseDir string) (totalBytes int64, count int) {
	_ = fs.WalkDir(os.DirFS(baseDir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		totalBytes += info.Size()
		count++
		return nil
	})
	return totalBytes, count
}
