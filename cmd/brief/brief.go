// Package brief is the `gx brief` command, a thin wrapper over the brief
// package's tools.
package brief

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/gocachegate"
)

// Deps is what `gx brief` reads from its surroundings.
type Deps struct {
	cmdbase.Env
	// CacheGate holds the hooks of `brief go-clean-cache`; the zero value is
	// production (tests shorten the cap).
	CacheGate gocachegate.Deps
	// Sample reads load and process count for the index line; nil is the real
	// machine.
	Sample brief.SampleFunc
}

// DefaultDeps is the production wiring: the zero value of every hook.
func DefaultDeps() Deps { return Deps{} }

const goCleanCache = "go-clean-cache"

// resolveDir turns the -C value into the directory a tool runs in: cwd when
// empty, otherwise the value made absolute.
func resolveDir(d Deps, dir string) (string, error) {
	cwd, err := d.Getwd()
	if err != nil {
		return "", err
	}
	if dir == "" {
		return cwd, nil
	}
	if !filepath.IsAbs(dir) {
		return resolveRelativeC(cwd, dir), nil
	}
	return dir, nil
}

// New builds the `brief` command.
func New(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "brief [-C dir] <tool> [args...]",
		Short: "run a dev tool and print a short report plus the path of the full log",
		Long: `Run a dev tool and print one result line and the log path (always the last
line). Exit 0 means pass, 1 fail, 2 brief could not run the tool.

Tools: go-test, go-build, go-vet, vitest, tsc, biome, playwright, conflicts,
go-clean-cache (the node
tools run from the node_modules/.bin found walking up to the git root).
go-build and go-vet print one line when clean and group errors by file.
playwright needs -C webui:
it adds -c e2e/playwright.config.ts, uses the line reporter and runs
make web-build only when webui/dist/index.html is missing. Bare numbers after
the test file select flows (playwright disk.e2e.ts 76 77); -g is passed through.

conflicts lists the unmerged files of an in-progress merge or rebase with the
line numbers of their conflict markers (exit 1 while any remain, 0 when none).

failures [log [test]] prints every failing test of a brief log with its full
text, untrimmed. Without a log path it reads the latest failed run (exit 1
while the log holds failures, 0 when none). With a test name it prints only
that failure and the command that re-runs it. Use it instead of rg over the
log.
failures --by-file [log] prints one "count file" line per file, largest first
(a biome log: findings per file, so you see what is left). failures --runs [N]
prints one line per run for the last N playwright runs (default 10): its time,
then FAIL and the failing e2e files, or PASS.

health [--limit N] [--all-runs] prints the first-attempt pass rate of full
trunk gx check runs and the current streak, the tests that failed then passed
on one tree, and p50/p95 per test. It always exits 0 (2 when the data cannot
be read).

go-test and vitest take --repeat N [--parallel M]: N runs of the tool, one
process and one log per run, under the proc-cap protection. It prints
"N runs: P passed, F failed" and the log path of the first failing run last.
Exit 0 when all pass, 1 on any failure, 2 if a run could not execute.
--parallel (default 1, at most 4) is read only together with --repeat; alone
it is go test's own -parallel flag.

go-clean-cache waits until no Go build runs, then runs go clean -cache. Use it
instead of calling go clean -cache, which breaks running builds.

-C <dir> runs the tool in that directory. It must come before the tool name;
everything after the tool name goes to the tool unchanged.`,
		// Flag parsing is off so tool flags such as -run pass through; -C is
		// read by hand because it is only valid before the tool name.
		DisableFlagParsing: true,
		RunE: func(c *cobra.Command, args []string) error {
			return runBrief(c, d, args)
		},
	}
	cmd.AddCommand(newHealth(d), newFailures(d))
	return cmd
}

// briefUsageError prints one line and exits 2, so exit 1 keeps meaning "the
// tool failed".
func briefUsageError(c *cobra.Command, msg string) error {
	fmt.Fprintf(c.OutOrStdout(), "brief: %s\n", msg)
	return &cmdbase.ExitError{Code: brief.ExitCantRun}
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

type repeatOpts struct{ n, parallel int }

// splitRepeat takes --repeat N and --parallel M out of the tool's args.
// --parallel is read only together with --repeat: alone it is go test's own
// flag and stays in the args.
func splitRepeat(args []string) ([]string, repeatOpts, error) {
	rest, v, found := extractFlag(args, "--repeat")
	if !found {
		return args, repeatOpts{}, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return nil, repeatOpts{}, errors.New("--repeat needs a positive number")
	}
	rep := repeatOpts{n: n, parallel: 1}
	rest, v, found = extractFlag(rest, "--parallel")
	if found {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > brief.MaxRepeatParallel {
			return nil, repeatOpts{}, fmt.Errorf("--parallel with --repeat takes 1 to %d, at most %d", brief.MaxRepeatParallel, brief.MaxRepeatParallel)
		}
		rep.parallel = p
	}
	return rest, rep, nil
}

// extractFlag removes `name value` or `name=value` from args. A name with no
// value after it yields "".
func extractFlag(args []string, name string) (rest []string, val string, found bool) {
	for i := range args {
		if v, ok := strings.CutPrefix(args[i], name+"="); ok {
			return slices.Concat(args[:i], args[i+1:]), v, true
		}
		if args[i] == name {
			if i+1 < len(args) {
				val = args[i+1]
			}
			return slices.Concat(args[:i], args[min(i+2, len(args)):]), val, true
		}
	}
	return args, "", false
}

func runBrief(c *cobra.Command, d Deps, args []string) error {
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
	if sub := subcommand(c, args[0]); sub != nil {
		return runSub(sub, cFlag, args[1:])
	}
	tool, ok := brief.ToolByName(args[0])
	if !ok && args[0] != "conflicts" && args[0] != goCleanCache {
		// conflicts and go-clean-cache are not brief.Tools: they keep no log.
		return briefUsageError(c, fmt.Sprintf("unknown tool %q", args[0]))
	}
	if slices.Contains(args[1:], "-C") {
		return briefUsageError(c, "put -C before the tool: gx brief -C <dir> "+args[0])
	}
	dir, err := resolveDir(d, dir)
	if err != nil {
		return err
	}
	if args[0] == "conflicts" {
		if code := brief.Conflicts(dir, c.OutOrStdout()); code != brief.ExitPass {
			return &cmdbase.ExitError{Code: code}
		}
		return nil
	}
	stateDir, err := d.Dirs().StateDir()
	if err != nil {
		return err
	}
	if args[0] == goCleanCache {
		if code := brief.GoCleanCache(dir, stateDir, d.Env.Env, d.CacheGate, c.OutOrStdout()); code != brief.ExitPass {
			return &cmdbase.ExitError{Code: code}
		}
		return nil
	}
	dataDir, err := d.Dirs().DataDir()
	if err != nil {
		return err
	}
	toolArgs, rep, err := splitRepeat(args[1:])
	if err != nil {
		return briefUsageError(c, err.Error())
	}
	if rep.n > 0 && args[0] != "go-test" && args[0] != "vitest" {
		return briefUsageError(c, "--repeat works only with go-test and vitest")
	}
	o := brief.Options{Dir: dir, Args: toolArgs, CDir: cFlag, DataDir: dataDir, StateDir: stateDir, Out: c.OutOrStdout(),
		Env: d.Env.Env, CheckRun: d.Dirs().Getenv(brief.EnvCheckRun), CheckKind: d.Dirs().Getenv(brief.EnvCheckKind), Sample: d.Sample}
	var code int
	if rep.n > 0 {
		code = brief.Repeat(tool, o, rep.n, rep.parallel)
	} else {
		code = brief.Run(tool, o)
	}
	if code != brief.ExitPass {
		return &cmdbase.ExitError{Code: code}
	}
	return nil
}
