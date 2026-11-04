package settings

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

// defaultWatchDebounce turns an editor's burst of events into one reload.
const defaultWatchDebounce = 300 * time.Millisecond

// watch is the config watcher's lifecycle; every field is guarded by Holder.mu.
type watch struct {
	parent  context.Context // Serve's context; nil until Serve starts
	cancel  context.CancelFunc
	running sync.WaitGroup
}

// StartConfigWatch arms the watcher under ctx if server.watch-config is on.
func (h *Holder) StartConfigWatch(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.watch.parent = ctx
	h.setWatchLocked(h.Load().WatchConfig)
}

// WaitConfigWatch waits for the watcher to stop; cancel its context first.
func (h *Holder) WaitConfigWatch() { h.watch.running.Wait() }

// ApplyWatchConfig is the reload step that follows server.watch-config.
func (h *Holder) ApplyWatchConfig(old, next Settings) func() {
	if next.WatchConfig != old.WatchConfig {
		h.setWatchLocked(next.WatchConfig)
	}
	return nil
}

// setWatchLocked stops any watcher by cancelling its context and starts
// a fresh one when on. It never waits for the old watcher: a reload the
// watcher itself triggered calls this, and waiting would deadlock.
func (h *Holder) setWatchLocked(on bool) {
	w := &h.watch
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	if !on || w.parent == nil || w.parent.Err() != nil || h.opts.ConfigPath == "" {
		return
	}
	// Armed before returning, so a save right after startup or a reload is seen.
	cw := h.armWatch()
	if cw == nil {
		return
	}
	ctx, cancel := context.WithCancel(w.parent)
	w.cancel = cancel
	w.running.Add(1)
	go func() {
		defer w.running.Done()
		defer cw.fs.Close()
		h.watchConfig(ctx, cw)
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

// armWatch watches the config file's directory, not the file, because
// editors save by renaming a temp file over the original, which drops a watch
// on the file itself. nil means the watch could not start.
func (h *Holder) armWatch() *configWatcher {
	path := filepath.Clean(h.opts.ConfigPath)
	cw := &configWatcher{path: path, dir: filepath.Dir(path)}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		h.opts.Log.Warn("config watch unavailable", "err", err)
		return nil
	}
	if err := fw.Add(cw.dir); err != nil {
		fw.Close()
		h.opts.Log.Warn("config watch stopped: config directory unavailable", "dir", cw.dir, "err", err)
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
func (h *Holder) watchConfig(ctx context.Context, cw *configWatcher) {
	debounce := h.opts.WatchDebounce
	if debounce <= 0 {
		debounce = defaultWatchDebounce
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
			h.opts.Log.Warn("config watch error", "err", err)
		case <-quiet:
			quiet = nil
			if _, err := os.Stat(cw.dir); errors.Is(err, fs.ErrNotExist) {
				h.opts.Log.Warn("config watch stopped: config directory removed", "dir", cw.dir)
				return
			}
			h.reloadSaved(cw)
		}
	}
}

// reloadSaved reloads unless the file's bytes match the last attempt, and
// reports a missing file once until it exists again.
func (h *Holder) reloadSaved(cw *configWatcher) {
	data, err := os.ReadFile(cw.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if cw.missing {
			return
		}
		cw.missing, cw.last = true, nil
	case err == nil && cw.last != nil && bytes.Equal(data, cw.last):
		h.opts.Log.Debug("config unchanged since the last reload attempt", "path", cw.path)
		return
	default:
		cw.missing, cw.last = false, data
	}
	h.Reload(TriggerWatch)
}
