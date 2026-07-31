package cmd

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"

	"github.com/spf13/cobra"
)

func newRalphLoopCmd(d deps) *cobra.Command {
	var skill string
	var maxParallel int
	cmd := &cobra.Command{
		Use:   "ralph-loop <epic-name>",
		Short: "drive Claude Code agents through a to-tickets epic, up to --max-parallel at a time",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runRalphLoop(args[0], skill, maxParallel, d)
		},
	}
	cmd.Flags().StringVar(&skill, "skill", "implement", "skill invoked as the initial slash-command prompt in each iteration")
	cmd.Flags().IntVar(&maxParallel, "max-parallel", 2, "how many iterations run concurrently")
	return cmd
}

func runRalphLoop(epicName, skill string, maxParallel int, d deps) error {
	cwd, err := d.getwd()
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return err
	}

	return ralphloop.Run(ralphloop.RunOptions{
		EpicName:    epicName,
		Skill:       skill,
		RepoDir:     repo.Root,
		MaxParallel: maxParallel,
	}, ralphloop.DefaultDeps(), d.stdout)
}
