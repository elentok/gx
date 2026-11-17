package brief

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/suitehealth"
)

// runHealth is `gx brief health [--limit N] [--all-runs]`.
func runHealth(c *cobra.Command, d Deps, dir string, args []string) error {
	opts := suitehealth.Options{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--all-runs":
			opts.AllRuns = true
		case "--limit":
			if i+1 >= len(args) {
				return briefUsageError(c, "--limit needs a number")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				return briefUsageError(c, fmt.Sprintf("bad --limit %q", args[i]))
			}
			opts.Limit = n
		default:
			return briefUsageError(c, fmt.Sprintf("unknown health flag %q", args[i]))
		}
	}
	dataDir, err := d.Dirs().DataDir()
	if err != nil {
		return err
	}
	opts.DataDir = dataDir
	opts.Project = check.ProjectForDirs(d.Dirs(), dir)
	if code := suitehealth.Health(opts, c.OutOrStdout()); code != suitehealth.ExitPass {
		return &cmdbase.ExitError{Code: code}
	}
	return nil
}
