package rows

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tickets/queue/header"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tree"
)

// tk builds a ticket literal: id is both Identifier and Number, parent is ""
// for a top-level ticket.
func tk(epic, id, title, status, parent string, blockedBy ...string) tickets.Ticket {
	n, _ := strconv.Atoi(id)
	t := tickets.Ticket{
		Number:     n,
		Identifier: id,
		Title:      title,
		Path:       "/store/" + epic + "/issues/" + id + ".md",
		Type:       "implement",
		Status:     status,
		BlockedBy:  blockedBy,
	}
	if parent != "" {
		t.Parent = &parent
	}
	return t
}

func epicOf(name string, ts ...tickets.Ticket) tickets.Epic {
	return tickets.Epic{Name: name, Path: "/store/" + name, HasTicketMD: true, Tickets: ts}
}

func testEnv() RowsEnv {
	return RowsEnv{Header: header.Header{Icons: ui.Icons(false), Now: time.Now}, MaxParallel: 2, Suggested: func(tickets.Epic, tickets.Ticket) bool { return false }}
}

// loadRows builds Rows over epics with every given ticket checked, the way the
// Queue tab does after the app shell delivers epics and the selection.
func loadRows(epics []tickets.Epic, checked ...tickets.Ticket) Rows {
	tr := tree.NewModel[Node]()
	tr.SetIsSelectable(func(n Node) bool { return n.Kind == NodeTicket || n.Kind == NodeEpicName })
	r := NewRows(tr)
	r.Epics = epics
	for _, t := range checked {
		r.Checked[t.Path] = true
	}
	r.reload(testEnv())
	return r
}

func (r *Rows) reload(env RowsEnv) {
	r.Tree.SetEntries(r.BuildEntriesCached(env))
	r.Tree.SkipUnselectable(1)
}

func renderRows(r Rows) string {
	env := testEnv()
	return strings.Join(r.Tree.RenderLines(200, r.RenderOpts(env, 200)), "\n")
}

// ticketEntries filters the tree's entries down to ticket rows, dropping the
// per-epic separator/status/error entries BuildEntries interleaves — the
// tree's own Depth/HasChildren/Expanded on each Entry is the nesting truth
// these tests assert against.
func ticketEntries(r Rows) []tree.Entry[Node] {
	var out []tree.Entry[Node]
	for _, e := range r.Tree.Entries() {
		if e.Value.Kind == NodeTicket {
			out = append(out, e)
		}
	}
	return out
}

func selectFirstTicketRow(t *testing.T, r *Rows) {
	t.Helper()
	for i, e := range r.Tree.Entries() {
		if e.Value.Kind == NodeTicket {
			r.Tree.SetSelectedIndex(i)
			return
		}
	}
	t.Fatalf("expected at least one ticket row, found none")
}

func TestRowsRendersFlatDependencyOrderedEpicPlan(t *testing.T) {
	t.Parallel()
	a := []tickets.Ticket{
		tk("alpha", "01", "Foundation", "open", ""),
		tk("alpha", "02", "Dependent", "open", "", "01"),
		tk("alpha", "03", "Independent", "open", ""),
		tk("alpha", "04", "Independent", "open", ""),
	}
	b := tk("beta", "01", "Other", "open", "")
	r := loadRows([]tickets.Epic{epicOf("alpha", a...), epicOf("beta", b)}, append(a, b)...)
	content := renderRows(r)

	if strings.Contains(content, "parallel") || strings.Contains(content, "then") {
		t.Fatalf("expected a flat dependency-ordered list, not wave grouping:\n%s", content)
	}

	// maxParallel caps each wave: with the default cap, 01/03/04 (all
	// unblocked) don't all fit in wave 1, so 04 spills into wave 2 alongside
	// 02 (only just unblocked once 01 lands) — still ticket-number order
	// within each wave, and 02's row still lands after its blocker 01's.
	alpha := strings.Index(content, "alpha")
	foundation := strings.Index(content, "Foundation")
	independent3 := strings.Index(content, "03")
	dependent := strings.Index(content, "Dependent")
	independent4 := strings.Index(content, "04")
	beta := strings.Index(content, "beta")
	if alpha < 0 || foundation < alpha || independent3 < foundation || dependent < independent3 || independent4 < dependent || beta < independent4 {
		t.Fatalf("expected blockers-before-dependents order within alpha, then epic grouping into beta:\n%s", content)
	}
}

// TestRowsOrdersRowsByDependencyNotTicketNumber covers ticket 11: the
// Queue tab's row order must reflect actual execution order (blockers before
// dependents via schedule.PlanWaves), not plain ticket-number order — even
// when the blocker has a higher ticket number than its dependent.
func TestRowsOrdersRowsByDependencyNotTicketNumber(t *testing.T) {
	t.Parallel()
	ts := []tickets.Ticket{
		tk("alpha", "20", "Dependent", "open", "", "50"),
		tk("alpha", "50", "Blocker", "open", ""),
		tk("alpha", "60", "Unrelated", "open", ""),
		tk("alpha", "10", "Unrelated", "open", ""),
	}
	rows := ticketEntries(loadRows([]tickets.Epic{epicOf("alpha", ts...)}, ts...))
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d: %+v", len(rows), rows)
	}

	indexOf := func(id string) int {
		for i, r := range rows {
			if r.Value.Ticket.Ticket.Identifier == id {
				return i
			}
		}
		t.Fatalf("ticket %q missing from rows: %+v", id, rows)
		return -1
	}

	// Wave 1 (no unmet blocker): 10, 50, 60, tiebroken by ticket number.
	// Wave 2 (unblocked once 50 lands): 20.
	if got := []int{indexOf("10"), indexOf("50"), indexOf("60")}; got[0] > got[1] || got[1] > got[2] {
		t.Fatalf("expected wave-1 tickets 10,50,60 in ticket-number order, got indexes %v", got)
	}
	if indexOf("20") < indexOf("50") {
		t.Fatalf("expected blocker 50's row before its dependent 20's row, got rows %+v", rows)
	}
}

// TestRowsNestsChildrenUnderParentAndCollapses covers ticket 10's
// acceptance criteria: a checked ticket's children (Parent/Children, ticket
// 03) render nested underneath it in the Queue tab, and collapsing and
// re-expanding the parent hides and restores that nesting.
func TestRowsNestsChildrenUnderParentAndCollapses(t *testing.T) {
	t.Parallel()
	ts := []tickets.Ticket{
		tk("alpha", "01", "Parent", "open", ""),
		tk("alpha", "02", "Child", "open", "01"),
		tk("alpha", "03", "Other", "open", ""),
	}
	r := loadRows([]tickets.Epic{epicOf("alpha", ts...)}, ts...)

	rows := ticketEntries(r)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].Value.Ticket.Ticket.Identifier != "01" || rows[0].Depth != 0 || !rows[0].HasChildren || !rows[0].Expanded {
		t.Fatalf("expected parent 01 at depth 0 with children, got %+v", rows[0])
	}
	if rows[1].Value.Ticket.Ticket.Identifier != "02" || rows[1].Depth != 1 {
		t.Fatalf("expected child 02 nested at depth 1 right after its parent, got %+v", rows[1])
	}
	if rows[2].Value.Ticket.Ticket.Identifier != "03" || rows[2].Depth != 0 {
		t.Fatalf("expected unrelated ticket 03 at depth 0, got %+v", rows[2])
	}

	content := renderRows(r)
	if !strings.Contains(content, "▼") {
		t.Fatalf("expected an expanded-triangle glyph for the parent row:\n%s", content)
	}

	// Select the parent row (now behind the epic's header rows) before collapsing its children.
	selectFirstTicketRow(t, &r)
	r.Tree.CollapseSelected()
	r.reload(testEnv())
	rows = ticketEntries(r)
	if len(rows) != 2 || rows[0].Value.Ticket.Ticket.Identifier != "01" || rows[1].Value.Ticket.Ticket.Identifier != "03" {
		t.Fatalf("expected only 01 and 03 after collapsing 01's children, got %+v", rows)
	}
	if strings.Contains(renderRows(r), "Child") {
		t.Fatalf("expected child ticket hidden after collapse:\n%s", renderRows(r))
	}

	r.Tree.ExpandSelected()
	r.reload(testEnv())
	rows = ticketEntries(r)
	if len(rows) != 3 {
		t.Fatalf("expected expanding to restore all 3 rows, got %d: %+v", len(rows), rows)
	}
}

// An investigate child nests under its one-off exactly as any fork child nests
// under an epic ticket.
func TestRowsNestsInvestigateChildUnderOneOff(t *testing.T) {
	t.Parallel()
	prompt := tk("fix-it", "01", "Prompt", "paused", "")
	prompt.PauseReason = schema.PauseBlockedPane
	child := tk("fix-it", "02", "Investigate", "open", "01")
	child.Type = "investigate"
	r := loadRows([]tickets.Epic{epicOf("fix-it", prompt, child)}, prompt, child)

	rows := ticketEntries(r)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].Value.Ticket.Ticket.Identifier != "01" || rows[0].Depth != 0 || !rows[0].HasChildren {
		t.Fatalf("expected one-off 01 at depth 0 with children, got %+v", rows[0])
	}
	if rows[1].Value.Ticket.Ticket.Identifier != "02" || rows[1].Depth != 1 {
		t.Fatalf("expected investigate child 02 nested at depth 1, got %+v", rows[1])
	}
}

// TestRowsNestsChildrenAtArbitraryDepthAndRespectsCollapse ports
// queue_rows_test.go's TestQueueRowsForEpic_NestsChildrenAtArbitraryDepthAndRespectsCollapse
// (ticket 19): a ticket (01) with a child (02) that itself has a child (03)
// nests two levels deep, leaving an unrelated ticket (04) at the top level,
// and collapsing the grandparent hides both descendants while leaving 04
// visible.
func TestRowsNestsChildrenAtArbitraryDepthAndRespectsCollapse(t *testing.T) {
	t.Parallel()
	ts := []tickets.Ticket{
		tk("alpha", "01", "A", "open", ""),
		tk("alpha", "02", "B", "open", "01"),
		tk("alpha", "03", "C", "open", "02"),
		tk("alpha", "04", "D", "open", ""),
	}
	r := loadRows([]tickets.Epic{epicOf("alpha", ts...)}, ts...)

	rows := ticketEntries(r)
	wantIDs := []string{"01", "02", "03", "04"}
	wantDepths := []int{0, 1, 2, 0}
	if len(rows) != len(wantIDs) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(wantIDs), rows)
	}
	for i, row := range rows {
		if row.Value.Ticket.Ticket.Identifier != wantIDs[i] {
			t.Fatalf("row %d ticket = %q, want %q", i, row.Value.Ticket.Ticket.Identifier, wantIDs[i])
		}
		if row.Depth != wantDepths[i] {
			t.Fatalf("row %d (%s) depth = %d, want %d", i, wantIDs[i], row.Depth, wantDepths[i])
		}
	}
	if !rows[0].HasChildren || !rows[0].Expanded {
		t.Fatalf("expected ticket 01 to report hasChildren+expanded, got %+v", rows[0])
	}

	selectFirstTicketRow(t, &r)
	r.Tree.CollapseSelected()
	r.reload(testEnv())
	rows = ticketEntries(r)
	wantAfterCollapse := []string{"01", "04"}
	if len(rows) != len(wantAfterCollapse) {
		t.Fatalf("expected %d rows after collapsing 01, got %d: %+v", len(wantAfterCollapse), len(rows), rows)
	}
	for i, want := range wantAfterCollapse {
		if rows[i].Value.Ticket.Ticket.Identifier != want {
			t.Fatalf("row %d after collapse = %q, want %q", i, rows[i].Value.Ticket.Ticket.Identifier, want)
		}
	}
	if rows[0].Expanded {
		t.Fatalf("expected collapsed ticket 01 to report expanded=false")
	}
}

// TestRowsInjectsNonCandidateAncestorDimmedInsteadOfPromotingToRoot
// covers ticket 08: when a checked ticket's real parent isn't itself a
// scheduling candidate (unchecked), the tree stays connected by injecting
// that parent as a dimmed, non-actionable row rather than promoting the
// checked ticket to the top level.
func TestRowsInjectsNonCandidateAncestorDimmedInsteadOfPromotingToRoot(t *testing.T) {
	t.Parallel()
	ts := []tickets.Ticket{
		tk("alpha", "01", "A", "open", ""),
		tk("alpha", "02", "B", "open", "01"),
		tk("alpha", "03", "C", "open", ""),
	}
	// 01 (the parent) is deliberately left unchecked: not a scheduling
	// candidate on its own.
	r := loadRows([]tickets.Epic{epicOf("alpha", ts...)}, ts[1], ts[2])

	rows := ticketEntries(r)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (injected 01 + checked 02/03), got %d: %+v", len(rows), rows)
	}
	if rows[0].Value.Ticket.Ticket.Identifier != "01" || rows[0].Depth != 0 {
		t.Fatalf("expected injected ancestor 01 at depth 0, got %+v", rows[0])
	}
	if rows[0].Value.Ticket.Actionable {
		t.Fatalf("expected injected ancestor 01 to be marked non-actionable, got %+v", rows[0].Value.Ticket)
	}
	if rows[1].Value.Ticket.Ticket.Identifier != "02" || rows[1].Depth != 1 {
		t.Fatalf("expected checked ticket 02 nested under injected 01 at depth 1, got %+v", rows[1])
	}
	if !rows[1].Value.Ticket.Actionable {
		t.Fatalf("expected checked ticket 02 to stay marked actionable, got %+v", rows[1].Value.Ticket)
	}
	if rows[2].Value.Ticket.Ticket.Identifier != "03" || rows[2].Depth != 0 {
		t.Fatalf("expected unrelated checked ticket 03 at depth 0, got %+v", rows[2])
	}

	plain := ansi.Strip(renderRows(r))
	if !strings.Contains(plain, "01") {
		t.Fatalf("expected injected ancestor 01's row to render:\n%s", plain)
	}
}

// TestRowsSharedNonCandidateAncestorInjectedOnce covers the dedup path
// of attachQueueAncestors: two checked tickets sharing the same non-candidate
// parent must only inject that parent once, with both children nested under
// the single injected row.
func TestRowsSharedNonCandidateAncestorInjectedOnce(t *testing.T) {
	t.Parallel()
	ts := []tickets.Ticket{
		tk("alpha", "01", "A", "open", ""),
		tk("alpha", "02", "B", "open", "01"),
		tk("alpha", "03", "C", "open", "01"),
	}
	r := loadRows([]tickets.Epic{epicOf("alpha", ts...)}, ts[1], ts[2])

	rows := ticketEntries(r)
	wantIDs := []string{"01", "02", "03"}
	wantDepths := []int{0, 1, 1}
	if len(rows) != len(wantIDs) {
		t.Fatalf("expected a single injected 01 with both 02 and 03 nested under it, got %d rows: %+v", len(rows), rows)
	}
	for i, want := range wantIDs {
		if rows[i].Value.Ticket.Ticket.Identifier != want {
			t.Fatalf("row %d = %q, want %q: %+v", i, rows[i].Value.Ticket.Ticket.Identifier, want, rows)
		}
		if rows[i].Depth != wantDepths[i] {
			t.Fatalf("row %d (%s) depth = %d, want %d", i, want, rows[i].Depth, wantDepths[i])
		}
	}
}

// TestRowsDoneParentWithOpenForkChildStaysVisible ports
// queue_rows_test.go's TestQueueRowsForEpic_DoneParentWithOpenForkChildStaysVisible
// (ticket 19): a done parent whose fork child (Parent: ticket 03) is still
// open must not be dropped by HideComplete's filterDoneTickets — the child
// stays nested under its still-live parent instead of being reattached to
// the top level.
func TestRowsDoneParentWithOpenForkChildStaysVisible(t *testing.T) {
	t.Parallel()
	ts := []tickets.Ticket{
		tk("alpha", "01", "A", "done", ""),
		tk("alpha", "02", "B", "open", "01"),
	}
	r := loadRows([]tickets.Epic{epicOf("alpha", ts...)}, ts...)
	r.HideComplete = true
	r.reload(testEnv())

	rows := ticketEntries(r)
	if len(rows) != 2 || rows[0].Value.Ticket.Ticket.Identifier != "01" || rows[1].Value.Ticket.Ticket.Identifier != "02" {
		t.Fatalf("expected 01 (waiting-for-children) and nested 02, got %+v", rows)
	}
	if rows[1].Depth != 1 {
		t.Fatalf("expected ticket 02 nested under 01 at depth 1, got depth=%d", rows[1].Depth)
	}
}

func TestRowsNeverShowsATicketRunnableWhenOutOfScopeBlockerIsUnmet(t *testing.T) {
	t.Parallel()
	ts := []tickets.Ticket{
		tk("alpha", "01", "Foundation", "open", ""),
		tk("alpha", "02", "Dependent", "open", "", "01"),
	}
	// Only the dependent ticket is checked — its blocker never got selected,
	// and it isn't done, so ralph-loop's own scheduler would never claim it
	// either (schedule.RunScope.Frontier resolves blockers epic-wide).
	content := renderRows(loadRows([]tickets.Epic{epicOf("alpha", ts...)}, ts[1]))

	if strings.Contains(content, "parallel") || strings.Contains(content, "then") {
		t.Fatalf("expected no runnable wave for a ticket whose out-of-scope blocker is unmet:\n%s", content)
	}
	if !strings.Contains(content, "Dependent") {
		t.Fatalf("expected the checked ticket to remain visible for toggling:\n%s", content)
	}
	if !strings.Contains(content, "no unblocked tickets left") {
		t.Fatalf("expected an actionable plan error instead of a misleading wave:\n%s", content)
	}
}

func TestRowsSurfacesActionableErrorForDependencyCycle(t *testing.T) {
	t.Parallel()
	ts := []tickets.Ticket{
		tk("alpha", "01", "First", "open", "", "02"),
		tk("alpha", "02", "Second", "open", "", "01"),
	}
	content := renderRows(loadRows([]tickets.Epic{epicOf("alpha", ts...)}, ts...))

	if !strings.Contains(content, "no unblocked tickets left") {
		t.Fatalf("expected an actionable cycle error:\n%s", content)
	}
	if !strings.Contains(content, "First") || !strings.Contains(content, "Second") {
		t.Fatalf("expected both cyclic tickets to remain visible for toggling:\n%s", content)
	}
}

func TestRowsIncludesSelectionsAddedAfterLoad(t *testing.T) {
	t.Parallel()
	later := tk("alpha", "01", "Later", "open", "")
	r := loadRows([]tickets.Epic{epicOf("alpha", later)})

	r.Checked[later.Path] = true
	r.reload(testEnv())
	if content := renderRows(r); !strings.Contains(content, "Later") {
		t.Fatalf("expected cached rows to include a later shared selection:\n%s", content)
	}
}

func TestRenderQueueTicketRow_CommitlessSuffix(t *testing.T) {
	t.Parallel()
	var rs Rows
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Open ticket", Status: "open", Commitless: true},
	}}

	line := rs.RenderTicketRow(testEnv(), Row{Epic: epic, Ticket: epic.Tickets[0]}, 0)
	if !strings.Contains(line, "Open ticket (commitless)") {
		t.Fatalf("title line = %q, want title followed by \" (commitless)\"", line)
	}
}

// TestRenderQueueTicketRow_IconColumnAlignsRegardlessOfChildren mirrors
// TestRenderTicketRow_IconColumnAlignsRegardlessOfChildren (ticket 10) for
// the Queue tab's own row renderer: same-depth siblings' icon columns must
// line up whether or not each one has children, since the triangle column is
// reserved at a fixed width for every row.
func TestRenderQueueTicketRow_IconColumnAlignsRegardlessOfChildren(t *testing.T) {
	t.Parallel()
	var rs Rows
	env := testEnv()
	icons := env.Header.Icons
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Parent ticket", Status: "open"},
		{Identifier: "02", Title: "Leaf ticket", Status: "open"},
	}}

	withChildren := rs.RenderTicketRow(env, Row{Epic: epic, Ticket: epic.Tickets[0], HasChildren: true, Expanded: true}, 0)
	childless := rs.RenderTicketRow(env, Row{Epic: epic, Ticket: epic.Tickets[1]}, 1)

	iconOffset := func(line string) int {
		stripped := ansi.Strip(line)
		return lipgloss.Width(stripped[:strings.Index(stripped, icons.TicketOpen)])
	}
	if got, want := iconOffset(withChildren), iconOffset(childless); got != want {
		t.Fatalf("withChildren ticket's icon column = %d, want %d (same as childless sibling)\nchildless: %q\nwithChildren: %q", got, want, childless, withChildren)
	}
	if strings.Contains(ansi.Strip(childless), icons.TriangleExpanded) || strings.Contains(ansi.Strip(childless), icons.TriangleCollapsed) {
		t.Fatalf("childless row unexpectedly contains a triangle glyph: %q", childless)
	}
}

func leadingWhitespace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " "))]
}

// TestRenderQueueTicketRow_LiveRowIndentMatchesNormalRow covers ticket 10:
// rowfmt.RenderLiveTicketRow already prefixes its output with 2 spaces, so the
// caller must not add its own on top or a running row ends up indented 4
// spaces instead of 2.
func TestRenderQueueTicketRow_LiveRowIndentMatchesNormalRow(t *testing.T) {
	t.Parallel()
	var rs Rows
	env := testEnv()
	env.Header.RunningEpics = map[string]bool{"epic": true}
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Normal ticket", Status: "open"},
		{Identifier: "02", Title: "Running ticket", Status: "claimed"},
	}}
	env.Header.Live = map[string]map[string]rowfmt.LiveTicketState{
		"epic": {"02": {Running: true, Label: "iter-01"}},
	}

	normalLine := rs.RenderTicketRow(env, Row{Epic: epic, Ticket: epic.Tickets[0]}, 0)
	liveLine := rs.RenderTicketRow(env, Row{Epic: epic, Ticket: epic.Tickets[1]}, 1)

	normalIndent := leadingWhitespace(ansi.Strip(normalLine))
	liveIndent := leadingWhitespace(ansi.Strip(liveLine))
	if liveIndent != normalIndent {
		t.Fatalf("live row indent = %q, want %q (matching normal row): live=%q normal=%q", liveIndent, normalIndent, liveLine, normalLine)
	}
}

func TestRenderQueueTicketRow_DoneMetricsLineMatchesTitleColor(t *testing.T) {
	t.Parallel()
	var rs Rows
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 5, ActualContextWindow: 100},
	}}

	line := rs.RenderTicketRow(testEnv(), Row{Epic: epic, Ticket: epic.Tickets[0]}, 0)
	wantSuffix := " " + rowfmt.StatusDoneStyle.Italic(true).Render(rowfmt.FormatMetricsLine(5, 100, 0, false))
	if !strings.HasSuffix(line, wantSuffix) {
		t.Fatalf("row line = %q, want it to end with %q", line, wantSuffix)
	}
}

// TestRenderQueueTicketRow_NonDoneWithMetricsShowsLine covers ticket 03: a
// paused or claimed ticket that already carries
// non-zero elapsed_time/actual_context_window must show it, not just done
// tickets.
func TestRenderQueueTicketRow_NonDoneWithMetricsShowsLine(t *testing.T) {
	t.Parallel()
	var rs Rows
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Paused ticket", Status: string(schema.StatusPaused), PauseReason: schema.PauseBlockedPane, ElapsedTime: 5, ActualContextWindow: 100},
	}}

	line := rs.RenderTicketRow(testEnv(), Row{Epic: epic, Ticket: epic.Tickets[0]}, 0)
	if !strings.Contains(line, rowfmt.FormatMetricsLine(5, 100, 0, false)) {
		t.Fatalf("row line = %q, want it to contain metrics %q", line, rowfmt.FormatMetricsLine(5, 100, 0, false))
	}
}

// TestRenderQueueTicketRow_NonDoneWithZeroMetricsOmitsLine covers ticket 03:
// a non-done ticket with nothing recorded yet still omits the metrics line.
func TestRenderQueueTicketRow_NonDoneWithZeroMetricsOmitsLine(t *testing.T) {
	t.Parallel()
	var rs Rows
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Paused ticket", Status: string(schema.StatusPaused), PauseReason: schema.PauseSpinning},
	}}

	line := rs.RenderTicketRow(testEnv(), Row{Epic: epic, Ticket: epic.Tickets[0]}, 0)
	if strings.Contains(line, "tok") {
		t.Fatalf("row line = %q, want no metrics suffix", line)
	}
}

// TestRenderQueueTicketRow_SearchDimWinsOverNonCandidateDim covers ticket 13:
// a row that's both an injected non-candidate ancestor (ticket 08) and
// search-dimmed (an active query it doesn't match) must render its icon,
// title, badge, and suffix all in the same dim style — previously the title
// resolved nonCandidateDim first while the other elements resolved searchDim
// first, producing a two-tone row.
func TestRenderQueueTicketRow_SearchDimWinsOverNonCandidateDim(t *testing.T) {
	t.Parallel()
	var rs Rows
	env := testEnv()
	env.HasQuery = true
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Ancestor ticket", Status: "open"},
	}}
	rs.Tree.Search().SetPassiveResults("no-match", nil)

	row := Row{Epic: epic, Ticket: epic.Tickets[0], Actionable: false}
	line := rs.RenderTicketRow(env, row, 0)
	stripped := ansi.Strip(line)

	title := epic.Tickets[0].DisplayNumber() + " " + epic.Tickets[0].Title
	wantTitle := ui.StyleDim.Render(title)
	if !strings.Contains(line, wantTitle) {
		t.Fatalf("row line = %q, want title rendered in ui.StyleDim (matching icon/badge/suffix), got literal stripped text %q", line, stripped)
	}
	if strings.Contains(line, rowfmt.StatusDoneStyle.Render(title)) {
		t.Fatalf("row line = %q, title still rendered in statusDoneStyle instead of ui.StyleDim", line)
	}
}
