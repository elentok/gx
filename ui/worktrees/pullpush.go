package worktrees

import (
	"fmt"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"

	tea "charm.land/bubbletea/v2"
)

type pullResultMsg struct {
	err error
	log string
}
type pushResultMsg struct {
	err        error
	prURL      string
	log        string
	divergence *git.PushDivergence
}
type pushFetchResultMsg struct {
	err error
	log string
	wt  git.Worktree
}
type forcePushResultMsg struct {
	err error
	log string
}

type promptableJobStartMsg struct {
	kind       promptableJobKind
	wt         git.Worktree
	initialLog string
	stashed    bool
}

type stashPullResultMsg struct {
	err     error
	log     string
	stashed bool
	wtPath  string
}
type stashPullStartedMsg struct {
	err error
	wt  git.Worktree
	log string
}

type rebasePreflightMsg struct {
	repo git.Repo
	wt   git.Worktree
}

type rebaseResultMsg struct {
	err     error
	log     string
	stashed bool
	wtPath  string
}

type stashPopResultMsg struct {
	err     error
	opLabel string
	log     string
}

func cmdStartPromptableJob(kind promptableJobKind, wt git.Worktree, initialLog string, stashed bool) tea.Cmd {
	return func() tea.Msg {
		return promptableJobStartMsg{kind: kind, wt: wt, initialLog: initialLog, stashed: stashed}
	}
}

func promptableJobArgs(repo git.Repo, kind promptableJobKind, wt git.Worktree) []string {
	switch kind {
	case promptableJobPull:
		return []string{"pull"}
	case promptableJobPushFetch:
		return []string{"fetch", git.BranchRemote(repo, wt.Branch)}
	case promptableJobPush:
		remote := git.BranchRemote(repo, wt.Branch)
		return []string{"push", remote, wt.Branch}
	case promptableJobForcePush:
		remote := git.BranchRemote(repo, wt.Branch)
		return []string{"push", "--force", remote, wt.Branch}
	default:
		return nil
	}
}

func promptableJobLabel(kind promptableJobKind, wt git.Worktree) string {
	switch kind {
	case promptableJobPull:
		return "Pulling " + wt.Name + "…"
	case promptableJobPushFetch:
		return "Checking remote divergence…"
	case promptableJobPush:
		return "Pushing " + wt.Name + "…"
	case promptableJobForcePush:
		return "Force-pushing " + wt.Name + "…"
	default:
		return ""
	}
}

func promptableJobOutputTitle(kind promptableJobKind) string {
	switch kind {
	case promptableJobPull:
		return "Pull output"
	case promptableJobPushFetch:
		return "Fetch output"
	case promptableJobPush:
		return "Push output"
	case promptableJobForcePush:
		return "Force-push output"
	default:
		return "Command output"
	}
}

func cmdOpenURL(url string) tea.Cmd {
	return ui.CmdOpenURL(url)
}

func cmdStashPull(wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		out, err := git.Stash(wt.Path)
		log := ui.NewCommandOutputLog()
		log.AppendCommand("git", []string{"stash"}, out)
		if err != nil {
			return stashPullStartedMsg{err: fmt.Errorf("stash failed: %w", err), wt: wt, log: log.String()}
		}
		return stashPullStartedMsg{wt: wt, log: log.String()}
	}
}

func cmdRebasePreflight(repo git.Repo, wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		return rebasePreflightMsg{repo: repo, wt: wt}
	}
}

func cmdRebase(repo git.Repo, wt git.Worktree, stash bool) tea.Cmd {
	return func() tea.Msg {
		log := ui.NewCommandOutputLog()
		stashed := false
		if stash {
			out, err := git.Stash(wt.Path)
			log.AppendCommand("git", []string{"stash"}, out)
			if err != nil {
				return rebaseResultMsg{err: fmt.Errorf("stash failed: %w", err), log: log.String(), wtPath: wt.Path}
			}
			stashed = true
		}
		out, err := git.Rebase(wt.Path, repo.MainBranch)
		log.AppendCommand("git", []string{"rebase", repo.MainBranch}, out)
		return rebaseResultMsg{err: err, log: log.String(), stashed: stashed, wtPath: wt.Path}
	}
}

func cmdRebaseRef(wt git.Worktree, ref, initialLog string) tea.Cmd {
	return func() tea.Msg {
		out, err := git.Rebase(wt.Path, ref)
		log := ui.CommandOutputLogFrom(initialLog)
		log.AppendCommand("git", []string{"rebase", ref}, out)
		return rebaseResultMsg{err: err, log: log.String(), stashed: false, wtPath: wt.Path}
	}
}

func cmdStashPop(wtPath, opLabel, initialLog string) tea.Cmd {
	return func() tea.Msg {
		out, err := git.StashPop(wtPath)
		log := ui.CommandOutputLogFrom(initialLog)
		log.AppendCommand("git", []string{"stash", "pop"}, out)
		return stashPopResultMsg{err: err, opLabel: opLabel, log: log.String()}
	}
}

func forcePushPrompt(wt git.Worktree) string {
	return fmt.Sprintf("Push rejected for %s. Force push?", wt.Branch)
}
