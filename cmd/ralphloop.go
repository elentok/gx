package cmd

import (
	"fmt"
	"io"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui/tickets"

	"github.com/spf13/cobra"
)

func newRalphLoopCmd(d deps) *cobra.Command {
	var agent string
	var skill string
	var maxParallel int
	var smartZone int
	cmd := &cobra.Command{
		Use:   "ralph-loop <epic-name>",
		Short: "drive Claude Code or Codex agents through a to-tickets epic, up to --max-parallel at a time",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(_ *cobra.Command, _ []string) error {
			return ralphloop.ValidateAgentKind(ralphloop.AgentKind(agent))
		},
		RunE: func(_ *cobra.Command, args []string) error {
			return runRalphLoop(args[0], agent, skill, maxParallel, smartZone, d)
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "claude", "agent to run: claude or codex")
	cmd.Flags().StringVar(&skill, "skill", "implement", "skill invoked as the initial prompt in each iteration")
	cmd.Flags().IntVar(&maxParallel, "max-parallel", 2, "how many iterations run concurrently")
	cmd.Flags().IntVar(&smartZone, "smart-zone", 150_000, "context-token ceiling before pausing an iteration")
	cmd.AddCommand(newRalphLoopResumeCmd(d))
	cmd.AddCommand(newRalphLoopReportCmd(d))
	return cmd
}

func runRalphLoop(epicName, agent, skill string, maxParallel, smartZone int, d deps) error {
	cwd, err := d.getwd()
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return err
	}

	opts := ralphloop.RunOptions{
		EpicName: epicName,
		Agent:    ralphloop.AgentKind(agent),
		Skill:    skill,
		RepoDir:  repo.Root,
		// .scratch is gitignored, so it only exists in cwd's own checkout, not
		// in the per-iteration worktrees agents run in. Resolving it here to an
		// absolute path (rather than leaving it relative for Run's default)
		// means every ticket path threaded through it — the initial prompt,
		// run-log, resume signal — resolves regardless of the agent pane's cwd.
		ScratchDir:  filepath.Join(cwd, ".scratch"),
		MaxParallel: maxParallel,
		SmartZone:   smartZone,
	}

	if !isTerminalWriter(d.stdout) {
		return ralphloop.Run(opts, ralphloop.DefaultDeps(), ralphloop.NewTextEventSink(d.stdout))
	}
	return runRalphLoopTUI(opts, cwd, d)
}

// runRalphLoopTUI launches the standalone ralph-loop TUI (mirroring
// cmd/bump.go's own tea.NewProgram, rather than a tab registered in the main
// gx app shell): the orchestrator loop runs in the background exactly as it
// does headlessly, its text report discarded since nothing renders it yet
// (see ticket 04's live orchestrator state), while the TUI polls
// `.scratch/`'s Status: lines directly off disk to drive a flat, navigable,
// previewable ticket list.
func runRalphLoopTUI(opts ralphloop.RunOptions, worktreeRoot string, d deps) error {
	go func() {
		_ = ralphloop.Run(opts, ralphloop.DefaultDeps(), ralphloop.NewTextEventSink(io.Discard))
	}()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	settings := settingsFromConfig(cfg)
	settings.EnableNavigation = false

	m := tickets.NewFlatModel(worktreeRoot, opts.EpicName, settings)
	p := tea.NewProgram(m, tea.WithInput(d.stdin), tea.WithOutput(d.stdout))
	_, err = p.Run()
	return err
}

func newRalphLoopResumeCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "resume <epic-name>",
		Short: "recheck and resume a paused gx ralph-loop invocation",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runRalphLoopResume(args[0], d)
		},
	}
}

func runRalphLoopResume(epicName string, d deps) error {
	if err := ralphloop.Resume("", epicName); err != nil {
		return err
	}
	fmt.Fprintf(d.stdout, "sent resume signal for epic %q\n", epicName)
	return nil
}

func newRalphLoopReportCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "report <epic-name>",
		Short: "print an epic's chronological task order, concurrency, and per-ticket duration/context/cost",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runRalphLoopReport(args[0], d)
		},
	}
}

func runRalphLoopReport(epicName string, d deps) error {
	return ralphloop.Report(ralphloop.ReportOptions{EpicName: epicName}, d.stdout)
}
