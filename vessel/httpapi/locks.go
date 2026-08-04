// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"sort"
	"sync"
	"time"

	"github.com/captv89/ovl/vessel/auth"
)

// lockTTL is architecture 9.5's "Locks expire after inactivity (default
// 5 minutes, configurable)" — not actually configurable yet (same "needs
// a settings screen" reasoning as vessel/main.go's syncInterval), so a
// bare constant until one exists.
const lockTTL = 5 * time.Minute

// sectionLock is one held section soft lock (architecture 9.5).
type sectionLock struct {
	ReportID   string
	Section    string
	UserID     string
	Username   string
	Role       auth.Role
	AcquiredAt time.Time
	RenewedAt  time.Time
}

func (l sectionLock) expired(now time.Time, ttl time.Duration) bool {
	return now.After(l.RenewedAt.Add(ttl))
}

// lockEvent is what subscribers of a report's lock stream receive —
// broadcast by acquire, release, forceRelease, and the expiry sweep.
type lockEvent struct {
	Type string // "locked" | "unlocked"
	Lock sectionLock
}

type lockKey [2]string // {reportID, section}

// lockManager is an in-memory table of active section soft locks,
// keyed by (reportID, section). It does not survive a vessel binary
// restart — same "acceptable, and not specified otherwise" reasoning as
// sessionStore: a restart releasing every held lock is correct behavior,
// not a data-loss risk (nothing about a lock itself is durable state).
// now is a field (not time.Now called directly) so tests can drive
// expiry deterministically without sleeping, the same "pure function of
// now" idiom vessel/main.go's nextNightlyRun already uses.
type lockManager struct {
	mu    sync.Mutex
	locks map[lockKey]sectionLock
	ttl   time.Duration
	now   func() time.Time

	subMu sync.Mutex
	subs  map[string]map[chan lockEvent]struct{} // keyed by reportID
}

func newLockManager() *lockManager {
	return &lockManager{
		locks: make(map[lockKey]sectionLock),
		subs:  make(map[string]map[chan lockEvent]struct{}),
		ttl:   lockTTL,
		now:   time.Now,
	}
}

// acquire claims reportID/section for userID, succeeding if it is
// unlocked, expired, or already held by userID (a renewal — AcquiredAt
// is preserved across a renewal, only RenewedAt advances). Returns
// ok=false and the existing lock if it's held by someone else and not
// expired, so the caller can report who holds it.
func (m *lockManager) acquire(reportID, section, userID, username string, role auth.Role) (sectionLock, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := lockKey{reportID, section}
	now := m.now()
	existing, found := m.locks[key]
	if found && existing.UserID != userID && !existing.expired(now, m.ttl) {
		return existing, false
	}
	acquiredAt := now
	if found && existing.UserID == userID {
		acquiredAt = existing.AcquiredAt
	}
	lock := sectionLock{
		ReportID: reportID, Section: section,
		UserID: userID, Username: username, Role: role,
		AcquiredAt: acquiredAt, RenewedAt: now,
	}
	m.locks[key] = lock
	return lock, true
}

// release removes reportID/section's lock, but only if userID is the
// current holder — releasing someone else's lock this way is silently a
// no-op (use forceRelease for that).
func (m *lockManager) release(reportID, section, userID string) (sectionLock, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := lockKey{reportID, section}
	existing, found := m.locks[key]
	if !found || existing.UserID != userID {
		return sectionLock{}, false
	}
	delete(m.locks, key)
	return existing, true
}

// forceRelease removes reportID/section's lock regardless of holder —
// architecture 9.3/9.5's Master-only force-release. Authorization is the
// caller's job (see requireSuperAdmin).
func (m *lockManager) forceRelease(reportID, section string) (sectionLock, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := lockKey{reportID, section}
	existing, found := m.locks[key]
	if !found {
		return sectionLock{}, false
	}
	delete(m.locks, key)
	return existing, true
}

// holder returns reportID/section's current unexpired lock, if any — the
// backstop handleSaveSection checks before persisting a change.
func (m *lockManager) holder(reportID, section string) (sectionLock, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, found := m.locks[lockKey{reportID, section}]
	if !found || existing.expired(m.now(), m.ttl) {
		return sectionLock{}, false
	}
	return existing, true
}

// snapshot returns every active (unexpired) lock for reportID, sorted by
// section for a stable response body.
func (m *lockManager) snapshot(reportID string) []sectionLock {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	out := make([]sectionLock, 0, len(m.locks))
	for key, lock := range m.locks {
		if key[0] != reportID || lock.expired(now, m.ttl) {
			continue
		}
		out = append(out, lock)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Section < out[j].Section })
	return out
}

// sweep removes every lock expired as of now, across all reports, and
// returns them — the caller broadcasts each as "unlocked" so an idle
// lock's release is visible live, not just the next time someone happens
// to touch it via acquire/release/holder.
func (m *lockManager) sweep(now time.Time) []sectionLock {
	m.mu.Lock()
	defer m.mu.Unlock()
	var expired []sectionLock
	for key, lock := range m.locks {
		if lock.expired(now, m.ttl) {
			expired = append(expired, lock)
			delete(m.locks, key)
		}
	}
	return expired
}

// subscribe registers a channel for reportID's lock events. cancel must
// always be called (e.g. via defer) when the subscriber — an SSE handler
// — returns, or the channel and its map entry leak.
func (m *lockManager) subscribe(reportID string) (ch chan lockEvent, cancel func()) {
	ch = make(chan lockEvent, 16)
	m.subMu.Lock()
	if m.subs[reportID] == nil {
		m.subs[reportID] = make(map[chan lockEvent]struct{})
	}
	m.subs[reportID][ch] = struct{}{}
	m.subMu.Unlock()

	cancel = func() {
		m.subMu.Lock()
		defer m.subMu.Unlock()
		delete(m.subs[reportID], ch)
		if len(m.subs[reportID]) == 0 {
			delete(m.subs, reportID)
		}
	}
	return ch, cancel
}

// broadcast fans ev out to every current subscriber of reportID. A slow
// subscriber's channel is dropped-from (non-blocking send), never
// blocking the broadcaster — a missed live event is corrected by that
// subscriber's own next snapshot/reconnect, not retried here.
func (m *lockManager) broadcast(reportID string, ev lockEvent) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	for ch := range m.subs[reportID] {
		select {
		case ch <- ev:
		default:
		}
	}
}
