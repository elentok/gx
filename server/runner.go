package server

import "time"

// WaitSettled waits for every settle goroutine; see runs.Guards.WaitSettled.
func (s *Server) WaitSettled(timeout time.Duration) bool {
	return s.guards.WaitSettled(timeout)
}
