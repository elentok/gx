package check

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/git"
)

// Exit codes of a run.
const (
	ExitPass    = 0
	ExitFail    = 1
	ExitCantRun = 2 // invalid config, not a git repo, bad ref, bad flags
	ExitNone    = 3 // no applicable checks
)

// DefaultArgLimit is the most bytes one expanded command may have. It stays
// under the single-argument cap of `sh -c <cmd>` on Linux (128 KiB).
const DefaultArgLimit = 100_000

// tailLines is how many log lines a failing kind prints.
const tailLines = 20

// Status is one kind's outcome.
type Status string

const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// Options configure a run.
type Options struct {
	Dir     string // any directory inside the repo or worktree
	DataDir string // logs go to <DataDir>/check/<run>/
	Mode    Mode
	Since   string   // the ref for ModeSince
	Kinds   []string // empty means all
	// ArgLimit overrides DefaultArgLimit; a test sets it small to force batches.
	ArgLimit int
	// ProjectChecks supplies the project.json fallback config. It is called only
	// when the repo has no .gx/checks.json.
	ProjectChecks func() (json.RawMessage, error)
	// TicketStore is the ticket store's root, where a run record finds its
	// project and the trunk. Empty means no store.
	TicketStore string
	// ProjectFor names the project a run record is filed under; nil means
	// ProjectIn the TicketStore.
	ProjectFor func(dir string) string
	// Env is the environment every command a run starts gets, and the PATH
	// tools are looked up on. Empty means none, never the process env.
	Env []string
}

// KindResult is one kind's line of the result.
type KindResult struct {
	Kind     string
	Status   Status
	Duration time.Duration
	Reason   string   // why a kind was skipped
	Log      string   // set when the kind ran
	Tail     []string // last lines of the log, set when the kind failed
	// Failures name the failing tests of a failing gx default as brief renders
	// them (`package.TestName`).
	Failures []string
	// Notes are lines printed under the kind: a skipped row of a gx default,
	// what format rewrote.
	Notes []string
}

// Result is the outcome of a run.
type Result struct {
	Summary string // path of the summary file
	Kinds   []KindResult
	Exit    int
}

// Run selects files, runs the configured kinds and writes the logs and the
// summary. An error means gx could not run (exit 2).
func Run(opts Options) (Result, error) {
	root, err := repoRoot(opts.Dir)
	if err != nil {
		return Result{}, err
	}
	kinds := opts.Kinds
	if len(kinds) == 0 {
		kinds = Kinds
	}
	for _, k := range kinds {
		if !slices.Contains(Kinds, k) {
			return Result{}, fmt.Errorf("unknown kind %q (want %s)", k, strings.Join(Kinds, ", "))
		}
	}
	cfg, err := LoadConfig(root, opts.ProjectChecks)
	if err != nil {
		return Result{}, err
	}
	files, err := selectFiles(root, opts.Mode, opts.Since)
	if err != nil {
		return Result{}, err
	}
	started := time.Now()
	runID := started.UTC().Format("20060102T150405.000000000")
	runDir := filepath.Join(opts.DataDir, "check", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return Result{}, err
	}
	r := &runner{root: root, runDir: runDir, runID: runID, dataDir: opts.DataDir, files: files, mode: opts.Mode, cfg: cfg, limit: opts.ArgLimit, env: opts.Env, ticketStore: opts.TicketStore}
	if r.limit <= 0 {
		r.limit = DefaultArgLimit
	}

	// Kinds in Kinds order, whatever order --kind named them.
	var ordered []string
	for _, k := range Kinds {
		if slices.Contains(kinds, k) {
			ordered = append(ordered, k)
		}
	}
	results := make([]KindResult, len(ordered))
	var wg sync.WaitGroup
	for i, k := range ordered {
		if k == "format" {
			results[i] = r.runKind(k)
		}
	}
	// The tree after format ran is the one the other kinds check.
	r.treeID, _ = git.TreeID(root)
	for i, k := range ordered {
		if k != "format" {
			wg.Go(func() { results[i] = r.runKind(k) })
		}
	}
	wg.Wait()

	res := Result{Summary: filepath.Join(runDir, "summary.txt"), Kinds: results, Exit: exitCode(results)}
	if err := os.WriteFile(res.Summary, []byte(res.Render()), 0o644); err != nil {
		return Result{}, err
	}
	projectFor := opts.ProjectFor
	if projectFor == nil {
		projectFor = func(dir string) string { return ProjectIn(opts.TicketStore, dir) }
	}
	if err := appendRecord(opts.DataDir, projectFor(root), r.newRecord(opts, started, res), started); err != nil {
		return Result{}, err
	}
	return res, nil
}

func exitCode(results []KindResult) int {
	ran := false
	for _, k := range results {
		switch k.Status {
		case StatusFail:
			return ExitFail
		case StatusPass:
			ran = true
		}
	}
	if !ran {
		return ExitNone
	}
	return ExitPass
}

// childEnv is the environment of a command a kind starts: the caller's, plus
// the run (and kind) that `gx brief` tags its index lines with.
func (r *runner) childEnv(kind string) []string {
	env := append(slices.Clone(r.env), brief.EnvCheckRun+"="+r.runID)
	if kind != "" {
		env = append(env, brief.EnvCheckKind+"="+kind)
	}
	return env
}

type runner struct {
	root, runDir, dataDir string
	runID, treeID         string
	files                 []string
	mode                  Mode
	cfg                   Config
	limit                 int
	env                   []string
	ticketStore           string

	groupsOnce sync.Once
	grp        *groups

	installMu sync.Mutex
	installs  map[string]installDone
}

func (r *runner) runKind(kind string) KindResult {
	res := KindResult{Kind: kind}
	skip := func(reason string) KindResult {
		res.Status, res.Reason = StatusSkip, reason
		return res
	}
	c, ok := r.cfg[kind]
	if ok && c.Off {
		return skip("off")
	}
	cmd := c.Full
	if r.mode != ModeFull {
		if len(r.files) == 0 {
			return skip("no changed files")
		}
		cmd = c.Changed
	}
	useDefault := !ok || cmd == ""

	res.Log = filepath.Join(r.runDir, kind+".log")
	start := time.Now()
	res.Status = StatusPass
	logFile, err := os.Create(res.Log)
	if err != nil {
		res.Status, res.Tail = StatusFail, []string{err.Error()}
		return res
	}
	defer logFile.Close()
	var before map[string]string
	if kind == "format" {
		before = hashFiles(r.root, r.files)
	}
	if useDefault {
		r.runDefault(kind, logFile, &res)
		if res.Status == StatusSkip {
			logFile.Close()
			os.Remove(res.Log)
			res.Log = ""
			return res
		}
	} else {
		for _, batch := range expandBatches(cmd, r.files, r.limit) {
			sh := exec.Command("sh", "-c", batch)
			sh.Dir = r.root
			sh.Env = r.childEnv(kind)
			sh.Stdout, sh.Stderr = logFile, logFile
			if err := sh.Run(); err != nil {
				res.Status = StatusFail
			}
		}
	}
	res.Duration = time.Since(start)
	if before != nil {
		r.afterFormat(before, &res)
	}
	if res.Status == StatusFail && len(res.Failures) == 0 && res.Tail == nil {
		res.Tail = lastLines(res.Log, tailLines)
	}
	return res
}

func lastLines(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(bytes.TrimRight(data, "\n")), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
