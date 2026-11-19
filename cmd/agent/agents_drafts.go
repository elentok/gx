package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/spf13/cobra"
)

// newAgentsDraftsCmd is `gx agents drafts`: chat drafts kept by the server.
func newAgentsDraftsCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "drafts", Short: "chat drafts kept by the server", Args: cobra.NoArgs}
	cmd.AddCommand(newAgentsDraftsCreateCmd(d))
	cmd.AddCommand(newAgentsDraftsDeleteCmd(d))
	return cmd
}

// newAgentsDraftsDeleteCmd is `gx agents drafts delete`. The server saves the
// draft to the Draft history before it removes it.
func newAgentsDraftsDeleteCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "delete <id> [--json]",
		Short: "delete a chat draft (it is kept in the Draft history)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runAgentsDraftsDelete(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func runAgentsDraftsDelete(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, id string) error {
	res, err := cl.Sessions.DeleteChatDraft(ctx, ops.ChatDraftReadRequest{ID: id})
	if apiclient.IsNotRunning(err) {
		res, err = ops.ChatDraftResult{Refused: true, Reason: wire.ReasonServerNotRunning}, nil
	}
	if err != nil {
		return fmt.Errorf("server call failed: %w", err)
	}
	if jsonOut {
		if err := writeDraftJSON(w, res); err != nil {
			return err
		}
		return cmdbase.JSONRefusalExit(res.Refused)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s)", res.Reason)
	}
	_, err = fmt.Fprintln(w, "deleted", id)
	return err
}

// newAgentsDraftsCreateCmd is `gx agents drafts create`. The server resolves
// the workspace and the source folder from from_session; with no gx session
// the draft lands in Default.
func newAgentsDraftsCreateCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var req ops.ChatDraftCreateRequest
	cmd := &cobra.Command{
		Use:   "create --project <p> --brief text|- [--name n] [--harness h] [--model m] [--effort e] [--folder source|trunk|worktree]",
		Short: "save a chat draft and print its id",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			brief, err := readIdea(req.Brief, c.InOrStdin())
			if err != nil {
				return err
			}
			if strings.TrimSpace(brief) == "" {
				return fmt.Errorf("a brief is required")
			}
			req.Brief = brief
			req.FromSession = os.Getenv(holder.AgentSessionEnv)
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runAgentsDraftsCreate(c.Context(), cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	f := cmd.Flags()
	f.StringVar(&req.Project, "project", "", "project of the chat (required)")
	f.StringVar(&req.Brief, "brief", "", "the brief, or - to read it from stdin (required)")
	f.StringVar(&req.Name, "name", "", "chat name")
	f.StringVar(&req.Harness, "harness", "", "harness (default: the project's)")
	f.StringVar(&req.Model, "model", "", "model")
	f.StringVar(&req.Effort, "effort", "", "effort (needs --model)")
	f.StringVar(&req.Folder, "folder", ops.ChatDraftFolderSource, "folder: source, trunk or worktree")
	f.BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("brief")
	return cmd
}

func runAgentsDraftsCreate(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req ops.ChatDraftCreateRequest) error {
	res, err := cl.Sessions.CreateChatDraft(ctx, req)
	if apiclient.IsNotRunning(err) {
		res, err = ops.ChatDraftResult{Refused: true, Reason: wire.ReasonServerNotRunning}, nil
	}
	if err != nil {
		return fmt.Errorf("server call failed: %w", err)
	}
	if jsonOut {
		if err := writeDraftJSON(w, res); err != nil {
			return err
		}
		return cmdbase.JSONRefusalExit(res.Refused)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s)", res.Reason)
	}
	_, err = fmt.Fprintln(w, res.Draft.ID)
	return err
}
