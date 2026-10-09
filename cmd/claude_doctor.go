package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/claudedoctor"
	"github.com/elentok/gx/nativerunner"
)

type claudeDoctorOptions struct {
	runner string
	json   bool
	record string
}

func newClaudeDoctorCmd(d deps) *cobra.Command {
	var opts claudeDoctorOptions

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "check the claude features gx's native runners rely on",
		Long: `Check the claude features gx's native runners rely on.

Prints one PASS|FAIL|SKIP line per check and exits 1 on any FAIL. A check
whose dependency did not pass is skipped. Never checks herdr.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			src := liveDoctorSource()
			if opts.record != "" {
				src = claudedoctor.Record(src, opts.record)
			}
			var fixtureVersion string
			if fixtures, err := claudedoctor.FixtureSource(); err == nil {
				fixtureVersion = fixtures.ClaudeVersion
			}
			return runClaudeDoctor(d, opts, src, fixtureVersion)
		},
	}
	cmd.Flags().StringVar(&opts.runner, "runner", "headless", "runner to check: headless|pty")
	cmd.Flags().BoolVar(&opts.json, "json", false, "print results as a JSON array of {runner,name,status,detail}")
	cmd.Flags().StringVar(&opts.record, "record", "", "save each canned session's raw events to this directory, in test-fixture format")
	return cmd
}

// liveDoctorSource runs checks against the installed claude.
func liveDoctorSource() claudedoctor.Source {
	src := claudedoctor.Source{
		Live: true,
		Session: func(name string) ([]json.RawMessage, error) {
			return nil, fmt.Errorf("no canned session %s", name)
		},
	}
	if out, err := exec.Command("claude", "--version").Output(); err == nil {
		src.ClaudeVersion = claudedoctor.ParseVersion(string(out))
	}
	return src
}

func runClaudeDoctor(d deps, opts claudeDoctorOptions, src claudedoctor.Source, fixtureVersion string) error {
	if opts.runner != "headless" && opts.runner != "pty" {
		return fmt.Errorf("unknown --runner %q (want headless or pty)", opts.runner)
	}
	if src.Live && claudedoctor.NewerThanFixtures(src.ClaudeVersion, fixtureVersion) {
		fmt.Fprintf(d.stderr, "warning: claude %s is newer than the test fixtures (%s); refresh them with --record\n", src.ClaudeVersion, fixtureVersion)
	}

	results := claudedoctor.Run(claudedoctor.Table, opts.runner, src)

	if opts.json {
		if results == nil {
			results = []claudedoctor.Result{}
		}
		data, err := json.Marshal(results)
		if err != nil {
			return err
		}
		fmt.Fprintln(d.stdout, string(data))
	} else {
		version := src.ClaudeVersion
		if version == "" {
			version = "unknown"
		}
		fmt.Fprintf(d.stdout, "claude %s (MinClaudeVersion %s)\n", version, nativerunner.MinClaudeVersion)
		for _, r := range results {
			fmt.Fprintf(d.stdout, "%s %s - %s\n", r.Status, r.Name, r.Detail)
		}
	}

	for _, r := range results {
		if r.Status == claudedoctor.Fail {
			return &ExitError{Code: 1}
		}
	}
	return nil
}
