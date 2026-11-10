package ticketops_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

const optionsAskBody = "## Question\n\nWhich option?\n\n```gx-question\n" + servertest.OptionsBlock + "```\n\n## Handoff\n\nHalf done.\n"

// askedQueueAgent is a PTY Queue agent whose first turn asks; later turns are
// recorded. The agent stays alive, so its Queue row and chat stay readable.
type askedQueueAgent struct {
	h          *servertest.Harness
	ticketPath string
	session    string

	mu    sync.Mutex
	turns []string
}

func (a *askedQueueAgent) texts() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.turns...)
}

// startAskedQueueAgent launches the agent and waits for its ask. With block
// set the first turn blocks on a permission prompt instead and asks nothing.
func startAskedQueueAgent(t *testing.T, block bool) *askedQueueAgent {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	a := &askedQueueAgent{ticketPath: store + "/proj/epic-a/issues/01-first.md"}
	a.h = servertest.StartWithStore(t, store)
	a.h.OnTurn(func(turn *servertest.Turn) {
		a.mu.Lock()
		first := len(a.turns) == 0
		a.turns = append(a.turns, turn.Text)
		a.mu.Unlock()
		switch {
		case first && block:
			turn.Block("rg foo")
		case first:
			if res, err := a.h.Client.Tickets.Ask(context.Background(), answerAddr, optionsAskBody); err != nil || res.Refused {
				t.Errorf("ask = %+v, %v", res, err)
			}
		default:
			// An answered agent that ends its turn leaves, taking its Queue row
			// and chat with it; stalling keeps them readable.
			turn.Stall()
		}
	})
	if res, err := a.h.Client.Queue.Add(context.Background(), answerAddr, "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	if !block {
		waitStatus(t, a.ticketPath, schema.StatusLegacyAnswer)
		servertest.WaitFor(t, 30*time.Second, "the asked run flagged waiting", func() bool {
			data, _ := os.ReadFile(filepath.Join(a.h.StateDir, "runs.json"))
			return strings.Contains(string(data), `"waiting_since"`)
		})
	}
	servertest.WaitFor(t, 30*time.Second, "the Queue agent row", func() bool {
		rows, _ := a.h.Client.Sessions.List(context.Background())
		for _, r := range rows {
			if strings.HasPrefix(r.ID, queue.AgentIDPrefix) {
				a.session = r.ID
			}
		}
		return a.session != ""
	})
	return a
}

func (a *askedQueueAgent) cards(t *testing.T) []wire.QuestionCard {
	t.Helper()
	res, err := a.h.Client.Sessions.Chat(context.Background(), a.session)
	if err != nil {
		t.Fatal(err)
	}
	return res.Questions
}

func (a *askedQueueAgent) answerCard(t *testing.T, cardID string) wire.SessionResult {
	t.Helper()
	res, err := a.h.Client.Sessions.Answer(context.Background(), wire.SessionAnswerRequest{
		ID: a.session, Source: wire.CardSourceTicketAsk, CardID: cardID,
		Answers: []wire.SessionAnswer{{Choice: "a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (a *askedQueueAgent) detail(t *testing.T) wire.TicketDetail {
	t.Helper()
	d, err := a.h.Client.Reads.Detail(context.Background(), answerAddr)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestTicketAskCard_QueueAgentListsOnlyTheCardAndAnswersItThroughTheTicketAnswer(t *testing.T) {
	t.Parallel()
	a := startAskedQueueAgent(t, false)
	cards := a.cards(t)
	if len(cards) != 1 || cards[0].Source != wire.CardSourceTicketAsk {
		t.Fatalf("cards = %+v, want exactly one ticket-ask card", cards)
	}
	if res := a.answerCard(t, cards[0].ID); res.Refused {
		t.Fatalf("answer refused: %+v", res)
	}
	servertest.WaitFor(t, 30*time.Second, "the answer reaching the agent", func() bool { return len(a.texts()) >= 2 })
	if n := answerCount(t, a.ticketPath); n != 1 {
		t.Errorf("pause.answer lines = %d, want 1", n)
	}
	if d := a.detail(t); d.Latest == nil || d.Latest.Answer == nil {
		t.Errorf("ticket page latest = %+v, want an answered round", d.Latest)
	}
	if got := a.cards(t); len(got) != 0 {
		t.Errorf("cards after the answer = %+v, want none", got)
	}
}

func TestTicketAskCard_AnsweringOnTheTicketPageClearsTheCard(t *testing.T) {
	t.Parallel()
	a := startAskedQueueAgent(t, false)
	cards := a.cards(t)
	if len(cards) != 1 {
		t.Fatalf("cards = %+v, want one", cards)
	}
	_, state, _ := chat.SplitTicketAskID(cards[0].ID)
	res, err := a.h.Client.Tickets.Answer(context.Background(), wire.QueueRequest{Address: answerAddr, State: state, Choice: "a"})
	if err != nil || res.Refused {
		t.Fatalf("ticket answer = %+v, %v", res, err)
	}
	servertest.WaitFor(t, 30*time.Second, "the answer reaching the agent", func() bool { return len(a.texts()) >= 2 })
	if got := a.cards(t); len(got) != 0 {
		t.Errorf("cards after the ticket-page answer = %+v, want none", got)
	}
	// The card's id is now stale, so answering it again reaches nothing.
	if res := a.answerCard(t, cards[0].ID); res.Reason != wire.ReasonStale {
		t.Errorf("late card answer = %+v, want %s", res, wire.ReasonStale)
	}
}

func TestTicketAskCard_StaleIDIsRefusedAndNothingReachesTheAgent(t *testing.T) {
	t.Parallel()
	a := startAskedQueueAgent(t, false)
	res := a.answerCard(t, answerAddr+"#99:answer")
	if res.Reason != wire.ReasonStale {
		t.Fatalf("answer = %+v, want %s", res, wire.ReasonStale)
	}
	if n := answerCount(t, a.ticketPath); n != 0 {
		t.Errorf("pause.answer lines = %d, want 0", n)
	}
	if got := a.texts(); len(got) != 1 {
		t.Errorf("turns = %q, want only the launch turn", got)
	}
	if got := a.cards(t); len(got) != 1 {
		t.Errorf("the card must stay open, got %+v", got)
	}
}

func TestTicketAskCard_PermissionPromptShowsNoPermissionCardAndKeepsTheBanner(t *testing.T) {
	t.Parallel()
	a := startAskedQueueAgent(t, true)
	servertest.WaitFor(t, 30*time.Second, "the agent blocked on its permission prompt", func() bool {
		rows, _ := a.h.Client.Sessions.List(context.Background())
		for _, r := range rows {
			if r.ID == a.session && r.Status.State == wire.SessionBlocked {
				return true
			}
		}
		return false
	})
	for _, c := range a.cards(t) {
		if c.Source == wire.CardSourcePermission {
			t.Errorf("a Queue agent lists a permission card: %+v", c)
		}
	}
	if d := a.detail(t); d.Permission == nil {
		t.Errorf("ticket page lost the permission banner data")
	}
}
