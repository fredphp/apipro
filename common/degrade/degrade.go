package degrade

// Global degrade state machine.
//
// The Manager tracks a single Mode (NORMAL / DEGRADED / PROTECTED / EMERGENCY)
// that every other infrastructure component (CacheManager, multi-limiter, DB
// semaphore, HTTP middleware) consults to decide whether to service a request
// at the current consistency Level.
//
// Design (audit-1C binding):
//   - Mode is stored in an atomic.Int32 so the read path (Mode() /
//     CanServeLevel() / ModeGauge()) is lock-free. These are called on every
//     request.
//   - Mutations (Set / PromoteIfWorse / DemoteIfBetter) take the write side
//     of `mu`. The same mutex also guards the `watchers` slice. Mutation is
//     rare (state transitions, operator overrides) so the lock is cheap.
//   - Watchers are notified AFTER the lock is released so a slow consumer
//     cannot block the state machine. Sends are non-blocking: when the
//     8-slot buffer is full, the oldest pending value is dropped to make
//     room for the newest. Consumers always see the most recent mode.
//
// Level semantics (audit-1C):
//   Level 1 (Display) — /matches.json, /live/hot, /match/cateList, ...
//     Stale-ok, CDN-friendly. Served in every mode except EMERGENCY.
//   Level 2 (Auth)    — /login/login, /user/detail, /live/detail ...
//     Per-user / per-resource. Served in every mode (login must keep
//     answering so users can re-auth during an incident).
//   Level 3 (Write)   — /login/reg, /login/refresh, payment ...
//     Strong consistency. Closed (503) as soon as we leave NORMAL.

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Mode is a degrade-mode ordinal. Stored as int32 inside atomic.Int32.
type Mode int32

const (
	// ModeNormal — full service. All Levels served.
	ModeNormal Mode = iota
	// ModeDegraded — L1 cache fallback, L2 Redis unhealthy.
	// Level 1/2 served, Level 3 closed (503).
	ModeDegraded
	// ModeProtected — same serving matrix as Degraded but writes are
	// fast-failed globally. Used when the DB is the sick dependency.
	ModeProtected
	// ModeEmergency — only /health + /login/login answer. Everything
	// else returns 429. Level 2 stays open so users can re-auth.
	ModeEmergency
)

// String returns a stable uppercase name for logs / metrics labels.
func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeDegraded:
		return "DEGRADED"
	case ModeProtected:
		return "PROTECTED"
	case ModeEmergency:
		return "EMERGENCY"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int32(m))
	}
}

// Severity returns 0 (NORMAL) .. 3 (EMERGENCY). Used to compare modes when
// deciding whether to promote or demote. Unknown modes map to 0 so a
// garbage value can never wedge the manager into a worse-than-EMERGENCY
// state.
func (m Mode) Severity() int {
	switch m {
	case ModeNormal:
		return 0
	case ModeDegraded:
		return 1
	case ModeProtected:
		return 2
	case ModeEmergency:
		return 3
	default:
		return 0
	}
}

// Level is the business-consistency class of an endpoint. See package docs.
type Level int

const (
	LevelDisplay Level = 1
	LevelAuth    Level = 2
	LevelWrite   Level = 3
)

// Manager is the global degrade state machine. Zero-value is NOT valid;
// always construct via New().
type Manager struct {
	mode atomic.Int32

	// mu guards the watchers slice AND the load-compare-store critical
	// section in PromoteIfWorse / DemoteIfBetter (prevents ABA between
	// concurrent promotes / demotes).
	mu       sync.RWMutex
	watchers []chan Mode
}

// New returns a Manager in ModeNormal.
func New() *Manager {
	m := &Manager{}
	m.mode.Store(int32(ModeNormal))
	return m
}

// Mode returns the current Mode. Lock-free.
func (m *Manager) Mode() Mode {
	return Mode(m.mode.Load())
}

// Set unconditionally replaces the current Mode and notifies watchers if
// the value actually changed. Operator overrides and authoritative
// recovery signals use this; component-driven transitions should use
// PromoteIfWorse / DemoteIfBetter instead.
func (m *Manager) Set(mode Mode) {
	m.mu.Lock()
	old := Mode(m.mode.Swap(int32(mode)))
	watchers := m.watchers
	m.mu.Unlock()
	if old != mode {
		m.notify(watchers, mode)
	}
}

// CanServeLevel is the truth table consulted by HTTP middleware, the cache
// layer, and the multi-limiter. Lock-free.
//
//	Mode        | L1 Display | L2 Auth | L3 Write
//	------------+------------+---------+---------
//	NORMAL      |     ✓      |    ✓    |    ✓
//	DEGRADED    |     ✓      |    ✓    |    ✗
//	PROTECTED   |     ✓      |    ✓    |    ✗
//	EMERGENCY   |     ✗      |    ✓    |    ✗
func (m *Manager) CanServeLevel(level Level) bool {
	switch Mode(m.mode.Load()) {
	case ModeNormal:
		return true
	case ModeDegraded, ModeProtected:
		return level == LevelDisplay || level == LevelAuth
	case ModeEmergency:
		// /login/login must keep answering so users can re-auth
		// during an incident — Level 2 stays open.
		return level == LevelAuth
	default:
		return false
	}
}

// PromoteIfWorse escalates to `mode` only if it is more severe than the
// current mode. Returns true iff the state was actually updated.
//
// Used by independent detectors (cache-miss storm, DB pool saturation,
// Redis ping failure) that each observed an incident — only the most
// severe observation wins.
func (m *Manager) PromoteIfWorse(mode Mode) bool {
	m.mu.Lock()
	cur := Mode(m.mode.Load())
	if mode.Severity() > cur.Severity() {
		m.mode.Store(int32(mode))
		watchers := m.watchers
		m.mu.Unlock()
		m.notify(watchers, mode)
		return true
	}
	m.mu.Unlock()
	return false
}

// DemoteIfBetter de-escalates to `mode` only if it is less severe than the
// current mode. Returns true iff the state was actually updated.
//
// Used by recovery probes (Redis ping ok, DB pool draining ok). A single
// recovering component cannot fully clear an EMERGENCY that was triggered
// by another component — it can only step down one notch at a time as
// each subsystem reports recovery.
func (m *Manager) DemoteIfBetter(mode Mode) bool {
	m.mu.Lock()
	cur := Mode(m.mode.Load())
	if mode.Severity() < cur.Severity() {
		m.mode.Store(int32(mode))
		watchers := m.watchers
		m.mu.Unlock()
		m.notify(watchers, mode)
		return true
	}
	m.mu.Unlock()
	return false
}

// Watch returns a buffered (cap 8) channel that receives the new Mode on
// every transition. Consumers MUST select with a default / time.After —
// the manager never blocks on a slow consumer. The channel is never
// closed (Manager lifetime == process lifetime).
func (m *Manager) Watch() <-chan Mode {
	ch := make(chan Mode, 8)
	m.mu.Lock()
	m.watchers = append(m.watchers, ch)
	m.mu.Unlock()
	return ch
}

// ModeGauge returns the current Mode as a float64 for the
// `degrade_mode` Prometheus gauge (0=NORMAL / 1=DEGRADED /
// 2=PROTECTED / 3=EMERGENCY). Lock-free.
func (m *Manager) ModeGauge() float64 {
	return float64(m.mode.Load())
}

// notify fans out the new Mode to every watcher. Non-blocking: when a
// watcher's buffer is full the oldest queued value is dropped so the
// newest always gets through.
//
// Called with no lock held so a slow consumer cannot stall Set /
// PromoteIfWorse / DemoteIfBetter. The `watchers` snapshot was taken
// under the lock; a Watch() that arrives during notify will simply
// miss this transition and receive the next one.
func (m *Manager) notify(watchers []chan Mode, mode Mode) {
	for _, ch := range watchers {
		select {
		case ch <- mode:
		default:
			// Buffer full — drop oldest, then retry once.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- mode:
			default:
			}
		}
	}
}
