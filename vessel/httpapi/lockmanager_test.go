// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"sync"
	"testing"
	"time"

	"github.com/captv89/ovl/vessel/auth"
)

func newTestLockManager(now time.Time) *lockManager {
	m := newLockManager()
	m.now = func() time.Time { return now }
	return m
}

func TestLockManager_AcquireRenewRelease(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := newTestLockManager(base)

	lock, ok := m.acquire("r1", "engine.consumption", "u1", "2/O Sharma", auth.RoleSecondOfficer)
	if !ok {
		t.Fatal("first acquire should succeed")
	}
	if lock.AcquiredAt != base || lock.RenewedAt != base {
		t.Errorf("lock = %+v, want AcquiredAt=RenewedAt=%v", lock, base)
	}

	// A different user cannot acquire the same still-live lock.
	if _, ok := m.acquire("r1", "engine.consumption", "u2", "C/E Nguyen", auth.RoleChiefEngineer); ok {
		t.Error("second user's acquire on a live lock should fail")
	}
	if conflict, ok := m.acquire("r1", "engine.consumption", "u2", "C/E Nguyen", auth.RoleChiefEngineer); ok || conflict.UserID != "u1" {
		t.Errorf("conflicting acquire should report the existing holder u1, got %+v ok=%v", conflict, ok)
	}

	// The same user re-acquiring renews: AcquiredAt is preserved, RenewedAt advances.
	m.now = func() time.Time { return base.Add(2 * time.Minute) }
	renewed, ok := m.acquire("r1", "engine.consumption", "u1", "2/O Sharma", auth.RoleSecondOfficer)
	if !ok {
		t.Fatal("renewal by the current holder should succeed")
	}
	if renewed.AcquiredAt != base {
		t.Errorf("AcquiredAt after renewal = %v, want unchanged %v", renewed.AcquiredAt, base)
	}
	if renewed.RenewedAt != base.Add(2*time.Minute) {
		t.Errorf("RenewedAt after renewal = %v, want %v", renewed.RenewedAt, base.Add(2*time.Minute))
	}

	// A non-holder's release is a no-op.
	if _, ok := m.release("r1", "engine.consumption", "u2"); ok {
		t.Error("release by a non-holder should be a no-op")
	}
	if _, ok := m.holder("r1", "engine.consumption"); !ok {
		t.Error("lock should still be held after a non-holder's release attempt")
	}

	// The real holder can release.
	if _, ok := m.release("r1", "engine.consumption", "u1"); !ok {
		t.Error("release by the holder should succeed")
	}
	if _, ok := m.holder("r1", "engine.consumption"); ok {
		t.Error("lock should be gone after the holder releases it")
	}
}

func TestLockManager_Expiry(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := newTestLockManager(base)
	m.ttl = 5 * time.Minute

	if _, ok := m.acquire("r1", "position", "u1", "2/O Sharma", auth.RoleSecondOfficer); !ok {
		t.Fatal("acquire should succeed")
	}

	// Just before expiry: still held.
	m.now = func() time.Time { return base.Add(5*time.Minute - time.Second) }
	if _, ok := m.holder("r1", "position"); !ok {
		t.Error("lock should still be held just before TTL elapses")
	}

	// Past expiry: a new user can now acquire it, and holder reports none.
	m.now = func() time.Time { return base.Add(5*time.Minute + time.Second) }
	if _, ok := m.holder("r1", "position"); ok {
		t.Error("holder should report not-found once expired")
	}
	if _, ok := m.acquire("r1", "position", "u2", "C/E Nguyen", auth.RoleChiefEngineer); !ok {
		t.Error("acquire by a different user should succeed once the old lock expired")
	}
}

func TestLockManager_ForceRelease(t *testing.T) {
	m := newTestLockManager(time.Now())
	m.acquire("r1", "cargo", "u1", "2/O Sharma", auth.RoleSecondOfficer)

	if _, ok := m.forceRelease("r1", "cargo"); !ok {
		t.Error("force-release of a held lock should succeed")
	}
	if _, ok := m.holder("r1", "cargo"); ok {
		t.Error("lock should be gone after force-release")
	}
	if _, ok := m.forceRelease("r1", "cargo"); ok {
		t.Error("force-releasing an already-unheld section should be a no-op, not an error condition")
	}
}

func TestLockManager_Snapshot(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := newTestLockManager(base)
	m.ttl = 5 * time.Minute

	m.acquire("r1", "weather", "u1", "2/O Sharma", auth.RoleSecondOfficer)
	m.acquire("r1", "cargo", "u2", "C/E Nguyen", auth.RoleChiefEngineer)
	m.acquire("r2", "weather", "u3", "3/O Alvarez", auth.RoleThirdOfficer)

	// An expired lock on r1 must be excluded from the snapshot.
	m.now = func() time.Time { return base.Add(-10 * time.Minute) }
	m.acquire("r1", "rob", "u4", "Stale Holder", auth.RoleSecondOfficer)
	m.now = func() time.Time { return base }

	got := m.snapshot("r1")
	if len(got) != 2 {
		t.Fatalf("snapshot(r1) = %d locks, want 2 (cargo, weather — rob expired, r2's lock excluded)", len(got))
	}
	if got[0].Section != "cargo" || got[1].Section != "weather" {
		t.Errorf("snapshot(r1) sections = [%s, %s], want sorted [cargo, weather]", got[0].Section, got[1].Section)
	}
}

func TestLockManager_Sweep(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := newTestLockManager(base)
	m.ttl = 5 * time.Minute

	m.acquire("r1", "weather", "u1", "2/O Sharma", auth.RoleSecondOfficer) // will expire
	m.now = func() time.Time { return base.Add(1 * time.Minute) }
	m.acquire("r1", "cargo", "u2", "C/E Nguyen", auth.RoleChiefEngineer) // stays fresh

	expired := m.sweep(base.Add(6 * time.Minute))
	if len(expired) != 1 || expired[0].Section != "weather" {
		t.Fatalf("sweep returned %+v, want exactly the expired weather lock", expired)
	}
	if _, ok := m.holder("r1", "weather"); ok {
		t.Error("swept lock should no longer be held")
	}
	if _, ok := m.holder("r1", "cargo"); !ok {
		t.Error("unexpired lock should survive the sweep")
	}
}

func TestLockManager_Subscribe_ScopedToReportID(t *testing.T) {
	m := newLockManager()
	chA, cancelA := m.subscribe("r1")
	defer cancelA()
	chB, cancelB := m.subscribe("r2")
	defer cancelB()

	m.broadcast("r1", lockEvent{Type: "locked", Lock: sectionLock{ReportID: "r1", Section: "weather"}})

	select {
	case ev := <-chA:
		if ev.Lock.Section != "weather" {
			t.Errorf("chA got %+v, want weather", ev)
		}
	default:
		t.Error("chA (subscribed to r1) should have received the broadcast")
	}
	select {
	case ev := <-chB:
		t.Errorf("chB (subscribed to r2) should not have received r1's broadcast, got %+v", ev)
	default:
	}
}

func TestLockManager_ConcurrentAcquire(t *testing.T) {
	m := newLockManager()
	const n = 50
	var wg sync.WaitGroup
	results := make([]bool, n)
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, ok := m.acquire("r1", "weather", string(rune('a'+i)), "user", auth.RoleSecondOfficer)
			results[i] = ok
		}(i)
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, ok := range results {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Errorf("concurrent acquire: %d goroutines won, want exactly 1", wins)
	}
}
