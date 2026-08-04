// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DBPath returns the SQLite file path Open(dataDir) uses — exported so
// callers that need to restore a snapshot (which must happen with the
// Store closed, so it can't be a method taking a receiver they no longer
// hold) know exactly which file to replace.
func DBPath(dataDir string) string {
	return filepath.Join(dataDir, dbFileName)
}

// SnapshotTo writes a consistent point-in-time copy of the database to
// destPath using SQLite's own VACUUM INTO (architecture 9.6: "nightly
// SQLite snapshot") — this also compacts free space, unlike a raw file
// copy, and (unlike a plain file copy) is safe to run against a live
// database in WAL mode without stopping writers.
func (s *Store) SnapshotTo(ctx context.Context, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
		return fmt.Errorf("create snapshot directory: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", destPath); err != nil {
		return fmt.Errorf("vacuum into %s: %w", destPath, err)
	}
	return nil
}

// RestoreDatabase replaces the database file at dataDir with the snapshot
// at snapshotPath. The caller must Close() the Store that had dataDir
// open *before* calling this (SQLite must not have the destination file
// open while it's replaced) and Open() a fresh Store at dataDir
// afterward — this is a free function rather than a method for exactly
// that reason: there is no live Store to call it on. Also removes any
// stale -wal/-shm sidecar files at the destination so the fresh Open
// doesn't try to replay a WAL that belongs to the database it just
// replaced.
func RestoreDatabase(dataDir, snapshotPath string) error {
	dest := DBPath(dataDir)
	if err := copyFile(snapshotPath, dest); err != nil {
		return fmt.Errorf("restore database: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dest + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale %s: %w", dest+suffix, err)
		}
	}
	return nil
}

// copyFile is only ever called by CopyAllTo's own internal walk (or a
// snapshot path this process built itself) — src/dest are never external
// input.
func copyFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dest), err)
	}
	in, err := os.Open(src) // #nosec G304 -- see copyFile's doc comment
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dest) // #nosec G304 -- see copyFile's doc comment
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy %s to %s: %w", src, dest, err)
	}
	return out.Close()
}
