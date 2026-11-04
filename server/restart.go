package server

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"sort"
	"syscall"

	"github.com/elentok/gx/server/wire"
)

func (s *Server) restartWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.RestartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// The flag stays set after a start: the new server replaces this one, so a
	// second request would race it. It is cleared only when the spawn fails.
	if !s.restarting.CompareAndSwap(false, true) {
		writeJSON(w, wire.RestartResult{Refused: true, Reason: wire.ReasonRestartInProgress, Restarting: true,
			Message: "a restart is already running"})
		return
	}
	res := s.restart(r, req)
	if !res.Started {
		s.restarting.Store(false)
	}
	writeJSON(w, res)
}

func (s *Server) restart(r *http.Request, req wire.RestartRequest) wire.RestartResult {
	rep := s.restartCheck(r.Context())
	if rep.Status != wire.RestartCheckOK {
		return wire.RestartResult{Refused: true, Reason: wire.ReasonRestartCheck, Report: &rep,
			Message: "the restart check failed: " + rep.Error}
	}
	if !restartConfirmed(req, rep) {
		return wire.RestartResult{Refused: true, Reason: wire.ReasonRestartChanged, Report: &rep,
			Message: "the check changed since you looked at it"}
	}
	spawn := s.cfg.SpawnRestart
	if spawn == nil {
		spawn = s.spawnRestart
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
func (s *Server) spawnRestart(bin string) error {
	cmd := exec.Command(bin, "server", "restart", "--continue")
	cmd.Env = s.cfg.Env
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
