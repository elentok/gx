package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cli/confirm"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/logger"
	"github.com/elentok/gx/sessionlookup"
	"github.com/elentok/gx/skills"
	"github.com/elentok/gx/transcript"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/terminalrun"

	"github.com/spf13/cobra"
)

// LogOptions configures how the log UI opens. Ref is an optional starting
// commit-ish; File, when set, is a repo-relative path the log is pre-filtered
// to (equivalent to the status "gh" mapping; follows renames).
type LogOptions struct {
	Ref  string
	File string
}

type deps struct {
	stdin                io.Reader
	stdout               io.Writer
	stderr               io.Writer
	getwd                func() (string, error)
	runWorktrees         func(string) error
	runStatus            func(string) error
	runLog               func(LogOptions) error
	runShow              func(string) error
	runStash             func() error
	runPRs               func(allRepos bool) error
	runTab               func(nav.TabID) error
	confirmForce         func(string) (bool, error)
	choosePushDivergence func(io.Reader, io.Writer, *git.PushDivergence) (int, error)
	initConfig           func() (string, error)
	loadConfig           func() (config.Config, error)
	getenv               func(string) string
	goos                 string
	userHomeDir          func() (string, error)
	runEditor            func(editor, path string, in io.Reader, out, err io.Writer) error
	stdinIsTerminal      func() bool
	readClaudeOccupancy  func(cwd, sessionID string) (occupancy int, ok bool, err error)
	verifyCodexSession   func(cwd, sessionID string) (ok bool, err error)
	readCodexContext     func(cwd, sessionID string) (tokens int, ok bool, err error)
	// readAutoAdvance reads a gx session's auto-advance switch from the server.
	readAutoAdvance    func(sessionID string) (bool, error)
	skillsAgentRoots   func() ([]string, error)
	skillsManifestPath func() (string, error)
	// skillsStatusPath is where `gx server start` records a skill install
	// problem for the server to show.
	skillsStatusPath func() (string, error)
	skillsBackupDir  func() (string, error)
	// skillsCodexHome is $CODEX_HOME; nil skips the duplicate-skill warning.
	skillsCodexHome func() string
	explainAgent    func(target string) (herdr.AgentExplainResult, error)
	probeGhostty    func() error
	// serverClient dials the server; nil means the real one (tests inject a fake).
	serverClient func() (*apiclient.Client, error)
	// env is the process environment every config dir is resolved from.
	env []string
}

// dirs resolves the config, data and state directories from the deps env.
func (d deps) dirs() config.Dirs { return config.DirsFrom(d.env) }

// dial returns the injected server client factory, or the real one.
func (d deps) dial() func() (*apiclient.Client, error) {
	if d.serverClient != nil {
		return d.serverClient
	}
	return func() (*apiclient.Client, error) { return serverClient(d.dirs()) }
}

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
		runWorktrees:         func(target string) error { return runWorktrees(dirs, target) },
		runStatus:            func(target string) error { return runStatus(dirs, target) },
		runLog:               func(opts LogOptions) error { return runLog(dirs, opts) },
		runShow:              func(ref string) error { return runShow(dirs, ref) },
		runStash:             func() error { return runStash(dirs) },
		runPRs:               func(allRepos bool) error { return runPRs(dirs, allRepos) },
		runTab:               func(tab nav.TabID) error { return runTab(dirs, tab) },
		choosePushDivergence: choosePushDivergence,
		initConfig:           dirs.Init,
		loadConfig:           dirs.Load,
		getenv:               os.Getenv,
		goos:                 runtime.GOOS,
		userHomeDir:          dirs.Home,
		runEditor: func(editor, path string, in io.Reader, out, errOut io.Writer) error {
			return runEditorCommand(env, editor, path, in, out, errOut)
		},
		stdinIsTerminal: func() bool { return isTerminalFile(os.Stdin) },
		readClaudeOccupancy: func(cwd, sessionID string) (int, bool, error) {
			return transcript.LastAssistantOccupancyIn(roots.Home, cwd, sessionID)
		},
		verifyCodexSession: func(cwd, sessionID string) (bool, error) {
			return codexsession.VerifyIdentityIn(roots.CodexHome, cwd, sessionID)
		},
		readCodexContext: func(cwd, sessionID string) (int, bool, error) {
			return codexsession.LastContextTokensIn(roots.CodexHome, cwd, sessionID)
		},
		readAutoAdvance:    func(sessionID string) (bool, error) { return readServerAutoAdvance(dirs, sessionID) },
		skillsAgentRoots:   func() ([]string, error) { return defaultSkillsAgentRoots(dirs) },
		skillsManifestPath: func() (string, error) { return defaultSkillsManifestPath(dirs) },
		skillsStatusPath:   func() (string, error) { return defaultSkillsStatusPath(dirs) },
		skillsBackupDir: func() (string, error) {
			base, err := dirs.DataDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(base, "skills-backups"), nil
		},
		skillsCodexHome: func() string { return roots.CodexHome },
		explainAgent:    herdr.AgentExplain,
		probeGhostty:    terminalrun.ProbeGhostty,
	}
}

// defaultSkillsAgentRoots resolves gx's canonical skill bundle's two install
// targets under the real user's home directory: Claude Code's user skill
// discovery root and Codex's user skill discovery root.
func defaultSkillsAgentRoots(dirs config.Dirs) ([]string, error) {
	home, err := dirs.Home()
	if err != nil {
		return nil, fmt.Errorf("resolve user home dir: %w", err)
	}
	return []string{skills.ClaudeSkillsRoot(home), skills.CodexSkillsRoot(home)}, nil
}

func defaultSkillsManifestPath(dirs config.Dirs) (string, error) {
	base, err := dirs.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return skills.ManifestPath(base, skills.BundleID), nil
}

func defaultSkillsStatusPath(dirs config.Dirs) (string, error) {
	stateDir, err := dirs.StateDir()
	if err != nil {
		return "", err
	}
	return skills.StatusPath(stateDir), nil
}

// Execute runs gx with the provided arguments.
func Execute(args []string) error {
	config.WarnOnMigrateFailure(os.Stderr)
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
		newAgentCmd(d),
		newAgentsCmd(d),
		newDesignsCmd(d),
		newClaudeCmd(d),
		newWorktreesCmd(d),
		newPushCmd(d),
		newStatusCmd(d),
		newLogCmd(d),
		newShowCmd(d),
		newStashCmd(d),
		newPRsCmd(d),
		newTicketsCmd(d),
		newServerCmd(d),
		newRecoveryCmd(d),
		newEvolveCmd(d),
		newProjectCmd(d),
		newBudgetCmd(d),
		newUsageCmd(d),
		newAFKCmd(d),
		newMergeCmd(d),
		newCleanupCmd(d),
		newSkillsCmd(d),
		newConfigCmd(d),
		newBumpCmd(d),
		newNotifyCmd(d),
		newStashifyCmd(d),
		newRunCmd(d),
		newAgentHolderCmd(),
		newTermCmd(d),
		newBriefCmd(d),
		newCheckCmd(d),
		newDoctorCmd(d),
		newVersionCmd(d),
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
