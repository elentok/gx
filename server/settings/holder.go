package settings

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/storecommit"
)

// Publisher streams one event to the clients.
type Publisher interface {
	PublishEvent(ev wire.Event)
}

// Startup is what the server started with. A reload only compares the config
// against it, to warn about a key that needs a restart.
type Startup struct {
	TicketStore string
	TCPListen   bool
	WebAddr     string
	WebDevURL   string
	AgentRunner string
}

// Options wires a Holder to the root.
type Options struct {
	// ConfigPath is the config file a reload reads; empty means reload is
	// unavailable.
	ConfigPath string
	Startup    Startup
	// WatchDebounce turns an editor's burst of events into one reload; zero
	// means the default.
	WatchDebounce time.Duration
	Notifier      osnotify.Notifier
	Log           *slog.Logger
	Events        Publisher
	// Env is the environment the store commit loop runs git under.
	Env []string
	// HardStopAt is when the hard-limit grace countdown ends; zero when none runs.
	HardStopAt func() time.Time
}

// Holder is the live settings and what changes them: the reload, its status,
// the config watcher and the store commit loop that follows the settings. The
// zero value holds empty settings and cannot reload.
type Holder struct {
	cur  atomic.Pointer[Settings]
	opts Options
	// mu serializes config reloads. It is also taken by whatever must not
	// interleave with one (the budget's grace countdown).
	mu       sync.Mutex
	appliers Appliers
	history  history
	// commits is nil until the store commit loop starts; guarded by mu so a
	// reload racing the start is never lost.
	commits *storecommit.Loop
	// watch runs the config watcher; guarded by mu, so a reload toggling
	// server.watch-config only ever cancels a context.
	watch watch
}

// Init gives the holder its wiring; call it once, before the server serves.
func (h *Holder) Init(opts Options) { h.opts = opts }

// SetAppliers registers what a reload applies outside this package.
func (h *Holder) SetAppliers(a Appliers) { h.appliers = a }

// Load returns the live settings. A reader loads it once per decision.
func (h *Holder) Load() *Settings { return h.cur.Load() }

// Store replaces the live settings; a reload swaps in a new value and never
// mutates the old one.
func (h *Holder) Store(s *Settings) { h.cur.Store(s) }

// Lock and Unlock hold off reloads. The holder is passed by pointer, never copied.
func (h *Holder) Lock()   { h.mu.Lock() }
func (h *Holder) Unlock() { h.mu.Unlock() }

// LockReload holds the reload lock until the returned func runs, so a test can
// keep a reload in flight.
func (h *Holder) LockReload() (unlock func()) {
	h.mu.Lock()
	return h.mu.Unlock
}

// HardStopAt is the config status's grace countdown deadline.
func (h *Holder) HardStopAt() time.Time { return h.Status().HardStopAt }

// LiveDiskSpace is the disk-space settings the server currently runs on.
func (h *Holder) LiveDiskSpace() config.DiskSpaceConfig { return h.Load().DiskSpace }

// StartStoreCommits makes the server the store's committer and commits edits
// made while it was down.
func (h *Holder) StartStoreCommits() (stop func(), err error) {
	if h.opts.Startup.TicketStore == "" {
		return func() {}, nil
	}
	h.mu.Lock()
	set := h.Load()
	loop, err := storecommit.StartWith(h.opts.Startup.TicketStore, storecommit.Options{
		Debounce:   set.storeCommitDebounce(),
		PushRemote: set.StorePushRemote,
		OnError:    func(err error) { h.opts.Log.Warn("store push failed", "err", err) },
		Env:        h.opts.Env,
	})
	h.commits = loop
	h.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if err := loop.Flush(); err != nil {
		h.opts.Log.Warn("committing down-time store edits failed", "err", err)
	}
	return loop.Stop, nil
}

// ApplyStoreCommits is the reload step that retunes the running commit loop.
func (h *Holder) ApplyStoreCommits(old, next Settings) func() {
	if h.commits != nil && (next.StoreCommitDebounce != old.StoreCommitDebounce || next.StorePushRemote != old.StorePushRemote) {
		h.commits.SetSettings(next.storeCommitDebounce(), next.StorePushRemote)
	}
	return nil
}

// configBannerGroup makes each config banner replace the previous one.
const configBannerGroup = "gx-config"

// Banner shows one OS banner and streams it as a config-reloaded event, for
// the TUI to toast at its level.
func (h *Holder) Banner(level, title, body string) {
	go func() {
		if err := h.opts.Notifier.Notify(context.Background(), osnotify.Notification{Title: title, Body: body, Group: configBannerGroup}); err != nil {
			h.opts.Log.Warn("OS banner failed", "title", title, "error", err)
		}
	}()
	h.opts.Events.PublishEvent(wire.Event{Type: wire.EventConfigReloaded, Level: level, Message: joinBanner(title, body)})
}
