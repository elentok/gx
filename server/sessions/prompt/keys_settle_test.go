package prompt_test

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

const settleRedraw = 40 * time.Millisecond

// startSettleSession is an Idle claude session whose screen redraws a moment
// after a key, so a read right after the send still shows the old rows.
func startSettleSession(t *testing.T, tune func(*server.Config)) launchFixture {
	t.Helper()
	f := startLaunchSessionWith(t, tune, agentfake.TUIRedrawDelayEnv+"="+settleRedraw.String())
	servertest.WaitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	return f
}

func (f launchFixture) keys(t *testing.T, req wire.SessionKeysRequest) wire.SessionResult {
	t.Helper()
	req.ID = f.id
	res, err := f.h.Client.Sessions.Keys(f.ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSessionKeysSettle_ReturnsTheRedrawnScreen(t *testing.T) {
	t.Parallel()
	f := startSettleSession(t, nil)
	res := f.keys(t, wire.SessionKeysRequest{Text: "settled-text", Settle: true})
	if got := strings.Join(res.Rows, "\n"); !strings.Contains(got, "settled-text") {
		t.Errorf("rows = %q, want the typed text on the redrawn screen", got)
	}
}

func TestSessionKeysSettle_WithoutTheFlagReturnsNoRows(t *testing.T) {
	t.Parallel()
	f := startSettleSession(t, nil)
	res := f.keys(t, wire.SessionKeysRequest{Text: "plain-text"})
	if res.Refused || len(res.Rows) != 0 {
		t.Errorf("result = %+v, want a success with no rows", res)
	}
}

func TestSessionKeysSettle_GuardedRequestsIgnoreTheFlag(t *testing.T) {
	t.Parallel()
	f := startSettleSession(t, nil)
	for name, req := range map[string]wire.SessionKeysRequest{
		"only_blocked": {Key: "Esc", OnlyBlocked: true, Settle: true},
		"permission":   {Permission: "allow", Settle: true},
	} {
		if res := f.keys(t, req); len(res.Rows) != 0 {
			t.Errorf("%s: rows = %q, want none", name, res.Rows)
		}
	}
}

func TestSessionKeysSettle_ReleasesMetaMuWhileSettling(t *testing.T) {
	t.Parallel()
	// A slow step keeps the settle in flight long after the redraw.
	f := startSettleSession(t, func(c *server.Config) {
		c.KeysSettleStep = 600 * time.Millisecond
		c.KeysSettleCap = 5 * time.Second
	})
	settled := make(chan wire.SessionResult, 1)
	go func() { settled <- f.keys(t, wire.SessionKeysRequest{Text: "slow-text", Settle: true}) }()
	servertest.WaitFor(t, 5*time.Second, "the typed text on screen", func() bool {
		scr, err := f.h.Client.Sessions.Screen(f.ctx, f.id)
		return err == nil && strings.Contains(strings.Join(scr.Rows, "\n"), "slow-text")
	})
	if res := f.keys(t, wire.SessionKeysRequest{Key: "Esc"}); res.Refused {
		t.Fatalf("second keys = %+v", res)
	}
	select {
	case <-settled:
		t.Error("the settle ended before the second keys call returned; want it still in flight")
	default:
	}
	<-settled
}

func TestSessionKeysSettle_CodexSessionReturnsRows(t *testing.T) {
	t.Parallel()
	f := servertest.StartCodexPromptSession(t)
	res := f.Keys(t, wire.SessionKeysRequest{Text: "codex-text", Settle: true})
	if res.Refused || len(res.Rows) == 0 {
		t.Errorf("result = %+v, want rows", res)
	}
}
