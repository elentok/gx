package status

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/cmdoutput"
	"github.com/elentok/gx/ui/components"

	tea "charm.land/bubbletea/v2"
)

type stageConfirmAction int

const (
	confirmNone stageConfirmAction = iota
	confirmRebase
	confirmRebasePopStash
	confirmAmend
	confirmDiscardStatus
	confirmDiscardUnstaged
)

type stageActionKind int

const (
	actionRebase stageActionKind = iota
	actionPopStashRebase
	actionAmend
)

type stageActionResult struct {
	kind           stageActionKind
	err            error
	output         string
	promptPopStash bool
}

type stageActionRunner struct {
	kind stageActionKind
	root string
	// store is the ticket store the rebase target falls back to resolving
	// trunk in when remote is empty.
	store string

	remote string
	branch string

	runner *components.CommandRunner
	log    *cmdoutput.CommandOutputLog
	done   chan stageActionResult
	res    stageActionResult
	ok     bool
}

func newStageActionRunner(kind stageActionKind, root, remote, branch string) *stageActionRunner {
	return newStageActionRunnerWithOutput(kind, root, remote, branch, "")
}

func newStageActionRunnerWithOutput(kind stageActionKind, root, remote, branch, initialOutput string) *stageActionRunner {
	return &stageActionRunner{
		kind:   kind,
		root:   root,
		remote: remote,
		branch: branch,
		runner: components.NewCommandRunnerWithPolicy(root, "git", components.CredentialPolicyPrompt),
		log:    cmdoutput.CommandOutputLogFrom(initialOutput),
		done:   make(chan stageActionResult, 1),
	}
}

func (r *stageActionRunner) Start() {
	go func() {
		res := r.run()
		res.output = r.log.String()
		r.done <- res
	}()
}

func (r *stageActionRunner) Cancel() {
	r.runner.Cancel()
}

func (r *stageActionRunner) Consume() string {
	return r.runner.Consume()
}

func (r *stageActionRunner) Prompt() (components.CredentialPrompt, bool) {
	return r.runner.Prompt()
}

func (r *stageActionRunner) SubmitPromptInput(input string) error {
	return r.runner.SubmitPromptInput(input)
}

func (r *stageActionRunner) Result() (stageActionResult, bool) {
	if r.ok {
		return r.res, true
	}
	select {
	case res := <-r.done:
		r.res = res
		r.ok = true
		return res, true
	default:
		return stageActionResult{}, false
	}
}

func (r *stageActionRunner) run() stageActionResult {
	switch r.kind {
	case actionRebase:
		return r.runRebase()
	case actionPopStashRebase:
		res := stageActionResult{kind: r.kind}
		res.err = r.execGit("stash", "pop")
		return res
	case actionAmend:
		res := stageActionResult{kind: r.kind}
		res.err = r.execGit("commit", "--amend", "--no-edit")
		return res
	default:
		return stageActionResult{kind: r.kind, err: fmt.Errorf("unsupported action")}
	}
}

func (r *stageActionRunner) runRebase() stageActionResult {
	res := stageActionResult{kind: actionRebase}
	changes, err := git.UncommittedChanges(r.root)
	if err != nil {
		res.err = err
		return res
	}
	stashed := len(changes) > 0
	if stashed {
		if err := r.execGit("stash", "push", "-u", "-m", "gx-stage-auto-stash"); err != nil {
			res.err = fmt.Errorf("stash failed: %w", err)
			return res
		}
	}
	target := strings.TrimSpace(r.remote)
	if target == "" {
		if target, err = detectRebaseTarget(r.store, r.root); err != nil {
			res.err = err
			return res
		}
	}
	if err := r.fetchFor(target); err != nil {
		res.err = err
		if stashed {
			res.promptPopStash = true
		}
		return res
	}
	if err := r.execGit("rebase", target); err != nil {
		res.err = err
		if stashed {
			res.promptPopStash = true
		}
		return res
	}
	if stashed {
		if err := r.execGit("stash", "pop"); err != nil {
			res.err = err
			return res
		}
	}
	return res
}

// fetchFor fetches the remote a remote-tracking target lives on. A local
// trunk (no origin/<trunk>) needs no fetch, and its name may contain a slash.
func (r *stageActionRunner) fetchFor(target string) error {
	if git.RefExists(r.root, "refs/heads/"+target) {
		return nil
	}
	fetchRemote := "origin"
	if i := strings.Index(target, "/"); i > 0 {
		fetchRemote = strings.TrimSpace(target[:i])
	}
	return r.execGit("fetch", fetchRemote)
}

func (r *stageActionRunner) execGit(args ...string) error {
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

func actionPollCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return actionPollMsg{}
	})
}

func stageActionOutputTitle(kind stageActionKind) string {
	switch kind {
	case actionRebase:
		return "Rebase output"
	case actionPopStashRebase:
		return "Stash output"
	case actionAmend:
		return "Amend output"
	default:
		return "Command output"
	}
}

func (m *Model) recordCommandOutput(title, out string) {
	m.git.SetOutput(title, out)
}

func (m *Model) openOutputModal() {
	m.git.OpenOutput(m.width, m.height)
	m.keys.Reset()
}

func (m *Model) openConfirm(title string, lines []string, action stageConfirmAction, remote, branch string) {
	m.keys.Reset()
	m.modals.openConfirm(confirmRequest{title: title, lines: lines, action: action, remote: remote, branch: branch})
}

func (m *Model) openRunning(title string, runner *stageActionRunner) {
	m.keys.Reset()
	m.modals.openRunning(m.width, m.height, title, runner)
}
