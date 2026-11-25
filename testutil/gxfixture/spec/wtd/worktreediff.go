// Package wtd defines the worktree-diff scenario. It is a sub-package of spec
// because spec is at its sizeguard limit.
package wtd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/limitseed"
	"github.com/elentok/gx/testutil/gxfixture/output"
	"github.com/elentok/gx/testutil/gxfixture/spec"
	"github.com/elentok/gx/tickets"
)

const project = limitseed.Project

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
	wtdClaimed = "wtd-claimed-impl" // 01 is claimed
	wtdStacked = "wtd-stack-impl"   // 01 is claimed, 02 starts from its branch
	wtdIdle    = "wtd-idle-impl"    // 01 is never claimed
	wtdFix     = "wtd-fix-impl"     // 01 is paused for repair
	wtdAsk     = "wtd-ask-impl"     // 01 asks and its agent stays up
	wtdPause   = "wtd-pause-impl"   // 01 is paused with no run entry
	// 01 is a blocker with a branch only; 02 is paused with no run entry.
	wtdPauseStack  = "wtd-pstack-impl"
	wtdCappedDir   = "capped-worktree"
	wtdOversizeDir = "oversize-worktree"
	wtdCleanDir    = "clean-worktree"
	wtdChatDir     = "chat-worktree"   // under the fixture root
	wtdCloneDir    = "chat-clone"      // under the fixture root
	wtdChatPR      = "hello"           // prompt of the two chats
	wtdMaxAgents   = `,"max-agents":8` // the queue agents, a Design and a Fix agent hold slots
	wtdWaitLimit   = 30 * time.Second
)

func wtdAddr(epic, id string) string { return project + ":" + epic + "/" + id }

// repoGit makes a worktree's commits; the first failure sticks and turns every
// later call into a no-op.
type repoGit struct {
	git  spec.Git
	repo string
	err  error
}

func (s *repoGit) run(args ...string) {
	if s.err == nil {
		s.err = s.git(s.repo, args...)
	}
}

func (s *repoGit) write(path string, data string) {
	if s.err != nil {
		return
	}
	full := filepath.Join(s.repo, path)
	if s.err = os.MkdirAll(filepath.Dir(full), 0o755); s.err == nil {
		s.err = os.WriteFile(full, []byte(data), 0o644)
	}
}

func (s *repoGit) commit(subject string) {
	s.run("add", "-A")
	s.run("commit", "-m", subject)
}

// numbered is n lines of text, "<prefix> <i>" each.
func numbered(prefix string, n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "%s %d\n", prefix, i)
	}
	return b.String()
}

// waitSession polls until a session accepted by match has a directory on disk.
func waitSession(e output.PostStartEnv, what string, match func(wire.SessionInfo) bool) (wire.SessionInfo, error) {
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

// Scenario seeds one target for each way a Worktree diff opens.
// GitSetup makes the chat worktree and the clone, which exist before the
// server starts; PostStart commits and edits in the worktrees the queue and
// the Design make, which exist only after the server is up.
func Scenario() spec.Scenario {
	open := func(epic string, tickets ...spec.Ticket) spec.Epic {
		return spec.Epic{Project: project, Name: epic, Tickets: tickets}
	}
	work := func(id, slug string, opts servertest.TicketOpts) spec.Ticket {
		return spec.Ticket{ID: id, Slug: slug, Opts: opts}
	}
	liveDesign := project + ":" + wtdMap
	sc := spec.Scenario{
		Epics: []spec.Epic{
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
		Queue:        []string{project + ":" + wtdClaimed, project + ":" + wtdStacked, project + ":" + wtdAsk},
		ProjectExtra: map[string]string{project: wtdMaxAgents},
		Behaviour:    map[string]string{wtdAddr(wtdAsk, "01"): agentfake.BehaviourWriteAsk},
		Hold:         true,
		Chat:         true,
		FixAgents:    []string{wtdAddr(wtdFix, "01")},
		Sessions: []spec.SessionSpec{
			{Project: project, Prompt: wtdChatPR, Read: true, Dir: spec.RootToken + "/" + wtdChatDir},
			{Project: project, Prompt: wtdChatPR, Read: true, Dir: spec.RootToken + "/" + wtdCloneDir},
			{Project: project, Prompt: wtdChatPR, Read: true, Dir: spec.RootToken + "/" + wtdCappedDir},
			{Project: project, Prompt: wtdChatPR, Read: true, Dir: spec.RootToken + "/" + wtdOversizeDir},
			{Project: project, Prompt: wtdChatPR, Read: true, Dir: spec.RootToken + "/" + wtdCleanDir},
		},
		StepsAfterSessions: [][]string{{"server", "triage", "open", project}},
	}
	sc.Epics = append(sc.Epics,
		open(wtdAsk, work("01", "asks", servertest.TicketOpts{})),
		open(wtdPause, work("01", "paused", servertest.TicketOpts{PauseReason: "self-reported"})),
		open(wtdPauseStack, work("01", "blocker", servertest.TicketOpts{}), work("02", "paused", servertest.TicketOpts{BlockedBy: []string{"01"}, PauseReason: "self-reported"})),
		spec.FollowUps(project, 1, 0))
	sc.GitSetup = func(git spec.Git, repo string) error {
		g := &repoGit{git: git, repo: repo}
		g.write(WorktreeDiffModified, "one\n")
		g.write(WorktreeDiffRenamedOld, numbered("// old", 20))
		g.write(WorktreeDiffDeleted, "gone\n")
		g.write(".gitignore", "*.log\n")
		g.commit("Add the sources")
		root := filepath.Dir(repo)
		chat := filepath.Join(root, wtdChatDir)
		g.run("worktree", "add", "-q", "-b", wtdChatDir, chat, "main")
		w := &repoGit{git: git, repo: chat, err: g.err}
		w.run("mv", WorktreeDiffRenamedOld, WorktreeDiffRenamed)
		w.commit("Rename the old file")
		w.write("src/added.txt", "added\n")
		w.commit("Add a file")
		w.dirty()
		g.run("clone", "-q", repo, filepath.Join(root, wtdCloneDir))
		return firstErr(g.err, w.err, edgeWorktrees(git, g, root))
	}
	sc.PostStart = worktreeDiffPostStart
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
func (s *repoGit) dirty() {
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
func commitAndEdit(git spec.Git, dir, name string) error {
	s := &repoGit{git: git, repo: dir}
	s.write("work/"+name+".txt", name+"\n")
	s.commit(name + ": a commit")
	s.write(WorktreeDiffModified, "edited by "+name+"\n")
	return s.err
}

func worktreeDiffPostStart(env output.PostStartEnv) (*output.WorktreeDiffOutput, error) {
	out := &output.WorktreeDiffOutput{Unclaimed: wtdAddr(wtdIdle, "01")}
	queue := func(epic string) (output.WorktreeTarget, error) {
		addr := wtdAddr(epic, "01")
		si, err := waitSession(env, addr, func(si wire.SessionInfo) bool { return si.Queue && si.Ticket == addr })
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
	si, err := waitSession(env, "design", func(si wire.SessionInfo) bool { return si.Flow == "design" && si.FlowKey == design })
	if err != nil {
		return nil, err
	}
	out.Design = output.WorktreeTarget{ID: si.ID, Path: si.Dir, Address: design}
	if err := commitAndEdit(env.Git, si.Dir, wtdMap); err != nil {
		return nil, err
	}
	fixAddr := wtdAddr(wtdFix, "01")
	if si, err = waitSession(env, "fix agent", func(si wire.SessionInfo) bool { return si.FixOf == fixAddr }); err != nil {
		return nil, err
	}
	out.FixAgent = output.WorktreeTarget{ID: si.ID, Path: si.Dir, Address: fixAddr}
	// The Fix agent reads the trunk checkout: edits only, no commits.
	trunk := &repoGit{git: env.Git, repo: si.Dir}
	trunk.write(WorktreeDiffModified, "edited in the trunk checkout\n")
	if trunk.err != nil {
		return nil, trunk.err
	}
	if err := edgePostStart(env, out, filepath.Dir(blocker.Path)); err != nil {
		return nil, err
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
		case filepath.Join(env.Root, wtdCappedDir):
			out.Capped = output.WorktreeTarget{ID: si.ID, Path: si.Dir}
		case filepath.Join(env.Root, wtdOversizeDir):
			out.Oversize = output.WorktreeTarget{ID: si.ID, Path: si.Dir}
		case filepath.Join(env.Root, wtdCleanDir):
			out.Clean = output.WorktreeTarget{ID: si.ID, Path: si.Dir}
		}
	}
	return out, nil
}
