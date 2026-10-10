package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/spf13/cobra"
)

// CodeEvolveNoGxProject is the stable refusal code every `gx evolve` verb gives
// in maintainer mode when the store has no `gx` project to file findings in.
const CodeEvolveNoGxProject = "evolve-no-gx-project"

// evolveGxProject is the project maintainer-mode findings are filed in.
const evolveGxProject = "gx"

// evolveEnv is what the evolve verbs need from the outside world.
type evolveEnv struct {
	storePath string
	// cwdProject names the project owning the current directory, "" when none;
	// it resolves a bare epic name.
	cwdProject string
	cfg        config.Config
	// submit hands the one-off to the server.
	submit func(context.Context, server.OneOffRequest) (server.OneOffResult, error)
	now    func() time.Time
}

// evolveTarget is an epic an evolve verb acts on.
type evolveTarget struct {
	project, name string
	epic          tickets.Epic
}

// evolveGuard refuses a verb in maintainer mode with no `gx` project: the retro
// would have nowhere to file its findings. Every evolve verb goes through it.
func evolveGuard(env evolveEnv, projectDir string) error {
	pf, err := config.ReadProjectFile(projectDir)
	if err != nil {
		return err
	}
	if env.cfg.EvolveMode(pf) != config.EvolveMaintainer {
		return nil
	}
	if _, err := tickets.ProjectDirByName(env.storePath, evolveGxProject); err != nil {
		var ae *tickets.AddressError
		if errors.As(err, &ae) && ae.Code == tickets.CodeUnknownProject {
			return &tickets.AddressError{Code: CodeEvolveNoGxProject,
				Msg: "evolve is in maintainer mode but there is no `gx` project; register it with `gx project add`"}
		}
		return err
	}
	return nil
}

// resolveEvolveTarget parses "project:epic" (or a bare epic of the cwd project),
// runs the guard and loads the epic.
func resolveEvolveTarget(env evolveEnv, arg string) (evolveTarget, error) {
	project, name, ok := tickets.ParseEpicAddress(arg)
	if !ok {
		project, name = env.cwdProject, arg
		if project == "" {
			return evolveTarget{}, fmt.Errorf("%q is not a project:epic address and the current directory is in no project", arg)
		}
	}
	dir, err := tickets.ProjectDirByName(env.storePath, project)
	if err != nil {
		return evolveTarget{}, err
	}
	if err := evolveGuard(env, dir); err != nil {
		return evolveTarget{}, err
	}
	epics, err := tickets.Load(dir)
	if err != nil {
		return evolveTarget{}, err
	}
	for _, e := range epics {
		if e.Name == name {
			return evolveTarget{project: project, name: name, epic: e}, nil
		}
	}
	return evolveTarget{}, &tickets.AddressError{Code: tickets.CodeUnknownEpic, Msg: fmt.Sprintf("no epic %q in project %q", name, project)}
}

// runEvolveRun submits the epic's retro with the dedupe key the completion hook
// uses, so a live retro is never doubled.
func runEvolveRun(ctx context.Context, env evolveEnv, w, errW io.Writer, arg string, force bool) error {
	t, err := resolveEvolveTarget(env, arg)
	if err != nil {
		return err
	}
	if !t.epic.EvolvedAt.IsZero() && !force {
		return fmt.Errorf("epic %s:%s was already evolved at %s; pass --force to run it again", t.project, t.name, t.epic.EvolvedAt.Format(time.RFC3339))
	}
	res, err := env.submit(ctx, server.RetroSubmit(t.project, t.name, env.now()))
	switch {
	case apiclient.IsNotRunning(err):
		res = server.OneOffResult{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	d := decideOneOff(res, false)
	if d.notice != "" {
		fmt.Fprintln(errW, d.notice)
	}
	if d.printAddress {
		fmt.Fprintln(w, res.Address)
	}
	return d.err
}

// runEvolveSkip marks the epic as deliberately not evolved: evolved_at and
// evolve_skipped, never an evolve_ticket.
func runEvolveSkip(env evolveEnv, w io.Writer, arg string) error {
	t, err := resolveEvolveTarget(env, arg)
	if err != nil {
		return err
	}
	err = tickets.StampEpicFields(t.epic.Path, map[string]any{"evolved_at": env.now().UTC(), "evolve_skipped": true})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "skipped %s:%s\n", t.project, t.name)
	return nil
}

func newEvolveCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evolve",
		Short: "retro a finished epic and file what it learned",
		Args:  cobra.NoArgs,
		RunE:  func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	env := func() (evolveEnv, error) { return defaultEvolveEnv(d) }

	var force bool
	run := &cobra.Command{
		Use:   "run <epic> [--force]",
		Short: "submit the epic's retro now",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runEvolveRun(ctx, e, c.OutOrStdout(), c.ErrOrStderr(), args[0], force)
		},
	}
	run.Flags().BoolVar(&force, "force", false, "run again for an epic that was already evolved")

	skip := &cobra.Command{
		Use:   "skip <epic>",
		Short: "mark the epic as evolved without running a retro",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return runEvolveSkip(e, c.OutOrStdout(), args[0])
		},
	}

	cmd.AddCommand(run, skip)
	return cmd
}

func defaultEvolveEnv(d deps) (evolveEnv, error) {
	cfg, err := d.loadConfig()
	if err != nil {
		return evolveEnv{}, err
	}
	env := evolveEnv{storePath: cfg.TicketStore.Path, cfg: cfg, now: time.Now}
	if cwd, err := os.Getwd(); err == nil {
		if root, err := ticketRoot(cwd); err == nil {
			env.cwdProject = tickets.ProjectName(root)
		}
	}
	env.submit = func(ctx context.Context, req server.OneOffRequest) (server.OneOffResult, error) {
		cl, err := serverClient()
		if err != nil {
			return server.OneOffResult{}, err
		}
		return cl.OneOff(ctx, req)
	}
	return env, nil
}
