package cmd

import (
	"os"
	"runtime"

	"github.com/elentok/gx/cli/confirm"
	agentcmd "github.com/elentok/gx/cmd/agent"
	"github.com/elentok/gx/cmd/brief"
	"github.com/elentok/gx/cmd/check"
	claudecmd "github.com/elentok/gx/cmd/claude"
	"github.com/elentok/gx/cmd/cmdbase"
	codexcmd "github.com/elentok/gx/cmd/codex"
	configcmd "github.com/elentok/gx/cmd/config"
	"github.com/elentok/gx/cmd/doctor"
	"github.com/elentok/gx/cmd/evolve"
	"github.com/elentok/gx/cmd/gitops"
	"github.com/elentok/gx/cmd/limits"
	"github.com/elentok/gx/cmd/project"
	"github.com/elentok/gx/cmd/serverapi"
	"github.com/elentok/gx/cmd/serverctl"
	skillscmd "github.com/elentok/gx/cmd/skills"
	"github.com/elentok/gx/cmd/term"
	"github.com/elentok/gx/cmd/tui"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/logger"
	"github.com/elentok/gx/sessionlookup"

	"github.com/spf13/cobra"
)

// Help group IDs; root assigns them by command name.
const (
	groupTicketContent = "ticket-content"
	groupServer        = "server"
)

// ExitError is the one alias root keeps, so main.go needs no import of cmdbase.
type ExitError = cmdbase.ExitError

// Deps is the root's runtime context: the shared Env plus the hooks more than
// one command package reads, and each package's own hooks.
type Deps struct {
	cmdbase.Env
	ConfirmForce    func(string) (bool, error)
	GOOS            string
	UserHomeDir     func() (string, error)
	StdinIsTerminal func() bool
	// tui holds the hooks that open the full-screen UIs (its Env is unset;
	// tuiDeps fills it). `gx tickets` and `gx agents` read RunTab too.
	tui tui.Deps
	// agent holds the `gx agent`, `gx agents` and `gx designs` hooks (its Env
	// and OpenTab are unset; agentDeps fills them).
	agent agentcmd.Deps
	// skills holds the `gx skills` hooks (its Env is unset; skillsDeps fills
	// it). `gx server start` reads them too.
	skills skillscmd.Deps
	// doctor holds the `gx doctor` hooks (its Env, ConfirmForce and GOOS are
	// unset; doctorDeps fills them).
	doctor doctor.Deps
	// config holds the `gx config` and `gx notify` hooks (its Env is unset;
	// configDeps fills it).
	config configcmd.Deps
}

// skillsDeps is the `gx skills` hooks joined with the shared env.
func (d Deps) skillsDeps() skillscmd.Deps {
	s := d.skills
	s.Env = d.Env
	return s
}

// doctorDeps is the `gx doctor` hooks joined with the shared env.
func (d Deps) doctorDeps() doctor.Deps {
	s := d.doctor
	s.Env = d.Env
	s.ConfirmForce = d.ConfirmForce
	s.GOOS = d.GOOS
	return s
}

// configDeps is the `gx config` and `gx notify` hooks joined with the shared env.
func (d Deps) configDeps() configcmd.Deps {
	s := d.config
	s.Env = d.Env
	return s
}

// claudeDeps is the `gx claude` hook joined with the shared env. userHomeDir
// stays in root deps because `gx evolve` also reads it.
func (d Deps) claudeDeps() claudecmd.Deps {
	return claudecmd.Deps{Env: d.Env, UserHomeDir: d.UserHomeDir}
}

// codexDeps is the `gx codex` hook joined with the shared env.
func (d Deps) codexDeps() codexcmd.Deps {
	return codexcmd.Deps{Env: d.Env, StdinIsTerminal: d.StdinIsTerminal}
}

// projectDeps is the `gx project` hooks joined with the shared env. The editor
// runs under the process env, so tests leave RunEditor to the package default.
func (d Deps) projectDeps() project.Deps {
	s := project.DefaultDeps(d.Env.Env, d.StdinIsTerminal)
	s.Env = d.Env
	return s
}

// tuiDeps is the full-screen launchers' hooks joined with the shared env.
func (d Deps) tuiDeps() tui.Deps {
	s := d.tui
	s.Env = d.Env
	return s
}

// agentDeps is the `gx agent`, `gx agents` and `gx designs` hooks joined with
// the shared env. RunTab stays in the tui deps because `gx tickets` also uses it.
func (d Deps) agentDeps() agentcmd.Deps {
	s := d.agent
	s.Env = d.Env
	s.OpenTab = d.tui.RunTab
	return s
}

// gitopsDeps is the git-work verbs' hooks joined with the shared env.
// confirmForce stays in root deps because `gx doctor` also reads it.
func (d Deps) gitopsDeps() gitops.Deps {
	s := gitops.DefaultDeps()
	s.Env = d.Env
	s.ConfirmForce = d.ConfirmForce
	return s
}

// evolveDeps is the `gx evolve` hooks joined with the shared env. userHomeDir
// and confirmForce stay in root deps because other commands read them too.
func (d Deps) evolveDeps() evolve.Deps {
	return evolve.Deps{Env: d.Env, UserHomeDir: d.UserHomeDir, ConfirmForce: d.ConfirmForce}
}

// serverapiDeps is the `gx server` API verbs' hook joined with the shared env.
// stdinIsTerminal stays in root deps because other commands read it too.
func (d Deps) serverapiDeps() serverapi.Deps {
	return serverapi.Deps{Env: d.Env, StdinIsTerminal: d.StdinIsTerminal}
}

// serverctlDeps is the server lifecycle verbs' hooks joined with the shared env.
func (d Deps) serverctlDeps() serverctl.Deps {
	return serverctl.Deps{Env: d.Env, Skills: d.skills}
}

func defaultDeps() Deps {
	env := os.Environ()
	dirs := config.DirsFrom(env)
	cfg, _ := dirs.Load()
	roots := sessionlookup.RootsFrom(env)
	return Deps{
		Env: cmdbase.Env{
			Env:        env,
			Stdin:      os.Stdin,
			Stdout:     os.Stdout,
			Stderr:     os.Stderr,
			Getwd:      os.Getwd,
			Getenv:     os.Getenv,
			LoadConfig: dirs.Load,
		},
		ConfirmForce: func(prompt string) (bool, error) {
			return confirm.RunWithNerd(prompt, cfg.UseNerdFontIcons)
		},
		GOOS:            runtime.GOOS,
		UserHomeDir:     dirs.Home,
		StdinIsTerminal: func() bool { return cmdbase.IsTerminalFile(os.Stdin) },
		tui:             tui.DefaultDeps(dirs),
		agent:           agentcmd.DefaultDeps(dirs, roots),
		skills:          skillscmd.DefaultDeps(dirs, roots.CodexHome),
		doctor:          doctor.DefaultDeps(),
		config:          configcmd.DefaultDeps(dirs, env),
	}
}

// Execute runs gx with the provided arguments.
func Execute(args []string) error {
	config.WarnOnMigrateFailure(os.Stderr)
	defer cmdbase.RegisterMigrationWaitNotice(os.Stderr)()
	d := defaultDeps()
	if dir, err := d.Dirs().StateDir(); err == nil {
		logger.SetDir(dir)
	}
	return execute(args, d)
}

// execute builds the cobra command tree from the given deps and runs it against
// args. It is the seam used by tests (fake deps + captured args/output) and by
// Execute (real deps + os.Args).
func execute(args []string, d Deps) error {
	if args == nil {
		// A nil slice would make cobra fall back to os.Args[1:]; force an
		// explicit empty arg list so "gx" with no arguments opens status.
		args = []string{}
	}
	root := newRootCmd(d)
	root.SetArgs(args)
	root.SetOut(d.Stdout)
	root.SetErr(d.Stderr)
	return root.Execute()
}

// newRootCmd builds the full gx command tree. Each RunE closes over d so that
// tests can inject fakes. Errors and usage are silenced so that main.go remains
// the single error printer and can unwrap *ExitError for stashify's exit-code
// pass-through.
func newRootCmd(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "gx",
		Short: "git worktree, status, log and more — as a TUI",
		Long: `gx is a git TUI.

Run without a command to open the status UI.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		Version:       getVersion(),
		RunE: func(_ *cobra.Command, _ []string) error {
			// From a bare repo root there's no worktree to show a status for,
			// so open the worktree UI — that's the management view for this layout.
			if cwd, err := d.Getwd(); err == nil {
				if info, err := git.IdentifyDir(cwd); err == nil && info.Repo.IsBare && info.WorktreeRoot == "" {
					return d.tui.RunWorktrees("")
				}
			}
			return d.tui.RunStatus("")
		},
	}
	root.SetVersionTemplate("gx {{.Version}}\n")

	root.AddCommand(
		agentcmd.New(d.agentDeps()),
		agentcmd.NewAgents(d.agentDeps()),
		agentcmd.NewDesigns(d.agentDeps()),
		claudecmd.New(d.claudeDeps()),
		codexcmd.New(d.codexDeps()),
		tui.NewWorktrees(d.tuiDeps(), gitops.WorktreeCommands(d.gitopsDeps())...),
		gitops.NewPush(d.gitopsDeps()),
		tui.NewStatus(d.tuiDeps()),
		tui.NewLog(d.tuiDeps()),
		tui.NewShow(d.tuiDeps()),
		tui.NewStash(d.tuiDeps()),
		tui.NewPRs(d.tuiDeps()),
		newTicketsCmd(d),
		serverctl.New(d.serverctlDeps(), serverapi.Commands(d.serverapiDeps())...),
		newRecoveryCmd(d),
		evolve.New(d.evolveDeps()),
		project.New(d.projectDeps()),
		limits.New(limits.Deps{Env: d.Env}),
		limits.NewUsage(limits.Deps{Env: d.Env}),
		limits.NewAFK(limits.Deps{Env: d.Env}),
		limits.NewViMode(limits.Deps{Env: d.Env}),
		gitops.NewMerge(d.gitopsDeps()),
		gitops.NewCleanup(d.gitopsDeps()),
		skillscmd.New(d.skillsDeps()),
		configcmd.New(d.configDeps()),
		gitops.NewBump(d.gitopsDeps()),
		configcmd.NewNotify(d.configDeps()),
		gitops.NewStashify(d.gitopsDeps()),
		gitops.NewRun(d.gitopsDeps()),
		agentcmd.NewHolder(),
		term.New(term.Deps{Env: d.Env, GOOS: d.GOOS}),
		brief.New(brief.Deps{Env: d.Env, CacheGate: brief.DefaultDeps().CacheGate}),
		check.New(check.Deps{Env: d.Env}),
		doctor.New(d.doctorDeps()),
		newVersionCmd(d),
		serverctl.NewRestartCheck(d.serverctlDeps()),
	)

	root.AddGroup(
		&cobra.Group{ID: groupTicketContent, Title: "Ticket content (direct):"},
		&cobra.Group{ID: groupServer, Title: "Server:"},
	)
	for _, c := range root.Commands() {
		switch c.Name() {
		case "tickets":
			c.GroupID = groupTicketContent
		case "server":
			c.GroupID = groupServer
		}
	}

	return root
}
