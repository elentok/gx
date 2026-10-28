package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/brief"
)

// briefTools lists the wrapped tools by subcommand name.
var briefTools = map[string]brief.Tool{
	"go-test":  brief.GoTest{},
	"go-build": brief.GoBuild{},
	"go-vet":   brief.GoVet{},
	"vitest":   brief.Vitest{},
	"tsc":      brief.Tsc{},

	"playwright": brief.Playwright{},
}

func newBriefCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "brief [-C dir] <tool> [args...]",
		Short: "run a dev tool and print a short report plus the path of the full log",
		Long: `Run a dev tool and print one result line and the log path (always the last
line). Exit 0 means pass, 1 fail, 2 brief could not run the tool.

Tools: go-test, go-build, go-vet, vitest, tsc, playwright, conflicts (the node
tools run from the node_modules/.bin found walking up to the git root).
go-build and go-vet print one line when clean and group errors by file.
playwright needs -C webui:
it adds -c e2e/playwright.config.ts, uses the line reporter and runs
make web-build only when webui/dist/index.html is missing. Bare numbers after
the test file select flows (playwright disk.e2e.ts 76 77); -g is passed through.

conflicts lists the unmerged files of an in-progress merge or rebase with the
line numbers of their conflict markers (exit 1 while any remain, 0 when none).

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

// briefUsageError prints one line and exits 2, so exit 1 keeps meaning "the
// tool failed".
func briefUsageError(c *cobra.Command, msg string) error {
	fmt.Fprintf(c.OutOrStdout(), "brief: %s\n", msg)
	return &ExitError{Code: brief.ExitCantRun}
}

// resolveRelativeC joins a relative -C value onto cwd. When that directory is
// missing and cwd is already the directory named (`-C webui` from inside
// webui), it returns cwd.
func resolveRelativeC(cwd, dir string) string {
	joined := filepath.Join(cwd, dir)
	if _, err := os.Stat(joined); err != nil && filepath.Base(cwd) == filepath.Clean(dir) {
		return cwd
	}
	return joined
}

func runBrief(c *cobra.Command, d deps, args []string) error {
	dir := ""
	if len(args) >= 2 && args[0] == "-C" {
		dir, args = args[1], args[2:]
	} else if len(args) >= 1 && strings.HasPrefix(args[0], "-C") && len(args[0]) > 2 {
		dir, args = args[0][2:], args[1:]
	}
	cFlag := dir
	if len(args) == 0 && cFlag != "" || len(args) == 1 && args[0] == "-C" {
		return briefUsageError(c, "missing tool name after -C")
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return c.Help()
	}
	tool, ok := briefTools[args[0]]
	if !ok && args[0] != "conflicts" {
		// conflicts is not a brief.Tool: it runs no child tool and keeps no log.
		return briefUsageError(c, fmt.Sprintf("unknown tool %q", args[0]))
	}
	cwd, err := d.getwd()
	if err != nil {
		return err
	}
	if dir == "" {
		dir = cwd
	} else if !filepath.IsAbs(dir) {
		dir = resolveRelativeC(cwd, dir)
	}
	if args[0] == "conflicts" {
		if code := brief.Conflicts(dir, c.OutOrStdout()); code != brief.ExitPass {
			return &ExitError{Code: code}
		}
		return nil
	}
	dataDir, err := d.dirs().DataDir()
	if err != nil {
		return err
	}
	code := brief.Run(tool, brief.Options{Dir: dir, Args: args[1:], CDir: cFlag, DataDir: dataDir, Out: c.OutOrStdout(),
		Env: d.env, CheckRun: d.dirs().Getenv(brief.EnvCheckRun), CheckKind: d.dirs().Getenv(brief.EnvCheckKind)})
	if code != brief.ExitPass {
		return &ExitError{Code: code}
	}
	return nil
}
