package server

import (
	"github.com/elentok/gx/server/devvite"
	"github.com/elentok/gx/server/wire"
)

// devStatus builds the snapshot's Dev block. The Dev checkout is resolved from
// the live settings on every call, so a reloaded dev-dir shows at once.
func (s *Server) devStatus() wire.DevStatus {
	root, _, reason := s.devPlan(*s.settings.Load())
	dev := wire.DevStatus{State: wire.DevStateOff, HasCheckout: root != "", Checkout: root, Reason: reason}
	if s.vite != nil {
		if snap := s.vite.Snapshot(); snap.State != devvite.StateOff {
			dev.State, dev.Reason, dev.LogTail = wire.DevState(snap.State), snap.Reason, snap.Logs
		}
	}
	return dev
}
