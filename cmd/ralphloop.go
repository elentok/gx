package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"

	"github.com/spf13/cobra"
)

func newRalphLoopCmd(d deps) *cobra.Command {
	var skill string
	var maxParallel int
	var smartZone int
	cmd := &cobra.Command{
		Use:   "ralph-loop <epic-name>",
		Short: "drive Claude Code agents through a to-tickets epic, up to --max-parallel at a time",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runRalphLoop(args[0], skill, maxParallel, smartZone, d)
		},
	}
	cmd.Flags().StringVar(&skill, "skill", "implement", "skill invoked as the initial slash-command prompt in each iteration")
	cmd.Flags().IntVar(&maxParallel, "max-parallel", 2, "how many iterations run concurrently")
	cmd.Flags().IntVar(&smartZone, "smart-zone", 150_000, "context-token ceiling before pausing an iteration")
	cmd.AddCommand(newRalphLoopResumeCmd(d))
	cmd.AddCommand(newRalphLoopReportCmd(d))
	return cmd
}

func runRalphLoop(epicName, skill string, maxParallel, smartZone int, d deps) error {
	cwd, err := d.getwd()
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return err
	}

	return ralphloop.Run(ralphloop.RunOptions{
		EpicName: epicName,
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
	}, ralphloop.DefaultDeps(), d.stdout)
}

func newRalphLoopResumeCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "resume <epic-name>",
		Short: "wake a gx ralph-loop invocation blocked on a smart-zone pause",
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
