package server

import "github.com/elentok/gx/events"

// PauseAs pauses a ticket as the given kind the way the server's own pauses do,
// including starting recovery. Tests in other packages call it, so it cannot
// live in an export_test.go.
func (s *Server) PauseAs(address string, kind events.Kind, reason string) error {
	return s.writer.PauseAs(address, kind, reason)
}
