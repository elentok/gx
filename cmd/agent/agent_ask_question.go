package agent

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/wire"
)

// newAgentAskQuestionCmd is the Claude PreToolUse hook for AskUserQuestion. It
// registers the question with the server and holds until the server decides.
// Every failure prints nothing and exits 0, never 2, so Claude's own dialog
// shows: a hook must not make the agent's question unanswerable.
func newAgentAskQuestionCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:           "ask-question",
		Short:         "PreToolUse hook: hold Claude's AskUserQuestion until gx answers",
		Hidden:        true,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, _ []string) error {
			raw, err := io.ReadAll(c.InOrStdin())
			if err != nil {
				return nil
			}
			parent := c.Context()
			if parent == nil {
				parent = context.Background()
			}
			ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
			defer stop()
			_ = runAskQuestion(ctx, d.Dial(), os.Getenv(holder.AgentSessionEnv), raw, c.OutOrStdout(), jsonOut)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print the server's result instead of the hook output")
	return cmd
}

// runAskQuestion holds the call in raw and prints the hook output for the
// server's decision. A release prints nothing.
func runAskQuestion(ctx context.Context, dial func() (*apiclient.Client, error), session string, raw []byte, w io.Writer, jsonOut bool) error {
	req, ok := askHookRequestOf(session, raw)
	if !ok {
		return nil
	}
	cl, err := dial()
	if err != nil {
		return err
	}
	// The marker tells the holder's watcher the session is Blocked while held.
	// A marker that fails to write only costs the Blocked state.
	remove, _ := harness.AskHoldMarker(os.Getenv(harness.AskHoldDirEnv), os.Getpid())
	defer remove()
	res, err := cl.Sessions.AskHold(ctx, req)
	if err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(w).Encode(res)
	}
	out, ok := claudeAskOutput(res)
	if !ok {
		return nil
	}
	return json.NewEncoder(w).Encode(out)
}

// askHookRequestOf reads the hook payload. One it cannot read, or one without a
// session, a tool_use_id and a question, gives no request.
func askHookRequestOf(session string, raw []byte) (wire.AskHookRequest, bool) {
	var in struct {
		ToolName  string `json:"tool_name"`
		ToolUseID string `json:"tool_use_id"`
		ToolInput struct {
			Questions []struct {
				Question    string `json:"question"`
				Header      string `json:"header"`
				MultiSelect bool   `json:"multiSelect"`
				Options     []struct {
					Label       string `json:"label"`
					Description string `json:"description"`
				} `json:"options"`
			} `json:"questions"`
		} `json:"tool_input"`
	}
	if session == "" || json.Unmarshal(raw, &in) != nil || in.ToolUseID == "" || len(in.ToolInput.Questions) == 0 {
		return wire.AskHookRequest{}, false
	}
	req := wire.AskHookRequest{Session: session, ToolUseID: in.ToolUseID}
	for _, q := range in.ToolInput.Questions {
		wq := wire.AskHookQuestion{Question: q.Question, Header: q.Header, MultiSelect: q.MultiSelect}
		for _, o := range q.Options {
			wq.Options = append(wq.Options, wire.AskHookOption{Label: o.Label, Description: o.Description})
		}
		req.Questions = append(req.Questions, wq)
	}
	return req, true
}

// claudeAskOutput is the PreToolUse output for a decision; ok is false for a
// release or an unknown decision, which print nothing.
func claudeAskOutput(res wire.AskHookResult) (map[string]any, bool) {
	out := map[string]any{"hookEventName": "PreToolUse"}
	switch res.Decision {
	case wire.AskHookAllow:
		out["permissionDecision"] = "allow"
		if len(res.UpdatedInput) > 0 {
			out["updatedInput"] = res.UpdatedInput
		}
	case wire.AskHookDeny:
		out["permissionDecision"] = "deny"
		out["permissionDecisionReason"] = res.Reason
	default:
		return nil, false
	}
	return map[string]any{"hookSpecificOutput": out}, true
}
