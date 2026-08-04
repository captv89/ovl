// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/backupcrypto"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/restorebundle"
	"github.com/captv89/ovl/vessel/store"
)

// postRestoreBundle POSTs raw ciphertext bytes to the import endpoint —
// testClient.do only knows how to marshal JSON bodies (see
// attachments_test.go's uploadAttachment for the same reasoning), and
// this endpoint's body is an opaque age-encrypted blob, not JSON.
func postRestoreBundle(t *testing.T, c *testClient, confirm bool, ciphertext []byte) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/admin/restore-bundle/import"
	if confirm {
		path += "?confirm=true"
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(ciphertext))
	req.Header.Set("Content-Type", "application/octet-stream")
	for _, ck := range c.jar {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	c.server.Handler().ServeHTTP(rec, req)
	return rec
}

// TestHandleImportRestoreBundle_Roundtrip is architecture 12.5's DR exit
// criterion exercised at the vessel's own HTTP surface: a bundle shaped
// exactly like office/httpapi's buildRestoreBundle would produce,
// encrypted against this vessel's own DR public key (exactly as
// office/httpapi's handleGenerateRestoreBundle would encrypt it), gets
// decrypted and applied, and the vessel's own store ends up holding the
// same reports/events/chat the bundle carried.
func TestHandleImportRestoreBundle_Roundtrip(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	st := s.storeOrNil()

	identity, err := backupcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	if err := st.SaveDRIdentity(t.Context(), &store.DRIdentity{
		PublicKey: identity.PublicKey, PrivateKey: identity.PrivateKey, IssuedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveDRIdentity: %v", err)
	}

	eventTime := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	bundle := restorebundle.Bundle{
		VesselID: "vessel-1", VesselName: "MV Test", VesselIMO: "9074729",
		GeneratedAt: time.Now().UTC(),
		Reports: []restorebundle.BundleReport{
			{
				ReportID: "restored-report-1",
				Versions: []*domain.Report{{
					ReportID: "restored-report-1", VersionNo: 1,
					SchemaName: "log-abstract", EventType: "Departure", EventTime: eventTime,
					Fields: map[string]any{"IMO": 9074729.0}, State: domain.StateSubmitted,
					CreatedAt: eventTime, CreatedBy: "master", UpdatedAt: eventTime,
					SubmittedAt: eventTime, SubmittedBy: "master",
				}},
				Events: []domain.Event{
					{ReportID: "restored-report-1", VersionNo: 1, Type: domain.EventSubmitted, At: eventTime, Actor: "master"},
				},
				Chat: []domain.ChatMessage{
					{ID: "chat-restored-1", ReportID: "restored-report-1", Sender: "reviewer1", Body: "confirmed", SentAt: eventTime, Direction: domain.ChatFromOffice},
				},
			},
		},
	}
	plaintext, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	ciphertext, err := backupcrypto.Encrypt(plaintext, identity.PublicKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if rec := postRestoreBundle(t, c, false, ciphertext); rec.Code != http.StatusBadRequest {
		t.Errorf("import without confirm: status %d, want %d", rec.Code, http.StatusBadRequest)
	}

	rec := postRestoreBundle(t, c, true, ciphertext)
	if rec.Code != http.StatusOK {
		t.Fatalf("import: status %d, body %s", rec.Code, rec.Body)
	}
	result := decodeBody[restoreBundleImportResult](t, rec)
	if result.Reports != 1 || result.Versions != 1 || result.Events != 1 || result.ChatMessages != 1 {
		t.Errorf("result = %+v, want one of each", result)
	}

	rec = c.do(http.MethodGet, "/api/reports/restored-report-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get imported report: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[reportView](t, rec)
	if got.Fields["IMO"] != 9074729.0 {
		t.Errorf("Fields[IMO] = %v, want 9074729", got.Fields["IMO"])
	}

	rec = c.do(http.MethodGet, "/api/reports/restored-report-1/chat", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get imported chat: status %d, body %s", rec.Code, rec.Body)
	}
	chat := decodeBody[[]chatMessageView](t, rec)
	if len(chat) != 1 || chat[0].Body != "confirmed" {
		t.Errorf("chat = %+v, want one message with body %q", chat, "confirmed")
	}
}

func TestHandleImportRestoreBundle_NoDRIdentity(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	rec := postRestoreBundle(t, c, true, []byte("garbage"))
	if rec.Code != http.StatusConflict {
		t.Errorf("import with no DR identity on file: status %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleImportRestoreBundle_WrongKey(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	st := s.storeOrNil()
	identity, err := backupcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	if err := st.SaveDRIdentity(t.Context(), &store.DRIdentity{
		PublicKey: identity.PublicKey, PrivateKey: identity.PrivateKey, IssuedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveDRIdentity: %v", err)
	}

	other, err := backupcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	ciphertext, err := backupcrypto.Encrypt([]byte(`{}`), other.PublicKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	rec := postRestoreBundle(t, c, true, ciphertext)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("import encrypted against a different vessel's key: status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
