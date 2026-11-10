package prflow

import (
	"time"

	"github.com/elentok/gx/git/pr"
)

// seenHead is the head SHA the watcher last saw for an epic and when it first did.
type seenHead struct {
	sha string
	at  time.Time
}

// act is the per-poll dispatch: later PR tickets add their action here, each in
// its own function. An error follows the watcher's outage rules.
func (w *Watcher) act(r pr.Runner, repo string, t Target, v View) error {
	if handled, err := w.closeOrMerge(t, v); handled || err != nil {
		return err
	}
	if err := w.markReady(r, repo, t, v); err != nil {
		return err
	}
	if stop, err := w.startConflictFix(t, v); stop || err != nil {
		return err
	}
	return w.startCIFix(r, repo, t, v)
}

// markReady takes a green draft out of draft, once per head SHA. A PR with no
// checks counts as green once the grace window has passed, measured from the
// later of "PR opened" and the moment this head was first seen. A cancelled,
// stale or action-required check is neither green nor failed, so it waits.
func (w *Watcher) markReady(r pr.Runner, repo string, t Target, v View) error {
	k := epicKey{t.Project, t.Epic}
	now := w.now()
	w.mu.Lock()
	seen := w.seen[k]
	if seen.sha != v.HeadSHA {
		seen = seenHead{sha: v.HeadSHA, at: now}
		w.seen[k] = seen
	}
	done := w.readied[k] == v.HeadSHA
	w.mu.Unlock()
	if v.State != "OPEN" || !v.Draft || done {
		return nil
	}
	switch v.CI {
	case pr.ChecksGreen:
	case pr.ChecksNone:
		since := seen.at
		if opened, err := LastOpened(t.Dir); err == nil && opened.After(since) {
			since = opened
		}
		if now.Sub(since) < w.host.NoChecksGrace() {
			return nil
		}
	default:
		return nil
	}
	if err := r.MarkReady(repo, t.Number); err != nil {
		return err
	}
	w.mu.Lock()
	w.readied[k] = v.HeadSHA
	v.Draft = false
	w.views[k] = v
	w.mu.Unlock()
	_ = Append(t.Dir, Entry{Kind: KindMarkedReady, HeadSHA: v.HeadSHA})
	return nil
}
