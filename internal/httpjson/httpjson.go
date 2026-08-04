// SPDX-License-Identifier: AGPL-3.0-only

// Package httpjson is the shared JSON request/response helpers for both
// ovl-vessel's and ovl-office's HTTP APIs — identical in both binaries
// (unlike the per-binary session/SPA-serving code, which has genuinely
// diverged), so kept as one copy instead of two that could silently
// drift apart.
package httpjson

import (
	"encoding/json"
	"net/http"
)

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

// DecodeJSON decodes the request body into v, rejecting unknown fields
// so a client typo surfaces as an error rather than being silently
// ignored.
func DecodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
