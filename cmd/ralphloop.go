package cmd

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"

	"github.com/spf13/cobra"
)

func newRalphLoopCmd(d deps) *cobra.Command {
	var skill string
	cmd := &cobra.Command{
		Use:   "ralph-loop <epic-name>",
		Short: "drive Claude Code agents through a to-tickets epic, one iteration worktree at a time",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runRalphLoop(args[0], skill, d)
		},
	}
	cmd.Flags().StringVar(&skill, "skill", "implement", "skill invoked as the initial slash-command prompt in each iteration")
	return cmd
}

func runRalphLoop(epicName, skill string, d deps) error {
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
	}, ralphloop.DefaultDeps(), d.stdout)
}
