package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/devcheckout"
	"github.com/elentok/gx/server/devvite"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/version"
)

// devHost is the Dev mode logic. It is its own type, embedding the Server, so
// these methods stay off Server.
type devHost struct{ *Server }

// newDevVite builds the Vite supervisor, off until the settings ask for it. The
// web UI follows its state: Vite's proxy while it runs, the built files otherwise.
func (s devHost) newDevVite() error {
	sup, err := devvite.New(devvite.Config{
		LogDir:   filepath.Join(s.cfg.StateDir, "logs", "vite"),
		Env:      s.cfg.Env,
		Launcher: s.cfg.ViteLauncher,
		Observer: func(snap devvite.Snapshot) {
			if snap.State == devvite.StateRunning {
				s.web.UseViteProxy(snap.Port)
				if !s.viteServed.Swap(true) {
					s.events.Publish(wire.EventReload, "")
				}
			} else {
				// No reload here: a page on a Vite that went down is reloaded
				// by nothing, and the pages of a stopping Vite were already told
				// by applyDevMode.
				s.web.UseBuiltUI()
				s.viteServed.Store(false)
			}
			s.events.Publish(wire.EventDevChanged, "")
		},
	})
	if err != nil {
		return err
	}
	s.vite = sup
	s.applyDevMode(*s.settings.Load())
	return nil
}

// applyDevMode tells the supervisor what is wanted. It returns at once, so it
// is safe under the reload lock.
func (s devHost) applyDevMode(next Settings) {
	root, on, _ := s.devPlan(next)
	if !on || s.vite.Snapshot().Root != root {
		// The supervisor reports a stop only after Vite has stopped; until then
		// the proxy would answer 502.
		s.web.UseBuiltUI()
		if s.viteServed.Swap(false) {
			s.events.Publish(wire.EventReload, "")
		}
	}
	s.vite.Set(on, root)
}

// devPlan is the one rule for whether Dev mode is on: the Dev checkout it runs
// from, and when it cannot run, why not. The reason is empty when on is only
// false because dev-frontend is off.
func (s devHost) devPlan(st Settings) (root string, on bool, reason string) {
	home := harness.LookupEnv(s.cfg.Env, "HOME")
	root = devcheckout.Resolve(st.WebDevDir, version.DevDir, home)
	var reasons []string
	if root == "" {
		reasons = append(reasons, "gx has no Dev checkout")
	}
	if s.web.Listener() == nil {
		why := strings.TrimSpace(s.web.Err())
		if why == "" {
			why = "the web listener is off"
		}
		reasons = append(reasons, why)
	}
	return root, st.WebDevFrontend && len(reasons) == 0, strings.Join(reasons, "; ")
}

// devRoutes is the Vite restart, which only the web UI offers (ADR 0028).
func (s devHost) devRoutes() []wire.Route {
	return []wire.Route{{Pattern: "POST /v1/dev/restart", Web: wire.WebWrite, WebHandler: s.devRestart}}
}

func (s devHost) devRestart(w http.ResponseWriter, _ *http.Request) {
	if err := s.vite.Restart(); err != nil {
		wire.WriteJSON(w, wire.DevRestartResult{Refused: true, Reason: wire.ReasonDevNotDown,
			Message: "Vite can only be restarted while it is down"})
		return
	}
	wire.WriteJSON(w, wire.DevRestartResult{})
}

// ApplyDevMode also announces a changed Dev checkout or reason: the supervisor
// only reports state changes, and a settings page enables its dev-frontend row
// from has_checkout.
func (h reloadHost) ApplyDevMode(prev, next Settings) {
	d := devHost{h.s}
	prevRoot, _, prevReason := d.devPlan(prev)
	nextRoot, _, nextReason := d.devPlan(next)
	d.applyDevMode(next)
	if prevRoot != nextRoot || prevReason != nextReason {
		h.s.events.Publish(wire.EventDevChanged, "")
	}
}

// devStatus builds the snapshot's Dev block. The Dev checkout is resolved from
// the live settings on every call, so a reloaded dev-dir shows at once.
func (s devHost) devStatus() wire.DevStatus {
	root, _, reason := s.devPlan(*s.settings.Load())
	dev := wire.DevStatus{State: wire.DevStateOff, HasCheckout: root != "", Checkout: root, Reason: reason}
	if s.vite != nil {
		if snap := s.vite.Snapshot(); snap.State != devvite.StateOff {
			dev.State, dev.Reason, dev.LogTail = wire.DevState(snap.State), snap.Reason, snap.Logs
		}
	}
	return dev
}

// warnIgnoredDevURL logs once when the config file still holds the removed
// server.web.dev-url. The file is only read, never rewritten.
func warnIgnoredDevURL(log *slog.Logger, configPath string) {
	if configPath == "" {
		return
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	var raw struct {
		Server struct {
			Web map[string]json.RawMessage `json:"web"`
		} `json:"server"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return
	}
	if _, ok := raw.Server.Web["dev-url"]; ok {
		log.Warn("server.web.dev-url is ignored: Dev mode now runs its own Vite from the Dev checkout", "key", "server.web.dev-url", "config", configPath)
	}
}
