package cmd

import (
	"io"
	"os"
	"runtime"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cli/confirm"
	agentcmd "github.com/elentok/gx/cmd/agent"
	"github.com/elentok/gx/cmd/brief"
	"github.com/elentok/gx/cmd/check"
	claudecmd "github.com/elentok/gx/cmd/claude"
	"github.com/elentok/gx/cmd/cmdbase"
	codexcmd "github.com/elentok/gx/cmd/codex"
	configcmd "github.com/elentok/gx/cmd/config"
	"github.com/elentok/gx/cmd/doctor"
	"github.com/elentok/gx/cmd/gitops"
	"github.com/elentok/gx/cmd/limits"
	"github.com/elentok/gx/cmd/project"
	"github.com/elentok/gx/cmd/serverapi"
	skillscmd "github.com/elentok/gx/cmd/skills"
	"github.com/elentok/gx/cmd/term"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/logger"
	"github.com/elentok/gx/sessionlookup"
	"github.com/elentok/gx/ui/nav"

	"github.com/spf13/cobra"
)

// LogOptions configures how the log UI opens. Ref is an optional starting
// commit-ish; File, when set, is a repo-relative path the log is pre-filtered
// to (equivalent to the status "gh" mapping; follows renames).
type LogOptions struct {
	Ref  string
	File string
}

// ExitError is the one alias root keeps, so main.go needs no import of cmdbase.
type ExitError = cmdbase.ExitError

type deps struct {
	stdin           io.Reader
	stdout          io.Writer
	stderr          io.Writer
	getwd           func() (string, error)
	runWorktrees    func(string) error
	runStatus       func(string) error
	runLog          func(LogOptions) error
	runShow         func(string) error
	runStash        func() error
	runPRs          func(allRepos bool) error
	runTab          func(nav.TabID) error
	confirmForce    func(string) (bool, error)
	loadConfig      func() (config.Config, error)
	getenv          func(string) string
	goos            string
	userHomeDir     func() (string, error)
	stdinIsTerminal func() bool
	// agent holds the `gx agent`, `gx agents` and `gx designs` hooks (its Env
	// and OpenTab are unset; agentDeps fills them).
	agent agentcmd.Deps
	// skills holds the `gx skills` hooks (its Env is unset; skillsDeps fills
	// it). `gx server start` also reads them until it moves out of root.
	skills skillscmd.Deps
	// doctor holds the `gx doctor` hooks (its Env, ConfirmForce and GOOS are
	// unset; doctorDeps fills them).
	doctor doctor.Deps
	// config holds the `gx config` and `gx notify` hooks (its Env is unset;
	// configDeps fills it).
	config configcmd.Deps
	// serverClient dials the server; nil means the real one (tests inject a fake).
	serverClient func() (*apiclient.Client, error)
	// env is the process environment every config dir is resolved from.
	env []string
	// writeFile writes ticket files in `tickets publish`; nil means os.WriteFile
	// (tests inject a faulty one).
	writeFile func(path string, data []byte, perm os.FileMode) error
}

// base builds the shared runtime context from the deps' nine common members.
func (d deps) base() cmdbase.Env {
	return cmdbase.Env{
		Stdin: d.stdin, Stdout: d.stdout, Stderr: d.stderr, Getwd: d.getwd,
		Env: d.env, Getenv: d.getenv, LoadConfig: d.loadConfig, NewClient: d.serverClient,
	}
}

// skillsDeps is the `gx skills` hooks joined with the shared env.
func (d deps) skillsDeps() skillscmd.Deps {
	s := d.skills
	s.Env = d.base()
	return s
}

// doctorDeps is the `gx doctor` hooks joined with the shared env.
func (d deps) doctorDeps() doctor.Deps {
	s := d.doctor
	s.Env = d.base()
	s.ConfirmForce = d.confirmForce
	s.GOOS = d.goos
	return s
}

// configDeps is the `gx config` and `gx notify` hooks joined with the shared env.
func (d deps) configDeps() configcmd.Deps {
	s := d.config
	s.Env = d.base()
	return s
}

// claudeDeps is the `gx claude` hook joined with the shared env. userHomeDir
// stays in root deps because `gx evolve` also reads it.
func (d deps) claudeDeps() claudecmd.Deps {
	return claudecmd.Deps{Env: d.base(), UserHomeDir: d.userHomeDir}
}

// codexDeps is the `gx codex` hook joined with the shared env.
func (d deps) codexDeps() codexcmd.Deps {
	return codexcmd.Deps{Env: d.base(), StdinIsTerminal: d.stdinIsTerminal}
}

// projectDeps is the `gx project` hooks joined with the shared env. The editor
// runs under the process env, so tests leave RunEditor to the package default.
func (d deps) projectDeps() project.Deps {
	s := project.DefaultDeps(d.env, d.stdinIsTerminal)
	s.Env = d.base()
	return s
}

// agentDeps is the `gx agent`, `gx agents` and `gx designs` hooks joined with
// the shared env. runTab stays in root deps because `gx tickets` also uses it.
func (d deps) agentDeps() agentcmd.Deps {
	s := d.agent
	s.Env = d.base()
	s.OpenTab = d.runTab
	return s
}

// gitopsDeps is the git-work verbs' hooks joined with the shared env.
// confirmForce stays in root deps because `gx doctor` also reads it.
func (d deps) gitopsDeps() gitops.Deps {
	s := gitops.DefaultDeps()
	s.Env = d.base()
	s.ConfirmForce = d.confirmForce
	return s
}

// serverapiDeps is the `gx server` API verbs' hook joined with the shared env.
// stdinIsTerminal stays in root deps because other commands read it too.
func (d deps) serverapiDeps() serverapi.Deps {
	return serverapi.Deps{Env: d.base(), StdinIsTerminal: d.stdinIsTerminal}
}

// dirs resolves the config, data and state directories from the deps env.
func (d deps) dirs() config.Dirs { return d.base().Dirs() }

// dial returns the injected server client factory, or the real one.
func (d deps) dial() func() (*apiclient.Client, error) { return d.base().Dial() }

func defaultDeps() deps {
	env := os.Environ()
	dirs := config.DirsFrom(env)
	cfg, _ := dirs.Load()
	roots := sessionlookup.RootsFrom(env)
	return deps{
		env:    env,
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		getwd:  os.Getwd,
		confirmForce: func(prompt string) (bool, error) {
			return confirm.RunWithNerd(prompt, cfg.UseNerdFontIcons)
		},
		runWorktrees:    func(target string) error { return runWorktrees(dirs, target) },
		runStatus:       func(target string) error { return runStatus(dirs, target) },
		runLog:          func(opts LogOptions) error { return runLog(dirs, opts) },
		runShow:         func(ref string) error { return runShow(dirs, ref) },
		runStash:        func() error { return runStash(dirs) },
		runPRs:          func(allRepos bool) error { return runPRs(dirs, allRepos) },
		runTab:          func(tab nav.TabID) error { return runTab(dirs, tab) },
		loadConfig:      dirs.Load,
		getenv:          os.Getenv,
		goos:            runtime.GOOS,
		userHomeDir:     dirs.Home,
		stdinIsTerminal: func() bool { return cmdbase.IsTerminalFile(os.Stdin) },
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
	if dir, err := d.dirs().StateDir(); err == nil {
		logger.SetDir(dir)
	}
	return execute(args, d)
}

// execute builds the cobra command tree from the given deps and runs it against
// args. It is the seam used by tests (fake deps + captured args/output) and by
// Execute (real deps + os.Args).
func execute(args []string, d deps) error {
	if args == nil {
		// A nil slice would make cobra fall back to os.Args[1:]; force an
		// explicit empty arg list so "gx" with no arguments opens status.
		args = []string{}
	}
	root := newRootCmd(d)
	root.SetArgs(args)
	root.SetOut(d.stdout)
	root.SetErr(d.stderr)
	return root.Execute()
}

// newRootCmd builds the full gx command tree. Each RunE closes over d so that
// tests can inject fakes. Errors and usage are silenced so that main.go remains
// the single error printer and can unwrap *ExitError for stashify's exit-code
// pass-through.
func newRootCmd(d deps) *cobra.Command {
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
			if cwd, err := d.getwd(); err == nil {
				if info, err := git.IdentifyDir(cwd); err == nil && info.Repo.IsBare && info.WorktreeRoot == "" {
					return d.runWorktrees("")
				}
			}
			return d.runStatus("")
		},
	}
	root.SetVersionTemplate("gx {{.Version}}\n")

	root.AddCommand(
		agentcmd.New(d.agentDeps()),
		agentcmd.NewAgents(d.agentDeps()),
		agentcmd.NewDesigns(d.agentDeps()),
		claudecmd.New(d.claudeDeps()),
		codexcmd.New(d.codexDeps()),
		newWorktreesCmd(d),
		gitops.NewPush(d.gitopsDeps()),
		newStatusCmd(d),
		newLogCmd(d),
		newShowCmd(d),
		newStashCmd(d),
		newPRsCmd(d),
		newTicketsCmd(d),
		newServerCmd(d),
		newRecoveryCmd(d),
		newEvolveCmd(d),
		project.New(d.projectDeps()),
		limits.New(limits.Deps{Env: d.base()}),
		limits.NewUsage(limits.Deps{Env: d.base()}),
		limits.NewAFK(limits.Deps{Env: d.base()}),
		limits.NewViMode(limits.Deps{Env: d.base()}),
		gitops.NewMerge(d.gitopsDeps()),
		gitops.NewCleanup(d.gitopsDeps()),
		skillscmd.New(d.skillsDeps()),
		configcmd.New(d.configDeps()),
		gitops.NewBump(d.gitopsDeps()),
		configcmd.NewNotify(d.configDeps()),
		gitops.NewStashify(d.gitopsDeps()),
		gitops.NewRun(d.gitopsDeps()),
		agentcmd.NewHolder(),
		term.New(term.Deps{Env: d.base(), GOOS: d.goos}),
		brief.New(brief.Deps{Env: d.base(), CacheGate: brief.DefaultDeps().CacheGate}),
		check.New(check.Deps{Env: d.base()}),
		doctor.New(d.doctorDeps()),
		newVersionCmd(d),
		newRestartCheckCmd(d),
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
