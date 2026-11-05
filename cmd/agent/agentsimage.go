package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

// newAgentsImageCmd is `gx agents image`: the Prompt images of a session.
func newAgentsImageCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "image", Short: "Prompt images of a session", Args: cobra.NoArgs}
	cmd.AddCommand(newAgentsImageAddCmd(d))
	return cmd
}

func newAgentsImageAddCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "add <session> <file>",
		Short: "save an image file in a session's state folder",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runAgentsImageAdd(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0], args[1])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// runAgentsImageAdd uploads the file's bytes; the server picks the saved name
// and decides the type from the bytes, never from the file name.
func runAgentsImageAdd(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, session, file string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	res, err := cl.Sessions.AddImage(ctx, session, data)
	if apiclient.IsNotRunning(err) {
		res, err = wire.SessionImageResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}, nil
	}
	if err != nil {
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err = fmt.Fprintln(w, res.ID)
	return err
}
