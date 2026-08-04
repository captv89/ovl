// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/schema"
	ovlschemas "github.com/captv89/ovl/schemas"
)

// enumValuesView is the JSON shape for a resolved enumRef's valid codes —
// generic across every curated field whose enumRef schema.ResolveEnum
// knows how to read (see pkg/schema/enums.go), not just event-types.
type enumValuesView struct {
	Values []string `json:"values"`
}

// handleGetEnum serves the valid codes for a curated field's enumRef (e.g.
// the "operational-modes" enum backing the Mode field). 404s for an
// enumRef with no generic resolver (schema.ResolveEnum's error case) —
// the form engine falls back to unrestricted text entry for those.
func (s *Server) handleGetEnum(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	name := r.PathValue("name")
	values, err := schema.ResolveEnum(ovlschemas.FS, name)
	if err != nil {
		httpjson.WriteError(w, http.StatusNotFound, "unknown enum: "+name)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, enumValuesView{Values: values})
}
