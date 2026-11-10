package sessions

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
)

func holdAsync(mg *Manager, session, id, question string) chan wire.AskHookResult {
	out := make(chan wire.AskHookResult, 1)
	req := wire.AskHookRequest{Session: session, ToolUseID: id, Questions: []wire.AskHookQuestion{{Question: question}}}
	go func() { out <- mg.HoldAsk(context.Background(), req) }()
	return out
}

func waitHeld(t *testing.T, mg *Manager, session string, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(mg.HeldAsks(session)) != n {
		if time.Now().After(deadline) {
			t.Fatalf("held asks never reached %d", n)
		}
		runtime.Gosched()
	}
}

func TestAskHold_ReleasedIsForgottenOnSessionEndOrNewBlocker(t *testing.T) {
	mg := &Manager{}
	res := holdAsync(mg, "s1", "t1", "Which one?")
	waitHeld(t, mg, "s1", 1)
	if !mg.ReleaseAsk("s1", "t1") {
		t.Fatal("ReleaseAsk = false")
	}
	<-res
	if !mg.AskReleased("s1", "Which one?") {
		t.Fatal("released ask not remembered")
	}
	mg.ForgetReleasedAsks("s1")
	if mg.AskReleased("s1", "Which one?") {
		t.Fatal("released ask still remembered after the session's entries were cleared")
	}
}

func TestAskHold_HeldAsksSortedByToolUseID(t *testing.T) {
	mg := &Manager{}
	for _, id := range []string{"c", "a", "d", "b"} {
		holdAsync(mg, "s1", id, "q")
	}
	waitHeld(t, mg, "s1", 4)
	for range 20 {
		var got string
		for _, h := range mg.HeldAsks("s1") {
			got += h.ToolUseID
		}
		if got != "abcd" {
			t.Fatalf("HeldAsks order = %q, want abcd", got)
		}
	}
}

func TestAskHold_ReAskAfterReleaseNeverRegistersAHold(t *testing.T) {
	for i := range 200 {
		mg := &Manager{}
		res := holdAsync(mg, "s1", "t1", "q")
		waitHeld(t, mg, "s1", 1)
		go mg.ReleaseAsk("s1", "t1")
		<-res
		// The hook re-asks the moment its release returns.
		again := make(chan wire.AskHookResult, 1)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		go func() { again <- mg.HoldAsk(ctx, wire.AskHookRequest{Session: "s1", ToolUseID: "t1"}) }()
		select {
		case r := <-again:
			if r.Decision != wire.AskHookRelease {
				t.Fatalf("iteration %d: re-ask decision = %q", i, r.Decision)
			}
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatalf("iteration %d: re-ask registered a hold instead of releasing at once", i)
		}
		cancel()
	}
}

// The page refetches the moment its answer is accepted, so the held card must be gone by then,
// not whenever the hook goroutine wakes up.
func TestAskHold_ResolveAndReleaseDropTheHeldAskAtOnce(t *testing.T) {
	for name, finish := range map[string]func(mg *Manager) bool{
		"resolve": func(mg *Manager) bool {
			return mg.ResolveAsk("s1", "t1", wire.AskHookResult{Decision: wire.AskHookDeny})
		},
		"release": func(mg *Manager) bool { return mg.ReleaseAsk("s1", "t1") },
	} {
		t.Run(name, func(t *testing.T) {
			mg := &Manager{}
			holdAsync(mg, "s1", "t1", "Which one?")
			waitHeld(t, mg, "s1", 1)
			if !finish(mg) {
				t.Fatal("finish = false")
			}
			if n := len(mg.HeldAsks("s1")); n != 0 {
				t.Fatalf("held asks right after finish = %d, want 0", n)
			}
		})
	}
}
