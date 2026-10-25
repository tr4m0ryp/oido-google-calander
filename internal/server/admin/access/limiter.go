package access

import (
	"sync"
	"time"

	"github.com/tr4m0ryp/oido-google-calander/internal/config"
)

// Failed-login throttling, in memory for a single instance. Per IP,
// MaxFailures failures inside a sliding Window lock that IP out for Lockout.
// Globally, GlobalMaxFailures failures across all IPs inside Window block
// every attempt until the oldest ages out, slowing distributed guessing.
// A successful login clears the caller's record.

const (
	maxTrackedIPs = 10_000
	pruneInterval = time.Minute
)

type ipRecord struct {
	failures    []time.Time // inside the window, oldest first
	lockedUntil time.Time
}

// Status is the outcome of a limiter check.
type Status struct {
	Blocked    bool
	RetryAfter time.Duration
	Remaining  int // attempts left before lockout (when not blocked)
}

// Limiter tracks failed admin logins.
type Limiter struct {
	limits config.LoginLimits
	now    func() time.Time

	mu        sync.Mutex
	ips       map[string]*ipRecord
	global    []time.Time
	lastPrune time.Time
}

// NewLimiter builds a limiter; now is injectable for tests.
func NewLimiter(limits config.LoginLimits, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{limits: limits, now: now, ips: map[string]*ipRecord{}}
}

// Check reports whether ip may attempt a login now. Call it before
// comparing the password so a locked-out caller learns nothing.
func (l *Limiter) Check(ip string) Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.check(ip, l.now())
}

func (l *Limiter) check(ip string, t time.Time) Status {
	rec := l.ips[ip]
	if rec != nil && rec.lockedUntil.After(t) {
		return Status{Blocked: true, RetryAfter: rec.lockedUntil.Sub(t)}
	}
	if wait := l.globalWait(t); wait > 0 {
		return Status{Blocked: true, RetryAfter: wait}
	}
	used := 0
	if rec != nil {
		used = len(l.live(rec.failures, t))
	}
	return Status{Remaining: l.limits.MaxFailures - used}
}

// Failure records a wrong password from ip and reports the resulting status.
func (l *Limiter) Failure(ip string) Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.now()
	l.prune(t)
	if l.limits.GlobalMaxFailures > 0 {
		l.global = append(l.live(l.global, t), t)
		if over := len(l.global) - l.limits.GlobalMaxFailures; over > 0 {
			l.global = l.global[over:]
		}
	}
	rec := l.ips[ip]
	if rec == nil {
		if len(l.ips) >= maxTrackedIPs {
			l.evictOne()
		}
		rec = &ipRecord{}
		l.ips[ip] = rec
	}
	rec.failures = append(l.live(rec.failures, t), t)
	if len(rec.failures) >= l.limits.MaxFailures {
		rec.failures = nil
		rec.lockedUntil = t.Add(l.limits.Lockout)
	}
	return l.check(ip, t)
}

// Success clears ip's failure record.
func (l *Limiter) Success(ip string) {
	l.mu.Lock()
	delete(l.ips, ip)
	l.mu.Unlock()
}

func (l *Limiter) globalWait(t time.Time) time.Duration {
	max := l.limits.GlobalMaxFailures
	if max <= 0 {
		return 0
	}
	live := l.live(l.global, t)
	if len(live) < max {
		return 0
	}
	return live[len(live)-max].Add(l.limits.Window).Sub(t)
}

func (l *Limiter) live(failures []time.Time, t time.Time) []time.Time {
	cutoff := t.Add(-l.limits.Window)
	i := 0
	for i < len(failures) && !failures[i].After(cutoff) {
		i++
	}
	return failures[i:]
}

// prune drops records with no lockout and no failures left in the window.
func (l *Limiter) prune(t time.Time) {
	if t.Sub(l.lastPrune) < pruneInterval && len(l.ips) < maxTrackedIPs {
		return
	}
	l.lastPrune = t
	for ip, rec := range l.ips {
		if !rec.lockedUntil.After(t) && len(l.live(rec.failures, t)) == 0 {
			delete(l.ips, ip)
		}
	}
}

// evictOne frees a slot when the table is full, preferring unlocked records.
func (l *Limiter) evictOne() {
	t := l.now()
	for ip, rec := range l.ips {
		if !rec.lockedUntil.After(t) {
			delete(l.ips, ip)
			return
		}
	}
	for ip := range l.ips {
		delete(l.ips, ip)
		return
	}
}
