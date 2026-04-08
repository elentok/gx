package worktrees

import (
	"fmt"
	"os/exec"

	"gx/git"
	"gx/ui"

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

func cmdPull(wt git.Worktree) tea.Cmd {
	rec := ui.NewCommandOutputRecorder()
	c := exec.Command("git", "pull")
	c.Dir = wt.Path
	rec.Attach(c)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		log := ui.NewCommandOutputLog()
		log.AppendCommand("git", []string{"pull"}, rec.Output().String())
		return pullResultMsg{err: err, log: log.String()}
	})
}

func cmdPush(repo git.Repo, wt git.Worktree) tea.Cmd {
	remote := git.BranchRemote(repo, wt.Branch)
	rec := ui.NewCommandOutputRecorder()
	c := exec.Command("git", "fetch", remote)
	c.Dir = wt.Path
	rec.Attach(c)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		log := ui.NewCommandOutputLog()
		log.AppendCommand("git", []string{"fetch", remote}, rec.Output().String())
		return pushFetchResultMsg{err: err, log: log.String(), wt: wt}
	})
}

func cmdPushInteractive(repo git.Repo, wt git.Worktree, initialLog string) tea.Cmd {
	remote := git.BranchRemote(repo, wt.Branch)
	rec := ui.NewCommandOutputRecorder()
	c := exec.Command("git", "push", remote, wt.Branch)
	c.Dir = wt.Path
	rec.Attach(c)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		log := ui.CommandOutputLogFrom(initialLog)
		log.AppendCommand("git", []string{"push", remote, wt.Branch}, rec.Output().String())
		return pushResultMsg{err: err, log: log.String()}
	})
}

func cmdForcePush(repo git.Repo, wt git.Worktree, initialLog string) tea.Cmd {
	remote := git.BranchRemote(repo, wt.Branch)
	rec := ui.NewCommandOutputRecorder()
	c := exec.Command("git", "push", "--force", remote, wt.Branch)
	c.Dir = wt.Path
	rec.Attach(c)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		log := ui.CommandOutputLogFrom(initialLog)
		log.AppendCommand("git", []string{"push", "--force", remote, wt.Branch}, rec.Output().String())
		return forcePushResultMsg{err: err, log: log.String()}
	})
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

func cmdPullAfterStash(wt git.Worktree, initialLog string) tea.Cmd {
	rec := ui.NewCommandOutputRecorder()
	c := exec.Command("git", "pull")
	c.Dir = wt.Path
	rec.Attach(c)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		log := ui.CommandOutputLogFrom(initialLog)
		log.AppendCommand("git", []string{"pull"}, rec.Output().String())
		return stashPullResultMsg{err: err, log: log.String(), stashed: true, wtPath: wt.Path}
	})
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
