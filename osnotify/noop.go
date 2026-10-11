package osnotify

import "context"

type noop struct{}

func (noop) Notify(context.Context, string, string) error { return nil }
