package prflow

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/oneoff"
)

// Outcome is what one Open attempt did.
type Outcome string

// Open outcomes.
const (
	// OutcomeSkipped: the epic is not ready for a PR (the gate) and nothing changed.
	OutcomeSkipped Outcome = "skipped"
	// OutcomeBlocked: gh, the push or the repo failed; Reasons holds why, and the next attempt retries.
	OutcomeBlocked Outcome = "blocked"
	// OutcomeWriting: a pr-text ticket is running or was just started.
	OutcomeWriting Outcome = "writing"
	// OutcomeAdopted: an open PR on the epic branch was stamped on the epic.
	OutcomeAdopted Outcome = "adopted"
	// OutcomeMerged: the epic branch's PR is already merged; the host took over.
	OutcomeMerged Outcome = "merged"
	// OutcomeOpened: a draft PR was created and stamped.
	OutcomeOpened Outcome = "opened"
)

// Reasons holds why an epic's last Open attempt could not go on. It is in
// memory only: the next success clears it.
type Reasons struct {
	mu sync.Mutex
	m  map[queue.RootRef]string
}

// Reason is why root's last attempt was blocked, "" when it was not.
func (r *Reasons) Reason(root queue.RootRef) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[root]
}

func (r *Reasons) set(root queue.RootRef, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil {
		r.m = map[queue.RootRef]string{}
	}
	if reason == "" {
		delete(r.m, root)
		return
	}
	r.m[root] = reason
}

// OpenHost is what Open needs from the server root.
type OpenHost interface {
	// Env is the environment gh and git run under.
	Env() []string
	RepoDir(project string) (string, bool)
	// Trunk is the project's trunk, the PR's base.
	Trunk(project, repo string) (string, error)
	EpicDir(project, epic string) string
	// Worktree is where the epic's feature worktree lives, for the pr-text agent to read.
	Worktree(project, epic string) string
	// PRTextSkill is config skills.pr-text for project, "" for the bundled skill.
	PRTextSkill(project string) string
	Reasons() *Reasons
	// Kick asks the watcher for an early poll.
	Kick()
	// PRMerged is called when the epic branch's PR is already merged.
	PRMerged(root queue.RootRef, p pr.HeadPR)
}

// Open runs the open step of the PR flow for root (spec "Opening the PR"). Every
// failure before the PR exists leaves the epic untouched.
func Open(h OpenHost, root queue.RootRef) (Outcome, error) {
	dir := h.EpicDir(root.Project, root.Epic)
	if !openGate(dir, root) {
		return OutcomeSkipped, nil
	}
	repoDir, ok := h.RepoDir(root.Project)
	if !ok {
		return OutcomeSkipped, nil
	}
	blocked := func(reason string) (Outcome, error) {
		h.Reasons().set(root, reason)
		return OutcomeBlocked, nil
	}
	r := pr.Runner{Env: h.Env(), Dir: repoDir}
	repo, err := r.Preflight()
	if err != nil {
		return blocked(err.Error())
	}
	found, err := r.FindByHead(repo, root.Epic)
	if err != nil {
		return blocked(err.Error())
	}
	if found != nil {
		switch found.State {
		case "OPEN":
			if err := tickets.StampEpicFields(dir, map[string]any{"pr_number": found.Number, "pr_url": found.URL}); err != nil {
				return OutcomeSkipped, err
			}
			h.Reasons().set(root, "")
			_ = Append(dir, Entry{Kind: KindPRAdopted, Detail: found.URL, HeadSHA: found.HeadSHA})
			h.Kick()
			return OutcomeAdopted, nil
		case "MERGED":
			h.Reasons().set(root, "")
			h.PRMerged(root, *found)
			return OutcomeMerged, nil
		}
	}
	title, body, ok := LatestText(dir)
	if !ok {
		_, _, err := StartPRText(dir, TextJob{Epic: root.Project + ":" + root.Epic, Worktree: h.Worktree(root.Project, root.Epic), Skill: h.PRTextSkill(root.Project)})
		if err != nil {
			return OutcomeSkipped, err
		}
		h.Reasons().set(root, "")
		return OutcomeWriting, nil
	}
	trunk, err := h.Trunk(root.Project, repoDir)
	if err != nil {
		return blocked(err.Error())
	}
	if !git.RefExists(repoDir, "refs/heads/"+root.Epic) {
		return blocked(fmt.Sprintf("the epic branch %q does not exist", root.Epic))
	}
	if _, _, err := git.Run(repoDir, nonInteractive(h.Env()), "push", "origin", root.Epic); err != nil {
		return blocked("push failed: " + err.Error())
	}
	number, url, err := createDraft(r, repo, root.Epic, trunk, title, body)
	if err != nil {
		return blocked(err.Error())
	}
	if err := tickets.StampEpicFields(dir, map[string]any{"pr_number": number, "pr_url": url}); err != nil {
		return OutcomeSkipped, err
	}
	h.Reasons().set(root, "")
	_ = Append(dir, Entry{Kind: KindPROpened, Detail: "draft " + url})
	h.Kick()
	return OutcomeOpened, nil
}

// openGate is step 1: a Complete epic (pr-text tickets aside), not a one-off or
// a map, with no PR yet.
func openGate(dir string, root queue.RootRef) bool {
	if oneoff.IsOneOffsEpicName(root.Epic) {
		return false
	}
	e := tickets.LoadEpicAt(dir)
	return !e.IsMap && e.PRNumber == 0 && e.MergedAt.IsZero() && complete(e)
}

func createDraft(r pr.Runner, repo, head, base, title, body string) (int, string, error) {
	f, err := os.CreateTemp("", "gx-pr-body-*.md")
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(body); err != nil {
		_ = f.Close()
		return 0, "", err
	}
	if err := f.Close(); err != nil {
		return 0, "", err
	}
	return r.CreateDraft(repo, head, base, title, f.Name())
}

// nonInteractive is env with git prompts off and ssh in BatchMode, so a push
// that needs a credential fails instead of waiting on a terminal.
func nonInteractive(env []string) []string {
	env = slices.Clone(env)
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	if !slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "GIT_SSH_COMMAND=") }) {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return env
}
