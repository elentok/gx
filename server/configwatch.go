package server

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// defaultConfigWatchDebounce turns an editor's burst of events into one reload.
const defaultConfigWatchDebounce = 300 * time.Millisecond

// configWatch is the config watcher's lifecycle; every field is guarded by
// Server.reloadMu.
type configWatch struct {
	parent  context.Context // Serve's context; nil until Serve starts
	cancel  context.CancelFunc
	running sync.WaitGroup
}

// startConfigWatch arms the watcher under ctx if server.watch-config is on.
func (s *Server) startConfigWatch(ctx context.Context) {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	s.configWatch.parent = ctx
	s.setConfigWatchLocked(s.settings.Load().WatchConfig)
}

// setConfigWatchLocked stops any watcher by cancelling its context and starts
// a fresh one when on. It never waits for the old watcher: a reload the
// watcher itself triggered calls this, and waiting would deadlock.
func (s *Server) setConfigWatchLocked(on bool) {
	w := &s.configWatch
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	if !on || w.parent == nil || w.parent.Err() != nil || s.cfg.ConfigPath == "" {
		return
	}
	// Armed before returning, so a save right after startup or a reload is seen.
	cw := s.armConfigWatch()
	if cw == nil {
		return
	}
	ctx, cancel := context.WithCancel(w.parent)
	w.cancel = cancel
	w.running.Add(1)
	go func() {
		defer w.running.Done()
		defer cw.fs.Close()
		s.watchConfig(ctx, cw)
	}()
}

// configWatcher is one armed watcher's state.
type configWatcher struct {
	fs        *fsnotify.Watcher
	path, dir string
	names     map[string]bool // paths whose events count
	last      []byte          // bytes of the last reload attempt; nil after a missing file
	missing   bool            // config-missing already reported
}

// armConfigWatch watches the config file's directory, not the file, because
// editors save by renaming a temp file over the original, which drops a watch
// on the file itself. nil means the watch could not start.
func (s *Server) armConfigWatch() *configWatcher {
	path := filepath.Clean(s.cfg.ConfigPath)
	cw := &configWatcher{path: path, dir: filepath.Dir(path)}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		s.log.Warn("config watch unavailable", "err", err)
		return nil
	}
	if err := fw.Add(cw.dir); err != nil {
		fw.Close()
		s.log.Warn("config watch stopped: config directory unavailable", "dir", cw.dir, "err", err)
		return nil
	}
	cw.fs = fw
	cw.names = map[string]bool{path: true, cw.dir: true}
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if target, err := filepath.EvalSymlinks(path); err == nil && fw.Add(filepath.Dir(target)) == nil {
			cw.names[target] = true
		}
	}
	// The running settings came from these bytes, so an untouched save is no reload.
	var readErr error
	cw.last, readErr = os.ReadFile(path)
	cw.missing = errors.Is(readErr, fs.ErrNotExist)
	return cw
}

// watchConfig reloads the config file once each burst of events goes quiet.
func (s *Server) watchConfig(ctx context.Context, cw *configWatcher) {
	debounce := s.cfg.ConfigWatchDebounce
	if debounce <= 0 {
		debounce = defaultConfigWatchDebounce
	}
	var quiet <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-cw.fs.Events:
			if !ok {
				return
			}
			if cw.names[filepath.Clean(ev.Name)] {
				quiet = time.After(debounce)
			}
		case err, ok := <-cw.fs.Errors:
			if !ok {
				return
			}
			s.log.Warn("config watch error", "err", err)
		case <-quiet:
			quiet = nil
			if _, err := os.Stat(cw.dir); errors.Is(err, fs.ErrNotExist) {
				s.log.Warn("config watch stopped: config directory removed", "dir", cw.dir)
				return
			}
			s.reloadSaved(cw)
		}
	}
}

// reloadSaved reloads unless the file's bytes match the last attempt, and
// reports a missing file once until it exists again.
func (s *Server) reloadSaved(cw *configWatcher) {
	data, err := os.ReadFile(cw.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if cw.missing {
			return
		}
		cw.missing, cw.last = true, nil
	case err == nil && cw.last != nil && bytes.Equal(data, cw.last):
		s.log.Debug("config unchanged since the last reload attempt", "path", cw.path)
		return
	default:
		cw.missing, cw.last = false, data
	}
	s.reloadConfig(reloadTriggerWatch)
}
