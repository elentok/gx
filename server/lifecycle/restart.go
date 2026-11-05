package lifecycle

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"sort"
	"sync/atomic"
	"syscall"

	"github.com/elentok/gx/server/wire"
)

// RestartConfig is what the restart routes read from the server's Config.
type RestartConfig struct {
	Build    string
	StateDir string
	Env      []string
	// RestartBinary names the binary a restart would start; nil means this one.
	RestartBinary func() string
	// SpawnRestart replaces the real spawn; nil means spawn the checked binary.
	SpawnRestart func(bin string) error
}

// Restarter serves the restart check and the restart itself.
type Restarter struct {
	cfg RestartConfig
	// agents counts loop agents and live interactive sessions.
	agents func() int
	// instanceID is new on every start: a restart is done when it changes.
	instanceID string
	// restarting is set once a restart is under way and never cleared on
	// success: the new server replaces this one.
	restarting atomic.Bool
}

// NewRestarter returns a Restarter with a fresh instance id.
func NewRestarter(cfg RestartConfig, agents func() int) *Restarter {
	return &Restarter{cfg: cfg, agents: agents, instanceID: newInstanceID()}
}

func newInstanceID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Instance is the id that changes on every server start.
func (rs *Restarter) Instance() string { return rs.instanceID }

// ServeWrite handles POST /v1/restart.
func (rs *Restarter) ServeWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.RestartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// The flag stays set after a start: the new server replaces this one, so a
	// second request would race it. It is cleared only when the spawn fails.
	if !rs.restarting.CompareAndSwap(false, true) {
		wire.WriteJSON(w, wire.RestartResult{Refused: true, Reason: wire.ReasonRestartInProgress, Restarting: true,
			Message: "a restart is already running"})
		return
	}
	res := rs.restart(r, req)
	if !res.Started {
		rs.restarting.Store(false)
	}
	wire.WriteJSON(w, res)
}

func (rs *Restarter) restart(r *http.Request, req wire.RestartRequest) wire.RestartResult {
	rep := rs.Check(r.Context())
	if rep.Status != wire.RestartCheckOK {
		return wire.RestartResult{Refused: true, Reason: wire.ReasonRestartCheck, Report: &rep,
			Message: "the restart check failed: " + rep.Error}
	}
	if !restartConfirmed(req, rep) {
		return wire.RestartResult{Refused: true, Reason: wire.ReasonRestartChanged, Report: &rep,
			Message: "the check changed since you looked at it"}
	}
	spawn := rs.cfg.SpawnRestart
	if spawn == nil {
		spawn = rs.spawnRestart
	}
	if err := spawn(rep.Binary); err != nil {
		return wire.RestartResult{Refused: true, Reason: wire.ReasonRestartCheck, Report: &rep,
			Message: "could not start the restart: " + err.Error()}
	}
	return wire.RestartResult{Started: true, Restarting: true, RestartLog: rep.RestartLog}
}

// restartConfirmed reports whether the fresh check still matches what the
// dialog showed.
func restartConfirmed(req wire.RestartRequest, rep wire.RestartCheckReport) bool {
	return req.NewVersion == rep.NewVersion && req.ConfigOK == rep.Config.OK &&
		slices.Equal(sortedCopy(req.FailingProjects), failingProjects(rep))
}

func failingProjects(rep wire.RestartCheckReport) []string {
	out := []string{}
	for _, p := range rep.Projects {
		if !p.OK {
			out = append(out, p.Name)
		}
	}
	sort.Strings(out)
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

// spawnRestart starts `gx server restart --continue` from the checked binary in
// its own session, so it outlives this server, which it stops. It logs to
// restart.log itself.
func (rs *Restarter) spawnRestart(bin string) error {
	cmd := exec.Command(bin, "server", "restart", "--continue")
	cmd.Env = rs.cfg.Env
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
