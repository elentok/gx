package tree

import tea "charm.land/bubbletea/v2"

// LazyState is a LazySection's load state.
type LazyState int

const (
	LazyIdle LazyState = iota
	LazyLoading
	LazyLoaded
	LazyFailed
)

// LazyResultMsg is the tea.Msg a LazySection's Expand command produces and
// Deliver consumes.
type LazyResultMsg[T any] struct {
	Rows []T
	Err  error
}

// LazySection holds a tree node's not-yet-populated children: an up-front
// count (known before anything loads, e.g. a cheap directory listing), a
// load state, the loaded rows once available, and any load error. It is a
// standalone generic value type, not a new tree.Model node kind — indices
// into a not-yet-populated slice don't exist until the load result is
// assigned, so a consumer builds its own child rows from Rows() once loaded
// rather than ui/tree modeling the load itself. ui/tree renders nothing for
// it: rendering requires labeling a T, which is opaque to this package.
type LazySection[T any] struct {
	count       int
	loadedCount int
	state       LazyState
	rows        []T
	err         error
	load        func() ([]T, error)
}

// NewLazySection constructs a LazySection with an up-front count and the
// load function Expand's returned command calls.
func NewLazySection[T any](count int, load func() ([]T, error)) LazySection[T] {
	return LazySection[T]{count: count, load: load}
}

// Expand returns the load command the first time (or after Invalidate, or
// after a prior load failed) — a no-op once already loading or loaded.
func (l *LazySection[T]) Expand() tea.Cmd {
	if l.state == LazyLoading || l.state == LazyLoaded {
		return nil
	}
	l.state = LazyLoading
	load := l.load
	return func() tea.Msg {
		rows, err := load()
		return LazyResultMsg[T]{Rows: rows, Err: err}
	}
}

// Deliver feeds a load-result message in, reporting whether it was
// consumed — a caller with more than one in-flight lazy load (or a message
// switch dispatching to several) can try each in turn.
func (l *LazySection[T]) Deliver(msg tea.Msg) bool {
	result, ok := msg.(LazyResultMsg[T])
	if !ok {
		return false
	}
	if result.Err != nil {
		l.state = LazyFailed
		l.err = result.Err
		l.rows = nil
		return true
	}
	l.state = LazyLoaded
	l.err = nil
	l.rows = result.Rows
	l.loadedCount = l.count
	return true
}

// SetCount updates the up-front count. If it differs from the count the
// cache was loaded at, the cache is auto-invalidated so the header and the
// loaded rows can never silently disagree — e.g. a background auto-refresh
// changing the count is enough to force a re-scan on the next expand,
// without requiring a manual refresh.
func (l *LazySection[T]) SetCount(n int) {
	l.count = n
	if l.state == LazyLoaded && n != l.loadedCount {
		l.Invalidate()
	}
}

// Invalidate explicitly clears the cache, forcing the next Expand to return
// a load command again.
func (l *LazySection[T]) Invalidate() {
	l.state = LazyIdle
	l.rows = nil
	l.err = nil
}

// Rows returns the loaded children, for the consumer to splice into its own
// child-building. Empty until State is LazyLoaded.
func (l LazySection[T]) Rows() []T {
	return l.rows
}

// State reports the current load state.
func (l LazySection[T]) State() LazyState {
	return l.state
}

// Err returns the most recent load error, set when State is LazyFailed.
func (l LazySection[T]) Err() error {
	return l.err
}
