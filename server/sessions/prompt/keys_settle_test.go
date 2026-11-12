package prompt_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/sessions/prompt"
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

func TestSessionKeysSettle(t *testing.T) {
	t.Parallel()
	t.Run("ReturnsTheRedrawnScreen", func(t *testing.T) {
		t.Parallel()
		// The settle returns the old screen if two reads land before the fake
		// redraws. A step several times the redraw delay keeps that from
		// happening on a loaded machine.
		f := startSettleSession(t, func(c *server.Config) {
			c.KeysSettleStep = 8 * settleRedraw
			c.KeysSettleCap = 5 * time.Second
		})
		res := f.keys(t, wire.SessionKeysRequest{Text: "settled-text", Settle: true})
		if got := strings.Join(res.Rows, "\n"); !strings.Contains(got, "settled-text") {
			t.Errorf("rows = %q, want the typed text on the redrawn screen", got)
		}
	})

	t.Run("WithoutTheFlagReturnsNoRows", func(t *testing.T) {
		t.Parallel()
		f := startSettleSession(t, nil)
		res := f.keys(t, wire.SessionKeysRequest{Text: "plain-text"})
		if res.Refused || len(res.Rows) != 0 {
			t.Errorf("result = %+v, want a success with no rows", res)
		}
	})

	t.Run("GuardedRequestsIgnoreTheFlag", func(t *testing.T) {
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
	})

	t.Run("ReleasesMetaMuWhileSettling", func(t *testing.T) {
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
	})

	t.Run("CodexSessionReturnsRows", func(t *testing.T) {
		t.Parallel()
		f := servertest.StartCodexPromptSession(t)
		res := f.Keys(t, wire.SessionKeysRequest{Text: "codex-text", Settle: true})
		if res.Refused || len(res.Rows) == 0 {
			t.Errorf("result = %+v, want rows", res)
		}
	})
}

// scripted returns a read function that plays out reads in order, repeating
// the last one; an entry with err set fails.
type scriptedRead struct {
	rows []string
	err  error
}

func scripted(reads ...scriptedRead) func() ([]string, error) {
	i := 0
	return func() ([]string, error) {
		r := reads[min(i, len(reads)-1)]
		i++
		return r.rows, r.err
	}
}

func TestSettleScreen(t *testing.T) {
	t.Parallel()
	a, b := []string{"a"}, []string{"b"}
	boom := errors.New("boom")
	tests := map[string]struct {
		reads []scriptedRead
		want  []string
	}{
		"stable screen returns after two equal reads": {[]scriptedRead{{rows: a}, {rows: a}}, a},
		"screen changing on the second read returns the new rows": {
			[]scriptedRead{{rows: a}, {rows: b}, {rows: b}}, b},
		"read error returns the last rows": {
			[]scriptedRead{{rows: a}, {rows: b}, {err: boom}}, b},
		"first read error returns none": {[]scriptedRead{{err: boom}}, nil},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := prompt.SettleScreen(scripted(tc.reads...), time.Millisecond, time.Second)
			if !slices.Equal(got, tc.want) {
				t.Errorf("rows = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSettleScreen_NeverSettlingReturnsTheLastRowsAtTheCap(t *testing.T) {
	t.Parallel()
	n := 0
	read := func() ([]string, error) {
		n++
		return []string{string(rune('a' + n%26))}, nil
	}
	start := time.Now()
	got := prompt.SettleScreen(read, 5*time.Millisecond, 50*time.Millisecond)
	if len(got) != 1 || got[0] != string(rune('a'+n%26)) {
		t.Errorf("rows = %v after %d reads, want the last one read", got, n)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v, want it to stop near the cap", elapsed)
	}
}
