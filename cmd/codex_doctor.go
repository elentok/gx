package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/codexdoctor"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/sessionlookup"
)

type codexDoctorOptions struct {
	json   bool
	record string
	// table overrides codexdoctor.Table so tests never run live checks.
	table []codexdoctor.Check
}

func newCodexDoctorCmd(d deps) *cobra.Command {
	var opts codexDoctorOptions

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "check the Codex features gx relies on",
		Long: `Check the Codex features gx relies on.

Prints one PASS|FAIL|WARN|SKIP line per check and exits 1 on any FAIL. A check
whose dependency did not pass is skipped.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			fixtures, err := codexdoctor.FixtureSource()
			if err != nil {
				return err
			}
			src := liveCodexDoctorSource(d, fixtures)
			if opts.record != "" {
				src = codexdoctor.Record(src, opts.record)
			}
			return runCodexDoctor(d, opts, src, fixtures.CodexVersion)
		},
	}
	cmd.Flags().BoolVar(&opts.json, "json", false, "print results as a JSON array of {name,mode,status,detail}")
	cmd.Flags().StringVar(&opts.record, "record", "", "save each canned session's raw PTY frames and rollouts to this directory, in test-fixture format")
	return cmd
}

// liveCodexDoctorSource runs checks against the installed Codex. The rollout
// check reads the newest rollout under $CODEX_HOME plus the fixtures.
func liveCodexDoctorSource(d deps, fixtures codexdoctor.Source) codexdoctor.Source {
	drivers := codexdoctor.NewDrivers(d.env)
	src := codexdoctor.Source{
		Live: true,
		Env:  d.env,
		Session: func(name string) (codexdoctor.Recording, error) {
			if name != codexdoctor.RolloutsSession {
				return drivers.Session(name)
			}
			base, err := fixtures.Session(name)
			if err != nil {
				return codexdoctor.Recording{}, err
			}
			return codexdoctor.WithLiveRollout(sessionlookup.RootsFrom(d.env).CodexHome, base)
		},
	}
	version := exec.Command("codex", "--version")
	version.Env = d.env
	if out, err := version.Output(); err == nil {
		src.CodexVersion = codexdoctor.ParseVersion(string(out))
	}
	return src
}

func runCodexDoctor(d deps, opts codexDoctorOptions, src codexdoctor.Source, fixtureVersion string) error {
	if src.Live && codexdoctor.NewerThanFixtures(src.CodexVersion, fixtureVersion) {
		fmt.Fprintf(d.stderr, "warning: codex %s is newer than the test fixtures (%s)\n", src.CodexVersion, fixtureVersion)
	}

	table := opts.table
	if table == nil {
		table = codexdoctor.Table
	}
	results := codexdoctor.Run(table, src)

	if opts.json {
		if results == nil {
			results = []codexdoctor.Result{}
		}
		data, err := json.Marshal(results)
		if err != nil {
			return err
		}
		fmt.Fprintln(d.stdout, string(data))
	} else {
		version := src.CodexVersion
		if version == "" {
			version = "unknown"
		}
		fmt.Fprintf(d.stdout, "codex %s (MinCodexVersion %s)\n", version, nativerunner.MinCodexVersion)
		for _, r := range results {
			fmt.Fprintf(d.stdout, "%s %s - %s\n", r.Status, r.Name, r.Detail)
		}
	}

	for _, r := range results {
		if r.Status == codexdoctor.Fail {
			return &ExitError{Code: 1}
		}
	}
	return nil
}
