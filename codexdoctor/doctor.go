// Package codexdoctor holds the one table of named checks that verifies the
// Codex features gx relies on. `gx codex doctor` runs it against a real Codex;
// CI runs it against the recorded fixtures in testdata/, so the two cannot
// drift apart. It has the shape of claudedoctor.
package codexdoctor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/elentok/gx/nativerunner"
)

type Status string

const (
	Pass Status = "PASS"
	Fail Status = "FAIL"
	Skip Status = "SKIP"
	// Warn is a failing Check with Warn set: reported, but never fails the run.
	Warn Status = "WARN"
)

// Mode says where a check can run. A Fixture check only reads recorded
// rollouts, so CI runs it too; a Live check needs a real Codex and is skipped
// against fixtures.
type Mode string

const (
	Fixture Mode = "fixture"
	Live    Mode = "live"
)

type Check struct {
	Name string
	Mode Mode
	// Session names the canned session whose recording Run reads; "" for none.
	Session   string
	DependsOn []string
	// Warn makes a failing run report Warn instead of Fail.
	Warn bool
	Run  func(Input) (detail string, ok bool)
}

// Rollout is one rollout file's name and bytes.
type Rollout struct {
	Name string
	Data []byte
}

// Recording is what one canned session produced.
type Recording struct {
	Rollouts []Rollout
	// Frames is the raw PTY output with the time each chunk arrived.
	Frames []Frame
	// Hooks names the hook events the recording script saw fire.
	Hooks []string
	// DenyHonoured is set when the denied action did not happen.
	DenyHonoured bool
	// Launch is the non-model `codex ... debug prompt-input` run.
	Launch Launch
	// Probe is the commit attempted from a Codex shell call.
	Probe Probe
}

// Frame is one chunk of PTY output, T seconds after the session started.
type Frame struct {
	T    float64
	Data []byte
}

// Launch is the result of running codex with gx's launch args.
type Launch struct {
	Args   []string
	Stderr string
	Err    string
}

// Probe is the result of a commit from a Codex shell call; Err is "" on success.
type Probe struct {
	Err string
}

type Input struct {
	// Live is set when the recording comes from a real Codex.
	Live         bool
	CodexVersion string
	// Env is the KEY=VALUE env live checks read CODEX_HOME and HOME from.
	// Empty means no home, not the process env.
	Env []string
	Recording
}

type Result struct {
	Name   string `json:"name"`
	Mode   Mode   `json:"mode"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
}

// Source supplies what the checks read: recorded fixtures, or a real Codex
// when Live is set.
type Source struct {
	Live         bool
	CodexVersion string
	Env          []string
	Session      func(name string) (Recording, error)
}

// ErrNoSession is what a Source returns for a session it has nothing for yet
// (for example no rollout on disk); the check is skipped, not failed.
var ErrNoSession = errors.New("no rollout yet")

// RolloutsSession is the session the rollout-format check reads.
const RolloutsSession = "rollouts"

// The canned live sessions. Each runs once and serves every check naming it.
const (
	LaunchSession       = "launch"
	TurnSession         = "turn"
	DenySession         = "deny"
	CommitSession       = "commit"
	LiveRolloutsSession = "live-rollouts"
)

// Table is the doctor's check list, in run order.
var Table = []Check{
	{Name: "codex-version", Mode: Live, Run: checkCodexVersion},
	{Name: "rollout-format", Mode: Fixture, Session: RolloutsSession, Run: checkRolloutFormat},
	{Name: "launch-flags", Mode: Live, Session: LaunchSession, DependsOn: []string{"codex-version"}, Run: checkLaunchFlags},
	{Name: "osc7501-title", Mode: Live, Session: TurnSession, DependsOn: []string{"codex-version"}, Run: checkOSC7501Title},
	{Name: "hooks-fire", Mode: Live, Session: TurnSession, DependsOn: []string{"codex-version"}, Run: checkHooksFire},
	{Name: "permission-deny", Mode: Live, Session: DenySession, DependsOn: []string{"codex-version"}, Run: checkPermissionDeny},
	{Name: "probe-commit", Mode: Live, Session: CommitSession, DependsOn: []string{"codex-version"}, Run: checkProbeCommit},
	{Name: "rollout-reader", Mode: Live, Session: LiveRolloutsSession, DependsOn: []string{"codex-version"}, Run: checkRolloutReader},
	{Name: "spawn-model-fields", Mode: Live, Warn: true, Run: checkSpawnModelFields},
	{Name: "managed-rules", Mode: Live, Warn: true, Run: checkManagedRules},
}

// Run runs table's checks in order. A check whose dependency did not pass is
// skipped rather than run.
func Run(table []Check, src Source) []Result {
	var results []Result
	status := map[string]Status{}
	sessions := map[string]Recording{}
	for _, c := range table {
		r := runCheck(c, src, status, sessions)
		status[c.Name] = r.Status
		results = append(results, r)
	}
	return results
}

func runCheck(c Check, src Source, status map[string]Status, sessions map[string]Recording) Result {
	r := Result{Name: c.Name, Mode: c.Mode}
	for _, dep := range c.DependsOn {
		if status[dep] != Pass {
			r.Status, r.Detail = Skip, "needs "+dep
			return r
		}
	}
	if c.Mode == Live && !src.Live {
		r.Status, r.Detail = Skip, "live only"
		return r
	}

	in := Input{CodexVersion: src.CodexVersion, Env: src.Env, Live: src.Live}
	if c.Session != "" {
		rec, ok := sessions[c.Session]
		if !ok {
			var err error
			if src.Session == nil {
				err = fmt.Errorf("no session source")
			} else {
				rec, err = src.Session(c.Session)
			}
			if errors.Is(err, ErrNoSession) {
				r.Status, r.Detail = Skip, err.Error()
				return r
			}
			if err != nil {
				r.Status, r.Detail = Fail, fmt.Sprintf("session %s: %v", c.Session, err)
				return r
			}
			sessions[c.Session] = rec
		}
		in.Recording = rec
	}

	detail, ok := c.Run(in)
	r.Status, r.Detail = Fail, detail
	if c.Warn {
		r.Status = Warn
	}
	if ok {
		r.Status = Pass
	}
	return r
}

func checkCodexVersion(in Input) (string, bool) {
	if in.CodexVersion == "" {
		return "could not read codex --version", false
	}
	if nativerunner.CompareVersions(in.CodexVersion, nativerunner.MinCodexVersion) < 0 {
		return fmt.Sprintf("%s is older than %s", in.CodexVersion, nativerunner.MinCodexVersion), false
	}
	return fmt.Sprintf("%s >= %s", in.CodexVersion, nativerunner.MinCodexVersion), true
}

// ParseVersion extracts the version from `codex --version` output
// (`codex-cli 0.166.0`): the field after codex-cli.
func ParseVersion(out string) string {
	fields := strings.Fields(out)
	for i, f := range fields {
		if f == "codex-cli" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// NewerThanFixtures reports whether installed Codex is newer than the Codex
// that recorded the fixtures, i.e. the fixtures may be stale.
func NewerThanFixtures(installed, fixtures string) bool {
	return installed != "" && fixtures != "" && nativerunner.CompareVersions(installed, fixtures) > 0
}
