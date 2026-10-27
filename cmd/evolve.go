package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/sessionlookup"
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
	// confirm asks a yes/no question; `evolve all` asks before each submit.
	confirm func(string) (bool, error)
	now     func() time.Time
	// merged reports the epic's tickets that carry a Ralph-Loop-Ticket trailer
	// on its branch; nil when there is no way to ask.
	merged func(project, epic string) (map[string]bool, error)
	// homeDir (claude transcripts), stateDir (native agent logs) and codexHome
	// (rollouts) are where `evolve inputs` finds sessions; "" skips that source.
	// tempDir is where its digest dir goes ("" = the OS temp dir).
	homeDir, stateDir, codexHome, tempDir string
	// dataDir holds the check record and brief index; "" skips the check rows.
	dataDir string
}

// evolveTarget is an epic an evolve verb acts on.
type evolveTarget struct {
	project, name string
	epic          tickets.Epic
	// mode is the project's effective evolve mode.
	mode string
}

// evolveGuard refuses a verb in maintainer mode with no `gx` project: the retro
// would have nowhere to file its findings. Every evolve verb goes through it. It
// returns the project's effective evolve mode.
func evolveGuard(env evolveEnv, projectDir string) (string, error) {
	pf, err := config.ReadProjectFile(projectDir)
	if err != nil {
		return "", err
	}
	mode := env.cfg.EvolveMode(pf)
	if err := server.EnsureOutboxProject(env.storePath, mode); err != nil {
		return "", err
	}
	if mode != config.EvolveMaintainer {
		return mode, nil
	}
	if _, err := tickets.ProjectDirByName(env.storePath, evolveGxProject); err != nil {
		var ae *tickets.AddressError
		if errors.As(err, &ae) && ae.Code == tickets.CodeUnknownProject {
			return "", &tickets.AddressError{Code: CodeEvolveNoGxProject,
				Msg: "evolve is in maintainer mode but there is no `gx` project; register it with `gx project add`"}
		}
		return "", err
	}
	return mode, nil
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
	mode, err := evolveGuard(env, dir)
	if err != nil {
		return evolveTarget{}, err
	}
	epics, err := tickets.Load(dir)
	if err != nil {
		return evolveTarget{}, err
	}
	for _, e := range epics {
		if e.Name == name {
			return evolveTarget{project: project, name: name, epic: e, mode: mode}, nil
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
		return fmt.Errorf("epic %s:%s was already evolved at %s%s; pass --force to run it again", t.project, t.name, t.epic.EvolvedAt.Format(time.RFC3339), evolveTicketNote(env, t))
	}
	if err := submitRetro(ctx, env, w, errW, t.project, t.name); err != nil {
		return err
	}
	return evolveReminder(env, t.mode, w)
}

// evolveTicketNote names the epic's retro ticket as " (retro <address>)",
// following it to its archived address when the one-off has been moved; "" when
// the epic has none.
func evolveTicketNote(env evolveEnv, t evolveTarget) string {
	if t.epic.EvolveTicket == "" {
		return ""
	}
	addr, err := tickets.ParseAddress(t.epic.EvolveTicket, tickets.AddressContext{})
	if err != nil || addr.Project == "" {
		return " (retro " + t.epic.EvolveTicket + ")"
	}
	if dir, err := tickets.ProjectDirByName(env.storePath, addr.Project); err == nil {
		if moved, _, err := tickets.FindOriginal(addr.Project, dir, addr); err == nil {
			return " (retro " + moved.String() + ")"
		}
	}
	return " (retro " + addr.String() + ")"
}

// submitRetro is the submit step `run` and `all` share.
func submitRetro(ctx context.Context, env evolveEnv, w, errW io.Writer, project, epic string) error {
	res, err := env.submit(ctx, server.RetroSubmit(project, epic, env.now()))
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
	return evolveReminder(env, t.mode, w)
}

// evolveAllWindow is how far back `evolve all` looks by default.
const evolveAllWindow = 30 * 24 * time.Hour

// parseEvolveSince is time.ParseDuration plus a "d" (days) suffix.
func parseEvolveSince(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid --since %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid --since %q", s)
	}
	return d, nil
}

// evolveCandidates lists the project's loop epics that completed inside the
// window and have not been evolved. Maps and one-offs never get a retro.
func evolveCandidates(env evolveEnv, projectDir string, since time.Duration) ([]tickets.Epic, error) {
	epics, err := tickets.Load(projectDir)
	if err != nil {
		return nil, err
	}
	cutoff := env.now().Add(-since)
	var out []tickets.Epic
	for _, e := range epics {
		if e.IsMap || server.IsOneOffEpic(e) || !e.EvolvedAt.IsZero() {
			continue
		}
		if e.CompletedAt.IsZero() || e.CompletedAt.Before(cutoff) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// runEvolveAll catches up on unevolved epics: it lists them and, unless yes,
// asks before submitting each through the same path as `run`. One epic's
// failure doesn't stop the rest; the failures come back joined.
func runEvolveAll(ctx context.Context, env evolveEnv, w, errW io.Writer, since time.Duration, allProjects, yes bool) error {
	var dirs []string
	if allProjects {
		var err error
		if dirs, err = tickets.ProjectDirs(env.storePath); err != nil {
			return err
		}
	} else {
		if env.cwdProject == "" {
			return errors.New("the current directory is in no project; pass --all-projects")
		}
		dir, err := tickets.ProjectDirByName(env.storePath, env.cwdProject)
		if err != nil {
			return err
		}
		dirs = []string{dir}
	}

	var errs []error
	found := 0
	contributor := false
	for _, dir := range dirs {
		mode, err := evolveGuard(env, dir)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		contributor = contributor || mode == config.EvolveContributor
		cands, err := evolveCandidates(env, dir, since)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		project := tickets.ProjectName(dir)
		for _, e := range cands {
			found++
			addr := project + ":" + e.Name
			if !yes {
				ok, err := env.confirm(fmt.Sprintf("Submit a retro for %s (completed %s)?", addr, e.CompletedAt.Format("2006-01-02")))
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
			}
			if err := submitRetro(ctx, env, w, errW, project, e.Name); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", addr, err))
			}
		}
	}
	if found == 0 {
		fmt.Fprintln(w, "no epics to evolve")
	}
	if contributor {
		errs = append(errs, evolveReminder(env, config.EvolveContributor, w))
	}
	return errors.Join(errs...)
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

	var since string
	var allProjects, yes bool
	all := &cobra.Command{
		Use:   "all [--since <dur>] [--all-projects] [--yes]",
		Short: "retro every recently completed, un-evolved epic",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			window := evolveAllWindow
			if since != "" {
				var err error
				if window, err = parseEvolveSince(since); err != nil {
					return err
				}
			}
			e, err := env()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runEvolveAll(ctx, e, c.OutOrStdout(), c.ErrOrStderr(), window, allProjects, yes)
		},
	}
	all.Flags().StringVar(&since, "since", "", "look back this far, e.g. 90d or 720h (default 30d)")
	all.Flags().BoolVar(&allProjects, "all-projects", false, "cover every registered project")
	all.Flags().BoolVar(&yes, "yes", false, "submit without asking")

	cmd.AddCommand(run, skip, all, newEvolveInputsCmd(env), newEvolveExportCmd(env))
	return cmd
}

func defaultEvolveEnv(d deps) (evolveEnv, error) {
	cfg, err := d.loadConfig()
	if err != nil {
		return evolveEnv{}, err
	}
	env := evolveEnv{storePath: cfg.TicketStore.Path, cfg: cfg, now: time.Now, confirm: d.confirmForce,
		merged: defaultMerged(cfg.TicketStore.Path)}
	// Missing dirs only cost digests, which then report "missing".
	env.homeDir, _ = d.userHomeDir()
	env.stateDir, _ = config.StateDir()
	env.dataDir, _ = config.DataDir()
	env.codexHome = sessionlookup.DefaultRoots().CodexHome
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
