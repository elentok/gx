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
}

// KindResult is one kind's line of the result.
type KindResult struct {
	Kind     string
	Status   Status
	Duration time.Duration
	Reason   string   // why a kind was skipped
	Log      string   // set when the kind ran
	Tail     []string // last lines of the log, set when the kind failed
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
	runDir := filepath.Join(opts.DataDir, "check", time.Now().UTC().Format("20060102T150405.000000000"))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return Result{}, err
	}
	r := &runner{root: root, runDir: runDir, files: files, mode: opts.Mode, cfg: cfg, limit: opts.ArgLimit}
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
			continue
		}
		wg.Go(func() { results[i] = r.runKind(k) })
	}
	wg.Wait()

	res := Result{Summary: filepath.Join(runDir, "summary.txt"), Kinds: results, Exit: exitCode(results)}
	if err := os.WriteFile(res.Summary, []byte(res.Render()), 0o644); err != nil {
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

type runner struct {
	root, runDir string
	files        []string
	mode         Mode
	cfg          Config
	limit        int
}

func (r *runner) runKind(kind string) KindResult {
	res := KindResult{Kind: kind}
	skip := func(reason string) KindResult {
		res.Status, res.Reason = StatusSkip, reason
		return res
	}
	c, ok := r.cfg[kind]
	switch {
	case !ok:
		return skip("gx default not built yet")
	case c.Off:
		return skip("off")
	}
	cmd := c.Full
	if r.mode != ModeFull {
		if len(r.files) == 0 {
			return skip("no changed files")
		}
		if c.Changed == "" {
			return skip("no changed command, gx default not built yet")
		}
		cmd = c.Changed
	}

	res.Log = filepath.Join(r.runDir, kind+".log")
	start := time.Now()
	res.Status = StatusPass
	logFile, err := os.Create(res.Log)
	if err != nil {
		res.Status, res.Tail = StatusFail, []string{err.Error()}
		return res
	}
	defer logFile.Close()
	for _, batch := range expandBatches(cmd, r.files, r.limit) {
		sh := exec.Command("sh", "-c", batch)
		sh.Dir = r.root
		sh.Stdout, sh.Stderr = logFile, logFile
		if err := sh.Run(); err != nil {
			res.Status = StatusFail
		}
	}
	res.Duration = time.Since(start)
	if res.Status == StatusFail {
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
