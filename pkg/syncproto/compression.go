// SPDX-License-Identifier: AGPL-3.0-only

package syncproto

import (
	"fmt"

	"connectrpc.com/connect"
	"github.com/klauspost/compress/zstd"
)

// ZstdName is the compression algorithm architecture 11.1 asks for
// ("zstd compression enabled"), registered identically on both the
// office SyncService handler and the vessel's client via ServerOptions/
// ClientOptions below — one shared implementation so the two sides can
// never silently disagree on what "zstd" means, the same reasoning
// pkg/authcrypto applies to password hashing.
const ZstdName = "zstd"

// newZstdDecompressor and newZstdCompressor adapt
// github.com/klauspost/compress/zstd (the de facto standard pure-Go
// zstd implementation — architecture 11.1 names zstd specifically, and
// this project avoids hand-rolling compression codecs the same way it
// avoids hand-rolling crypto) to connect.Decompressor/connect.Compressor,
// for use with connect.WithCompression (server) and
// connect.WithAcceptCompression (client).
func newZstdDecompressor() connect.Decompressor {
	// r may be nil here: connect's compression pool always calls Reset
	// with a real source before every use, so this only needs to
	// construct successfully, not read anything yet — zstd.NewReader's
	// own doc comment calls this out explicitly as the pooling pattern.
	// WithDecoderConcurrency(1) forces zstd's synchronous decode path
	// instead of spinning up a background goroutine per stream — those
	// goroutines are torn down by the real Decoder.Close(), which this
	// adapter deliberately never calls (see zstdDecompressor's own doc
	// comment), so avoiding them entirely is simpler than relying on GC
	// to eventually clean up an abandoned one.
	zr, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		panic(fmt.Errorf("syncproto: construct zstd reader: %w", err))
	}
	return &zstdDecompressor{zr}
}

func newZstdCompressor() connect.Compressor {
	// *zstd.Encoder already satisfies connect.Compressor directly
	// (Write, Close() error, Reset(io.Writer)) — no adapter needed,
	// unlike the decoder below.
	zw, err := zstd.NewWriter(nil)
	if err != nil {
		panic(fmt.Errorf("syncproto: construct zstd writer: %w", err))
	}
	return zw
}

// zstdDecompressor adapts *zstd.Decoder to connect.Decompressor.
//
// Close is a deliberate no-op, not a thin wrapper around the real
// (*zstd.Decoder).Close — connect's compression pool calls Close() and
// then Reset() again on the same instance before returning it to the
// pool for reuse (see connectrpc.com/connect's putDecompressor), but
// klauspost's own doc comment on Close is explicit: "It is NOT possible
// to reuse the decoder after this." Actually closing here would break
// every second request through a pooled decompressor. Reset alone fully
// prepares the decoder for a new stream (klauspost's Reset drains and
// re-initializes internal state itself), so skipping the real Close is
// correct, not just convenient — this is klauspost's documented pattern
// for reusing a Decoder without Close/New churn.
type zstdDecompressor struct {
	*zstd.Decoder
}

func (z *zstdDecompressor) Close() error {
	return nil
}

// ServerOptions returns the connect.HandlerOption(s) the office
// SyncService handler needs to support zstd (architecture 11.1).
func ServerOptions() []connect.HandlerOption {
	return []connect.HandlerOption{
		connect.WithCompression(ZstdName, newZstdDecompressor, newZstdCompressor),
	}
}

// ClientOptions returns the connect.ClientOption(s) a vessel's
// SyncService client needs to match the office: accept zstd-compressed
// responses (WithAcceptCompression) and actually send zstd-compressed
// requests (WithSendCompression — registering a compressor alone only
// makes it available for negotiation, it doesn't select it for outgoing
// requests).
func ClientOptions() []connect.ClientOption {
	return []connect.ClientOption{
		connect.WithAcceptCompression(ZstdName, newZstdDecompressor, newZstdCompressor),
		connect.WithSendCompression(ZstdName),
	}
}
