package prompt_test

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/sessions/prompt"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// plainChat starts an Idle plain chat with the given name. An empty name
// leaves an automatic one.
func plainChat(t *testing.T, f promptFixture, name string) (id, conv string) {
	t.Helper()
	res, err := f.h.Client.Sessions.Create(f.ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Name: name, Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id = res.Session.ID
	conv, _ = readMeta(t, f.h, id)["conversation_id"].(string)
	waitForHolder(t, filepath.Join(f.h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	return id, conv
}

func termTitle(t *testing.T, conv, title string) {
	t.Helper()
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionTitle, Title: title}); err != nil {
		t.Fatal(err)
	}
}

func (f promptFixture) rename(t *testing.T, id, name string) wire.SessionResult {
	t.Helper()
	res, err := f.h.Client.Sessions.Rename(f.ctx, wire.SessionRenameRequest{ID: id, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSessionRename_SetsTheTrimmedNameInSnapshotAndMeta(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id, _ := plainChat(t, f, "old")
	res := f.rename(t, id, "  new name \n")
	if res.Refused || res.Session == nil || res.Session.Name != "new name" {
		t.Fatalf("rename = %+v", res)
	}
	got := waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Name == "new name" })
	if got.AutoName {
		t.Error("renamed session still has auto_name set")
	}
	if name, _ := readMeta(t, f.h, id)["name"].(string); name != "new name" {
		t.Errorf("meta name = %q, want new name", name)
	}
}

func TestSessionRename_Refusals(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id, _ := plainChat(t, f, "old")
	for name, tc := range map[string]struct{ id, name, want string }{
		"unknown":    {"nope", "x", wire.ReasonSessionUnknown},
		"queue":      {queue.AgentIDPrefix + "1", "x", wire.ReasonSessionQueue},
		"design":     {f.id, "x", wire.ReasonSessionNotPlain},
		"too long":   {id, strings.Repeat("a", wire.MaxSessionPromptBytes+1), wire.ReasonPromptTextTooLong},
		"empty":      {id, "  ", wire.ReasonSessionBadName},
		"line break": {id, "a\nb", wire.ReasonSessionBadName},
	} {
		res := f.rename(t, tc.id, tc.name)
		if !res.Refused || res.Reason != tc.want {
			t.Errorf("%s: rename = %+v, want reason %q", name, res, tc.want)
		}
	}
	if name, _ := readMeta(t, f.h, id)["name"].(string); name != "old" {
		t.Errorf("refusals changed the name to %q", name)
	}
}

// changedBefore returns the addresses of the session-changed events that
// arrive before marker's.
func changedBefore(t *testing.T, f promptFixture, marker string) []string {
	t.Helper()
	var seen []string
	for {
		ev := servertest.WaitForEvent(f.ctx, t, f.evs, wire.EventSessionChanged, "")
		if ev.Address == marker {
			return seen
		}
		seen = append(seen, ev.Address)
	}
}

func TestSessionRename_SamePinnedNamePublishesNothing(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id, _ := plainChat(t, f, "mine")
	other, _ := plainChat(t, f, "other")
	// The first marker consumes the events the two chats' startup published.
	f.rename(t, other, "first")
	changedBefore(t, f, other)
	if res := f.rename(t, id, "mine"); res.Refused || res.Session == nil || res.Session.Name != "mine" {
		t.Fatalf("rename = %+v, want success", res)
	}
	// Events arrive in order, so the marker's event comes after any of id's.
	f.rename(t, other, "second")
	for _, addr := range changedBefore(t, f, other) {
		if addr == id {
			t.Fatal("renaming to the same pinned name published session-changed")
		}
	}
}

func TestSessionRename_SamePinnedNameTypesNothing(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id, conv := plainChat(t, f, "mine")
	f.rename(t, id, "mine")
	time.Sleep(2 * firstPromptGrace) // negative wait: a send would type within this window
	if got := submitted(t, conv); got != "" {
		t.Errorf("submitted = %q, want nothing for an unchanged pinned name", got)
	}
}

func TestSessionRename_SameAutomaticNamePinsAndPublishes(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id, conv := plainChat(t, f, "")
	termTitle(t, conv, "Fix bug")
	got := waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Name == "Fix bug" })
	if !got.AutoName {
		t.Fatal("a terminal title should leave auto_name set")
	}
	if res := f.rename(t, id, "Fix bug"); res.Refused || res.Session == nil || res.Session.AutoName {
		t.Fatalf("rename = %+v, want pinned", res)
	}
	servertest.WaitForEvent(f.ctx, t, f.evs, wire.EventSessionChanged, id)
	termTitle(t, conv, "Something else")
	report(t, conv, agentfake.Control{State: "done"})
	got = waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	if got.Name != "Fix bug" {
		t.Errorf("name = %q, want the pinned Fix bug", got.Name)
	}
}

func TestSessionRename_TerminalTitleChangePublishesSessionChanged(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id, conv := plainChat(t, f, "")
	termTitle(t, conv, "From the terminal")
	servertest.WaitForEvent(f.ctx, t, f.evs, wire.EventSessionChanged, id)
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Name == "From the terminal" })
}

func TestSessionRename_TypesTheAgentCommandOnceForIdleAndWorking(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"done", "working"} {
		t.Run(state, func(t *testing.T) {
			f := startPromptSession(t)
			id, conv := plainChat(t, f, "old")
			if state == "working" {
				report(t, conv, agentfake.Control{State: "working"})
				waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
			}
			f.rename(t, id, "new name")
			waitForSubmitted(t, conv, strconv.Quote("/rename new name")+"\n")
		})
	}
}

func TestSessionRename_BlockedChatTypesNothing(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id, conv := plainChat(t, f, "old")
	report(t, conv, agentfake.Control{State: "blocked", Kind: "permission", Message: "run ls?"})
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionBlocked })
	res := f.rename(t, id, "new name")
	if res.Refused || res.Session == nil || res.Session.Name != "new name" {
		t.Fatalf("rename = %+v", res)
	}
	time.Sleep(2 * firstPromptGrace) // negative wait: a send would type within this window
	if got := submitted(t, conv); got != "" {
		t.Errorf("submitted = %q, want nothing typed into a Blocked chat", got)
	}
}

func TestSessionPrompt_TypedRenamePinsAnUnchangedAutomaticName(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id, conv := plainChat(t, f, "")
	termTitle(t, conv, "Fix bug")
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Name == "Fix bug" })
	res, err := f.h.Client.Sessions.Prompt(f.ctx, wire.SessionPromptRequest{ID: id, Text: "/rename Fix bug"})
	if err != nil || res.Refused || res.Session == nil || res.Session.AutoName {
		t.Fatalf("prompt = %+v, %v; want the name pinned", res, err)
	}
}

func TestSessionRename_RateLimitedAndExitedChangeTheNameAndTypeNothing(t *testing.T) {
	t.Parallel()
	for _, state := range []string{wire.SessionRateLimited, wire.SessionExited} {
		t.Run(state, func(t *testing.T) {
			f := startPromptSession(t)
			id, conv := plainChat(t, f, "old")
			if state == wire.SessionExited {
				if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionExit}); err != nil {
					t.Fatal(err)
				}
			} else {
				report(t, conv, agentfake.Control{State: "ratelimited"})
			}
			waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Status.State == state })
			if res := f.rename(t, id, "new name"); res.Refused || res.Session == nil || res.Session.Name != "new name" {
				t.Fatalf("rename = %+v", res)
			}
			time.Sleep(2 * firstPromptGrace) // negative wait: a send would type within this window
			if got := submitted(t, conv); got != "" {
				t.Errorf("submitted = %q, want nothing typed", got)
			}
		})
	}
}

func TestSessionRename_StartingChangesTheNameAndTypesNothing(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	seedSession(t, f, "seeded-starting", wire.SessionStarting)
	res := f.rename(t, "seeded-starting", "new name")
	if res.Refused || res.Session == nil || res.Session.Name != "new name" {
		t.Fatalf("rename = %+v", res)
	}
}

func TestRenameHold_DroppedWhenBlockedClosesIntoExited(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id, conv := blockedChat(t, f)
	f.rename(t, id, "new name")
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionExit}); err != nil {
		t.Fatal(err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
	time.Sleep(2 * actionWatchSettle) // negative wait: a release would type within this window
	if got := submitted(t, conv); got != "" {
		t.Errorf("submitted = %q, want the held command dropped", got)
	}
}

// A slash command is typed once and never nudged, so Codex's rejection is the
// one way a rename send fails; the card keeps the command to resend (story 27).
func TestSessionRename_CodexUnrecognizedCommandRaisesSendFailed(t *testing.T) {
	t.Parallel()
	f := fromPrompt(rigs.StartCodexPromptSession(t))
	f.rename(t, f.id, "new name")
	got := waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Actionable.GxRaised }).Actionable
	if !strings.HasPrefix(got.Reason, "Send failed: ") || !strings.Contains(got.Reason, "Unrecognized command") {
		t.Errorf("reason = %q, want a Send failed card naming Unrecognized command", got.Reason)
	}
	if len(got.Action) != 1 || got.Action[0] != "/rename new name" {
		t.Errorf("action = %v, want the rename command to resend", got.Action)
	}
}

func TestSessionRename_DraftThatWillNotClearRaisesTheCard(t *testing.T) {
	t.Parallel()
	f := fromPrompt(rigs.StartPromptSessionAs(t, firstPromptGrace, wire.CreateSessionRequest{Name: "old"}, servertest.WithEnv(agentfake.ProgramDraftEnv+"=my draft", agentfake.ProgramClearNeverEnv+"=1")))
	f.rename(t, f.id, "new name")
	got := waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Actionable.GxRaised }).Actionable
	if !strings.Contains(got.Reason, prompt.DraftReason) {
		t.Errorf("reason = %q, want it to name %q", got.Reason, prompt.DraftReason)
	}
	if len(got.Action) != 1 || got.Action[0] != "/rename new name" {
		t.Errorf("action = %v, want the rename command to resend", got.Action)
	}
}

// A Blocked chat can only close into Starting through a restart with a dead
// holder, and the restart drops the in-memory hold with it.
func TestRenameHold_DroppedWhenBlockedClosesIntoStartingOnRestart(t *testing.T) {
	t.Parallel()
	f := fromPrompt(rigs.StartPromptSessionAs(t, firstPromptGrace, wire.CreateSessionRequest{Name: "old"}, func(c *server.Config) { c.AgentsResumeOnStart = true }))
	id, conv := blockedChat(t, f)
	f.rename(t, id, "new name")
	child := waitForHolder(t, filepath.Join(f.h.SessionsDir, id)).ChildPID()
	if err := f.h.Stop(); err != nil {
		t.Fatal(err)
	}
	// A reboot kills the holder and its child together.
	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(child)).Output()
	if err != nil {
		t.Fatal(err)
	}
	holder, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	_ = syscall.Kill(holder, syscall.SIGKILL)
	_ = syscall.Kill(-child, syscall.SIGKILL)

	f.h.Restart(t)
	waitUntil(t, "session resumed", func() bool {
		snap, err := f.h.Client.Snapshot(f.ctx)
		if err != nil {
			return false
		}
		for _, s := range snap.Sessions {
			if s.ID == id {
				return s.Status.State == wire.SessionStarting
			}
		}
		return false
	})
	waitForHolder(t, filepath.Join(f.h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	time.Sleep(2 * actionWatchSettle) // negative wait: a release would type within this window
	if got := submitted(t, conv); got != "" {
		t.Errorf("submitted = %q, want the held command dropped", got)
	}
}

func TestSessionRename_FixAgentIsRefused(t *testing.T) {
	t.Parallel()
	f := fromPrompt(rigs.StartCodexPromptSessionAs(t, func(c *server.Config) { c.PromptGrace = firstPromptGrace }, wire.CreateSessionRequest{Ticket: "proj:foo/01", FixOf: "proj:foo/01"}))
	if res := f.rename(t, f.id, "x"); !res.Refused || res.Reason != wire.ReasonSessionNotPlain {
		t.Errorf("rename = %+v, want refused %s", res, wire.ReasonSessionNotPlain)
	}
}

func TestSessionRename_DuringAComposerSendBothArriveWhole(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id, conv := plainChat(t, f, "old")
	// A slash command is typed once and never nudged, so the log holds one line per send.
	if res, err := f.h.Client.Sessions.Prompt(f.ctx, wire.SessionPromptRequest{ID: id, Text: "/cost"}); err != nil || res.Refused {
		t.Fatalf("prompt = %+v, %v", res, err)
	}
	f.rename(t, id, "new name")
	want := []string{strconv.Quote("/cost"), strconv.Quote("/rename new name")}
	slices.Sort(want)
	waitUntil(t, "both texts submitted whole", func() bool {
		lines := strings.Split(strings.TrimSuffix(submitted(t, conv), "\n"), "\n")
		slices.Sort(lines)
		return slices.Equal(lines, want)
	})
}
