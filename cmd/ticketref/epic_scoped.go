package ticketref

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"

	"github.com/spf13/cobra"
)

// projectEpicRe matches `project:epic`, the cross-project spelling of an epic.
var projectEpicRe = regexp.MustCompile(`^([^:/\s]+):([^:/\s]+)$`)

// SplitProjectEpic splits the cross-project spelling `project:epic`; ok is
// false for anything else.
func SplitProjectEpic(s string) (project, epic string, ok bool) {
	m := projectEpicRe.FindStringSubmatch(s)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// ResolveAddTarget turns add's argument into the epic to allocate in and the
// parent ticket id: an epic (name or path) as before, `project:epic` for an
// epic of another registered project, or a ticket address, which names the
// epic and the parent in one go.
func ResolveAddTarget(env cmdbase.Env, arg, parentFlag string) (epicPath, parent string, err error) {
	cwd, err := env.Getwd()
	if err != nil {
		return "", "", err
	}
	dirs := env.Dirs()
	epicPath = ResolveEpicArg(dirs, arg, cwd)
	if info, statErr := os.Stat(epicPath); statErr == nil && info.IsDir() {
		return epicPath, parentFlag, nil
	}
	if project, epic, ok := SplitProjectEpic(arg); ok {
		root, err := ProjectRoot(dirs, cwd, project)
		if err != nil {
			return "", "", err
		}
		return filepath.Join(root, epic), parentFlag, nil
	}
	_, t, findErr := FindTicket(dirs, cwd, arg)
	if findErr != nil {
		// Not an address either: let the epic path's own error surface.
		return epicPath, parentFlag, nil
	}
	if parentFlag != "" {
		return "", "", errors.New("--parent conflicts with a ticket address argument")
	}
	return filepath.Dir(filepath.Dir(t.Path)), t.Identifier, nil
}

// CompleteEpicNames lists the current epic directory names under cwd's repo,
// read live off disk, for epic-scoped commands' shell completion.
func CompleteEpicNames(dirs config.Dirs, cwd string) ([]string, error) {
	root, err := tickets.RootForDirs(dirs, cwd)
	if err != nil {
		return nil, err
	}
	epics, err := tickets.Load(root)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(epics))
	for _, epic := range epics {
		names = append(names, epic.Name)
	}
	return names, nil
}

// NewEpicScopedCmd builds a cobra command that takes a single epic argument:
// cwd resolution, ResolveEpicArg, and shell-completion via CompleteEpicNames
// are wired once here so epic-scoped subcommands (ensure-code-review, add,
// filter-run-log, ...) don't each hand-roll the same boilerplate. run
// receives the resolved epic path, the raw args (for any extra positional
// args beyond the epic), and the command's output writer.
func NewEpicScopedCmd(env cmdbase.Env, use, short string, run func(epicPath string, args []string, out io.Writer) error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := env.Getwd()
			if err != nil {
				return err
			}
			return run(ResolveEpicArg(env.Dirs(), args[0], cwd), args, c.OutOrStdout())
		},
		ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			cwd, err := env.Getwd()
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			names, err := CompleteEpicNames(env.Dirs(), cwd)
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			return names, cobra.ShellCompDirectiveNoFileComp
		},
	}
}
