package db

// Semaphore is a counting-semaphore gate sitting ABOVE the database/sql
// connection pool. Its purpose (audit-1B DZ-3 mitigation) is to bound the
// number of goroutines that may simultaneously contend for a pooled DB
// connection, so that when the pool (max 50 conns in db.go) is saturated,
// excess callers fail-fast via ctx cancellation instead of ALL blocking
// inside db.QueryContext waiting for a free connection — which would amplify
// tail latency and risk a goroutine leak under load.
//
// A nil *Semaphore means "no limit": every method is a no-op (or returns a
// benign value) so callers can disable throttling by passing nil.
//
// Usage:
//
//	sem := db.NewSemaphore("mysql-main", 50)
//	if err := sem.Acquire(ctx); err != nil { return err }
//	defer sem.Release()
//	rows, err := db.QueryContext(ctx, ...)
//
// Or with WithToken to guarantee pairing (and panic-safety):
//
//	err := sem.WithToken(ctx, func() error {
//	    return db.QueryRowContext(ctx, ...).Scan(&v)
//	})

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/zeromicro/go-zero/core/logx"
)

// Semaphore limits concurrent DB queries using a buffered channel as a
// counting semaphore. The channel is used in "send-to-acquire" mode:
// Acquire sends a struct{}{} into the buffered channel; Release receives.
// Hence len(s.tokens) == number of currently-held tokens, and the available
// count is total - len(s.tokens).
type Semaphore struct {
	name     string
	tokens   chan struct{}
	total    int32
	waiting  int64 // atomic — goroutines currently blocked in Acquire
	acquired int64 // atomic — cumulative successful Acquire/TryAcquire count
	rejected int64 // atomic — cumulative ctx-cancel rejections in Acquire
}

// NewSemaphore creates a Semaphore that allows at most maxConcurrent concurrent
// holders. If maxConcurrent <= 0, it returns nil (meaning "no limit").
func NewSemaphore(name string, maxConcurrent int) *Semaphore {
	if maxConcurrent <= 0 {
		return nil
	}
	return &Semaphore{
		name:   name,
		tokens: make(chan struct{}, maxConcurrent),
		total:  int32(maxConcurrent),
	}
}

// Acquire blocks until a token is available or ctx is canceled.
// Returns ctx.Err() when ctx is canceled before a token is acquired.
// On a nil Semaphore, returns nil immediately (no limit).
//
// The waiting counter is incremented on entry and decremented on exit (via
// defer), so it always reflects the number of goroutines currently parked
// inside Acquire. The rejected counter is bumped once per ctx-canceled Acquire.
func (s *Semaphore) Acquire(ctx context.Context) error {
	if s == nil {
		return nil
	}
	atomic.AddInt64(&s.waiting, 1)
	defer atomic.AddInt64(&s.waiting, -1)

	select {
	case s.tokens <- struct{}{}:
		atomic.AddInt64(&s.acquired, 1)
		return nil
	case <-ctx.Done():
		atomic.AddInt64(&s.rejected, 1)
		// waiting still includes this goroutine (defer runs on return) —
		// that's the meaningful number: "how many were queued when this one gave up".
		waiting := atomic.LoadInt64(&s.waiting)
		logx.Errorf("db semaphore %q: acquire canceled (ctx=%v), waiting=%d", s.name, ctx.Err(), waiting)
		return ctx.Err()
	}
}

// TryAcquire attempts to acquire a token without blocking.
// Returns true on success, false if no token is currently available.
// The caller MUST call Release only when this returns true.
// On a nil Semaphore, returns true (no limit).
func (s *Semaphore) TryAcquire() bool {
	if s == nil {
		return true
	}
	select {
	case s.tokens <- struct{}{}:
		atomic.AddInt64(&s.acquired, 1)
		return true
	default:
		return false
	}
}

// Release returns a token. Must be called exactly once for each successful
// Acquire or TryAcquire. On a nil Semaphore, it is a no-op.
//
// Release never blocks: a defensive select+default guards against bugs where
// Release is called without a prior successful Acquire (logged at Error level
// so the bug surfaces in production without deadlocking the pool).
func (s *Semaphore) Release() {
	if s == nil {
		return
	}
	select {
	case <-s.tokens:
	default:
		logx.Errorf("db semaphore %q: Release called without holding a token (possible unpaired Release)", s.name)
	}
}

// WithToken wraps Acquire -> f -> Release.
//   - If Acquire fails (ctx canceled), f is NOT invoked and the error is returned.
//   - If f panics, Release is still invoked via defer (panic propagates).
//   - On a nil Semaphore, f is invoked directly without acquire/release.
func (s *Semaphore) WithToken(ctx context.Context, f func() error) error {
	if s == nil {
		return f()
	}
	if err := s.Acquire(ctx); err != nil {
		return err
	}
	defer s.Release()
	return f()
}

// Stats returns a snapshot of the semaphore's state.
//   - available:      tokens currently free (total - held)
//   - total:          total token count (max concurrent)
//   - waiting:        goroutines currently blocked in Acquire
//   - acquiredTotal:  cumulative successful Acquire/TryAcquire calls
//   - rejectedTotal:  cumulative Acquire calls rejected due to ctx cancellation
//
// All fields are read atomically; safe for concurrent use.
// On a nil Semaphore, returns all zeros.
func (s *Semaphore) Stats() (available, total int, waiting, acquiredTotal, rejectedTotal int64) {
	if s == nil {
		return 0, 0, 0, 0, 0
	}
	held := int32(len(s.tokens)) // channel len is atomic in Go
	avail := s.total - held
	if avail < 0 {
		avail = 0
	}
	return int(avail), int(s.total),
		atomic.LoadInt64(&s.waiting),
		atomic.LoadInt64(&s.acquired),
		atomic.LoadInt64(&s.rejected)
}

// Name returns the semaphore's name. Returns "" for a nil Semaphore.
func (s *Semaphore) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

// String implements fmt.Stringer.
func (s *Semaphore) String() string {
	if s == nil {
		return "Semaphore(nil)"
	}
	avail, total, waiting, acquired, rejected := s.Stats()
	return fmt.Sprintf("Semaphore(name=%s, available=%d/%d, waiting=%d, acquired=%d, rejected=%d)",
		s.name, avail, total, waiting, acquired, rejected)
}
