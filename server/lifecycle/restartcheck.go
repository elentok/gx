package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/elentok/gx/migratefail"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

const restartCheckTimeout = 2 * time.Minute

// restartBinary is the binary a restart would start.
func (rs *Restarter) restartBinary() string {
	if rs.cfg.RestartBinary != nil {
		return rs.cfg.RestartBinary()
	}
	exe, _ := os.Executable()
	return exe
}

// ServeCheck handles GET /v1/restart/check.
func (rs *Restarter) ServeCheck(w http.ResponseWriter, r *http.Request) {
	wire.WriteJSON(w, rs.Check(r.Context()))
}

// Check runs the restart check from the binary a restart would start.
// It never touches this server.
func (rs *Restarter) Check(ctx context.Context) ops.RestartCheckReport {
	rep := ops.RestartCheckReport{
		Binary:         rs.restartBinary(),
		RunningVersion: rs.cfg.Build,
		Projects:       []ops.RestartCheckProject{},
		Agents:         rs.agents(),
		Instance:       rs.instanceID,
		RestartLog:     migratefail.RestartLogPath(rs.cfg.StateDir),
	}
	out, status, detail := runRestartCheckBinary(ctx, rep.Binary, rs.cfg.Env)
	rep.Status, rep.Error = status, detail
	switch status {
	case ops.RestartCheckOK:
		rep.NewVersion, rep.Config = out.Version, out.Config
		if out.Projects != nil {
			rep.Projects = out.Projects
		}
	case ops.RestartCheckTooOld:
		rep.Hint = "that binary has no restart check; use `gx server restart` instead"
	}
	return rep
}

func runRestartCheckBinary(ctx context.Context, bin string, env []string) (ops.RestartCheckOutput, string, string) {
	var out ops.RestartCheckOutput
	if bin == "" {
		return out, ops.RestartCheckUnreadable, "no binary to restart with"
	}
	if _, err := os.Stat(bin); err != nil {
		return out, ops.RestartCheckUnreadable, err.Error()
	}
	ctx, cancel := context.WithTimeout(ctx, restartCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "restart-check")
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		var exit *exec.ExitError
		if errors.As(err, &exit) && strings.Contains(msg, "unknown command") {
			return out, ops.RestartCheckTooOld, msg
		}
		if msg == "" {
			msg = err.Error()
		}
		return out, ops.RestartCheckUnreadable, msg
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return out, ops.RestartCheckUnreadable, "unreadable output: " + err.Error()
	}
	return out, ops.RestartCheckOK, ""
}
