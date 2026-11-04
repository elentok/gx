package index

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/fsnotify/fsnotify"
)

const (
	// DefaultPollInterval is the poll when WatchConfig.PollInterval is unset
	// and there is no live watch.
	DefaultPollInterval = 2 * time.Second
	// watchedPollInterval is the backstop poll while fsnotify is working.
	watchedPollInterval = 15 * time.Second
	// A burst of writes (an editor's save, a gx command) becomes one rescan.
	watchDebounce = 50 * time.Millisecond
)

// WatchConfig is what KeepFresh needs from the server.
type WatchConfig struct {
	// PollInterval is the rescan period; zero picks the default, and a live
	// watch stretches the default to a backstop.
	PollInterval time.Duration
	DisableWatch bool
	// Start runs once after the watch is armed, or at once when there is no
	// store. It reports false when ctx ended first, which ends KeepFresh.
	Start func(ctx context.Context) bool
}

// KeepFresh rescans the store on every poll tick and, unless the watch is
// disabled or unavailable, whenever the store changes. The poll is the
// guarantee; the watch only makes updates arrive sooner.
func (i *Index) KeepFresh(ctx context.Context, cfg WatchConfig) {
	if i.storePath == "" {
		cfg.Start(ctx)
		return
	}
	poll := cfg.PollInterval
	if poll <= 0 {
		poll = DefaultPollInterval
	}
	pollUnset := cfg.PollInterval <= 0
	var events <-chan fsnotify.Event
	if !cfg.DisableWatch {
		w, err := NewStoreWatch(i.storePath)
		if err != nil {
			i.log.Warn("store watch unavailable, polling only", "err", err)
		} else {
			defer w.Close()
			events = w.Events
			rewatch := w.Rewatch
			i.rewatch.Store(&rewatch)
			// The watcher closes on return; a late rescan must not touch it.
			defer i.rewatch.Store(nil)
		}
	}

	// With a live watch the poll is only a backstop for lost events, and each
	// scan reads and hashes every ticket file, so it can be slow.
	if pollUnset && events != nil {
		poll = watchedPollInterval
	}

	// Edits made before the watch was armed produced no event; the first scan
	// (inside Start) picks them up once the migrations end.
	if !cfg.Start(ctx) {
		return
	}

	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	var debounce <-chan time.Time
	var walk bool
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			i.Rescan()
		case ev := <-events:
			relevant, dir := storeEventKind(ev)
			if !relevant {
				continue
			}
			walk = walk || dir
			if debounce == nil {
				debounce = time.After(watchDebounce)
			}
		case <-debounce:
			debounce = nil
			i.rescanTree(walk)
			walk = false
		}
	}
}

// storeEventKind says whether an event can change the ticket index, and whether
// it is a directory appearing (which needs a re-walk to be watched). Appends to
// run-log.jsonl and other non-ticket files fire constantly and change nothing.
func storeEventKind(ev fsnotify.Event) (relevant, newDir bool) {
	// ".md" also matches fsutil.WriteFileAtomic's "<name>.md.tmp-*": kqueue can
	// report the rename that merges a ticket under the temp name only.
	if strings.Contains(filepath.Base(ev.Name), ".md") {
		return true, false
	}
	if ev.Has(fsnotify.Create) {
		fi, err := os.Stat(ev.Name)
		isDir := err == nil && fi.IsDir()
		return isDir, isDir
	}
	// A removed or renamed directory has no way to be told from a file now;
	// extensionless names are directories in the store layout.
	return ev.Has(fsnotify.Remove|fsnotify.Rename) && filepath.Ext(ev.Name) == "", false
}

// ticketChanged answers a direct write's ping with an immediate rescan, so the
// stream doesn't wait for the watch or the poll. The file stays the truth: the
// address is informational, the rescan reads every file. migrated closes when
// the start-up migrations end.
func (i *Index) ticketChanged(migrated func() <-chan struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.ChangedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Until the migrations end a rescan reads nothing, so the caller would
		// not see its own write. The wait ends with the request.
		select {
		case <-migrated():
		case <-r.Context().Done():
			return
		}
		i.Rescan()
		w.WriteHeader(http.StatusNoContent)
	}
}

// Rescan re-reads the store, as a ping or the watch would, then runs the
// OnRescan steps. It is the only entry for a rescan.
func (i *Index) Rescan() {
	i.rescanTree(true)
}

// rescanTree re-reads the store; walk also re-adds directories to the watch,
// which only a directory creation or the backstop poll needs.
func (i *Index) rescanTree(walk bool) {
	// A migration is rewriting tickets; the first scan after it ends reads them.
	if i.ready != nil && !i.ready() {
		return
	}
	if rewatch := i.rewatch.Load(); walk && rewatch != nil {
		(*rewatch)() // pick up directories created since the last scan
	}
	if err := i.Refresh(); err != nil {
		i.log.Warn("rescan ticket store", "err", err)
	}
	for _, step := range i.onRescan {
		step()
	}
}

// WatchTree adds every directory under root. fsnotify is not recursive, and
// re-adding a watched directory is a no-op. It skips .git: the tickets repo's
// object store holds thousands of directories and would exhaust the file limit.
func WatchTree(w *fsnotify.Watcher, root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		_ = w.Add(path)
		return nil
	})
}
