package servertest

import "github.com/elentok/gx/server"

// WithEvolve is a Start option that sets the evolve mode.
func WithEvolve(mode string) func(*server.Config) {
	return func(c *server.Config) { c.Evolve = mode }
}
