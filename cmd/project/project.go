// Package project is `gx project`: list, worktrees, add, remove, set-path,
// onboard, edit-config and retry-migration.
package project

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

// Deps is the runtime context of every command in this package.
type Deps struct {
	cmdbase.Env
	// RunEditor opens path in editor; nil means the real one (see DefaultDeps).
	RunEditor       func(editor, path string, in io.Reader, out, err io.Writer) error
	StdinIsTerminal func() bool
}

// DefaultDeps wires the production hooks. env is the process environment the
// editor runs under.
func DefaultDeps(env []string, stdinIsTerminal func() bool) Deps {
	return Deps{
		RunEditor: func(editor, path string, in io.Reader, out, errOut io.Writer) error {
			return cmdbase.RunEditorCommand(env, editor, path, in, out, errOut)
		},
		StdinIsTerminal: stdinIsTerminal,
	}
}

func isTerminal(d Deps) bool { return cmdbase.IsTerminal(d.StdinIsTerminal) }

// New builds `gx project`.
func New(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "ticket-store projects", Args: cobra.NoArgs}
	var jsonOut bool
	list := &cobra.Command{
		Use:   "list",
		Short: "list projects with their ticket counts by status (asks the server)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]wire.ProjectInfo, error) {
					return cl.Reads.Projects(ctx)
				},
				printProjects)
		},
	}
	list.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(list)

	var worktreesJSON bool
	worktrees := &cobra.Command{
		Use:   "worktrees",
		Short: "list each project's repo root and worktrees (asks the server)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), worktreesJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]wire.ProjectWorktrees, error) {
					return cl.Reads.ProjectWorktrees(ctx)
				},
				printProjectWorktrees)
		},
	}
	worktrees.Flags().BoolVar(&worktreesJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(worktrees)

	var addJSON bool
	var name, vcs string
	add := &cobra.Command{
		Use:   "add [path]",
		Short: "register a project (path defaults to the current directory; asks the server)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runProjectAdd(c.Context(), cl, d, c.OutOrStdout(), addJSON,
				wire.AddProjectRequest{Path: abs, Name: name, VCS: vcs})
		},
	}
	add.Flags().StringVar(&name, "name", "", "project name (default: the repo directory's name)")
	add.Flags().StringVar(&vcs, "vcs", "", `"none" registers a directory that is not a git repo`)
	add.Flags().BoolVar(&addJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(add)
	cmd.AddCommand(newProjectRetryMigrationCmd(d))

	var rmJSON bool
	remove := &cobra.Command{
		Use:   "remove <name>",
		Short: "unregister a project (its tickets stay in the store; refused while any are queued or running)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runProjectEdit(c.Context(), c.OutOrStdout(), rmJSON, "removed", cl.Reads.RemoveProject,
				wire.ProjectRequest{Name: args[0]})
		},
	}
	remove.Flags().BoolVar(&rmJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(remove)

	var setJSON bool
	setPath := &cobra.Command{
		Use:   "set-path <name> [path]",
		Short: "point a project at another repo (path defaults to the current directory)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			path := "."
			if len(args) == 2 {
				path = args[1]
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runProjectEdit(c.Context(), c.OutOrStdout(), setJSON, "updated", cl.Reads.SetProjectPath,
				wire.ProjectRequest{Name: args[0], Path: abs})
		},
	}
	setPath.Flags().BoolVar(&setJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(setPath, newProjectEditConfigCmd(d), newProjectOnboardCmd(d))
	return cmd
}

type projectEditCall func(context.Context, wire.ProjectRequest) (wire.ProjectResult, error)

func runProjectEdit(ctx context.Context, w io.Writer, jsonOut bool, verb string, call projectEditCall, req wire.ProjectRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := call(ctx, req)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.ProjectResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err = fmt.Fprintf(w, "%s project %s\n", verb, res.Name)
	return err
}

// addProjectJSON is the `project add --json` object: the add result, plus the
// unset check kinds and the command that sets them up when config is missing.
type addProjectJSON struct {
	wire.AddProjectResult
	ChecksMissing  []string `json:"checks_missing,omitempty"`
	OnboardCommand string   `json:"onboard_command,omitempty"`
}

func runProjectAdd(ctx context.Context, cl *apiclient.Client, d Deps, w io.Writer, jsonOut bool, req wire.AddProjectRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.Reads.AddProject(ctx, req)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.AddProjectResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		out := addProjectJSON{AddProjectResult: res}
		if !res.Refused {
			if pc, err := missingChecks(ctx, cl, res.Name); err != nil {
				return err
			} else if pc != nil {
				out.ChecksMissing, out.OnboardCommand = missingKinds(*pc), onboardCommand(res.Name)
			}
		}
		return cmdbase.EncodeProvenance(w, out, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	verb := "added"
	if res.Existing {
		verb = "already registered as"
	}
	if _, err = fmt.Fprintf(w, "%s project %s\n", verb, res.Name); err != nil {
		return err
	}
	_, err = onboardChecks(ctx, cl, d, w, res.Name)
	return err
}
