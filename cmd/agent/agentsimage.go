package agent

import (
	"context"
	"encoding/json"
	"errors"
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
			if jsonOut && out == "" {
				return errors.New("--json needs -o: the image bytes would mix with the JSON")
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			data, contentType, err := cl.Sessions.Image(ctx, args[0], args[1])
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
				return writeJSON(c.OutOrStdout(), map[string]any{"id": args[1], "path": out, "bytes": len(data), "contentType": contentType})
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), out)
			return err
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "write the image to this file instead of stdout")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the saved file's path, size and type as JSON (needs -o)")
	return cmd
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
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			res, err := cl.Sessions.RemoveImage(ctx, wire.SessionImageRequest{ID: args[0], Image: args[1]})
			if apiclient.IsNotRunning(err) {
				res, err = wire.SessionImageResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}, nil
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

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printImageResult prints an upload or delete answer: JSON, a refusal as an
// error, or the image id.
func printImageResult(w io.Writer, jsonOut bool, res wire.SessionImageResult) error {
	if jsonOut {
		return writeJSON(w, res)
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
	return printImageResult(w, jsonOut, res)
}
