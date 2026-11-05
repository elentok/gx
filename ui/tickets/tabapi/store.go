package tabapi

import (
	"path/filepath"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/tickets"
)

// storeWatchDebounce turns a burst of writes into one signal.
const storeWatchDebounce = 50 * time.Millisecond

// ScratchDir resolves worktreeRoot's repo to its project directory in the
// ticket store, so every linked worktree shares one tracker. Falls back to
// the plain join when worktreeRoot isn't inside a git repo (e.g. test
// fixtures that use a bare temp dir). A repo with no store project gets a
// path that doesn't exist, which renders the empty state rather than reading
// the legacy `.scratch`.
func ScratchDir(store, worktreeRoot string) string {
	if _, err := git.FindRepo(worktreeRoot); err != nil {
		return filepath.Join(worktreeRoot, ".scratch")
	}
	root, err := tickets.RootIn(store, worktreeRoot)
	if err != nil {
		return filepath.Join(worktreeRoot, ".gx-no-ticket-project")
	}
	return root
}

// WatchStore signals (debounced) on every change under dir. stop ends the
// watch and closes the channel.
func WatchStore(dir string) (events <-chan struct{}, stop func(), err error) {
	w, err := index.NewStoreWatch(dir)
	if err != nil {
		return nil, nil, err
	}
	out := make(chan struct{}, 1)
	go func() {
		defer close(out)
		var debounce <-chan time.Time
		for {
			select {
			case _, ok := <-w.Events:
				if !ok {
					return
				}
				if debounce == nil {
					debounce = time.After(storeWatchDebounce)
				}
			case <-debounce:
				debounce = nil
				w.Rewatch() // pick up directories created since
				select {
				case out <- struct{}{}:
				default: // a signal is already pending
				}
			}
		}
	}()
	return out, w.Close, nil
}
