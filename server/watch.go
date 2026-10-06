package server

import (
	"context"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	defaultPollInterval = 2 * time.Second
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
	var events <-chan fsnotify.Event
	if !s.cfg.DisableWatch {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			s.log.Warn("store watch unavailable, polling only", "err", err)
		} else {
			defer w.Close()
			watchTree(w, s.cfg.TicketStore)
			events = w.Events
			go func() {
				// Drain errors so the watcher never blocks; the poll covers any loss.
				for range w.Errors {
				}
			}()
			s.rewatch = func() { watchTree(w, s.cfg.TicketStore) }
		}
	}

	// Edits made before the watch was armed produced no event.
	s.rescan()

	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	var debounce <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.rescan()
		case <-events:
			if debounce == nil {
				debounce = time.After(watchDebounce)
			}
		case <-debounce:
			debounce = nil
			s.rescan()
		}
	}
}

func (s *Server) rescan() {
	if s.rewatch != nil {
		s.rewatch() // pick up directories created since the last scan
	}
	if err := s.idx.refresh(s.cfg.TicketStore); err != nil {
		s.log.Warn("rescan ticket store", "err", err)
	}
}

// watchTree adds every directory under root. fsnotify is not recursive, and
// re-adding a watched directory is a no-op.
func watchTree(w *fsnotify.Watcher, root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = w.Add(path)
		}
		return nil
	})
}
