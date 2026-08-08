package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/cli/confirm"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/skills"
	"github.com/elentok/gx/transcript"

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
	runTickets           func() error
	confirmForce         func(string) (bool, error)
	choosePushDivergence func(io.Reader, io.Writer, *git.PushDivergence) (int, error)
	initConfig           func() (string, error)
	loadConfig           func() (config.Config, error)
	getenv               func(string) string
	runEditor            func(editor, path string, in io.Reader, out, err io.Writer) error
	readClaudeOccupancy  func(cwd, sessionID string) (occupancy int, ok bool, err error)
	verifyCodexSession   func(cwd, sessionID string) (ok bool, err error)
	readCodexContext     func(cwd, sessionID string) (tokens int, ok bool, err error)
	skillsAgentRoots     func() ([]string, error)
	skillsManifestPath   func() (string, error)
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
		runTickets:           runTickets,
		choosePushDivergence: choosePushDivergence,
		initConfig:           config.Init,
		loadConfig:           config.Load,
		getenv:               os.Getenv,
		runEditor:            runEditorCommand,
		readClaudeOccupancy:  transcript.LastAssistantOccupancy,
		verifyCodexSession:   codexsession.VerifyIdentity,
		readCodexContext:     codexsession.LastContextTokens,
		skillsAgentRoots:     defaultSkillsAgentRoots,
		skillsManifestPath:   defaultSkillsManifestPath,
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

// Execute runs gx with the provided arguments.
func Execute(args []string) error {
	config.WarnOnMigrateFailure(os.Stderr)
	d := defaultDeps()
	warnOnScratchFoldFailure(os.Stderr, d.getwd, d.confirmForce)
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
		newClaudeCmd(d),
		newWorktreesCmd(d),
		newPushCmd(d),
		newStatusCmd(d),
		newLogCmd(d),
		newShowCmd(d),
		newStashCmd(d),
		newPRsCmd(d),
		newTicketsCmd(d),
		newCleanupCmd(d),
		newSkillsCmd(d),
		newConfigCmd(d),
		newBumpCmd(d),
		newNotifyCmd(d),
		newStashifyCmd(d),
		newRunCmd(d),
		newTermCmd(d),
		newDoctorCmd(d),
		newVersionCmd(d),
	)

	return root
}
