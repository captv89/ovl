// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// sessionCookieName is the cookie holding the session token. Distinct
// from vessel/httpapi's ovl_session so the two apps' cookies never
// collide if ever served from the same domain.
const sessionCookieName = "ovl_office_session"

// sessionTTL is a provisional default, matching vessel/httpapi's own
// (neither handoff doc discusses office session lifetime) — long enough
// to cover a work shift without re-login.
const sessionTTL = 12 * time.Hour

type sessionEntry struct {
	userID    string
	expiresAt time.Time
}

// sessionStore is an in-memory session table, same shape and same
// documented limitation as vessel/httpapi's: sessions do not survive a
// process restart, and a multi-replica office deployment would need a
// shared store instead — neither handoff doc specifies office session
// persistence or horizontal scaling, and the Docker Compose deployment
// (architecture 12.1) runs a single ovl-office instance.
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]sessionEntry
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]sessionEntry)}
}

// create issues a new session token for userID.
func (s *sessionStore) create(userID string) (token string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = sessionEntry{userID: userID, expiresAt: time.Now().Add(sessionTTL)}
	return token, nil
}

// lookup returns the userID for token, if it exists and hasn't expired.
// An expired entry is evicted as a side effect.
func (s *sessionStore) lookup(token string) (userID string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, found := s.sessions[token]
	if !found {
		return "", false
	}
	if time.Now().After(e.expiresAt) {
		delete(s.sessions, token)
		return "", false
	}
	return e.userID, true
}

// revoke removes token, if present.
func (s *sessionStore) revoke(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}
