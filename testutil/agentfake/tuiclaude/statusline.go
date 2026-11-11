package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/elentok/gx/testutil/agentfake/claudesettings"
)

// statusLineTimeout bounds the command so a hung one cannot stall the fake.
const statusLineTimeout = 10 * time.Second

// runStatusLine runs the settings' statusLine command after a turn. Like
// claude, it ignores the command's output and failure.
func (t *tui) runStatusLine() {
	command := claudesettings.StatusLineCommand(t.opts.settings)
	if command == "" {
		return
	}
	var usage *UsageConfig
	var env []string
	if cfg, err := LoadWorkerConfig(t.pid.Cwd); err == nil {
		usage = cfg.Usage
		env = cfg.Env
	}
	if usage != nil && t.limitResetsAt != 0 {
		limited := *usage
		limited.FiveHour.ResetsAt = t.limitResetsAt
		usage = &limited
	}
	if usage != nil && t.limitLifted {
		lifted := *usage
		lifted.FiveHour.UsedPercentage = 0
		usage = &lifted
	}
	payload, err := StatusLinePayload(t.opts.model, usage)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), statusLineTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = t.pid.Cwd
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = bytes.NewReader(payload)
	_ = cmd.Run()
}
