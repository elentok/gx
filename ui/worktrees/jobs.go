package worktrees

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/cmdoutput"
	"github.com/elentok/gx/ui/components"
)

type promptableJobKind int

const (
	promptableJobPushFetch promptableJobKind = iota
	promptableJobPush
	promptableJobForcePush
)

// jobs is the promptable git job (push, fetch, force-push) currently running.
// The credential modal shares its runner to answer prompts.
type jobs struct {
	runner   *components.CommandRunner
	kind     promptableJobKind
	worktree *git.Worktree
	log      *cmdoutput.CommandOutputLog
	stashed  bool
}

// finishedJob is a job's result once its runner is done.
type finishedJob struct {
	wt     git.Worktree
	kind   promptableJobKind
	output string // the runner's raw output
	log    string // accumulated log including this run
}

func (j *jobs) start(runner *components.CommandRunner, kind promptableJobKind, wt git.Worktree, log *cmdoutput.CommandOutputLog, stashed bool) {
	j.runner = runner
	j.kind = kind
	j.worktree = &wt
	j.log = log
	j.stashed = stashed
}

// running reports whether a runner is attached.
func (j jobs) running() bool {
	return j.runner != nil
}

// finish appends the run to the log and clears the job. ok is false, and
// nothing is cleared, when the job is only partly set up.
func (j *jobs) finish(repo git.Repo) (finishedJob, bool) {
	if j.runner == nil || j.worktree == nil || j.log == nil {
		return finishedJob{}, false
	}
	wt := *j.worktree
	output := j.runner.Output()
	j.log.AppendCommand("git", promptableJobArgs(repo, j.kind, wt), output)
	done := finishedJob{wt: wt, kind: j.kind, output: output, log: j.log.String()}
	*j = jobs{}
	return done, true
}
