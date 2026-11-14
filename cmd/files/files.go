// Package files is `gx files`: the verbs that ask the server about Hosted files.
package files

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

// Deps is the runtime context of every command in this package.
type Deps struct {
	cmdbase.Env
}

// New builds `gx files`.
func New(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "files", Short: "ask which files gx may host (asks the server)"}
	cmd.AddCommand(newResolve(d), newURL(d))
	return cmd
}

func newURL(d Deps) *cobra.Command {
	var jsonOut, raw bool
	var host string
	cmd := &cobra.Command{
		Use:   "url [--raw] [--host <host>] [--json] <path>",
		Short: "print the link of a file gx may host; a relative path joins the current folder",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			path, err := clientPath(d, args[0])
			if err != nil {
				return err
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Web.HostedURL(ctx(c), ops.HostedURLRequest{Path: path, Host: host, Raw: raw})
			return finishURL(c.OutOrStdout(), jsonOut, res, err)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.Flags().BoolVar(&raw, "raw", false, "print the raw /v1/files link instead of the viewer link")
	cmd.Flags().StringVar(&host, "host", "", "use this host in the link (an IP, localhost or an allowed host)")
	return cmd
}

// clientPath makes arg absolute against the caller's folder; the server only
// ever sees absolute paths.
func clientPath(d Deps, arg string) (string, error) {
	switch {
	case arg == "~" || strings.HasPrefix(arg, "~/"):
		home, err := d.Dirs().Home()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, arg[1:]), nil
	case filepath.IsAbs(arg):
		return filepath.Clean(arg), nil
	}
	cwd, err := getwd(d)
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, arg), nil
}

// finishURL prints the link, or the refusal. A server that is not running is a refusal.
func finishURL(w io.Writer, jsonOut bool, res ops.HostedURLResult, err error) error {
	switch {
	case apiclient.IsNotRunning(err):
		return fmt.Errorf("refused (%s): %s", wire.ReasonServerNotRunning, "no server is running; start it with `gx server start`")
	case err != nil:
		return fmt.Errorf("server read failed: %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenanceRefusal(w, res, res.Refused)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err = fmt.Fprintln(w, res.URL)
	return err
}

func newResolve(d Deps) *cobra.Command {
	var jsonOut bool
	var session, project, base string
	cmd := &cobra.Command{
		Use:   "resolve [--session <id> | --project <name> | --base <dir>] [--json] <path>...",
		Short: "say whether each path is a file gx may host; relative paths join the base (default: the current folder)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			req := ops.HostedResolveRequest{Candidates: args, Session: session, Project: project, Dir: base}
			if session == "" && project == "" && base == "" {
				cwd, err := getwd(d)
				if err != nil {
					return err
				}
				req.Dir = cwd
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Web.ResolveHostedFiles(ctx(c), req)
			return finish(c.OutOrStdout(), jsonOut, res, err)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.Flags().StringVar(&session, "session", "", "resolve relative paths against this session's folder")
	cmd.Flags().StringVar(&project, "project", "", "resolve relative paths against this project's main worktree")
	cmd.Flags().StringVar(&base, "base", "", "resolve relative paths against this folder")
	return cmd
}

func getwd(d Deps) (string, error) {
	if d.Getwd != nil {
		return d.Getwd()
	}
	return os.Getwd()
}

func ctx(c *cobra.Command) context.Context {
	if c.Context() != nil {
		return c.Context()
	}
	return context.Background()
}

// finish prints res, or the refusal. A server that is not running is a refusal.
func finish(w io.Writer, jsonOut bool, res ops.HostedResolveResult, err error) error {
	switch {
	case apiclient.IsNotRunning(err):
		return fmt.Errorf("refused (%s): %s", wire.ReasonServerNotRunning, "no server is running; start it with `gx server start`")
	case err != nil:
		return fmt.Errorf("server read failed: %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenanceRefusal(w, res, res.Refused)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	for _, f := range res.Files {
		var line string
		if f.Verdict == ops.HostedFound {
			line = fmt.Sprintf("found\t%s\t%s\t%d\t%s", f.Path, f.Kind, f.Size, f.Modified.Local().Format("2006-01-02 15:04"))
		} else {
			line = "missing\t" + f.Candidate
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
