package sessions_test

import (
	"context"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/skills"
	"github.com/elentok/gx/testutil/agentfake"
	"testing"
)

// Aliases for the servertest helpers the moved root session tests call.
const (
	chatPollTestInterval = servertest.ChatPollTestInterval
	chatLine             = servertest.ChatLine
	codexMessageLine     = servertest.CodexMessageLine
	answerAddr           = servertest.AnswerAddr
	answerLabel          = rigs.AnswerLabel
)

var (
	waitStatus         = servertest.WaitStatus
	chatEventWithin    = servertest.ChatEventWithin
	codexEventWithin   = rigs.CodexEventWithin
	startBlockable     = servertest.StartBlockable
	startAnswerEnv     = rigs.StartAnswerEnv
	pauseEnvZeroCommit = rigs.PauseEnvZeroCommit
	runsJSONHas        = servertest.RunsJSONHas
	pauseBlocked       = servertest.PauseBlocked
)

// A session whose holder died with the server ends Exited crashed, but the
// server dying is no news about it: its saved Unread stays and no banner fires.
func TestDeadSession_StartKeepsSavedUnread(t *testing.T) {
	t.Parallel()
	for name, wantUnread := range map[string]bool{"read stays read": false, "unread stays unread": true} {
		t.Run(name, func(t *testing.T) {
			h, ctx, evs, id, conv := startBannerSession(t)
			reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, wire.SessionIdle)
			if !wantUnread {
				markRead(ctx, t, h, id)
			}
			before := len(h.Banners.Sent())
			killWhileDown(t, h, id)

			h.Restart(t)
			got := waitForSnapshot(ctx, t, h, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
			if got.Exit != wire.SessionExitCrashed || got.Unread.Unread != wantUnread {
				t.Errorf("session = %+v, want exited crashed, unread %v", got, wantUnread)
			}
			if after := len(h.Banners.Sent()); after != before {
				t.Errorf("banners went from %d to %d, want none from the start", before, after)
			}
		})
	}
}

func TestSnapshot_CarriesTheRecordedSkillsInstallProblem(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)
	ctx := context.Background()

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.SkillsInstallProblem != "" {
		t.Fatalf("problem with no record = %q, want none", snap.SkillsInstallProblem)
	}

	// `gx server start` may record it while the server already runs.
	if err := skills.WriteProblem(skills.StatusPath(h.StateDir), "install refused"); err != nil {
		t.Fatal(err)
	}
	if snap, err = h.Client.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}
	if snap.SkillsInstallProblem != "install refused" {
		t.Errorf("problem = %q, want %q", snap.SkillsInstallProblem, "install refused")
	}
}
