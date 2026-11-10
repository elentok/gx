package server

import (
	"strings"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/devcheckout"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/version"
)

// devStatus builds the snapshot's Dev block. The Dev checkout is resolved from
// the live settings on every call, so a reloaded dev-dir shows at once.
func (s *Server) devStatus() wire.DevStatus {
	home := harness.LookupEnv(s.cfg.Env, "HOME")
	root := devcheckout.Resolve(s.settings.Load().WebDevDir, version.DevDir, home)
	dev := wire.DevStatus{State: wire.DevStateOff, HasCheckout: root != "", Checkout: root}
	var reasons []string
	if root == "" {
		reasons = append(reasons, "gx has no Dev checkout")
	}
	if s.web.Listener() == nil {
		reasons = append(reasons, strings.TrimSpace(s.web.Err()))
		if reasons[len(reasons)-1] == "" {
			reasons[len(reasons)-1] = "the web listener is off"
		}
	}
	dev.Reason = strings.Join(reasons, "; ")
	return dev
}
