package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/limitseed"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tokenledger"
	"github.com/elentok/gx/transcript"
)

const (
	liveQueueEpic = "limit-burn-epic"
	liveMapEpic   = "design-epic"
	// liveTokenPollInterval is short so a usage line appended by a flow shows within a second or two.
	liveTokenPollInterval = "1s"
)

// limitBurnLive replaces the limit-burn scenario for --shape live: one held
// Claude agent each for Queue (an implement ticket), One-off (a ticket in
// one-offs), Design (a session linked to a map) and Other (a session with no
// ticket). Their transcripts are printed (see liveTranscripts).
func limitBurnLive() scenario {
	return scenario{
		LimitBurn: true,
		TokenPoll: true,
		Chat:      true,
		Hold:      true,
		Epics: []epic{
			{Project: project, Name: liveQueueEpic, Tickets: []ticket{{ID: "01", Slug: "queue-held", Opts: servertest.TicketOpts{}}}},
			{Project: project, Name: "one-offs", Tickets: []ticket{{ID: "01", Slug: "one-off-held", Opts: servertest.TicketOpts{}}}},
			{Project: project, Name: liveMapEpic, Tickets: nil},
		},
		DesignSteps: map[string]string{project + ":" + liveMapEpic: string(tickets.StepResolveTickets)},
		Queue:       []string{project + ":" + liveQueueEpic, project + ":one-offs/01"},
		// The design session is opened on the map, not linked afterwards: the
		// token poll fixes a session's group when it first sees it.
		DesignByMap: map[string][]string{project + ":" + liveMapEpic: {agentfake.DesignTurnPlain}},
		OpenDesigns: []string{project + ":" + liveMapEpic},
		Sessions:    []sessionSpec{{Project: project, Prompt: "hello", Read: true}},
	}
}

// liveTranscripts are the Claude transcript paths of the live shape's four
// agents, keyed queue, one_off, design and other. The directories may not exist
// yet.
func (f *fixture) liveTranscripts(ctx context.Context) (map[string]string, error) {
	home := filepath.Join(f.root, "home")
	out := map[string]string{}
	for key, label := range map[string]string{"queue": liveQueueEpic + "-iter-01", "one_off": "one-offs-iter-01"} {
		m, err := f.waitAgentMetaLabel(ctx, label)
		if err != nil {
			return nil, err
		}
		out[key] = transcript.PathIn(home, m.Cwd, m.SessionID)
	}
	raw, err := f.cli("agents", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w\n%s", err, raw)
	}
	var list []wire.SessionInfo
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	for _, si := range list {
		if si.Queue {
			continue
		}
		key := "other"
		if si.Ticket != "" {
			key = "design"
		}
		raw, err := f.cli("agents", "chat", si.ID, "--json")
		if err != nil {
			return nil, fmt.Errorf("chat %s: %w\n%s", si.ID, err, raw)
		}
		var chat wire.SessionChatResponse
		if err := json.Unmarshal(raw, &chat); err != nil || chat.ConversationID == "" {
			return nil, fmt.Errorf("session %s has no conversation id in %s", si.ID, raw)
		}
		out[key] = transcript.PathIn(home, si.Dir, chat.ConversationID)
	}
	if len(out) != 4 {
		return nil, fmt.Errorf("want 4 live transcripts, got %v", out)
	}
	return out, nil
}

// seedLimitBurn writes the Claude limits file and the limit history the server
// loads at start. The history file is written as server.limitHistoryState:
// every sample per limit, and the newest one as latest.
func (f *fixture) seedLimitBurn(shape string) error {
	seed := limitseed.Shapes[shape](time.Now())
	latest := map[string]wire.LimitSample{}
	for id, s := range seed.Samples {
		latest[id] = s[len(s)-1]
	}
	if err := os.MkdirAll(f.stateD, 0o700); err != nil {
		return err
	}
	for name, v := range map[string]any{
		"claude-limits.json": seed.Report,
		"limit-history.json": map[string]any{"samples": seed.Samples, "latest": latest},
	} {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(f.stateD, name), data, 0o600); err != nil {
			return err
		}
	}
	if seed.Ledger != nil {
		return f.seedLedger(seed.Ledger)
	}
	return nil
}

// seedLedger writes the token ledger through the tokenledger package, rebuilt
// rows first (they stop at the first live day), and the stamped tickets.
func (f *fixture) seedLedger(l *limitseed.Ledger) error {
	rebuilt, err := tokenledger.Open(f.stateD)
	if err != nil {
		return err
	}
	if _, _, err := rebuilt.ApplyRebuilt(l.Rebuilt); err != nil {
		return err
	}
	live, err := tokenledger.OpenWithClock(f.stateD, func() time.Time { return l.LiveSince })
	if err != nil {
		return err
	}
	for _, r := range l.Live {
		if err := live.Add(tokenledger.Delta{Key: r.Key, Tokens: r.Tokens, Dollars: r.Dollars}); err != nil {
			return err
		}
	}
	for _, tk := range l.Stamped {
		if err := servertest.WriteTicketFile(f.store, project, limitseed.StatsHistoryEpic, tk.ID, tk.Slug, tk.Opts); err != nil {
			return err
		}
	}
	return nil
}
