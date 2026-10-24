package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/config"
)

// briefTools lists the wrapped tools by subcommand name.
var briefTools = map[string]brief.Tool{
	"go-test": brief.GoTest{},
	"vitest":  brief.Vitest{},
	"tsc":     brief.Tsc{},
}

func newBriefCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "brief [-C dir] <tool> [args...]",
		Short: "run a dev tool and print a short report plus the path of the full log",
		Long: `Run a dev tool and print one result line and the log path (always the last
line). Exit 0 means pass, 1 fail, 2 brief could not run the tool.

Tools: go-test, vitest, tsc (vitest and tsc run from the node_modules/.bin found
walking up to the git root)

-C <dir> runs the tool in that directory. It must come before the tool name;
everything after the tool name goes to the tool unchanged.`,
		// Flag parsing is off so tool flags such as -run pass through; -C is
		// read by hand because it is only valid before the tool name.
		DisableFlagParsing: true,
		RunE: func(c *cobra.Command, args []string) error {
			return runBrief(c, d, args)
		},
	}
	return cmd
}

func runBrief(c *cobra.Command, d deps, args []string) error {
	dir := ""
	if len(args) >= 2 && args[0] == "-C" {
		dir, args = args[1], args[2:]
	} else if len(args) >= 1 && strings.HasPrefix(args[0], "-C") && len(args[0]) > 2 {
		dir, args = args[0][2:], args[1:]
	}
	cFlag := dir
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return c.Help()
	}
	tool, ok := briefTools[args[0]]
	if !ok {
		return fmt.Errorf("unknown brief tool %q", args[0])
	}
	if dir == "" {
		cwd, err := d.getwd()
		if err != nil {
			return err
		}
		dir = cwd
	}
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	code := brief.Run(tool, brief.Options{Dir: dir, Args: args[1:], CDir: cFlag, DataDir: dataDir, Out: c.OutOrStdout()})
	if code != brief.ExitPass {
		return &ExitError{Code: code}
	}
	return nil
}
