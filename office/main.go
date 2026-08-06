// SPDX-License-Identifier: AGPL-3.0-only

// Command ovl-office is the office API server (architecture section 12):
// Postgres store, embedded React SPA. At this skeleton stage it has no
// OIDC auth yet — that's a deliberately deferred, later checklist item.
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/captv89/ovl/office/httpapi"
	"github.com/captv89/ovl/office/schemaversions"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/attachmentstore"
	"github.com/captv89/ovl/pkg/schema"
	ovlschemas "github.com/captv89/ovl/schemas"
)

const shutdownTimeout = 5 * time.Second

// devDSN is a local-dev-only default matching deploy/office/docker-
// compose.yml's credentials — never used against a real deployment,
// where OVL_OFFICE_DB_DSN (or -db-dsn) is expected to be set explicitly.
const devDSN = "postgres://ovl:ovl@localhost:5432/ovl_office?sslmode=disable" // #nosec G101 -- local-dev-only default, see doc comment above, never a real credential

// defaultDataDir is where office's own filesystem state lives — so far
// just attachments (architecture 12.1: "attachments in a filesystem
// volume using the same content-addressed layout as the vessel"), added
// in Phase 4 (the sync attachment chunk-upload step). Everything else
// office holds is in Postgres.
const defaultDataDir = "./data"

// version, commit, and date are stamped by the release build (Docker
// build args, once office/Dockerfile exists — see .goreleaser.yaml);
// "dev"/"none"/"unknown" are what a plain `go build`/`go run` produces.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// webdist is the vite build of web/office (see vite.config.ts's outDir
// and Makefile's web-build-office target). Only the tracked .gitkeep
// placeholder is embedded until `make web-build` has actually run —
// httpapi's SPA handler reports that clearly rather than crashing.
//
//go:embed all:webdist
var webdist embed.FS

func main() {
	// reset-admin-password is a subcommand, not a server flag: it opens
	// the store, does one thing, and exits — see runResetAdminPassword's
	// own doc comment (office-admin-lockout recovery, 2026-07-21) for why
	// this exists and why it's shaped this way rather than a new secret
	// to protect.
	if len(os.Args) > 1 && os.Args[1] == "reset-admin-password" {
		if err := runResetAdminPassword(os.Args[2:]); err != nil {
			slog.Error("reset-admin-password failed", "error", err)
			os.Exit(1)
		}
		return
	}

	dsn := flag.String("db-dsn", envOr("OVL_OFFICE_DB_DSN", devDSN), "Postgres connection string")
	port := flag.Int("port", 8080, "port to listen on")
	dataDir := flag.String("data-dir", envOr("OVL_OFFICE_DATA_DIR", defaultDataDir), "directory for office's own filesystem state (attachments)")
	// secureCookies must be on in production (office runs behind Caddy TLS)
	// and off for plain-HTTP local dev, where a Secure cookie is silently
	// dropped and login appears to fail. Off by default so `make run-office`
	// works unconfigured; deploy/office/docker-compose.yml sets it on.
	secureCookies := flag.Bool("secure-cookies", envOr("OVL_OFFICE_SECURE_COOKIES", "") == "true", "mark the session cookie Secure (HTTPS-only); enable when serving over TLS")
	flag.Parse()

	if err := run(*dsn, *port, *dataDir, *secureCookies); err != nil {
		slog.Error("ovl-office exiting", "error", err)
		os.Exit(1)
	}
}

// seededBySystem marks schema versions this boot-time seed publishes,
// distinct from a real office/auth.User.ID — nothing has authenticated
// yet the first time this runs (it happens before the HTTP server even
// starts), so there is no admin to attribute the seed to.
const seededBySystem = "system"

// seedCuratedSchemas ensures every curated OVD schema (schemas/ovd-3.13,
// the same tree vessel embeds and has served since Phase 2) exists as a
// published SchemaVersion in office's registry. Per the user's
// direction: the real flow is office -> vessel (office is the
// authoritative source, pushing config bundles/schema versions down to
// vessels once Phase 4 sync exists); vessel embedding its own copy was
// only ever a bootstrapping convenience from being built first. This
// seed is what makes office's copy the one going forward — B5's upload
// flow only ever adds *new* versions on top of this baseline, never
// creates the first one. Idempotent: skips any schema name that already
// has a published version, so re-running on every boot is safe.
func seedCuratedSchemas(ctx context.Context, st *store.Store, validator *schema.Validator) error {
	matches, err := fs.Glob(ovlschemas.FS, "ovd-3.13/*.json")
	if err != nil {
		return fmt.Errorf("glob curated schemas: %w", err)
	}
	for _, path := range matches {
		parsed, err := schema.Load(ovlschemas.FS, path, validator)
		if err != nil {
			return fmt.Errorf("load curated schema %s: %w", path, err)
		}
		if _, err := st.LatestSchemaVersion(ctx, parsed.SchemaName); err == nil {
			continue // already seeded (or a real version has since been published)
		} else if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("check existing schema version for %s: %w", parsed.SchemaName, err)
		}
		content, err := fs.ReadFile(ovlschemas.FS, path)
		if err != nil {
			return fmt.Errorf("read curated schema %s: %w", path, err)
		}
		sv, err := schemaversions.Publish(parsed.SchemaName, parsed.Version, schemaversions.SourceProjectCurated, content, seededBySystem)
		if err != nil {
			return fmt.Errorf("prepare seed schema version for %s: %w", parsed.SchemaName, err)
		}
		if err := st.CreateSchemaVersion(ctx, sv); err != nil {
			return fmt.Errorf("create seed schema version for %s: %w", parsed.SchemaName, err)
		}
		slog.Info("seeded curated schema", "schema", parsed.SchemaName, "version", parsed.Version)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func run(dsn string, port int, dataDir string, secureCookies bool) error {
	st, err := store.Open(dsn)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	validator, err := schema.NewValidator(ovlschemas.FS, "meta-schema.json")
	if err != nil {
		return fmt.Errorf("build schema validator: %w", err)
	}
	if err := seedCuratedSchemas(context.Background(), st, validator); err != nil {
		return fmt.Errorf("seed curated schemas: %w", err)
	}

	spa, err := fs.Sub(webdist, "webdist")
	if err != nil {
		return fmt.Errorf("resolve embedded web UI: %w", err)
	}

	// attachmentStagingDir is a sibling of the final attachment store's
	// own directory, not nested inside it — see
	// office/syncservice.New's doc comment for why.
	attachments, err := attachmentstore.New(filepath.Join(dataDir, "attachments"))
	if err != nil {
		return fmt.Errorf("open attachment store: %w", err)
	}
	attachmentStagingDir := filepath.Join(dataDir, "attachment-staging")

	srv := httpapi.NewServer(st, spa, validator, attachments, attachmentStagingDir, version, secureCookies)
	defer func() {
		if err := srv.Close(); err != nil {
			slog.Error("close store", "error", err)
		}
	}()

	addr := fmt.Sprintf(":%d", port)
	httpServer := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("ovl-office listening", "version", version, "commit", commit, "date", date, "addr", addr)
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
