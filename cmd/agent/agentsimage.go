package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

// newAgentsImageCmd is `gx agents image`: the Prompt images of a session.
func newAgentsImageCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "image", Short: "Prompt images of a session", Args: cobra.NoArgs}
	cmd.AddCommand(newAgentsImageAddCmd(d), newAgentsImageGetCmd(d), newAgentsImageRmCmd(d))
	return cmd
}

func newAgentsImageGetCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var out string
	cmd := &cobra.Command{
		Use:   "get <session> <id>",
		Short: "print a saved image's bytes, or save them with -o",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			return runImageGet(c, d, jsonOut, out, args[1], func(ctx context.Context, cl *apiclient.Client) ([]byte, string, error) {
				return cl.Sessions.Image(ctx, args[0], args[1])
			})
		},
	}
	addImageGetFlags(cmd, &out, &jsonOut)
	return cmd
}

func addImageGetFlags(cmd *cobra.Command, out *string, jsonOut *bool) {
	cmd.Flags().StringVarP(out, "output", "o", "", "write the image to this file instead of stdout")
	cmd.Flags().BoolVar(jsonOut, "json", false, "emit the saved file's path, size and type as JSON (needs -o)")
}

// runImageGet fetches one image and prints its bytes, or saves them to out.
func runImageGet(c *cobra.Command, d Deps, jsonOut bool, out, id string, fetch func(context.Context, *apiclient.Client) ([]byte, string, error)) error {
	if jsonOut && out == "" {
		return errors.New("--json needs -o: the image bytes would mix with the JSON")
	}
	cl, err := d.Dial()()
	if err != nil {
		return err
	}
	data, contentType, err := fetch(ctxOrBackground(c.Context()), cl)
	if err != nil {
		return err
	}
	if out == "" {
		_, err = c.OutOrStdout().Write(data)
		return err
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(c.OutOrStdout(), map[string]any{"id": id, "path": out, "bytes": len(data), "contentType": contentType})
	}
	_, err = fmt.Fprintln(c.OutOrStdout(), out)
	return err
}

func newAgentsImageRmCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "rm <session> <id>",
		Short: "delete a saved image that has not been sent",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			ctx := ctxOrBackground(c.Context())
			res, err := cl.Sessions.RemoveImage(ctx, wire.SessionImageRequest{ID: args[0], Image: args[1]})
			if apiclient.IsNotRunning(err) {
				res, err = wire.SessionImageResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: notRunningMessage}, nil
			}
			if err != nil {
				return fmt.Errorf("server write failed: %w", err)
			}
			return printImageResult(c.OutOrStdout(), jsonOut, res)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

const notRunningMessage = "no server is running; start it with `gx server start`"

// ctxOrBackground covers a command run without a context (cobra's is nil when
// the command is executed directly).
func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printImageResult prints an upload or delete answer: JSON, a refusal as an
// error, or the image id.
func printImageResult(w io.Writer, jsonOut bool, res wire.SessionImageResult) error {
	if jsonOut {
		if err := writeJSON(w, res); err != nil {
			return err
		}
		return cmdbase.JSONRefusalExit(res.Refused)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err := fmt.Fprintln(w, res.ID)
	return err
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
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	res, err := cl.Sessions.AddImage(ctxOrBackground(ctx), session, data)
	if apiclient.IsNotRunning(err) {
		res, err = wire.SessionImageResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: notRunningMessage}, nil
	}
	if err != nil {
		return fmt.Errorf("server write failed: %w", err)
	}
	return printImageResult(w, jsonOut, res)
}
