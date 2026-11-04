package runs

import (
	"strings"
	"sync"
	"time"
)

// Guards is the run state shared by the merge, late-finish and scheduler code:
// who is refused, which merges are in flight, which trunks are locked, and the
// settle goroutines and runner kick.
type Guards struct {
	// Refused holds the tickets the last claim pass refused for a changed file.
	Refused Refusals
	// HeldMerges holds the roots whose merge waits for a trunk-conflict ticket.
	HeldMerges Refusals
	Merges     MergeGuard
	Trunks     TrunkLocks
	// AdoptMu is held by a late-finish adoption from its finish to the cancel of
	// unstarted investigations; the runner claims under it. One owner for the
	// protocol, so merge and sched both take it from here.
	AdoptMu sync.Mutex

	settling sync.WaitGroup
	kick     chan struct{}
}

// NewGuards returns empty guards with the runner kick wired.
func NewGuards() *Guards {
	return &Guards{kick: make(chan struct{}, 1)}
}

// Refusals holds the tickets the last claim pass refused for a changed file,
// until a claim or a later pass that finds the file unchanged clears them.
type Refusals struct {
	mu   sync.Mutex
	addr map[string]bool
}

func (r *Refusals) Set(addr string, refused bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.addr == nil {
		r.addr = map[string]bool{}
	}
	if refused {
		r.addr[addr] = true
	} else {
		delete(r.addr, addr)
	}
}

func (r *Refusals) Has(addr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.addr[addr]
}

// MergeGuard lets a stop wait for merges in flight and keeps new ones from
// starting: a merge cut off halfway leaves a ticket the store cannot explain.
type MergeGuard struct {
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

// Begin reports false once the server is stopping; the caller must then not merge.
func (g *MergeGuard) Begin() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return false
	}
	g.wg.Add(1)
	return true
}

func (g *MergeGuard) End() { g.wg.Done() }

// CloseAndWait refuses new merges, then reports whether the running ones
// finished within timeout.
func (g *MergeGuard) CloseAndWait(timeout time.Duration) bool {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
	return waitTimeout(&g.wg, timeout)
}

// TrunkLocks serializes root merges per repo and trunk. Unserialized, two
// merges into one trunk race on git's own locks and the second fails instead
// of seeing a clean non-ff.
type TrunkLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func (l *TrunkLocks) Lock(repo, trunk string) (unlock func()) {
	m := l.mutex(repo, trunk)
	m.Lock()
	return m.Unlock
}

func (l *TrunkLocks) mutex(repo, trunk string) *sync.Mutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := repo + "\x00" + trunk
	m := l.locks[key]
	if m == nil {
		if l.locks == nil {
			l.locks = map[string]*sync.Mutex{}
		}
		m = &sync.Mutex{}
		l.locks[key] = m
	}
	return m
}

// TryLock is Lock without the wait: ok is false when the trunk is held.
func (l *TrunkLocks) TryLock(repo, trunk string) (unlock func(), ok bool) {
	m := l.mutex(repo, trunk)
	if !m.TryLock() {
		return nil, false
	}
	return m.Unlock, true
}

// Locked lists the trunks any merge has locked so far, in any repo.
func (l *TrunkLocks) Locked() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var trunks []string
	for key := range l.locks {
		_, trunk, _ := strings.Cut(key, "\x00")
		trunks = append(trunks, trunk)
	}
	return trunks
}

// GoSettle runs f, a finishRun, settleRun or recoverFrom, on its own
// goroutine. Each writes to the ticket store and may outlive Serve's stop.
func (g *Guards) GoSettle(f func()) {
	g.settling.Add(1)
	go func() {
		defer g.settling.Done()
		f()
	}()
}

// WaitSettled waits for every GoSettle goroutine and reports whether they all
// ended within timeout. Serve's stop leaves them running (agents outlive the
// server), so a test must wait before deleting their dirs.
func (g *Guards) WaitSettled(timeout time.Duration) bool {
	return waitTimeout(&g.settling, timeout)
}

// Kick asks the runner to look at the queue now instead of at its next tick.
func (g *Guards) Kick() {
	select {
	case g.kick <- struct{}{}:
	default:
	}
}

// Kicked fires once per Kick that has not been consumed yet.
func (g *Guards) Kicked() <-chan struct{} { return g.kick }

func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
