package servertest

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/sound"
)

// Banners is an osnotify.Notifier that records every banner.
type Banners struct {
	mu   sync.Mutex
	sent []osnotify.Notification
}

func (b *Banners) Notify(_ context.Context, note osnotify.Notification) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, note)
	return nil
}

// Sent returns the banners recorded so far.
func (b *Banners) Sent() []osnotify.Notification {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.sent)
}

// SessionBannerDelay is a harness's session banner delay: long enough for a
// test to merge a second trigger inside the window, shorter than the real one.
const SessionBannerDelay = 300 * time.Millisecond

// Sounds is a sound.Player that records every sound played.
type Sounds struct {
	mu     sync.Mutex
	played []sound.Name
}

func (p *Sounds) Play(_ context.Context, _ config.AgentsSoundsConfig, name sound.Name) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.played = append(p.played, name)
}

// Played returns the sounds recorded so far.
func (p *Sounds) Played() []sound.Name {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.played)
}
