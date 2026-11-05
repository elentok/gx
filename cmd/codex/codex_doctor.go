package codex

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/cmd/cmdbase"
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

func newCodexDoctorCmd(d Deps) *cobra.Command {
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
func liveCodexDoctorSource(d Deps, fixtures codexdoctor.Source) codexdoctor.Source {
	drivers := codexdoctor.NewDrivers(d.Env.Env)
	src := codexdoctor.Source{
		Live: true,
		Env:  d.Env.Env,
		Session: func(name string) (codexdoctor.Recording, error) {
			if name != codexdoctor.RolloutsSession {
				return drivers.Session(name)
			}
			base, err := fixtures.Session(name)
			if err != nil {
				return codexdoctor.Recording{}, err
			}
			return codexdoctor.WithLiveRollout(sessionlookup.RootsFrom(d.Env.Env).CodexHome, base)
		},
	}
	version := exec.Command("codex", "--version")
	version.Env = d.Env.Env
	if out, err := version.Output(); err == nil {
		src.CodexVersion = codexdoctor.ParseVersion(string(out))
	}
	return src
}

func runCodexDoctor(d Deps, opts codexDoctorOptions, src codexdoctor.Source, fixtureVersion string) error {
	if src.Live && codexdoctor.NewerThanFixtures(src.CodexVersion, fixtureVersion) {
		fmt.Fprintf(d.Stderr, "warning: codex %s is newer than the test fixtures (%s)\n", src.CodexVersion, fixtureVersion)
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
		fmt.Fprintln(d.Stdout, string(data))
	} else {
		version := src.CodexVersion
		if version == "" {
			version = "unknown"
		}
		fmt.Fprintf(d.Stdout, "codex %s (MinCodexVersion %s)\n", version, nativerunner.MinCodexVersion)
		for _, r := range results {
			fmt.Fprintf(d.Stdout, "%s %s - %s\n", r.Status, r.Name, r.Detail)
		}
	}

	for _, r := range results {
		if r.Status == codexdoctor.Fail {
			return &cmdbase.ExitError{Code: 1}
		}
	}
	return nil
}
