package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/output"
	"github.com/elentok/gx/tickets"
)

// Files of the worktree-diff scenario, relative to a worktree root.
const (
	WorktreeDiffModified   = "src/mod.txt"
	WorktreeDiffRenamedOld = "src/old.txt"
	WorktreeDiffRenamed    = "src/renamed.txt"
	WorktreeDiffDeleted    = "src/gone.txt"
	WorktreeDiffNew        = "notes/new.txt"
	WorktreeDiffIgnored    = "debug.log"
	WorktreeDiffPNG        = "shot.png"
)

// Names of the worktree-diff scenario.
const (
	wtdMap = "wtd" // the Design's map
	// Not wtd-impl: that is the Design's impl branch, which puts it past its Branch step.
	wtdClaimed   = "wtd-claimed-impl" // 01 is claimed
	wtdStacked   = "wtd-stack-impl"   // 01 is claimed, 02 starts from its branch
	wtdIdle      = "wtd-idle-impl"    // 01 is never claimed
	wtdFix       = "wtd-fix-impl"     // 01 is paused for repair
	wtdChatDir   = "chat-worktree"    // under the fixture root
	wtdCloneDir  = "chat-clone"       // under the fixture root
	wtdChatPR    = "hello"            // prompt of the two chats
	wtdMaxAgents = `,"max-agents":4`  // two queue agents and a Design hold slots
	wtdWaitLimit = 30 * time.Second
)

func wtdAddr(epic, id string) string { return project + ":" + epic + "/" + id }

// PostStartEnv is what a Scenario.PostStart hook works with once the server is up.
type PostStartEnv struct {
	Root, Repo, Store string
	Git               Git
	// Sessions lists the server's sessions.
	Sessions func() ([]wire.SessionInfo, error)
}

// waitSession polls until a session accepted by match has a directory on disk.
func (e PostStartEnv) waitSession(what string, match func(wire.SessionInfo) bool) (wire.SessionInfo, error) {
	for deadline := time.Now().Add(wtdWaitLimit); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		list, err := e.Sessions()
		if err != nil {
			return wire.SessionInfo{}, err
		}
		for _, si := range list {
			if match(si) && si.Dir != "" {
				if _, err := os.Stat(si.Dir); err == nil {
					return si, nil
				}
			}
		}
	}
	return wire.SessionInfo{}, fmt.Errorf("no %s session with a directory on disk", what)
}

// WorktreeDiffScenario seeds one target for each way a Worktree diff opens.
// GitSetup makes the chat worktree and the clone, which exist before the
// server starts; PostStart commits and edits in the worktrees the queue and
// the Design make, which exist only after the server is up.
func WorktreeDiffScenario() Scenario {
	open := func(epic string, tickets ...Ticket) Epic { return Epic{Project: project, Name: epic, Tickets: tickets} }
	work := func(id, slug string, opts servertest.TicketOpts) Ticket {
		return Ticket{ID: id, Slug: slug, Opts: opts}
	}
	liveDesign := project + ":" + wtdMap
	sc := Scenario{
		Epics: []Epic{
			open(wtdMap),
			open(wtdClaimed, work("01", "claimed", servertest.TicketOpts{})),
			open(wtdStacked, work("01", "blocker", servertest.TicketOpts{}), work("02", "stacked", servertest.TicketOpts{BlockedBy: []string{"01"}})),
			open(wtdIdle, work("01", "idle", servertest.TicketOpts{})),
			open(wtdFix, work("01", "broken", servertest.TicketOpts{PauseReason: "iteration-error"})),
		},
		EpicMeta:     map[string]string{liveDesign: "status: open\nkind: map\n"},
		DesignSteps:  map[string]string{liveDesign: string(tickets.StepSpec)},
		DesignByMap:  map[string][]string{liveDesign: {agentfake.DesignTurnPlain}},
		OpenDesigns:  []string{liveDesign},
		Queue:        []string{project + ":" + wtdClaimed, project + ":" + wtdStacked},
		ProjectExtra: map[string]string{project: wtdMaxAgents},
		Hold:         true,
		Chat:         true,
		FixAgents:    []string{wtdAddr(wtdFix, "01")},
		Sessions: []SessionSpec{
			{Project: project, Prompt: wtdChatPR, Read: true, Dir: RootToken + "/" + wtdChatDir},
			{Project: project, Prompt: wtdChatPR, Read: true, Dir: RootToken + "/" + wtdCloneDir},
		},
	}
	sc.GitSetup = func(git Git, repo string) error {
		g := &epicDiffGit{git: git, repo: repo}
		g.write(WorktreeDiffModified, "one\n")
		g.write(WorktreeDiffRenamedOld, numbered("// old", 20))
		g.write(WorktreeDiffDeleted, "gone\n")
		g.write(".gitignore", "*.log\n")
		g.commit("Add the sources")
		root := filepath.Dir(repo)
		chat := filepath.Join(root, wtdChatDir)
		g.run("worktree", "add", "-q", "-b", wtdChatDir, chat, "main")
		w := &epicDiffGit{git: git, repo: chat, err: g.err}
		w.run("mv", WorktreeDiffRenamedOld, WorktreeDiffRenamed)
		w.commit("Rename the old file")
		w.write("src/added.txt", "added\n")
		w.commit("Add a file")
		w.dirty()
		g.run("clone", "-q", repo, filepath.Join(root, wtdCloneDir))
		return firstErr(g.err, w.err)
	}
	sc.PostStart = func(env PostStartEnv) (*output.WorktreeDiffOutput, error) { return worktreeDiffPostStart(env) }
	return sc
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// dirty leaves every kind of uncommitted state in the worktree: modified,
// renamed then edited, deleted, untracked, ignored and an untracked PNG.
func (s *epicDiffGit) dirty() {
	s.write(WorktreeDiffModified, "one\ntwo\n")
	s.write(WorktreeDiffRenamed, numbered("// old", 20)+"// edited\n")
	s.write(WorktreeDiffNew, "new\n")
	s.write(WorktreeDiffIgnored, "log\n")
	s.write(WorktreeDiffPNG, "\x89PNG\r\n\x1a\n\x00"+string(make([]byte, 64)))
	if s.err == nil {
		s.err = os.Remove(filepath.Join(s.repo, WorktreeDiffDeleted))
	}
}

// commitAndEdit adds one commit to the worktree in dir and leaves one edit uncommitted.
func commitAndEdit(git Git, dir, name string) error {
	s := &epicDiffGit{git: git, repo: dir}
	s.write("work/"+name+".txt", name+"\n")
	s.commit(name + ": a commit")
	s.write(WorktreeDiffModified, "edited by "+name+"\n")
	return s.err
}

func worktreeDiffPostStart(env PostStartEnv) (*output.WorktreeDiffOutput, error) {
	out := &output.WorktreeDiffOutput{Unclaimed: wtdAddr(wtdIdle, "01")}
	queue := func(epic string) (output.WorktreeTarget, error) {
		addr := wtdAddr(epic, "01")
		si, err := env.waitSession(addr, func(si wire.SessionInfo) bool { return si.Queue && si.Ticket == addr })
		if err != nil {
			return output.WorktreeTarget{}, err
		}
		return output.WorktreeTarget{ID: si.ID, Path: si.Dir, Address: addr}, commitAndEdit(env.Git, si.Dir, epic)
	}
	var err error
	if out.Ticket, err = queue(wtdClaimed); err != nil {
		return nil, err
	}
	blocker, err := queue(wtdStacked)
	if err != nil {
		return nil, err
	}
	// The queue claims one ticket of the epic at a time, so the stacked ticket's
	// worktree is made here, from the blocker's iteration branch.
	out.BlockerBranch = worktree.IterationBranch(wtdStacked, "01")
	out.Stacked = output.WorktreeTarget{
		Path:    worktree.IterationWorktreePath(filepath.Dir(blocker.Path), wtdStacked, "02"),
		Address: wtdAddr(wtdStacked, "02"),
	}
	if err := env.Git(env.Repo, "worktree", "add", "-q", "-b", worktree.IterationBranch(wtdStacked, "02"), out.Stacked.Path, out.BlockerBranch); err != nil {
		return nil, err
	}
	if err := commitAndEdit(env.Git, out.Stacked.Path, wtdStacked+"-02"); err != nil {
		return nil, err
	}
	design := project + ":" + wtdMap
	si, err := env.waitSession("design", func(si wire.SessionInfo) bool { return si.Flow == "design" && si.FlowKey == design })
	if err != nil {
		return nil, err
	}
	out.Design = output.WorktreeTarget{ID: si.ID, Path: si.Dir, Address: design}
	if err := commitAndEdit(env.Git, si.Dir, wtdMap); err != nil {
		return nil, err
	}
	fixAddr := wtdAddr(wtdFix, "01")
	if si, err = env.waitSession("fix agent", func(si wire.SessionInfo) bool { return si.FixOf == fixAddr }); err != nil {
		return nil, err
	}
	out.FixAgent = output.WorktreeTarget{ID: si.ID, Path: si.Dir, Address: fixAddr}
	// The Fix agent reads the trunk checkout: edits only, no commits.
	trunk := &epicDiffGit{git: env.Git, repo: si.Dir}
	trunk.write(WorktreeDiffModified, "edited in the trunk checkout\n")
	if trunk.err != nil {
		return nil, trunk.err
	}
	list, err := env.Sessions()
	if err != nil {
		return nil, err
	}
	for _, si := range list {
		switch si.Dir {
		case filepath.Join(env.Root, wtdChatDir):
			out.Chat = output.WorktreeTarget{ID: si.ID, Path: si.Dir}
		case filepath.Join(env.Root, wtdCloneDir):
			out.Clone = output.WorktreeTarget{ID: si.ID, Path: si.Dir}
		}
	}
	return out, nil
}
