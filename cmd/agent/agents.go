package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ui/nav"
	"github.com/spf13/cobra"
)

// NewAgents is `gx agents`: the server's interactive sessions. With no
// subcommand it opens the TUI on the Agents tab.
func NewAgents(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "agents", Short: "interactive agent sessions owned by the server", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return d.OpenTab(nav.TabAgents) }}
	cmd.AddCommand(newAgentsCreateCmd(d.Dial()), newAgentsListCmd(d), newAgentsChatCmd(d), newAgentsScreenCmd(d), newAgentsCommandsCmd(d), newAgentsPermissionCmd(d), newAgentsKeysCmd(d),
		newAgentsSessionCmd(d, "stop", "end a live session's agent (it stays, Exited)", (*apiclient.Sessions).Stop),
		newAgentsSessionCmd(d, "interrupt", "end a Working session's current turn (the session stays)", (*apiclient.Sessions).Interrupt),
		newAgentsSessionCmd(d, "remove", "delete an Exited session", (*apiclient.Sessions).Remove),
		newAgentsSessionCmd(d, "mark-read", "clear a session's Unread", (*apiclient.Sessions).MarkRead),
		newAgentsSessionCmd(d, "resume", "resume an Exited session's conversation", (*apiclient.Sessions).Resume),
		newAgentsSessionCmd(d, "run-action", "send an Idle session's Action prompts, one per turn", (*apiclient.Sessions).RunAction),
		newAgentsSessionCmd(d, "focus", "show a session in the most recently focused TUI", (*apiclient.Sessions).Focus),
		newAgentsReportFocusCmd(d), newAgentsPromptCmd(d), newAgentsImageCmd(d), newAgentsAutoAdvanceCmd(d), newAgentsFixCmd(d),
		newAgentsSessionCmd(d, "auto-advance-cancel", "stop a design session's running auto-advance countdown", (*apiclient.Sessions).CancelAutoAdvance))
	return cmd
}

func newAgentsAutoAdvanceCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:       "auto-advance <id> <on|off>",
		Short:     "turn a design session's auto-advance on or off",
		Args:      cobra.MatchAll(cobra.ExactArgs(2), cobra.OnlyValidArgs),
		ValidArgs: []string{"on", "off"},
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
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
	return runAgentsSessionCall(ctx, w, jsonOut, func(ctx context.Context) (wire.SessionResult, error) {
		return cl.Sessions.SetAutoAdvance(ctx, id, on)
	})
}

// runAgentsSessionCall runs a session write and prints its outcome. It is the
// one place a nil command context becomes Background.
func runAgentsSessionCall(ctx context.Context, w io.Writer, jsonOut bool, call func(context.Context) (wire.SessionResult, error)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := call(ctx)
	return printSessionResult(w, jsonOut, res, err)
}

// runAgentsPrompt never starts the server: a dead socket is a refusal.
func runAgentsPrompt(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req wire.SessionPromptRequest) error {
	return runAgentsSessionCall(ctx, w, jsonOut, func(ctx context.Context) (wire.SessionResult, error) {
		return cl.Sessions.Prompt(ctx, req)
	})
}

func newAgentsPromptCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var req wire.SessionPromptRequest
	cmd := &cobra.Command{
		Use:   "prompt <id> <text>",
		Short: "type text into an Idle Claude session",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
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
func newAgentActionableCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var action []string
	cmd := &cobra.Command{
		Use:   "actionable <reason> [--send <prompt>]...",
		Short: "mark this gx session Actionable until the person's next input; outside one, do nothing",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			req := wire.MarkActionableRequest{ID: os.Getenv(holder.AgentSessionEnv), Reason: args[0], Action: action}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runAgentActionable(ctx, d.Dial(), c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().StringArrayVar(&action, "send", nil, "a prompt the Action sends; repeat for more, in order")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// runAgentActionable sends req, or, with no session id or no server, prints a
// note and succeeds, so skills can call it unconditionally.
func runAgentActionable(ctx context.Context, client func() (*apiclient.Client, error), w io.Writer, jsonOut bool, req wire.MarkActionableRequest) error {
	return runAgentSelfWrite(ctx, client, w, jsonOut, req.ID, "marked", func(cl *apiclient.Client) (wire.SessionResult, error) {
		return cl.Sessions.MarkActionable(ctx, req)
	})
}

// newAgentLinkCmd is `gx agent link`: the calling agent sets its own
// session's Session ticket.
func newAgentLinkCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "link <ticket-address>",
		Short: "set this gx session's ticket (a map epic's marks it a design session); outside one, do nothing",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			req := wire.LinkSessionRequest{ID: os.Getenv(holder.AgentSessionEnv), Ticket: args[0]}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runAgentLink(ctx, d.Dial(), c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// runAgentLink sends req, or, with no session id or no server, prints a note
// and succeeds, so skills can call it unconditionally.
func runAgentLink(ctx context.Context, client func() (*apiclient.Client, error), w io.Writer, jsonOut bool, req wire.LinkSessionRequest) error {
	return runAgentSelfWrite(ctx, client, w, jsonOut, req.ID, "linked", func(cl *apiclient.Client) (wire.SessionResult, error) {
		return cl.Sessions.Link(ctx, req)
	})
}

// newAgentTranscriptCmd is `gx agent transcript`: the path of the calling
// session's conversation file.
func newAgentTranscriptCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "transcript",
		Short: "print the conversation file of this gx session; exits 1 outside one or with no file",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runAgentTranscript(ctx, d.Dial(), c.OutOrStdout(), jsonOut, os.Getenv(holder.AgentSessionEnv))
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "always exit 0 and emit harness, conversation_id, path and found")
	return cmd
}

// runAgentTranscript reports the conversation file of session id. Text mode
// fails with the reason on stderr (the returned error) and nothing on stdout;
// --json reports found=false instead.
func runAgentTranscript(ctx context.Context, client func() (*apiclient.Client, error), w io.Writer, jsonOut bool, id string) error {
	var res wire.SessionTranscriptResponse
	var lookupErr error
	if id == "" {
		lookupErr = fmt.Errorf("not in a gx session (%s unset)", holder.AgentSessionEnv)
	} else if cl, err := client(); err != nil {
		lookupErr = err
	} else if res, err = cl.Sessions.Transcript(ctx, id); err != nil {
		lookupErr = fmt.Errorf("read transcript: %w", err)
	} else if !res.Found {
		lookupErr = fmt.Errorf("no transcript file found for session %s", id)
	}
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if lookupErr != nil {
		return lookupErr
	}
	_, err := fmt.Fprintln(w, res.Path)
	return err
}

// newAgentStepCmd is `gx agent step`: the calling agent reports the Design
// step it is running.
func newAgentStepCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var title string
	cmd := &cobra.Command{
		Use:   "step <step> [--title <ticket title>]",
		Short: "report the Design step this gx session is running; outside one, do nothing",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			req := wire.ReportStepRequest{ID: os.Getenv(holder.AgentSessionEnv), Step: args[0], Title: title}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runAgentStep(ctx, d.Dial(), c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "the ticket's title, during Resolve tickets")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// runAgentStep sends req, or, with no session id or no server, prints a note
// and succeeds, so skills can call it unconditionally.
func runAgentStep(ctx context.Context, client func() (*apiclient.Client, error), w io.Writer, jsonOut bool, req wire.ReportStepRequest) error {
	return runAgentSelfWrite(ctx, client, w, jsonOut, req.ID, "reported", func(cl *apiclient.Client) (wire.SessionResult, error) {
		return cl.Sessions.ReportStep(ctx, req)
	})
}

// runAgentSelfWrite is a write an agent makes on its own session id; with no
// id or no server it prints "nothing <done>" and succeeds.
func runAgentSelfWrite(ctx context.Context, client func() (*apiclient.Client, error), w io.Writer, jsonOut bool, id, done string, write func(*apiclient.Client) (wire.SessionResult, error)) error {
	if id == "" {
		_, err := fmt.Fprintf(w, "not in a gx session (%s unset); nothing %s\n", holder.AgentSessionEnv, done)
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

func newAgentsReportFocusCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var req wire.FocusReport
	cmd := &cobra.Command{
		Use:   "report-focus --client <id> [--focused] [--session <id>]",
		Short: "report a TUI's terminal focus and the session on screen (counts while the client's event stream is open)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			res, err := cl.Sessions.ReportFocus(ctx, req)
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
	var tier string
	var req wire.CreateSessionRequest
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
			req.Tier = agent.Tier(tier)
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
	cmd.Flags().StringVar(&req.Harness, "harness", "", "the harness for this session (default: the configured Default harness)")
	cmd.Flags().StringVar(&tier, "tier", string(agent.TierWorker), "the model tier: light, worker or strongest")
	cmd.Flags().IntVar(&req.Cols, "cols", 0, "the terminal's width (default the loop's)")
	cmd.Flags().IntVar(&req.Rows, "rows", 0, "the terminal's height (default the loop's)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

type sessionWriteFunc func(*apiclient.Sessions, context.Context, string) (wire.SessionResult, error)

// newAgentsSessionCmd is a session write that takes only the session's id.
func newAgentsSessionCmd(d Deps, verb, short string, write sessionWriteFunc) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   verb + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
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
	return runAgentsSessionCall(ctx, w, jsonOut, func(ctx context.Context) (wire.SessionResult, error) {
		return write(cl.Sessions, ctx, id)
	})
}

// runAgentsCreate never starts the server: a dead socket is a refusal.
func runAgentsCreate(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req wire.CreateSessionRequest) error {
	return runAgentsSessionCall(ctx, w, jsonOut, func(ctx context.Context) (wire.SessionResult, error) {
		return cl.Sessions.Create(ctx, req)
	})
}

// printSessionResult prints a session write's outcome, its session's id when
// human.
func printSessionResult(w io.Writer, jsonOut bool, res wire.SessionResult, err error) error {
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.SessionResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		stamped, err := cmdbase.StampProvenance(res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
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
	if _, err = fmt.Fprintln(w, res.Session.ID); err != nil {
		return err
	}
	if res.Session.RebaseNote == "" {
		return nil
	}
	_, err = fmt.Fprintln(w, res.Session.RebaseNote)
	return err
}

func newAgentsChatCmd(d Deps) *cobra.Command {
	var jsonOut, archive bool
	cmd := &cobra.Command{
		Use:   "chat <session>",
		Short: "show a session's chat: the visible messages of its newest conversation",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runAgentsChat(c.Context(), cl, c.OutOrStdout(), jsonOut, archive, args[0])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.Flags().BoolVar(&archive, "archive", false, "decode the session's archived Codex rollout (.jsonl.zst)")
	return cmd
}

func runAgentsChat(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut, archive bool, id string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	read := cl.Sessions.Chat
	if archive {
		read = cl.Sessions.ChatArchive
	}
	chat, err := read(ctx, id)
	if err != nil {
		return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, chat, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	for _, m := range chat.Messages {
		if _, err := fmt.Fprintln(w, chatLine(m)); err != nil {
			return err
		}
	}
	return nil
}

// chatLine is one message as a tab-separated line; a message with images ends
// in a short mark of how many, so an images-only message still shows.
func chatLine(m wire.SessionChatMessage) string {
	text := m.Text
	switch n := len(m.Images); n {
	case 0:
	case 1:
		text = strings.TrimSpace(text + " [1 image]")
	default:
		text = strings.TrimSpace(fmt.Sprintf("%s [%d images]", text, n))
	}
	return m.Role + "\t" + m.Kind + "\t" + text
}

func newAgentsScreenCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "screen <session>",
		Short: "show a session's screen as plain text rows",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
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
	screen, err := cl.Sessions.Screen(ctx, id)
	if err != nil {
		return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, screen, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	for _, row := range screen.Rows {
		if _, err := fmt.Fprintln(w, row); err != nil {
			return err
		}
	}
	return nil
}

func newAgentsListCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list interactive sessions",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.Dial()()
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
	list, err := cl.Sessions.List(ctx)
	if err != nil {
		return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, list, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	for _, s := range list {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.ID, s.Status.State, s.Project, s.Name, s.Dir); err != nil {
			return err
		}
	}
	return nil
}
