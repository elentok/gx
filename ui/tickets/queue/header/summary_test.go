package header

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

func TestEpicNameLine_NoTotalShowsNoCounterOrBar(t *testing.T) {
	t.Parallel()
	sum := wire.EpicSummary{Name: "one-offs", State: wire.EpicStateWaiting}
	got := ansi.Strip(EpicNameLine("one-offs", sum, lipgloss.NewStyle(), EpicLayoutWide, 40))
	if strings.TrimSpace(got) != "one-offs" {
		t.Errorf("name line = %q, want the name alone", got)
	}
	_, text, _ := epicStatusLine(ui.Icons(false), sum, "", 0)
	if strings.Contains(text, "merge") {
		t.Errorf("status = %q, must not read awaiting merge", text)
	}
}

func TestEpicStatusLine_UsesStateLabels(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)
	tests := []struct {
		state    wire.EpicState
		wantIcon string
		wantText string
	}{
		{wire.EpicStateAwaitingMerge, icons.TicketDone, "Awaiting merge"},
		{wire.EpicStateRunning, "S", "Running"},
		{wire.EpicStateWaiting, icons.EpicQueued, "Waiting for a slot"},
		{wire.EpicStateWaitingOnEpic, icons.EpicQueued, "Waiting"},
		{wire.EpicStatePaused, icons.Warning, "Paused"},
		{wire.EpicStateMerging, "S", "Merging"},
		{wire.EpicStateFinished, icons.TicketDone, "Merged"},
	}
	for _, tt := range tests {
		icon, text, _ := epicStatusLine(icons, wire.EpicSummary{State: tt.state}, "S", 0)
		if icon != tt.wantIcon || text != tt.wantText {
			t.Errorf("state %q: got icon=%q text=%q, want icon=%q text=%q", tt.state, icon, text, tt.wantIcon, tt.wantText)
		}
	}
}

// wireEpicStates parses server/wire/epiclist.go for every EpicState constant,
// so a new state cannot go without a label.
func wireEpicStates(t *testing.T) []wire.EpicState {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "../../../../server/wire/epiclist.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []wire.EpicState
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "EpicState" {
				continue
			}
			for _, v := range vs.Values {
				s, err := strconv.Unquote(v.(*ast.BasicLit).Value)
				if err != nil {
					t.Fatal(err)
				}
				out = append(out, wire.EpicState(s))
			}
		}
	}
	return out
}

func TestEpicStateLabels_CoverEveryWireState(t *testing.T) {
	t.Parallel()
	states := wireEpicStates(t)
	if len(states) == 0 {
		t.Fatal("found no EpicState constants")
	}
	for _, s := range states {
		if epicStateLabels[s] == "" {
			t.Errorf("wire state %q has no entry in epicStateLabels", s)
		}
	}
}

func TestEpicStatusLine_PausedShowsReasonAndMore(t *testing.T) {
	t.Parallel()
	sum := wire.EpicSummary{State: wire.EpicStatePaused, Reason: "Waiting for your answer", More: 2}
	_, text, _ := epicStatusLine(ui.Icons(false), sum, "", 0)
	if text != "Paused: Waiting for your answer +2 more" {
		t.Errorf("text = %q, want the label, the reason and the count of the others", text)
	}
}

func TestEpicStatusLine_MergeBlockedShowsReason(t *testing.T) {
	t.Parallel()
	_, text, style := epicStatusLine(ui.Icons(false), wire.EpicSummary{State: wire.EpicStatePaused, Reason: wire.EpicReasonMergeBlocked, MergeBlocked: true}, "", 0)
	if text != "Paused: Merge blocked" {
		t.Errorf("text = %q, want the label followed by the reason", text)
	}
	if style.Render("x") != EpicStatusProblemStyle.Render("x") {
		t.Errorf("merge blocked should use the warning style")
	}
}

func TestEpicStatusLine_WaitingShowsTheCapReason(t *testing.T) {
	t.Parallel()
	sum := wire.EpicSummary{State: wire.EpicStateWaiting, Verdict: wire.VerdictConcurrencyCap, VerdictReason: "2 of 2 agents"}
	_, text, _ := epicStatusLine(ui.Icons(false), sum, "", 0)
	if text != "Waiting for a slot: 2 of 2 agents" {
		t.Errorf("text = %q, want the label then the cap reason", text)
	}
}

func TestEpicStatusLine_WaitingOnEpic(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)
	moving := wire.EpicBlocker{Name: "a", State: wire.EpicListRunning}
	for _, tt := range []struct {
		name     string
		blockers []wire.EpicBlocker
		want     string
	}{
		{"one blocker", []wire.EpicBlocker{moving}, "Waiting on a"},
		{"two blockers", []wire.EpicBlocker{moving, {Name: "b"}}, "Waiting on a +1 more"},
		{"not queued", []wire.EpicBlocker{{Name: "a", State: wire.EpicListDraft}}, "Waiting on a (not queued)"},
		{"paused", []wire.EpicBlocker{{Name: "a", State: wire.EpicListPaused}, {Name: "b"}}, "Waiting on a +1 more (Paused)"},
		{"merge blocked", []wire.EpicBlocker{{Name: "a", State: wire.EpicListMergeBlocked}}, "Waiting on a (Merge blocked)"},
		{"deadlocked", []wire.EpicBlocker{{Name: "a", State: wire.EpicListPaused, Deadlocked: true}}, "Waiting on a (Deadlocked)"},
		{"waiting blocker uses its hint", []wire.EpicBlocker{{Name: "a", State: wire.EpicListQueued, Hint: &wire.BlockerHint{Name: "z", State: wire.EpicListPaused}}}, "Waiting on a (Paused)"},
		{"no blockers", nil, "Waiting"},
	} {
		icon, text, style := epicStatusLine(icons, wire.EpicSummary{State: wire.EpicStateWaitingOnEpic, Blockers: tt.blockers}, "", 0)
		if text != tt.want {
			t.Errorf("%s: text = %q, want %q", tt.name, text, tt.want)
		}
		if icon != icons.EpicQueued || style.Render("x") != lipgloss.NewStyle().Render("x") {
			t.Errorf("%s: want the queued icon in plain style, got %q", tt.name, icon)
		}
	}
}

func TestEpicStatusRow_WaitingOnCutsOnlyTheName(t *testing.T) {
	t.Parallel()
	h := testHeader(time.Now())
	h.Summaries = map[string]wire.EpicSummary{"e": {State: wire.EpicStateWaitingOnEpic, Blockers: []wire.EpicBlocker{
		{Name: "a-very-long-blocker-epic-name", State: wire.EpicListPaused}, {Name: "b"},
	}}}
	got := ansi.Strip(h.EpicStatusRow(tickets.Epic{Name: "e"}, 40))
	if lipgloss.Width(got) > 40 || !strings.Contains(got, "…") || !strings.HasSuffix(got, " +1 more (Paused)") {
		t.Errorf("row = %q, want the name cut with … and the tail whole within 40 cells", got)
	}
}

func TestEpicStatusLine_Colors(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)
	repair := wire.EpicSummary{State: wire.EpicStatePaused, HasRepair: true, Paused: []wire.SummaryTicket{
		{Status: "paused"}, {Status: "paused"},
	}}
	answer := wire.EpicSummary{State: wire.EpicStatePaused, Paused: []wire.SummaryTicket{{Status: "paused"}}}
	for _, tt := range []struct {
		name string
		sum  wire.EpicSummary
		want string
	}{
		{"awaiting-merge", wire.EpicSummary{State: wire.EpicStateAwaitingMerge}, epicStatusDoneStyle.Render(icons.TicketDone)},
		{"paused (repair)", repair, EpicStatusPausedRepairStyle.Render(icons.Warning)},
		{"paused (answer)", answer, EpicStatusPausedAnswerStyle.Render(icons.Warning)},
		{"deadlocked", wire.EpicSummary{State: wire.EpicStatePaused, Reason: wire.EpicReasonDeadlock}, EpicStatusPausedRepairStyle.Render(icons.Warning)},
	} {
		icon, _, style := epicStatusLine(icons, tt.sum, "", 0)
		if got := style.Render(icon); got != tt.want {
			t.Errorf("%s: icon rendered %q, want %q", tt.name, got, tt.want)
		}
	}
}
