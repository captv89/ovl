// SPDX-License-Identifier: AGPL-3.0-only

// Command ovl-vessel is the single-binary vessel application: HTTP
// server, embedded React SPA, SQLite store. Binds loopback-only in
// standalone mode, or every interface (LAN-reachable) in vessel-server
// mode (architecture 9.1, 9.5) — see bindAddr.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/captv89/ovl/vessel/bootstrap"
	"github.com/captv89/ovl/vessel/httpapi"
	"github.com/captv89/ovl/vessel/store"
)

const shutdownTimeout = 5 * time.Second

// nightlyBackupHourUTC is when the local-DR snapshot job runs (architecture
// 9.6: "nightly SQLite snapshot"). All timestamps in this app are UTC
// (CLAUDE.md/architecture: "no local time anywhere"), so the schedule is
// UTC too rather than the host machine's local time zone.
const nightlyBackupHourUTC = 2

// version, commit, and date are stamped by GoReleaser via -ldflags at
// release build time (see .goreleaser.yaml); "dev"/"none"/"unknown" are
// what a plain `go build`/`go run` produces.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// webdist is the vite build of web/vessel (see vite.config.ts's outDir
// and Makefile's web-build-vessel target). Only the tracked .gitkeep
// placeholder is embedded until `make web-build` has actually run —
// httpapi's SPA handler reports that clearly rather than crashing.
//
//go:embed all:webdist
var webdist embed.FS

func main() {
	configPath := flag.String("config", "", "path to the bootstrap config file (default: OS user config dir)")
	port := flag.Int("port", 8420, "port to listen on (127.0.0.1 only in standalone mode, every interface in vessel-server mode — architecture 9.1)")
	// secureCookies must be on if an operator ever puts TLS in front of
	// vessel-server/LAN mode — off by default so standalone plain-HTTP
	// use (this app's common case) doesn't silently drop the session
	// cookie and break login. Mirrors ovl-office's own -secure-cookies.
	secureCookies := flag.Bool("secure-cookies", os.Getenv("OVL_VESSEL_SECURE_COOKIES") == "true", "mark the session cookie Secure (HTTPS-only); enable when serving over TLS")
	flag.Parse()

	if err := run(*configPath, *port, *secureCookies); err != nil {
		slog.Error("ovl-vessel exiting", "error", err)
		os.Exit(1)
	}
}

// bindAddr resolves the listen address (architecture 9.1): loopback-only
// in standalone mode (or before wizard step 1 has run, i.e. cfg == nil),
// every interface once the operator has chosen vessel-server mode — Go
// has no portable "find the LAN NIC" API, and 0.0.0.0 also still serves
// loopback, which this feature's own two-origin manual verification
// deliberately relies on. A rebind only takes effect on the next process
// start (Go doesn't rebind a live listener), so switching mode via the
// wizard doesn't expose the LAN until ovl-vessel restarts.
func bindAddr(cfg *bootstrap.Config, port int) string {
	host := "127.0.0.1"
	if cfg != nil && cfg.Mode == bootstrap.ModeServer {
		host = "0.0.0.0"
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func run(configPath string, port int, secureCookies bool) error {
	if configPath == "" {
		p, err := bootstrap.DefaultPath()
		if err != nil {
			return fmt.Errorf("resolve default config path: %w", err)
		}
		configPath = p
	}

	cfg, err := bootstrap.Load(configPath)
	if err != nil {
		return fmt.Errorf("load bootstrap config: %w", err)
	}

	var st *store.Store
	if cfg.Configured() {
		st, err = store.Open(cfg.DataDir)
		if err != nil {
			return fmt.Errorf("open store at %s: %w", cfg.DataDir, err)
		}
	}

	spa, err := fs.Sub(webdist, "webdist")
	if err != nil {
		return fmt.Errorf("resolve embedded web UI: %w", err)
	}

	srv := httpapi.NewServer(configPath, cfg, st, spa, httpapi.BuildInfo{Version: version, Commit: commit, Date: date}, secureCookies)
	defer func() {
		if err := srv.Close(); err != nil {
			slog.Error("close store", "error", err)
		}
	}()

	addr := bindAddr(cfg, port)
	httpServer := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runNightlyBackupScheduler(ctx, srv)
	go runSyncScheduler(ctx, srv)
	go runLockSweepScheduler(ctx, srv)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("ovl-vessel listening", "version", version, "commit", commit, "date", date, "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}

// runNightlyBackupScheduler is architecture 9.6's "nightly SQLite
// snapshot" job: an in-process ticker rather than an external cron —
// consistent with 9.1's "one binary," no separate scheduler process or
// OS-level cron entry to install. Runs once at the next occurrence of
// nightlyBackupHourUTC and every 24h after that until ctx is canceled
// (server shutdown). Before first-run setup completes, srv.
// RunNightlySnapshot is a no-op (there's no store yet), so this is safe
// to start unconditionally at boot.
func runNightlyBackupScheduler(ctx context.Context, srv *httpapi.Server) {
	for {
		wait := time.Until(nextNightlyRun(time.Now().UTC()))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if _, err := srv.RunNightlySnapshot(ctx); err != nil {
			slog.Error("nightly snapshot", "error", err)
		}
	}
}

// nextNightlyRun returns the next occurrence of nightlyBackupHourUTC at
// or after now (UTC). A pure function of now specifically so it's
// testable without waiting on a real clock.
func nextNightlyRun(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), nightlyBackupHourUTC, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// runSyncScheduler is architecture 11.2's periodic sync cycle: an
// in-process ticker, same "no separate scheduler process" reasoning as
// runNightlyBackupScheduler. srv.RunSyncCycle is a no-op for an
// unenrolled vessel (see its own doc comment), so this is safe to start
// unconditionally at boot, same as the backup scheduler.
//
// Uses a plain time.Timer re-armed with srv.SyncIntervalDuration()
// before every wait, not a fixed time.Ticker — architecture 11.2's
// "configurable interval" is now a Master-editable Settings value
// (vessel/httpapi/settings.go), so a change the Master makes must take
// effect on the very next wait, not require a process restart. A
// vessel with good connectivity can set this as low as 30s (near real-
// time); one on a constrained satellite link can set it as high as 24h.
//
// Runs one cycle immediately rather than waiting for the first interval
// to elapse: a vessel that just redeemed an enrollment code would
// otherwise sit for up to the full interval with syncStatusView.Enrolled
// still false — and Home's "Sync now" button is disabled exactly on that
// flag (SyncStatusFooter, `disabled={!status?.enrolled || syncing}`), so
// the operator has no way to trigger a first sync themselves either
// (2026-07-14 manual-test feedback: reports "looked" synced but nothing
// was reaching the office — root-caused to this dead window, not a
// push/pull bug).
func runSyncScheduler(ctx context.Context, srv *httpapi.Server) {
	if status := srv.RunSyncCycle(ctx); status.Enrolled && status.LastError != "" {
		slog.Error("sync cycle", "error", status.LastError)
	}

	timer := time.NewTimer(srv.SyncIntervalDuration())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if status := srv.RunSyncCycle(ctx); status.Enrolled && status.LastError != "" {
			slog.Error("sync cycle", "error", status.LastError)
		}
		timer.Reset(srv.SyncIntervalDuration())
	}
}

// lockSweepInterval is how often expired section soft locks (architecture
// 9.5) are swept and their release broadcast live — independent of
// lockTTL itself (httpapi.lockTTL), which is how long a lock survives
// inactivity before it counts as expired in the first place.
const lockSweepInterval = 30 * time.Second

// runLockSweepScheduler is an in-process ticker, same "no separate
// scheduler process" reasoning as runNightlyBackupScheduler/
// runSyncScheduler. srv.SweepExpiredLocks does no I/O and is a no-op
// when nothing is locked, so this is safe to start unconditionally at
// boot, same as the other two schedulers.
func runLockSweepScheduler(ctx context.Context, srv *httpapi.Server) {
	ticker := time.NewTicker(lockSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		srv.SweepExpiredLocks()
	}
}
