package stagemodal

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets/ticketgit"
	"github.com/elentok/gx/ui/cmdoutput"
	"github.com/elentok/gx/ui/components"

	tea "charm.land/bubbletea/v2"
)

// ConfirmAction names what a confirm request does once accepted.
type ConfirmAction int

const (
	ConfirmNone ConfirmAction = iota
	ConfirmRebase
	ConfirmRebasePopStash
	ConfirmAmend
	ConfirmDiscardStatus
	ConfirmDiscardUnstaged
)

// ActionKind names a git action the running modal can host.
type ActionKind int

const (
	ActionRebase ActionKind = iota
	ActionPopStashRebase
	ActionAmend
)

// ActionResult is the outcome of a finished action.
type ActionResult struct {
	Kind           ActionKind
	Err            error
	Output         string
	PromptPopStash bool
}

// PollMsg asks the owner to call Model.Poll again.
type PollMsg struct{}

// Runner runs one git action in the background and exposes its output, prompts
// and result for polling.
type Runner struct {
	kind ActionKind
	root string
	// Store is the ticket store the rebase target falls back to resolving
	// trunk in when remote is empty.
	Store string

	remote string
	branch string

	runner *components.CommandRunner
	log    *cmdoutput.CommandOutputLog
	done   chan ActionResult
	res    ActionResult
	ok     bool
}

func NewRunner(kind ActionKind, root, remote, branch string) *Runner {
	return NewRunnerWithOutput(kind, root, remote, branch, "")
}

func NewRunnerWithOutput(kind ActionKind, root, remote, branch, initialOutput string) *Runner {
	return &Runner{
		kind:   kind,
		root:   root,
		remote: remote,
		branch: branch,
		runner: components.NewCommandRunnerWithPolicy(root, "git", components.CredentialPolicyPrompt),
		log:    cmdoutput.CommandOutputLogFrom(initialOutput),
		done:   make(chan ActionResult, 1),
	}
}

func (r *Runner) Start() {
	go func() {
		res := r.run()
		res.Output = r.log.String()
		r.done <- res
	}()
}

func (r *Runner) Cancel() {
	r.runner.Cancel()
}

func (r *Runner) Consume() string {
	return r.runner.Consume()
}

func (r *Runner) Prompt() (components.CredentialPrompt, bool) {
	return r.runner.Prompt()
}

func (r *Runner) SubmitPromptInput(input string) error {
	return r.runner.SubmitPromptInput(input)
}

func (r *Runner) Result() (ActionResult, bool) {
	if r.ok {
		return r.res, true
	}
	select {
	case res := <-r.done:
		r.res = res
		r.ok = true
		return res, true
	default:
		return ActionResult{}, false
	}
}

func (r *Runner) run() ActionResult {
	switch r.kind {
	case ActionRebase:
		return r.runRebase()
	case ActionPopStashRebase:
		res := ActionResult{Kind: r.kind}
		res.Err = r.execGit("stash", "pop")
		return res
	case ActionAmend:
		res := ActionResult{Kind: r.kind}
		res.Err = r.execGit("commit", "--amend", "--no-edit")
		return res
	default:
		return ActionResult{Kind: r.kind, Err: fmt.Errorf("unsupported action")}
	}
}

func (r *Runner) runRebase() ActionResult {
	res := ActionResult{Kind: ActionRebase}
	changes, err := git.UncommittedChanges(r.root)
	if err != nil {
		res.Err = err
		return res
	}
	stashed := len(changes) > 0
	if stashed {
		if err := r.execGit("stash", "push", "-u", "-m", "gx-stage-auto-stash"); err != nil {
			res.Err = fmt.Errorf("stash failed: %w", err)
			return res
		}
	}
	target := strings.TrimSpace(r.remote)
	if target == "" {
		if target, err = DetectRebaseTarget(r.Store, r.root); err != nil {
			res.Err = err
			return res
		}
	}
	if err := r.fetchFor(target); err != nil {
		res.Err = err
		if stashed {
			res.PromptPopStash = true
		}
		return res
	}
	if err := r.execGit("rebase", target); err != nil {
		res.Err = err
		if stashed {
			res.PromptPopStash = true
		}
		return res
	}
	if stashed {
		if err := r.execGit("stash", "pop"); err != nil {
			res.Err = err
			return res
		}
	}
	return res
}

// DetectRebaseTarget resolves trunk on every call, so a project.json edit
// applies at once. A missing trunk is an error, never a guessed branch.
func DetectRebaseTarget(store, root string) (string, error) {
	repo, err := ticketgit.OpenRepoIn(store, root)
	if err != nil {
		return "", err
	}
	return git.TrunkBaseRef(repo.Root, repo.Trunk), nil
}

// fetchFor fetches the remote a remote-tracking target lives on. A local
// trunk (no origin/<trunk>) needs no fetch, and its name may contain a slash.
func (r *Runner) fetchFor(target string) error {
	if git.RefExists(r.root, "refs/heads/"+target) {
		return nil
	}
	fetchRemote := "origin"
	if i := strings.Index(target, "/"); i > 0 {
		fetchRemote = strings.TrimSpace(target[:i])
	}
	return r.execGit("fetch", fetchRemote)
}

func (r *Runner) execGit(args ...string) error {
	r.runner = components.NewCommandRunnerWithPolicy(r.root, "git", components.CredentialPolicyPrompt, args...)
	r.runner.Start()
	err := r.runner.Wait()
	out := r.runner.Output()
	r.log.AppendCommand("git", args, out)
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return &git.RunError{Args: args, Dir: r.root, Stdout: strings.TrimSpace(out), Stderr: strings.TrimSpace(ee.Error()), Code: ee.ExitCode()}
		}
		return err
	}
	return nil
}

// PollCmd schedules the next PollMsg.
func PollCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return PollMsg{}
	})
}

// OutputTitle is the title the command-output modal shows for kind.
func OutputTitle(kind ActionKind) string {
	switch kind {
	case ActionRebase:
		return "Rebase output"
	case ActionPopStashRebase:
		return "Stash output"
	case ActionAmend:
		return "Amend output"
	default:
		return "Command output"
	}
}
