package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	defaultPollInterval = 2 * time.Second
	// watchedPollInterval is the backstop poll while fsnotify is working.
	watchedPollInterval = 15 * time.Second
	// A burst of writes (an editor's save, a gx command) becomes one rescan.
	watchDebounce = 50 * time.Millisecond
)

// keepFresh rescans the store on every poll tick and, unless the watch is
// disabled or unavailable, whenever the store changes. The poll is the
// guarantee; the watch only makes updates arrive sooner.
func (s *Server) keepFresh(ctx context.Context) {
	if s.cfg.TicketStore == "" {
		return
	}
	poll := s.cfg.PollInterval
	if poll <= 0 {
		poll = defaultPollInterval
	}
	pollUnset := s.cfg.PollInterval <= 0
	var events <-chan fsnotify.Event
	if !s.cfg.DisableWatch {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			s.log.Warn("store watch unavailable, polling only", "err", err)
		} else {
			// kqueue's Close leaves the per-file fds open; Remove closes them.
			defer func() {
				for _, p := range w.WatchList() {
					_ = w.Remove(p)
				}
				w.Close()
			}()
			WatchTree(w, s.cfg.TicketStore)
			events = w.Events
			go func() {
				// Drain errors so the watcher never blocks; the poll covers any loss.
				for range w.Errors {
				}
			}()
			rewatch := func() { WatchTree(w, s.cfg.TicketStore) }
			s.rewatch.Store(&rewatch)
			// The watcher closes on return; a late rescan must not touch it.
			defer s.rewatch.Store(nil)
		}
	}

	// With a live watch the poll is only a backstop for lost events, and each
	// scan reads and hashes every ticket file, so it can be slow.
	if pollUnset && events != nil {
		poll = watchedPollInterval
	}

	// Edits made before the watch was armed produced no event.
	s.rescan()

	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	var debounce <-chan time.Time
	var walk bool
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.rescan()
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
			s.rescanTree(walk)
			walk = false
		}
	}
}

// storeEventKind says whether an event can change the ticket index, and whether
// it is a directory appearing (which needs a re-walk to be watched). Appends to
// run-log.jsonl and other non-ticket files fire constantly and change nothing.
func storeEventKind(ev fsnotify.Event) (relevant, newDir bool) {
	// ".md" also matches fsutil.WriteFileAtomic's "<name>.md.tmp-*": kqueue can
	// report the rename that lands a ticket under the temp name only.
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

// ChangedRequest is the body of POST /v1/tickets/changed: a direct write's
// "address changed" ping.
type ChangedRequest struct {
	Address string `json:"address"`
}

// ticketChanged answers a direct write's ping with an immediate rescan, so the
// stream doesn't wait for the watch or the poll. The file stays the truth: the
// address is informational, the rescan reads every file.
func (s *Server) ticketChanged(w http.ResponseWriter, r *http.Request) {
	var req ChangedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.rescan()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) rescan() {
	s.rescanTree(true)
}

// rescanTree re-reads the store; walk also re-adds directories to the watch,
// which only a directory creation or the backstop poll needs.
func (s *Server) rescanTree(walk bool) {
	if rewatch := s.rewatch.Load(); walk && rewatch != nil {
		(*rewatch)() // pick up directories created since the last scan
	}
	if err := s.idx.refresh(s.cfg.TicketStore); err != nil {
		s.log.Warn("rescan ticket store", "err", err)
	}
	s.scanParentDefects()
	s.scanLateFinishes()
}

// WatchTree adds every directory under root. fsnotify is not recursive, and
// re-adding a watched directory is a no-op.
func WatchTree(w *fsnotify.Watcher, root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = w.Add(path)
		}
		return nil
	})
}
