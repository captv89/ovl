// SPDX-License-Identifier: AGPL-3.0-only

package restorebundle

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/backupcrypto"
	"github.com/captv89/ovl/pkg/domain"
)

// TestBundle_EncryptMarshalRoundtrip is the crypto+wire-format half of
// architecture 12.5's "office generates a bundle, vessel imports it,
// data matches" exit criteria — office/httpapi and vessel/httpapi each
// cover their own HTTP-level half in their own test suites (real
// Postgres/SQLite respectively); this proves the shared envelope itself
// (JSON marshal -> age encrypt -> age decrypt -> JSON unmarshal) never
// loses or corrupts data, independent of either side's own store.
func TestBundle_EncryptMarshalRoundtrip(t *testing.T) {
	identity, err := backupcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	original := Bundle{
		VesselID: "vessel-1", VesselName: "MV Test", VesselIMO: "9074729",
		GeneratedAt: time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC),
		Reports: []BundleReport{
			{
				ReportID: "report-1",
				Versions: []*domain.Report{
					{ReportID: "report-1", VersionNo: 1, SchemaName: "log-abstract", EventType: "Departure",
						EventTime: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), State: domain.StateSubmitted,
						Fields: map[string]any{"IMO": 9074729.0}},
				},
				Events: []domain.Event{
					{ReportID: "report-1", VersionNo: 1, Type: domain.EventSubmitted, At: time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC), Actor: "master"},
				},
				Chat: []domain.ChatMessage{
					{ID: "chat-1", ReportID: "report-1", Sender: "reviewer1", Body: "looks good", SentAt: time.Date(2026, 7, 1, 2, 0, 0, 0, time.UTC), Direction: domain.ChatFromOffice},
				},
			},
		},
	}

	plaintext, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	ciphertext, err := backupcrypto.Encrypt(plaintext, identity.PublicKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	decrypted, err := backupcrypto.Decrypt(ciphertext, identity.PrivateKey)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	var got Bundle
	if err := json.Unmarshal(decrypted, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.VesselID != original.VesselID || got.VesselIMO != original.VesselIMO {
		t.Errorf("vessel identity = %+v, want %+v", got, original)
	}
	if len(got.Reports) != 1 || len(got.Reports[0].Versions) != 1 || len(got.Reports[0].Events) != 1 || len(got.Reports[0].Chat) != 1 {
		t.Fatalf("Reports = %+v, want one report with one version/event/chat message each", got.Reports)
	}
	if got.Reports[0].Versions[0].Fields["IMO"] != 9074729.0 {
		t.Errorf("Fields[IMO] = %v, want 9074729", got.Reports[0].Versions[0].Fields["IMO"])
	}
	if got.Reports[0].Chat[0].Body != "looks good" {
		t.Errorf("Chat[0].Body = %q, want %q", got.Reports[0].Chat[0].Body, "looks good")
	}
}
