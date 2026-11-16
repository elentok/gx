package server

import (
	"context"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/sound"
)

// quietNotifier drops banners while quiet. A dropped banner is not retried.
// Push has no sender in gx; when it gets one it must ask the same predicate.
type quietNotifier struct {
	next  osnotify.Notifier
	quiet func() bool
}

func (n quietNotifier) Notify(ctx context.Context, note osnotify.Notification) error {
	if n.quiet() {
		return nil
	}
	return n.next.Notify(ctx, note)
}

// quietPlayer drops sounds while quiet.
type quietPlayer struct {
	next  sound.Player
	quiet func() bool
}

func (p quietPlayer) Play(ctx context.Context, cfg config.AgentsSoundsConfig, name sound.Name) {
	if p.quiet() {
		return
	}
	p.next.Play(ctx, cfg, name)
}
