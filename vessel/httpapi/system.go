// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"fmt"
	"io/fs"
	"net/http"

	"github.com/captv89/ovl/internal/httpjson"
	ovlschemas "github.com/captv89/ovl/schemas"

	"github.com/captv89/ovl/pkg/schema"
)

type schemaInfo struct {
	SchemaName string `json:"schemaName"`
	Version    string `json:"version"`
	OvdVersion string `json:"ovdVersion"`
}

type systemInfoResponse struct {
	Version string       `json:"version"`
	Commit  string       `json:"commit"`
	Date    string       `json:"date"`
	Schemas []schemaInfo `json:"schemas"`
}

// handleGetSystemInfo is design handoff A10's diagnostics section: "app
// version, schema and config bundle versions currently active." Any
// signed-in user may read it (A10: "others read-only where harmless") —
// there's nothing sensitive in a build stamp or a schema version list.
func (s *Server) handleGetSystemInfo(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	schemas, err := loadedSchemaInfo()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, systemInfoResponse{
		Version: s.buildInfo.Version,
		Commit:  s.buildInfo.Commit,
		Date:    s.buildInfo.Date,
		Schemas: schemas,
	})
}

// loadedSchemaInfo enumerates every curated OVD schema embedded in this
// binary (schemas.FS), the same source handleGetSchema loads
// individual schemas from.
func loadedSchemaInfo() ([]schemaInfo, error) {
	validator, err := getSchemaValidator()
	if err != nil {
		return nil, err
	}
	paths, err := fs.Glob(ovlschemas.FS, "ovd-3.13/*.json")
	if err != nil {
		return nil, fmt.Errorf("glob curated schemas: %w", err)
	}
	out := make([]schemaInfo, 0, len(paths))
	for _, p := range paths {
		sch, err := schema.Load(ovlschemas.FS, p, validator)
		if err != nil {
			return nil, fmt.Errorf("load schema %s: %w", p, err)
		}
		out = append(out, schemaInfo{SchemaName: sch.SchemaName, Version: sch.Version, OvdVersion: sch.OvdVersion})
	}
	return out, nil
}
