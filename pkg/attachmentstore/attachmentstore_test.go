// SPDX-License-Identifier: AGPL-3.0-only

package attachmentstore

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStore_PutAndOpen(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	content := "a bunker delivery note scan, pretend PDF bytes"
	sum := sha256.Sum256([]byte(content))
	wantHash := hex.EncodeToString(sum[:])

	hash, err := a.Put(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if hash != wantHash {
		t.Errorf("Put returned hash %q, want %q", hash, wantHash)
	}
	if !a.Has(hash) {
		t.Error("Has() = false after Put")
	}

	r, err := a.Open(hash)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != content {
		t.Errorf("read back %q, want %q", got, content)
	}
}

func TestStore_Dedup(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	content := "same content twice"

	hash1, err := a.Put(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Put (first): %v", err)
	}
	hash2, err := a.Put(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Put (second): %v", err)
	}
	if hash1 != hash2 {
		t.Errorf("hash1 = %q, hash2 = %q, want equal (dedup by content)", hash1, hash2)
	}
}

func TestStore_OpenMissing(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Open("0000000000000000000000000000000000000000000000000000000000000000"); err != ErrNotFound {
		t.Errorf("Open(missing) error = %v, want ErrNotFound", err)
	}
	if a.Has("0000") {
		t.Error("Has(missing) = true")
	}
}

func TestStore_PutFile(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	content := "assembled from chunks"
	sum := sha256.Sum256([]byte(content))
	wantHash := hex.EncodeToString(sum[:])

	staged := filepath.Join(t.TempDir(), "staged-file")
	if err := os.WriteFile(staged, []byte(content), 0o644); err != nil {
		t.Fatalf("write staged file: %v", err)
	}

	hash, err := a.PutFile(staged, wantHash)
	if err != nil {
		t.Fatalf("PutFile: %v", err)
	}
	if hash != wantHash {
		t.Errorf("PutFile returned hash %q, want %q", hash, wantHash)
	}
	if !a.Has(hash) {
		t.Error("Has() = false after PutFile")
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Error("staged file still exists after PutFile, want it moved (renamed) into the store")
	}
}

func TestStore_PutFile_HashMismatchIsRejected(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	staged := filepath.Join(t.TempDir(), "staged-file")
	if err := os.WriteFile(staged, []byte("actual content"), 0o644); err != nil {
		t.Fatalf("write staged file: %v", err)
	}

	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := a.PutFile(staged, wrongHash); err == nil {
		t.Fatal("PutFile with a mismatched hash = nil error, want an error")
	}
	if a.Has(wrongHash) {
		t.Error("the mismatched hash was promoted into the store despite the corruption")
	}
	if _, err := os.Stat(staged); err != nil {
		t.Error("staged file was removed despite PutFile failing, want it left in place for the caller")
	}
}

func TestStore_Chunk(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	content := "0123456789ABCDEFGHIJ" // 20 bytes, chunk size 6 -> chunks of 6,6,6,2
	hash, err := a.Put(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	tests := []struct {
		index int32
		want  string
	}{
		{0, "012345"},
		{1, "6789AB"},
		{2, "CDEFGH"},
		{3, "IJ"}, // last chunk, shorter than chunkSize
	}
	for _, tt := range tests {
		got, err := a.Chunk(hash, tt.index, 6)
		if err != nil {
			t.Fatalf("Chunk(%d): %v", tt.index, err)
		}
		if string(got) != tt.want {
			t.Errorf("Chunk(%d) = %q, want %q", tt.index, got, tt.want)
		}
	}
}

func TestStore_Chunk_NotFound(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Chunk("0000000000000000000000000000000000000000000000000000000000000000", 0, 10); err != ErrNotFound {
		t.Errorf("Chunk(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_CopyAllTo(t *testing.T) {
	src, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hash, err := src.Put(strings.NewReader("a bunker delivery note scan"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	destDir := filepath.Join(t.TempDir(), "attachments")
	if err := src.CopyAllTo(destDir); err != nil {
		t.Fatalf("CopyAllTo: %v", err)
	}

	dest := &Store{BaseDir: destDir}
	if !dest.Has(hash) {
		t.Fatalf("copied store missing %s", hash)
	}
	rc, err := dest.Open(hash)
	if err != nil {
		t.Fatalf("Open copied attachment: %v", err)
	}
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read copied attachment: %v", err)
	}
	if string(got) != "a bunker delivery note scan" {
		t.Errorf("copied attachment content = %q, want the original bytes", got)
	}
}

func TestStore_CopyAllTo_MissingBaseDirIsNoop(t *testing.T) {
	a := &Store{BaseDir: filepath.Join(t.TempDir(), "never-created")}
	if err := a.CopyAllTo(t.TempDir()); err != nil {
		t.Errorf("CopyAllTo with no BaseDir: %v, want nil (nothing to copy yet)", err)
	}
}

func TestStore_PutFile_DedupSkipsPromotionButStillReportsHash(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	content := "duplicate content"
	if _, err := a.Put(strings.NewReader(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	sum := sha256.Sum256([]byte(content))
	wantHash := hex.EncodeToString(sum[:])
	staged := filepath.Join(t.TempDir(), "staged-file")
	if err := os.WriteFile(staged, []byte(content), 0o644); err != nil {
		t.Fatalf("write staged file: %v", err)
	}

	hash, err := a.PutFile(staged, wantHash)
	if err != nil {
		t.Fatalf("PutFile: %v", err)
	}
	if hash != wantHash {
		t.Errorf("hash = %q, want %q", hash, wantHash)
	}
}
