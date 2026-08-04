// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"io/fs"
	"net/http"
	"strings"
)

// spaHandler serves the embedded React build, falling back to
// index.html for any path that isn't a real file — client-side routing
// then takes over in the browser. Identical in shape to vessel/httpapi's
// own spaHandler; kept as a separate small copy rather than a shared
// package since vessel/ and office/ are independent binaries (architecture
// section 4) and this is generic serving infrastructure, not a rule that
// could drift between the two.
type spaHandler struct {
	fs fs.FS
}

func newSPAHandler(root fs.FS) http.Handler {
	return spaHandler{fs: root}
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upath := strings.TrimPrefix(r.URL.Path, "/")
	if upath == "" {
		upath = "index.html"
	}
	// h.fs.Open already rejects a ".."-laden upath (io/fs.ValidPath), and
	// http.ServeFileFS independently refuses any request path containing
	// ".." — belt and suspenders against traversal, not a real gap.
	if f, err := h.fs.Open(upath); err == nil {
		_ = f.Close()
		http.ServeFileFS(w, r, h.fs, upath) // #nosec G703 -- see comment above
		return
	}
	if f, err := h.fs.Open("index.html"); err == nil {
		_ = f.Close()
		http.ServeFileFS(w, r, h.fs, "index.html")
		return
	}
	// No build output has ever been embedded (a fresh checkout before
	// `make web-build` has run) — office/webdist only has its tracked
	// .gitkeep placeholder in that case.
	http.Error(w, "ovl-office: no web UI build found — run `make web-build` and rebuild the binary", http.StatusServiceUnavailable)
}
