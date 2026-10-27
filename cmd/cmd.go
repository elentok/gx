package cmd

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cli/confirm"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
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
	explainAgent     func(target string) (herdr.AgentExplainResult, error)
	probeGhostty     func() error
	// serverClient dials the server; nil means the real one (tests inject a fake).
	serverClient func() (*apiclient.Client, error)
}

// dial returns the injected server client factory, or the real one.
func (d deps) dial() func() (*apiclient.Client, error) {
	if d.serverClient != nil {
		return d.serverClient
	}
	return serverClient
}

func defaultDeps() deps {
	cfg, _ := config.Load()
	return deps{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		getwd:  os.Getwd,
		confirmForce: func(prompt string) (bool, error) {
			return confirm.RunWithNerd(prompt, cfg.UseNerdFontIcons)
		},
		runWorktrees:         runWorktrees,
		runStatus:            runStatus,
		runLog:               runLog,
		runShow:              runShow,
		runStash:             runStash,
		runPRs:               runPRs,
		runTab:               runTab,
		choosePushDivergence: choosePushDivergence,
		initConfig:           config.Init,
		loadConfig:           config.Load,
		getenv:               os.Getenv,
		goos:                 runtime.GOOS,
		userHomeDir:          os.UserHomeDir,
		runEditor:            runEditorCommand,
		stdinIsTerminal:      func() bool { return isTerminalFile(os.Stdin) },
		readClaudeOccupancy:  transcript.LastAssistantOccupancy,
		verifyCodexSession:   codexsession.VerifyIdentity,
		readCodexContext:     codexsession.LastContextTokens,
		readAutoAdvance:      readServerAutoAdvance,
		skillsAgentRoots:     defaultSkillsAgentRoots,
		skillsManifestPath:   defaultSkillsManifestPath,
		skillsStatusPath:     defaultSkillsStatusPath,
		explainAgent:         herdr.AgentExplain,
		probeGhostty:         terminalrun.ProbeGhostty,
	}
}

// defaultSkillsAgentRoots resolves gx's canonical skill bundle's two install
// targets under the real user's home directory: Claude Code's user skill
// discovery root and Codex's user custom-prompt discovery root.
func defaultSkillsAgentRoots() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home dir: %w", err)
	}
	return []string{skills.ClaudeSkillsRoot(home), skills.CodexSkillsRoot(home)}, nil
}

func defaultSkillsManifestPath() (string, error) {
	return skills.ManifestPath(skills.BundleID)
}

func defaultSkillsStatusPath() (string, error) {
	stateDir, err := config.StateDir()
	if err != nil {
		return "", err
	}
	return skills.StatusPath(stateDir), nil
}

// Execute runs gx with the provided arguments.
func Execute(args []string) error {
	config.WarnOnMigrateFailure(os.Stderr)
	return execute(args, defaultDeps())
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
		newDesignsCmd(),
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
		newAFKCmd(),
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
