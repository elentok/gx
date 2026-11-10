// Package ghfake is a scripted stand-in for the gh CLI plus a local bare origin,
// for tests of the PR flow. The program (cmd/gh) reads and writes a JSON state
// file; a test edits that file through Fake and reads the call log back.
package ghfake

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// DirEnv names the directory holding the state file, the lock and the call log.
const DirEnv = "GHFAKE_DIR"

// State is everything the fake gh answers from.
type State struct {
	// Auth is the failure modes of a logged-in gh with repo access, so the zero
	// value is the happy path.
	LoggedOut    bool `json:"loggedOut"`
	NoRepoAccess bool `json:"noRepoAccess"`

	PRs []PR `json:"prs"`
	// Checks are keyed by head SHA.
	Checks map[string][]Check `json:"checks"`
	// Failures are tried in order; the first whose Match prefixes the call fires.
	Failures []Failure `json:"failures"`
	// Reruns scripts what `run rerun --job <id>` turns that job's check into,
	// keyed by the job id. An unscripted rerun ends COMPLETED/SUCCESS.
	Reruns map[string]Result `json:"reruns"`
	// Created is every `pr create`, in order.
	Created []Created `json:"created"`
}

// PR is one pull request, found by its head branch.
type PR struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	Head        string `json:"head"`
	Base        string `json:"base"`
	State       string `json:"state"` // OPEN, MERGED or CLOSED
	Draft       bool   `json:"draft"`
	HeadSHA     string `json:"headSha"`
	MergeCommit string `json:"mergeCommit"`
	Mergeable   string `json:"mergeable"`
}

// Check is a check run, or a legacy status when StatusState is set (Name is then
// the status context and DetailsURL its target URL).
type Check struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	JobID       int64  `json:"jobId"`
	RunID       int64  `json:"runId"`
	WorkflowID  int64  `json:"workflowId"`
	DetailsURL  string `json:"detailsUrl"`
	StatusState string `json:"statusState"`
}

// Result is a check's status and conclusion after a rerun.
type Result struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

// Failure makes matching calls exit with Code and print Stderr. Match is a
// prefix of the space-joined argv ("pr ready"). Times is how many calls it
// fires for; zero means every call.
type Failure struct {
	Match  string `json:"match"`
	Code   int    `json:"code"`
	Stderr string `json:"stderr"`
	Times  int    `json:"times"`
}

// Created is one `gh pr create`.
type Created struct {
	Repo  string `json:"repo"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Fake is a test's handle on one fake gh's state and call log.
type Fake struct {
	t   testing.TB
	dir string
}

// New makes an empty fake with the gh binary built.
func New(t testing.TB) *Fake {
	t.Helper()
	bin(t)
	return &Fake{t: t, dir: t.TempDir()}
}

// Env returns env with the fake gh first on PATH and its state directory set.
// The last PATH wins, so base is the PATH it goes in front of.
func (f *Fake) Env(env []string, basePath string) []string {
	return append(env, DirEnv+"="+f.dir, "PATH="+filepath.Dir(bin(f.t))+string(os.PathListSeparator)+basePath)
}

// Update edits the state under the lock the program takes.
func (f *Fake) Update(edit func(*State)) {
	f.t.Helper()
	if err := Locked(f.dir, func(s *State) error { edit(s); return nil }); err != nil {
		f.t.Fatal(err)
	}
}

// State returns a copy of the current state.
func (f *Fake) State() (s State) {
	f.t.Helper()
	if err := Locked(f.dir, func(cur *State) error { s = *cur; return nil }); err != nil {
		f.t.Fatal(err)
	}
	return s
}

// Calls returns every call's argv, oldest first.
func (f *Fake) Calls() [][]string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, "calls.log"))
	if err != nil && !os.IsNotExist(err) {
		f.t.Fatal(err)
	}
	var calls [][]string
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var argv []string
		if line != "" && json.Unmarshal([]byte(line), &argv) == nil {
			calls = append(calls, argv)
		}
	}
	return calls
}

// CallsMatching returns the argv of calls whose space-joined form starts with prefix.
func (f *Fake) CallsMatching(prefix string) (out [][]string) {
	for _, c := range f.Calls() {
		if strings.HasPrefix(strings.Join(c, " "), prefix) {
			out = append(out, c)
		}
	}
	return out
}

// Locked runs fn on the state file in dir under an exclusive lock and writes the
// state back when fn succeeds. The program and the test API share it.
func Locked(dir string, fn func(*State) error) error {
	lock, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	var s State
	path := filepath.Join(dir, "state.json")
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
	}
	if err := fn(&s); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
