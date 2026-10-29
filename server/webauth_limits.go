package server

import (
	"sync"
	"time"
)

const (
	authIPFreeFailures    = 5
	authIPBaseWait        = time.Minute
	authIPMaxWait         = time.Hour
	authGlobalFailures    = 50
	authGlobalWindow      = time.Hour
	authGlobalLockoutWait = 15 * time.Minute
)

type authIPState struct {
	failures     int
	blockedUntil time.Time
}

// authLimiter throttles failed logins in memory. Each IP waits after every
// round of 5 failures, doubling up to an hour. Too many failures from all IPs
// together make every check wait.
type authLimiter struct {
	mu          sync.Mutex
	now         func() time.Time
	ips         map[string]*authIPState
	recent      []time.Time
	globalUntil time.Time
}

func newAuthLimiter(now func() time.Time) *authLimiter {
	return &authLimiter{now: now, ips: map[string]*authIPState{}}
}

// Wait returns the seconds the caller must wait before ip may try again, or 0.
func (l *authLimiter) Wait(ip string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	until := l.globalUntil
	if s := l.ips[ip]; s != nil && s.blockedUntil.After(until) {
		until = s.blockedUntil
	}
	if !until.After(now) {
		return 0
	}
	return int((until.Sub(now) + time.Second - 1) / time.Second)
}

// Fail records a failed attempt from ip.
func (l *authLimiter) Fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()

	s := l.ips[ip]
	if s == nil {
		s = &authIPState{}
		l.ips[ip] = s
	}
	s.failures++
	if s.failures%authIPFreeFailures == 0 {
		wait := authIPBaseWait
		for round := s.failures / authIPFreeFailures; round > 1 && wait < authIPMaxWait; round-- {
			wait *= 2
		}
		s.blockedUntil = now.Add(min(wait, authIPMaxWait))
	}

	cutoff := now.Add(-authGlobalWindow)
	kept := l.recent[:0]
	for _, t := range l.recent {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.recent = append(kept, now)
	if len(l.recent) >= authGlobalFailures {
		l.globalUntil = now.Add(authGlobalLockoutWait)
		l.recent = nil
	}
}

// Success clears ip's failure count.
func (l *authLimiter) Success(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.ips, ip)
}

// Reset clears all counts and waits.
func (l *authLimiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ips = map[string]*authIPState{}
	l.recent = nil
	l.globalUntil = time.Time{}
}
