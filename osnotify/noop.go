package osnotify

import "context"

type noop struct{}

func (noop) Notify(context.Context, Notification) error { return nil }
