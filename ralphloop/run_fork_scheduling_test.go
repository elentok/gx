package ralphloop

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// claimOrderSink wraps an EventSink and records the identifier of every
// ticket TicketClaimed fires for, in call order. claimNext calls
// sink.TicketClaimed under scheduleMu (see loop.go), so this order is the
// scheduler's real claim order — not just the order concurrent iterations
// happen to finish their AgentPrompt calls in.
type claimOrderSink struct {
	EventSink
	mu    sync.Mutex
	order []string
}

func (s *claimOrderSink) TicketClaimed(ticket tickets.Ticket) {
	s.mu.Lock()
	s.order = append(s.order, ticket.Identifier)
	s.mu.Unlock()
	s.EventSink.TicketClaimed(ticket)
}

func (s *claimOrderSink) claimOrder() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.order...)
}

// indexOf returns the position of id within order, or -1 if absent.
func indexOf(order []string, id string) int {
	return slices.Index(order, id)
}

// TestRun_ForkChain_ClaimsInDependencyOrder covers AC1: a fork chain — 01's
// own AgentPrompt turn forks it into 01a (mid-run, mirroring a real
// gx-implement split: the child ticket file doesn't exist until the parent's
// turn writes it), and 01a's own turn forks it into 01b the same way — claims
// strictly in dependency order, proving the resolution model (Epic.Blocking,
// gated purely by Parent — see ticket 03/05) and the scheduler agree on the
// same chain those predicate-level tests already verified in isolation.
func TestRun_ForkChain_ClaimsInDependencyOrder(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-parent.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Parent\n",
	})
	issuesDir := filepath.Join(scratchDir, "epic", "issues")
	d, _, _ := fakeDeps()

	label01 := "pane-" + iterLabel("epic", "01")
	label01a := "pane-" + iterLabel("epic", "01a")
	var once01, once01a sync.Once
	origPrompt := d.AgentPrompt
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		switch opts.Target {
		case label01:
			once01.Do(func() {
				if err := writeChildTicket(issuesDir, "01a", "01"); err != nil {
					t.Errorf("writeChildTicket(01a): %v", err)
				}
			})
		case label01a:
			once01a.Do(func() {
				if err := writeChildTicket(issuesDir, "01b", "01a"); err != nil {
					t.Errorf("writeChildTicket(01b): %v", err)
				}
			})
		}
		return origPrompt(opts)
	}

	sink := &claimOrderSink{EventSink: NewTextEventSink(&bytes.Buffer{})}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []string{"01", "01a", "01b"}
	if got := sink.claimOrder(); !slices.Equal(got, want) {
		t.Fatalf("claim order = %v, want %v", got, want)
	}
}

// TestRun_ForkParallelChildren_BothClaimedAfterParentHandsOff covers AC2: 01's
// own AgentPrompt turn forks it into two parallel children, 01a and 01b
// (both parent: "01", neither blocked_by the other) — both must be claimed
// once 01 hands off, in either relative order, since nothing orders them
// against each other.
func TestRun_ForkParallelChildren_BothClaimedAfterParentHandsOff(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-parent.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Parent\n",
	})
	issuesDir := filepath.Join(scratchDir, "epic", "issues")
	d, _, _ := fakeDeps()

	label01 := "pane-" + iterLabel("epic", "01")
	var once sync.Once
	origPrompt := d.AgentPrompt
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if opts.Target == label01 {
			once.Do(func() {
				for _, id := range []string{"01a", "01b"} {
					if err := writeChildTicket(issuesDir, id, "01"); err != nil {
						t.Errorf("writeChildTicket(%s): %v", id, err)
					}
				}
			})
		}
		return origPrompt(opts)
	}

	sink := &claimOrderSink{EventSink: NewTextEventSink(&bytes.Buffer{})}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	order := sink.claimOrder()
	if len(order) != 3 || order[0] != "01" {
		t.Fatalf("claim order = %v, want 01 claimed first, then both children", order)
	}
	rest := map[string]bool{order[1]: true, order[2]: true}
	if !rest["01a"] || !rest["01b"] {
		t.Errorf("claim order = %v, want both 01a and 01b claimed after 01", order)
	}
}

// TestRun_DependentOfForkedTicket_WaitsForWholeSubtree covers AC3: ticket 02
// declares blocked_by: ["01"] alone, naming only the fork's root — but 01's
// own AgentPrompt turn forks it into 01a (parent: "01") mid-run, and that
// fork subtree must also finish before Epic.Blocking(01) clears, so 02 must
// not be claimed until 01a lands too, not merely once 01 itself does.
func TestRun_DependentOfForkedTicket_WaitsForWholeSubtree(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-parent.md":    "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Parent\n",
		"02-dependent.md": "---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# Dependent\n",
	})
	issuesDir := filepath.Join(scratchDir, "epic", "issues")
	d, _, _ := fakeDeps()

	label01 := "pane-" + iterLabel("epic", "01")
	var once sync.Once
	origPrompt := d.AgentPrompt
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if opts.Target == label01 {
			once.Do(func() {
				if err := writeChildTicket(issuesDir, "01a", "01"); err != nil {
					t.Errorf("writeChildTicket(01a): %v", err)
				}
			})
		}
		return origPrompt(opts)
	}

	sink := &claimOrderSink{EventSink: NewTextEventSink(&bytes.Buffer{})}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	order := sink.claimOrder()
	idx01, idx01a, idx02 := indexOf(order, "01"), indexOf(order, "01a"), indexOf(order, "02")
	if idx01 == -1 || idx01a == -1 || idx02 == -1 {
		t.Fatalf("claim order = %v, want all of 01, 01a, 02 claimed", order)
	}
	if idx02 < idx01 || idx02 < idx01a {
		t.Errorf("claim order = %v, want 02 claimed only after both 01 and its fork child 01a", order)
	}
}

// TestRun_BlockedBySpecificForkSibling_WaitsForExactlyThatSibling covers AC4:
// 01's own AgentPrompt turn forks it into two parallel children (01a, 01b,
// both parent: "01"), and ticket 02 declares blocked_by: ["01a"] — one
// specific fork sibling, not the shared parent 01 nor the other sibling 01b.
// 02 must be claimable as soon as 01a lands, without waiting for 01b —
// proven here by holding 01b's iteration open (via a blocked AgentPrompt
// call) until after 02 has already been claimed.
func TestRun_BlockedBySpecificForkSibling_WaitsForExactlyThatSibling(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-parent.md":    "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Parent\n",
		"02-dependent.md": "---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01a\"]\n---\n# Dependent\n",
	})
	issuesDir := filepath.Join(scratchDir, "epic", "issues")
	d, _, _ := fakeDeps()

	label01 := "pane-" + iterLabel("epic", "01")
	var forkOnce sync.Once
	origPromptForFork := d.AgentPrompt
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if opts.Target == label01 {
			forkOnce.Do(func() {
				for _, id := range []string{"01a", "01b"} {
					if err := writeChildTicket(issuesDir, id, "01"); err != nil {
						t.Errorf("writeChildTicket(%s): %v", id, err)
					}
				}
			})
		}
		return origPromptForFork(opts)
	}

	label01b := "pane-" + iterLabel("epic", "01b")
	unblock01b := make(chan struct{})
	var unblockOnce sync.Once
	ticket01bPath := filepath.Join(scratchDir, "epic", "issues", "01b-child.md")

	// stillClaimedAt01bRelease records 01b's on-disk status the instant 02 is
	// claimed (right before 01b's held-open AgentPrompt call is released) —
	// if the scheduler wrongly required 01b done before claiming 02, this
	// would already read "status: done"; instead it must still read
	// "status: claimed", proving 02 didn't wait for 01b to land.
	var stillClaimedAt01bRelease string

	origPrompt := d.AgentPrompt
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if opts.Target == label01b {
			<-unblock01b
		}
		return origPrompt(opts)
	}

	sink := &claimOrderSink{EventSink: NewTextEventSink(&bytes.Buffer{})}
	sink.EventSink = &unblockingSink{
		EventSink: sink.EventSink,
		onClaim: func(identifier string) {
			if identifier == "02" {
				unblockOnce.Do(func() {
					if raw, err := os.ReadFile(ticket01bPath); err == nil {
						stillClaimedAt01bRelease = string(raw)
					}
					close(unblock01b)
				})
			}
		},
	}

	runErr := make(chan error, 1)
	go func() {
		runErr <- Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)
	}()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() timed out — 02 was never claimed, meaning the scheduler wrongly waited for 01b (an unnamed blocker) before claiming 02")
	}

	if stillClaimedAt01bRelease == "" {
		t.Fatalf("02 was never claimed (unblock hook never fired)")
	}
	if !strings.Contains(stillClaimedAt01bRelease, "status: claimed") {
		t.Errorf("01b's status when 02 was claimed = %q, want still \"status: claimed\" (not yet done)", stillClaimedAt01bRelease)
	}

	order := sink.claimOrder()
	idx01a, idx02 := indexOf(order, "01a"), indexOf(order, "02")
	if idx01a == -1 || idx02 == -1 {
		t.Fatalf("claim order = %v, want both 01a and 02 claimed", order)
	}
	if idx02 < idx01a {
		t.Errorf("claim order = %v, want 02 claimed only after its named blocker 01a", order)
	}
}

// unblockingSink wraps an EventSink and invokes onClaim synchronously on
// every TicketClaimed call, letting a test react to a specific ticket's
// claim (e.g. releasing a different ticket's blocked fake agent call)
// without polling.
type unblockingSink struct {
	EventSink
	onClaim func(identifier string)
}

func (s *unblockingSink) TicketClaimed(ticket tickets.Ticket) {
	s.EventSink.TicketClaimed(ticket)
	if s.onClaim != nil {
		s.onClaim(ticket.Identifier)
	}
}

// TestRun_EpicWithWaitingForChildrenTicket_DoesNotReportComplete covers AC5:
// an epic-wide fork left mid-flight outside a scoped run's requested tickets
// — 01 already done, its fork child 01a (parent: "01") still open and never
// requested — renders 01 as waiting-for-children (Epic.Blocking(01) is still
// true because of 01a). The requested subset (02) finishing must not stamp
// the epic's own completed_at, since the epic as a whole isn't done.
func TestRun_EpicWithWaitingForChildrenTicket_DoesNotReportComplete(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-parent.md":   "---\nid: \"01\"\nstatus: done\ntype: task\n---\n# Parent\n",
		"01a-child.md":   "---\nid: \"01a\"\nstatus: open\ntype: task\nparent: \"01\"\n---\n# Child A\n",
		"02-unrelated.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Unrelated\n",
	})
	d, _, _ := fakeDeps()

	var out bytes.Buffer
	if err := Run(RunOptions{
		EpicName:   "epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  []string{"02"},
	}, d, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	epic := loadEpicByName(t, scratchDir, "epic")
	if !epic.CompletedAt.IsZero() {
		t.Errorf("CompletedAt = %v, want zero: 01 still renders waiting-for-children (01a is open), so the epic isn't complete", epic.CompletedAt)
	}

	got := epic.RenderedStatus(mustFindTicket(t, epic, "01"))
	if got != tickets.StatusWaitingForChildren {
		t.Fatalf("ticket 01 rendered status = %v, want StatusWaitingForChildren", got.Word())
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "02-unrelated.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket 02 = %q, want status: done (the requested subset still completes normally)", raw)
	}
}

func mustFindTicket(t *testing.T, epic tickets.Epic, id string) tickets.Ticket {
	t.Helper()
	for _, tk := range epic.Tickets {
		if tk.DisplayNumber() == id {
			return tk
		}
	}
	t.Fatalf("ticket %s not found in epic %q", id, epic.Name)
	return tickets.Ticket{}
}
