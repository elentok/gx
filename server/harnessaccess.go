package server

import (
	"github.com/elentok/gx/server/budget"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/webauth"
)

// queueHost is queue.Host: the harness methods the queue reads, and the rest
// from the root.
type queueHost struct {
	*Server
	*harnesschoice.Harness
}

// Harness is the server's harness choice. Tests outside the package read it
// through here.
func (s *Server) Harness() *harnesschoice.Harness { return s.harness }

// Limits is the usage limits and token poll. Tests outside the package read
// their seams through here.
func (s *Server) Limits() *limits.Limits { return s.limits }

// Budget is the daily spend budget. Tests outside the package read its seams
// through here.
func (s *Server) Budget() *budget.Budget { return s.budget }

// WebAuth is the web login gate, nil unless the web listener is bound. Tests
// outside the package read its seams through here.
func (s *Server) WebAuth() *webauth.Gate { return s.webAuth }
