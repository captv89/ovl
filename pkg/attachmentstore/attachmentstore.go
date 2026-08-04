// SPDX-License-Identifier: AGPL-3.0-only

// Package attachmentstore is the content-addressed filesystem store both
// ovl-vessel and ovl-office use for attachments (architecture 15): files
// are named and deduplicated by sha256, sharded two hex characters deep
// (git's object-store layout), so a base directory never holds more
// than a few hundred entries per subdirectory. One shared implementation
// — not a copy on each side — because architecture 12.1 requires the
// two to match exactly ("attachments in a filesystem volume using the
// same content-addressed layout as the vessel"): Phase 7's office
// restore-bundle flow only works if a bundle built from office's store
// drops directly into a vessel's own store unchanged.
package attachmentstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrNotFound is returned by Open when hash isn't stored.
var ErrNotFound = errors.New("attachmentstore: not found")

// Store is the content-addressed store rooted at BaseDir.
type Store struct {
	BaseDir string
}

// New creates baseDir if needed.
func New(baseDir string) (*Store, error) {
	if err := os.MkdirAll(baseDir, 0o750); err != nil {
		return nil, fmt.Errorf("create attachment store dir %s: %w", baseDir, err)
	}
	return &Store{BaseDir: baseDir}, nil
}

// Put streams r to the store, returning its content hash. Storing the
// same content twice is a no-op the second time (deduplication is
// automatic since the destination path is the hash itself) and returns
// the same hash both times.
func (a *Store) Put(r io.Reader) (hash string, err error) {
	tmp, err := os.CreateTemp(a.BaseDir, "upload-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup: a no-op once successfully renamed below, and
	// harmless if it fails (an orphaned temp file, not a correctness issue).
	defer func() { _ = os.Remove(tmpPath) }()

	digest := sha256.New()
	if _, err := io.Copy(tmp, io.TeeReader(r, digest)); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write attachment: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	hash = hex.EncodeToString(digest.Sum(nil))
	dest := a.Path(hash)
	if _, err := os.Stat(dest); err == nil {
		return hash, nil // already stored; temp file removed by the deferred cleanup
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return "", fmt.Errorf("create shard directory for %s: %w", hash, err)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return "", fmt.Errorf("store attachment %s: %w", hash, err)
	}
	return hash, nil
}

// PutFile moves the file at path into the store under its own sha256
// content hash, computed by reading it once. Used by office's
// chunk-upload assembly (pkg/syncproto/../office/syncservice): chunks
// arrive incrementally over RPC and are concatenated into a staging
// file first, then promoted here once the last chunk lands — unlike
// Put, which streams an already-available io.Reader directly, this
// covers the "the bytes already exist as a file, hand off ownership"
// case without a redundant copy. wantHash, if non-empty, must match the
// computed hash or PutFile fails without promoting the file (the
// resumable-upload corruption check) and leaves path in place for the
// caller to remove.
func (a *Store) PutFile(path, wantHash string) (hash string, err error) {
	f, err := os.Open(path) // #nosec G304 -- path is always this process's own staged/temp file, never a raw external value
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close %s: %w", path, err)
	}

	hash = hex.EncodeToString(digest.Sum(nil))
	if wantHash != "" && hash != wantHash {
		return hash, fmt.Errorf("attachmentstore: content hash mismatch: computed %s, want %s", hash, wantHash)
	}

	dest := a.Path(hash)
	if _, err := os.Stat(dest); err == nil {
		return hash, nil // already stored; caller is responsible for removing path
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return "", fmt.Errorf("create shard directory for %s: %w", hash, err)
	}
	if err := os.Rename(path, dest); err != nil {
		return "", fmt.Errorf("store attachment %s: %w", hash, err)
	}
	return hash, nil
}

// Open returns a reader for the content stored under hash. The caller
// must Close it. Returns ErrNotFound if hash isn't stored.
func (a *Store) Open(hash string) (io.ReadCloser, error) {
	f, err := os.Open(a.Path(hash))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open attachment %s: %w", hash, err)
	}
	return f, nil
}

// Chunk returns the bytes of one chunkSize-sized chunk (0-based index)
// from the content stored under hash, without loading the whole file
// into memory — a resumable-upload client (vessel/sync) uses this to
// read exactly the chunks the office asked for. The last chunk may be
// shorter than chunkSize (returns fewer bytes, not an error).
func (a *Store) Chunk(hash string, index int32, chunkSize int32) ([]byte, error) {
	f, err := os.Open(a.Path(hash))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open attachment %s: %w", hash, err)
	}
	defer func() { _ = f.Close() }()

	offset := int64(index) * int64(chunkSize)
	buf := make([]byte, chunkSize)
	n, err := f.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read chunk %d of %s: %w", index, hash, err)
	}
	return buf[:n], nil
}

// Has reports whether hash is already stored.
func (a *Store) Has(hash string) bool {
	_, err := os.Stat(a.Path(hash))
	return err == nil
}

// Path is BaseDir/<first 2 hex chars>/<full hash> — exported so callers
// that need to construct destination paths directly (e.g. promoting an
// assembled set of chunks) don't need a second implementation of the
// same sharding scheme.
func (a *Store) Path(hash string) string {
	if len(hash) < 2 {
		return filepath.Join(a.BaseDir, hash)
	}
	return filepath.Join(a.BaseDir, hash[:2], hash)
}

// CopyAllTo copies every stored attachment into destDir, preserving the
// two-level sha256 sharding layout (architecture 9.6's "attachment
// store copy" for the vessel's own nightly DR snapshot; also usable by
// office's own backup/restore tooling once that exists). A no-op (not
// an error) if BaseDir doesn't exist yet.
func (a *Store) CopyAllTo(destDir string) error {
	if _, err := os.Stat(a.BaseDir); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(a.BaseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(a.BaseDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		return copyFile(path, target)
	})
}

// copyFile is only ever called by CopyAllTo's own filepath.WalkDir over
// BaseDir — src/dest are derived from that internal walk, never external
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
