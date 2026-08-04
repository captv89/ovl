// SPDX-License-Identifier: AGPL-3.0-only

package syncproto

import (
	"bytes"
	"strings"
	"testing"
)

func TestZstdCompressorDecompressorRoundTrip(t *testing.T) {
	original := strings.Repeat("the quick brown fox jumps over the lazy dog ", 200)

	var compressed bytes.Buffer
	compressor := newZstdCompressor()
	compressor.Reset(&compressed)
	if _, err := compressor.Write([]byte(original)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if compressed.Len() == 0 {
		t.Fatal("compressed output is empty")
	}
	if compressed.Len() >= len(original) {
		t.Errorf("compressed length %d >= original length %d, want smaller for repetitive input", compressed.Len(), len(original))
	}

	decompressor := newZstdDecompressor()
	if err := decompressor.Reset(bytes.NewReader(compressed.Bytes())); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	var decompressed bytes.Buffer
	if _, err := decompressed.ReadFrom(decompressor); err != nil {
		t.Fatalf("read decompressed data: %v", err)
	}
	if err := decompressor.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if decompressed.String() != original {
		t.Error("round-tripped data does not match the original")
	}
}

// TestZstdDecompressor_ReusableAfterClose exercises the exact lifecycle
// connectrpc.com/connect's compression pool imposes on a pooled
// Decompressor (see compression.go's putDecompressor): Close(), then
// Reset() again on the very same instance, then real use — a pooled
// instance is expected to serve many requests, not one. This is the
// scenario that caught a real bug: klauspost's own (*zstd.Decoder).Close
// is documented as terminal ("NOT possible to reuse... after this"), so
// a naive Close adapter broke every second decompression through a
// pooled decompressor with "decoder used after Close."
func TestZstdDecompressor_ReusableAfterClose(t *testing.T) {
	compress := func(s string) []byte {
		var buf bytes.Buffer
		c := newZstdCompressor()
		c.Reset(&buf)
		if _, err := c.Write([]byte(s)); err != nil {
			t.Fatalf("Write: %v", err)
		}
		if err := c.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		return buf.Bytes()
	}

	decompressor := newZstdDecompressor()
	for i, want := range []string{"first stream", "second stream", "third stream"} {
		if err := decompressor.Reset(bytes.NewReader(compress(want))); err != nil {
			t.Fatalf("Reset (iteration %d): %v", i, err)
		}
		var got bytes.Buffer
		if _, err := got.ReadFrom(decompressor); err != nil {
			t.Fatalf("read decompressed data (iteration %d): %v", i, err)
		}
		if got.String() != want {
			t.Errorf("iteration %d: got %q, want %q", i, got.String(), want)
		}
		// Mimics connectrpc.com/connect's putDecompressor: Close(), then
		// Reset(http.NoBody)-equivalent, before the pool hands this same
		// instance back out for the next request.
		if err := decompressor.Close(); err != nil {
			t.Fatalf("Close (iteration %d): %v", i, err)
		}
	}
}

func TestServerOptions_ClientOptions_NotEmpty(t *testing.T) {
	if len(ServerOptions()) == 0 {
		t.Error("ServerOptions() is empty")
	}
	if len(ClientOptions()) == 0 {
		t.Error("ClientOptions() is empty")
	}
}
