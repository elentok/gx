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
	"github.com/elentok/gx/ui/nav"
	"github.com/spf13/cobra"
)

// newAgentsCmd is `gx agents`: the server's interactive sessions. With no
// subcommand it opens the TUI on the Agents tab.
func newAgentsCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{Use: "agents", Short: "interactive agent sessions owned by the server", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return d.runTab(nav.TabAgents) }}
	cmd.AddCommand(newAgentsCreateCmd(serverClient), newAgentsListCmd(), newAgentsChatCmd(), newAgentsScreenCmd(),
		newAgentsSessionCmd("stop", "end a live session's agent (it stays, Exited)", (*apiclient.Client).StopSession),
		newAgentsSessionCmd("interrupt", "end a Working session's current turn (the session stays)", (*apiclient.Client).InterruptSession),
		newAgentsSessionCmd("remove", "delete an Exited session", (*apiclient.Client).RemoveSession),
		newAgentsSessionCmd("mark-read", "clear a session's Unread", (*apiclient.Client).MarkSessionRead),
		newAgentsSessionCmd("resume", "resume an Exited session's conversation", (*apiclient.Client).ResumeSession),
		newAgentsSessionCmd("run-action", "send an Idle session's Action prompts, one per turn", (*apiclient.Client).RunSessionAction),
		newAgentsSessionCmd("focus", "show a session in the most recently focused TUI", (*apiclient.Client).FocusSession),
		newAgentsReportFocusCmd(), newAgentsPromptCmd(), newAgentsAutoAdvanceCmd(),
		newAgentsSessionCmd("auto-advance-cancel", "stop a design session's running auto-advance countdown", (*apiclient.Client).CancelAutoAdvance))
	return cmd
}

func newAgentsAutoAdvanceCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:       "auto-advance <id> <on|off>",
		Short:     "turn a design session's auto-advance on or off",
		Args:      cobra.MatchAll(cobra.ExactArgs(2), cobra.OnlyValidArgs),
		ValidArgs: []string{"on", "off"},
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runAgentsAutoAdvance(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0], args[1] == "on")
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func runAgentsAutoAdvance(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, id string, on bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.SetAutoAdvance(ctx, id, on)
	return printSessionResult(w, jsonOut, res, err)
}

// runAgentsPrompt never starts the server: a dead socket is a refusal.
func runAgentsPrompt(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req server.SessionPromptRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.PromptSession(ctx, req)
	return printSessionResult(w, jsonOut, res, err)
}

func newAgentsPromptCmd() *cobra.Command {
	var jsonOut bool
	var req server.SessionPromptRequest
	cmd := &cobra.Command{
		Use:   "prompt <id> <text>",
		Short: "type text into an Idle design session",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			req.ID, req.Text = args[0], args[1]
			return runAgentsPrompt(c.Context(), cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
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

// runAgentActionable sends req, or, with no session id or no server, prints a
// note and succeeds, so skills can call it unconditionally.
func runAgentActionable(ctx context.Context, client func() (*apiclient.Client, error), w io.Writer, jsonOut bool, req server.MarkActionableRequest) error {
	return runAgentSelfWrite(ctx, client, w, jsonOut, req.ID, "marked", func(cl *apiclient.Client) (server.SessionResult, error) {
		return cl.MarkSessionActionable(ctx, req)
	})
}

// newAgentLinkCmd is `gx agent link`: the calling agent sets its own
// session's Session ticket.
func newAgentLinkCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "link <ticket-address>",
		Short: "set this gx session's ticket (a map epic's marks it a design session); outside one, do nothing",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			req := server.LinkSessionRequest{ID: os.Getenv(nativerunner.AgentSessionEnv), Ticket: args[0]}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runAgentLink(ctx, serverClient, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// runAgentLink sends req, or, with no session id or no server, prints a note
// and succeeds, so skills can call it unconditionally.
func runAgentLink(ctx context.Context, client func() (*apiclient.Client, error), w io.Writer, jsonOut bool, req server.LinkSessionRequest) error {
	return runAgentSelfWrite(ctx, client, w, jsonOut, req.ID, "linked", func(cl *apiclient.Client) (server.SessionResult, error) {
		return cl.LinkSession(ctx, req)
	})
}

// runAgentSelfWrite is a write an agent makes on its own session id; with no
// id or no server it prints "nothing <done>" and succeeds.
func runAgentSelfWrite(ctx context.Context, client func() (*apiclient.Client, error), w io.Writer, jsonOut bool, id, done string, write func(*apiclient.Client) (server.SessionResult, error)) error {
	if id == "" {
		_, err := fmt.Fprintf(w, "not in a gx session (%s unset); nothing %s\n", nativerunner.AgentSessionEnv, done)
		return err
	}
	cl, err := client()
	if err != nil {
		return err
	}
	res, err := write(cl)
	if apiclient.IsNotRunning(err) {
		_, err := fmt.Fprintf(w, "gx server not running; nothing %s\n", done)
		return err
	}
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

func newAgentsCreateCmd(client func() (*apiclient.Client, error)) *cobra.Command {
	var jsonOut bool
	var req server.CreateSessionRequest
	cmd := &cobra.Command{
		Use:   "create --project <name> [--dir <dir> | --worktree <name>] [--prompt <text>]",
		Short: "start an interactive session (prints its id)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			// The route refuses a dir and a worktree together, so only a
			// dir-run session defaults to the current dir.
			if req.Worktree == "" {
				if req.Dir == "" {
					req.Dir = "."
				}
				dir, err := filepath.Abs(req.Dir)
				if err != nil {
					return err
				}
				req.Dir = dir
			}
			cl, err := client()
			if err != nil {
				return err
			}
			return runAgentsCreate(c.Context(), cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().StringVar(&req.Project, "project", "", "the project the session belongs to")
	cmd.Flags().StringVar(&req.Dir, "dir", "", "the dir the agent runs in (default the current dir)")
	cmd.Flags().StringVar(&req.Worktree, "worktree", "", "make a new worktree from trunk with this name and run there")
	cmd.Flags().StringVar(&req.Prompt, "prompt", "", "the first prompt, typed at the session's first Idle")
	cmd.MarkFlagsMutuallyExclusive("dir", "worktree")
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

func newAgentsChatCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "chat <session>",
		Short: "show a session's chat: the visible messages of its newest conversation",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runAgentsChat(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func runAgentsChat(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, id string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	chat, err := cl.SessionChat(ctx, id)
	if err != nil {
		return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, chat, viaServer, callerActor(os.Getwd))
	}
	for _, m := range chat.Messages {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", m.Role, m.Kind, m.Text); err != nil {
			return err
		}
	}
	return nil
}

func newAgentsScreenCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "screen <session>",
		Short: "show a session's screen as plain text rows",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runAgentsSessionScreen(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func runAgentsSessionScreen(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, id string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	screen, err := cl.SessionScreen(ctx, id)
	if err != nil {
		return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, screen, viaServer, callerActor(os.Getwd))
	}
	for _, row := range screen.Rows {
		if _, err := fmt.Fprintln(w, row); err != nil {
			return err
		}
	}
	return nil
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
