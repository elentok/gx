package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/testutil/agentfake"
)

// installBinaries builds or resolves the gx, fakeclaude and (when wanted) fakecodex
// binaries and lays out the fixture's PATH dir. It returns the resolved gx binary,
// the bin dir and whether codex is installed.
func installBinaries(root string, o opts, sc scenario) (gxBin, binDir string, codex bool, err error) {
	if gxBin, err = binary(o.gx, filepath.Join(root, "gx"), "github.com/elentok/gx"); err != nil {
		return "", "", false, err
	}
	binDir = filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", "", false, err
	}
	fakeBin, err := binary(o.fakeClaude, filepath.Join(binDir, "claude"), "github.com/elentok/gx/testutil/agentfake/cmd/fakeclaude")
	if err != nil {
		return "", "", false, err
	}
	if filepath.Base(fakeBin) != "claude" || filepath.Dir(fakeBin) != binDir {
		if err := os.Symlink(fakeBin, filepath.Join(binDir, "claude")); err != nil {
			return "", "", false, err
		}
	}
	// Without the flag codex stays off PATH, which is how a host without Codex looks. A scenario that
	// needs codex has it anyway; --hide-codex takes it off PATH once the scenario is seeded.
	fakeCodexBin := o.fakeCodex
	if fakeCodexBin != "" || sc.codex {
		if fakeCodexBin, err = binary(fakeCodexBin, filepath.Join(root, "codex"), "github.com/elentok/gx/testutil/agentfake/cmd/fakecodex"); err != nil {
			return "", "", false, err
		}
		if err := os.Symlink(fakeCodexBin, filepath.Join(binDir, "codex")); err != nil {
			return "", "", false, err
		}
	}
	// Agent hooks run a bare `gx`; it must be the build under test, not the host's.
	if err := os.Symlink(gxBin, filepath.Join(binDir, "gx")); err != nil {
		return "", "", false, err
	}
	return gxBin, binDir, fakeCodexBin != "", nil
}

func newFixture(root, gxBin, binDir string, codex bool) *fixture {
	f := &fixture{
		root:   root,
		gx:     gxBin,
		hold:   filepath.Join(root, "hold"),
		store:  filepath.Join(root, "data", "gx", "tickets"),
		stateD: filepath.Join(root, "state", "gx"),
		codex:  codex,
	}
	f.env, f.cliEnv = isolatedEnv(root, binDir)
	return f
}

// applyScenarioEnv adds the server env the flags and the scenario ask for.
func (f *fixture) applyScenarioEnv(o opts, sc scenario) error {
	if o.autoAdvance != "" {
		if _, err := time.ParseDuration(o.autoAdvance); err != nil {
			return fmt.Errorf("--auto-advance-delay: %w", err)
		}
		f.env = append(f.env, "GX_AUTO_ADVANCE_DELAY="+o.autoAdvance)
	}
	if o.commandListAge != "" {
		if _, err := time.ParseDuration(o.commandListAge); err != nil {
			return fmt.Errorf("--command-list-age: %w", err)
		}
		f.env = append(f.env, "GX_COMMAND_LIST_AGE="+o.commandListAge)
	}
	if sc.commands != nil {
		data, err := json.Marshal(sc.commands)
		if err != nil {
			return err
		}
		path := filepath.Join(f.root, "agentfake-commands.json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		f.env = append(f.env, agentfake.CommandsFileEnv+"="+path)
	}
	stallThreshold := o.stallThreshold
	if stallThreshold == "" {
		stallThreshold = sc.stallThreshold
	}
	if stallThreshold != "" {
		if _, err := time.ParseDuration(stallThreshold); err != nil {
			return fmt.Errorf("--stall-threshold: %w", err)
		}
		f.env = append(f.env, "GX_STALL_THRESHOLD="+stallThreshold)
	}
	if sc.dropAskedAgent {
		f.env = append(f.env, "GX_DROP_ASKED_AGENT=1")
	}
	if sc.unknownSkill {
		f.env = append(f.env, agentfake.TUIUnknownSkillEnv+"=1")
	}
	f.env = append(f.env, sc.env...)
	return nil
}

// prepareStore sets up the scenario's side dirs, writes the config and seeds
// and commits the ticket store.
func (f *fixture) prepareStore(sc scenario, port int, configs []string) error {
	if err := f.setupDisk(); err != nil {
		return err
	}
	if err := f.setupHealth(); err != nil {
		return err
	}
	// Any scenario that cleans needs the fake ps: the real one sees the `go test` running this fixture.
	if sc.fakeGo || sc.diskLow {
		if err := f.setupFakeGo(); err != nil {
			return err
		}
	}
	if sc.fakeNpm {
		if err := f.setupFakeNpm(sc.failingNpm); err != nil {
			return err
		}
	}
	if sc.restart != nil {
		if err := f.setupRestart(*sc.restart, sc.projectNames()); err != nil {
			return err
		}
	}
	if sc.migrate {
		if err := f.setupMigrate(sc.migrateHold); err != nil {
			return err
		}
	}
	if err := f.writeConfig(port, sc.recentlyFinished, append(slices.Clone(sc.config), configs...)); err != nil {
		return err
	}
	if err := f.seed(sc); err != nil {
		return err
	}
	// The server migrates on every start. The seed is already in the current
	// shape, so only the migrate scenarios leave steps pending.
	if !sc.migrate {
		for _, p := range sc.projectNames() {
			if err := migrations.WriteFullApplied(filepath.Join(f.store, p)); err != nil {
				return err
			}
		}
	}
	// The migration's steps read the store's git log.
	if err := f.commitStore(); err != nil {
		return fmt.Errorf("commit store: %w", err)
	}
	return nil
}

// addScenarioOutput fills the Output fields that only some scenarios have.
func (f *fixture) addScenarioOutput(res *Output, sc scenario, transcripts map[string]string) {
	if sc.fakeGo || sc.fakeNpm {
		res.GateDir = f.gateDir()
	}
	res.ConfigFile = f.configFile()
	res.Transcripts = transcripts
	if sc.limitBurn {
		res.CodexSessionsDir = filepath.Join(f.root, "home", ".codex", "sessions")
	}
	if sc.restart != nil {
		res.RestartDir = f.restartDir()
	}
	if sc.migrate {
		res.MigrateDir = f.migrateDir()
	}
	if sc.commands != nil {
		res.FeatureWorktree = filepath.Join(f.root, "repo", ".worktrees", "feature")
	}
	if cwd, err := os.Getwd(); err == nil {
		res.BrowserSession = filepath.Base(cwd)
	}
}

// seedBeforeStart seeds what must exist before the server starts: old logins,
// limits, limit burn and spend data, plus the env that goes with them.
func (f *fixture) seedBeforeStart(sc scenario, shape string) error {
	// The running server has no test clock, so the old logins are written first.
	if len(sc.extraLogins) > 0 {
		if err := os.MkdirAll(f.stateD, 0o700); err != nil {
			return err
		}
	}
	for i, ago := range sc.extraLogins {
		if _, err := webauth.SeedWebLogin(f.stateD, fmt.Sprintf("gxfixture browser %d", i+1), time.Now().Add(-ago)); err != nil {
			return fmt.Errorf("seed web login: %w", err)
		}
	}

	if sc.limits {
		if err := f.seedLimits(shape); err != nil {
			return fmt.Errorf("seed limits: %w", err)
		}
	}
	if sc.limitBurn {
		if err := f.seedLimitBurn(shape); err != nil {
			return fmt.Errorf("seed limit burn: %w", err)
		}
	}
	if sc.rateLimitFromUsage {
		f.env = append(f.env, "GX_RATE_LIMIT_BUFFER=1s")
	}
	if sc.promptGrace != "" {
		f.env = append(f.env, "GX_PROMPT_GRACE="+sc.promptGrace)
	}
	if sc.tokenPoll {
		f.env = append(f.env, "GX_TOKEN_POLL_INTERVAL="+liveTokenPollInterval)
	}
	if sc.spend {
		if err := f.seedSpend(shape); err != nil {
			return fmt.Errorf("seed spend: %w", err)
		}
		f.env = append(f.env, "GX_BUDGET_POLL_INTERVAL="+spendPollInterval)
	}
	return nil
}
