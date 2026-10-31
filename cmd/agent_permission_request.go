package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

const permissionPollInterval = 100 * time.Millisecond

// newAgentPermissionRequestCmd is the Codex PermissionRequest hook. It records
// the request in the agent dir, holds until the runner writes a decision, and
// prints that decision for Codex. With no agent dir, or when the wait is
// cancelled, it prints nothing and Codex shows its own dialog.
func newAgentPermissionRequestCmd() *cobra.Command {
	var dir string
	poll := permissionPollInterval
	cmd := &cobra.Command{
		Use:           "permission-request",
		Short:         "PermissionRequest hook: hold Codex's approval until gx answers",
		Hidden:        true,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, _ []string) error {
			if dir == "" {
				dir = os.Getenv(harness.CodexAgentDirEnv)
			}
			if info, err := os.Stat(dir); dir == "" || err != nil || !info.IsDir() {
				return nil
			}
			raw, err := io.ReadAll(c.InOrStdin())
			if err != nil {
				return err
			}
			req, ok := permissionRequestOf(raw)
			if !ok {
				return nil
			}
			parent := c.Context()
			if parent == nil {
				parent = context.Background()
			}
			ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
			defer stop()
			d, ok, err := holdForDecision(ctx, dir, req, poll)
			if err != nil || !ok {
				return err
			}
			return json.NewEncoder(c.OutOrStdout()).Encode(codexPermissionOutput(d))
		},
	}
	cmd.Flags().StringVar(&dir, "agent-dir", "", "the agent dir (default: $"+harness.CodexAgentDirEnv+")")
	cmd.Flags().DurationVar(&poll, "poll", permissionPollInterval, "how often to look for the decision")
	_ = cmd.Flags().MarkHidden("poll")
	return cmd
}

// permissionRequestOf reads the hook payload. A payload it cannot read gives
// no request, so the hook stays out of Codex's way.
func permissionRequestOf(raw []byte) (nativerunner.PermissionRequest, bool) {
	var in struct {
		ToolName  string          `json:"tool_name"`
		ToolInput json.RawMessage `json:"tool_input"`
	}
	if json.Unmarshal(raw, &in) != nil || in.ToolName == "" {
		return nativerunner.PermissionRequest{}, false
	}
	var shell struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(in.ToolInput, &shell)
	id, err := nativerunner.NewUUID()
	if err != nil {
		return nativerunner.PermissionRequest{}, false
	}
	return nativerunner.PermissionRequest{ID: id, Tool: in.ToolName, Input: in.ToolInput, Description: shell.Command}, true
}

// holdForDecision publishes req and waits for its decision. ok is false when
// the wait ended without one: cancelled, or the agent dir went away. Either way
// the files are cleared.
func holdForDecision(ctx context.Context, dir string, req nativerunner.PermissionRequest, poll time.Duration) (nativerunner.PermissionDecision, bool, error) {
	nativerunner.ClearPermission(dir)
	defer nativerunner.ClearPermission(dir)
	if err := nativerunner.WritePermissionRequest(dir, req); err != nil {
		return nativerunner.PermissionDecision{}, false, err
	}
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nativerunner.PermissionDecision{}, false, nil
		case <-tick.C:
		}
		if _, err := os.Stat(dir); err != nil {
			return nativerunner.PermissionDecision{}, false, nil
		}
		d, ok, err := nativerunner.ReadPermissionDecision(dir)
		if err != nil {
			return nativerunner.PermissionDecision{}, false, err
		}
		if ok && d.ID == req.ID {
			return d, true, nil
		}
	}
}

// codexPermissionOutput is the hook output Codex reads for a PermissionRequest.
func codexPermissionOutput(d nativerunner.PermissionDecision) map[string]any {
	decision := map[string]any{"behavior": "allow"}
	if !d.Allow {
		decision = map[string]any{"behavior": "deny", "message": d.Reason}
	}
	return map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PermissionRequest",
		"decision":      decision,
	}}
}
