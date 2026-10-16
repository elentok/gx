package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

// newAgentsCmd is `gx agents`: the server's interactive sessions.
func newAgentsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "agents", Short: "interactive agent sessions owned by the server", Args: cobra.NoArgs}
	cmd.AddCommand(newAgentsCreateCmd(), newAgentsListCmd(),
		newAgentsSessionCmd("stop", "end a live session's agent (it stays, Exited)", (*apiclient.Client).StopSession),
		newAgentsSessionCmd("remove", "delete an Exited session", (*apiclient.Client).RemoveSession),
		newAgentsSessionCmd("mark-read", "clear a session's Unread", (*apiclient.Client).MarkSessionRead),
		newAgentsSessionCmd("resume", "resume an Exited session's conversation", (*apiclient.Client).ResumeSession),
		newAgentsSessionCmd("run-action", "send an Idle session's Action prompts, one per turn", (*apiclient.Client).RunSessionAction),
		newAgentsSessionCmd("focus", "show a session in the most recently focused TUI", (*apiclient.Client).FocusSession),
		newAgentsReportFocusCmd())
	return cmd
}

// newAgentActionableCmd is `gx agent actionable`: the calling agent marks its
// own session Actionable.
func newAgentActionableCmd() *cobra.Command {
	var jsonOut bool
	var action []string
	cmd := &cobra.Command{
		Use:   "actionable <reason> [--send <prompt>]...",
		Short: "mark this gx session Actionable until the person's next input; outside one, do nothing",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			req := server.MarkActionableRequest{ID: os.Getenv(nativerunner.AgentSessionEnv), Reason: args[0], Action: action}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runAgentActionable(ctx, serverClient, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().StringArrayVar(&action, "send", nil, "a prompt the Action sends; repeat for more, in order")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// runAgentActionable sends req, or, with no session id, prints a note and
// succeeds, so skills can call it unconditionally.
func runAgentActionable(ctx context.Context, client func() (*apiclient.Client, error), w io.Writer, jsonOut bool, req server.MarkActionableRequest) error {
	if req.ID == "" {
		_, err := fmt.Fprintf(w, "not in a gx session (%s unset); nothing marked\n", nativerunner.AgentSessionEnv)
		return err
	}
	cl, err := client()
	if err != nil {
		return err
	}
	res, err := cl.MarkSessionActionable(ctx, req)
	return printSessionResult(w, jsonOut, res, err)
}

func newAgentsReportFocusCmd() *cobra.Command {
	var jsonOut bool
	var req server.FocusReport
	cmd := &cobra.Command{
		Use:   "report-focus --client <id> [--focused] [--session <id>]",
		Short: "report a TUI's terminal focus and the session on screen (counts while the client's event stream is open)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			res, err := cl.ReportFocus(ctx, req)
			return printSessionResult(c.OutOrStdout(), jsonOut, res, err)
		},
	}
	cmd.Flags().StringVar(&req.Client, "client", "", "the client id of the TUI's event stream")
	cmd.Flags().BoolVar(&req.Focused, "focused", false, "the TUI's terminal has focus")
	cmd.Flags().StringVar(&req.Session, "session", "", "the session on screen")
	cmd.Flags().StringVar(&req.TermProgram, "term-program", os.Getenv("TERM_PROGRAM"), "the TUI's terminal app")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	_ = cmd.MarkFlagRequired("client")
	return cmd
}

func newAgentsCreateCmd() *cobra.Command {
	var jsonOut bool
	var req server.CreateSessionRequest
	cmd := &cobra.Command{
		Use:   "create --project <name> [--dir <dir>]",
		Short: "start an interactive session (prints its id)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if req.Dir == "" {
				req.Dir = "."
			}
			dir, err := filepath.Abs(req.Dir)
			if err != nil {
				return err
			}
			req.Dir = dir
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runAgentsCreate(c.Context(), cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().StringVar(&req.Project, "project", "", "the project the session belongs to")
	cmd.Flags().StringVar(&req.Dir, "dir", "", "the dir the agent runs in (default the current dir)")
	cmd.Flags().StringVar(&req.Name, "name", "", "the session's name")
	cmd.Flags().StringVar(&req.Ticket, "ticket", "", "the Session ticket's address")
	cmd.Flags().StringVar(&req.Harness, "harness", "", "the harness to run (default claude)")
	cmd.Flags().IntVar(&req.Cols, "cols", 0, "the terminal's width (default the loop's)")
	cmd.Flags().IntVar(&req.Rows, "rows", 0, "the terminal's height (default the loop's)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

type sessionWriteFunc func(*apiclient.Client, context.Context, string) (server.SessionResult, error)

// newAgentsSessionCmd is a session write that takes only the session's id.
func newAgentsSessionCmd(verb, short string, write sessionWriteFunc) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   verb + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runAgentsSessionWrite(c.Context(), cl, c.OutOrStdout(), jsonOut, write, args[0])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func runAgentsSessionWrite(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, write sessionWriteFunc, id string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := write(cl, ctx, id)
	return printSessionResult(w, jsonOut, res, err)
}

// runAgentsCreate never starts the server: a dead socket is a refusal.
func runAgentsCreate(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req server.CreateSessionRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.CreateSession(ctx, req)
	return printSessionResult(w, jsonOut, res, err)
}

// printSessionResult prints a session write's outcome, its session's id when
// human.
func printSessionResult(w io.Writer, jsonOut bool, res server.SessionResult, err error) error {
	switch {
	case apiclient.IsNotRunning(err):
		res = server.SessionResult{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		stamped, err := stampProvenance(res, viaServer, callerActor(os.Getwd))
		if err != nil {
			return err
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(stamped)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	if res.Session == nil {
		return nil
	}
	_, err = fmt.Fprintln(w, res.Session.ID)
	return err
}

func newAgentsListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list interactive sessions",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runAgentsList(c.Context(), cl, c.OutOrStdout(), jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func runAgentsList(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	list, err := cl.Sessions(ctx)
	if err != nil {
		return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, list, viaServer, callerActor(os.Getwd))
	}
	for _, s := range list {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.ID, s.Status.State, s.Project, s.Name, s.Dir); err != nil {
			return err
		}
	}
	return nil
}
