// Package claudedoctor holds the one table of named checks that verifies the
// claude features gx's native runners rely on. `gx claude doctor` runs it
// against a real claude; CI runs it against the recorded fixtures in
// testdata/, so the two cannot drift apart.
package claudedoctor

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/elentok/gx/nativerunner"
)

type Status string

const (
	Pass Status = "PASS"
	Fail Status = "FAIL"
	Skip Status = "SKIP"
)

// Mode says where a check can run. A Fixture check only reads recorded
// session events, so CI runs it too; a Live check needs a real claude
// process (e.g. process survival) and is skipped against fixtures.
type Mode string

const (
	Fixture Mode = "fixture"
	Live    Mode = "live"
)

type Check struct {
	Runner string // "headless" or "pty"
	Name   string
	Mode   Mode
	// Session names the canned session whose events Run reads; "" for none.
	Session   string
	DependsOn []string
	Run       func(Input) (detail string, ok bool)
}

type Input struct {
	ClaudeVersion string
	Events        []json.RawMessage
}

type Result struct {
	Runner string `json:"runner"`
	Name   string `json:"name"`
	Mode   Mode   `json:"mode"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
}

// Source supplies what the checks read: recorded fixtures, or a real claude
// when Live is set.
type Source struct {
	Live          bool
	ClaudeVersion string
	Session       func(name string) ([]json.RawMessage, error)
}

// ErrNoLiveSession is what a live Source returns for a canned session it
// cannot drive against a real claude yet; the check is skipped, not failed.
var ErrNoLiveSession = errors.New("no live driver for this session")

// Table is the doctor's check list, in run order.
var Table = append([]Check{
	{Runner: "headless", Name: "claude-version", Mode: Fixture, Run: checkClaudeVersion},
}, headlessChecks...)

// Run runs runner's checks from table in order. A check whose dependency did
// not pass is skipped rather than run.
func Run(table []Check, runner string, src Source) []Result {
	var results []Result
	status := map[string]Status{}
	sessions := map[string][]json.RawMessage{}
	for _, c := range table {
		if c.Runner != runner {
			continue
		}
		r := runCheck(c, src, status, sessions)
		status[c.Name] = r.Status
		results = append(results, r)
	}
	return results
}

func runCheck(c Check, src Source, status map[string]Status, sessions map[string][]json.RawMessage) Result {
	r := Result{Runner: c.Runner, Name: c.Name, Mode: c.Mode}
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

	in := Input{ClaudeVersion: src.ClaudeVersion}
	if c.Session != "" {
		events, ok := sessions[c.Session]
		if !ok {
			var err error
			if src.Session == nil {
				err = fmt.Errorf("no session source")
			} else {
				events, err = src.Session(c.Session)
			}
			if errors.Is(err, ErrNoLiveSession) {
				r.Status, r.Detail = Skip, err.Error()
				return r
			}
			if err != nil {
				r.Status, r.Detail = Fail, fmt.Sprintf("session %s: %v", c.Session, err)
				return r
			}
			sessions[c.Session] = events
		}
		in.Events = events
	}

	detail, ok := c.Run(in)
	r.Status, r.Detail = Fail, detail
	if ok {
		r.Status = Pass
	}
	return r
}

func checkClaudeVersion(in Input) (string, bool) {
	if in.ClaudeVersion == "" {
		return "could not read claude --version", false
	}
	if compareVersions(in.ClaudeVersion, nativerunner.MinClaudeVersion) < 0 {
		return fmt.Sprintf("%s is older than %s", in.ClaudeVersion, nativerunner.MinClaudeVersion), false
	}
	return fmt.Sprintf("%s >= %s", in.ClaudeVersion, nativerunner.MinClaudeVersion), true
}

// ParseVersion extracts the version from `claude --version` output.
func ParseVersion(out string) string {
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// NewerThanFixtures reports whether installed claude is newer than the
// claude that recorded the fixtures, i.e. the fixtures may be stale.
func NewerThanFixtures(installed, fixtures string) bool {
	return installed != "" && fixtures != "" && compareVersions(installed, fixtures) > 0
}

// compareVersions compares dotted numeric versions; a non-numeric part
// counts as 0.
func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		na, nb := versionPart(pa, i), versionPart(pb, i)
		if na != nb {
			if na < nb {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionPart(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, _ := strconv.Atoi(parts[i])
	return n
}
