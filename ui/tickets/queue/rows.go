package queue

import (
	"reflect"
	"slices"
	"strings"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/ralphloop/schedule"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tree"
)

// depth/hasChildren/expanded/parentPath mirror the Tickets tab's row (ticket
// 09, model_data.go) one level down for the Queue tab's own row model: a
// ticket nested under another ticket via Parent/Children (ticket 03).
// parentPath is "" for a top-level row, keyed by Ticket.Path (globally
// unique) rather than an epic.Tickets index since Row doesn't carry
// one.
type Row struct {
	Epic        tickets.Epic
	Ticket      tickets.Ticket
	Depth       int
	HasChildren bool
	Expanded    bool
	ParentPath  string
	// Actionable is false for a row injected by attachQueueAncestors to keep
	// a scheduling candidate nested under its real parent chain (ticket 08):
	// the ticket itself isn't a candidate, so renderQueueTicketRow dims it to
	// tell it apart from rows the user can actually act on.
	Actionable bool
}

// NodeKind distinguishes the five row kinds the Queue tab's tree can
// hold: a per-epic blank separator, its status/context-window header lines,
// an optional plan-error line, and a ticket. tree.Model[T] has no notion of a
// heterogeneous node — this sum type plus buildQueueEntries' idFn/childrenFn
// is what makes one tree.Model[queueNode] stand in for buildQueueLines'
// former hand-spliced header/blank/error strings interleaved with the ticket
// rows. A ticket's pause-reason subtext is not a node of its own — it's a
// second physical line on the ticket's own entry (queueTicketReasonLine, set
// as Entry.Body in buildQueueEntries).
type NodeKind int

const (
	NodeEpicSeparator  NodeKind = iota // blank line before every epic
	NodeEpicName                       // selectable epic row: name, plus "(n/m)" and bar in the wide layout
	NodeEpicBar                        // narrow layouts only: bar (and/or "(n/m)") line under the name
	NodeEpicStatusLine                 // "Status: <icon> <state> · n compacts"
	NodeEpicSpacer                     // blank line between the header and the tickets
	NodeEpicError                      // present only when planErrs[epic.Name] != nil
	NodeTicket                         // wraps a queueRow
)

// Node is ui/tree.Model[Node]'s value type. err is set only for
// nodeEpicError, ticket only for nodeQueueTicket, layout for the epic's
// header rows.
type Node struct {
	Kind   NodeKind
	Epic   tickets.Epic
	Err    error
	Ticket Row
	Layout EpicHeaderLayout
}

// entriesCache memoizes buildQueueEntries' output (see QueueModel.
// entriesCache) against the four inputs that actually change its structure.
// Anything read live at draw time instead (running-epic elapsed seconds,
// spinner frame, paused/stalled state — queueRenderOpts' Label callback) is
// excluded on purpose: it doesn't affect which entries exist, only how a
// cached entry's row text renders, so it can't go stale from reusing this
// cache.
type entriesCache struct {
	epics        []tickets.Epic
	summaries    map[string]wire.EpicSummary
	checked      map[string]bool
	hideComplete bool
	project      string
	collapsed    map[string]bool
	labelWidth   int
	entries      []tree.Entry[Node]
}

// Rows is the Queue tab's tree builder and row renderer, with the state
// they own. It takes everything else through queueRowsEnv and never reads
// QueueModel.
type Rows struct {
	// Tree owns the selection/scroll/collapse state.
	Tree tree.Model[Node]
	// entriesCache memoizes buildQueueEntries' output across renders. It's a
	// pointer so the cache survives QueueModel being copied by value on every
	// Update. buildQueueEntries only depends on epics/checked/hideComplete/
	// collapsed-IDs (see queueEntriesCache); everything else live (running-epic
	// elapsed time, spinner frame, paused/stalled state) is read by
	// queueRenderOpts' Label callback at draw time, not baked into the cached
	// entries, so reusing a cached tree when those inputs didn't change is safe
	// even mid-run.
	entriesCache *entriesCache
	Epics        []tickets.Epic
	Checked      map[string]bool
	CheckOrder   map[string]uint64
	Candidates   map[string]bool
	// HideComplete backs the "tc" chord: when true, buildQueueEntries omits done
	// tickets from the rendered rows, independent of the clear keymaps (which
	// mutate the queue store) and of epicWaves' plan validation, which must
	// keep considering hidden-but-still-queued tickets.
	HideComplete bool
	// ProjectFilter narrows the server-mode rows to one project; "" shows all.
	ProjectFilter string
}

func NewRows(queueTree tree.Model[Node]) Rows {
	return Rows{
		Tree:         queueTree,
		entriesCache: &entriesCache{},
		Checked:      map[string]bool{},
		CheckOrder:   map[string]uint64{},
	}
}

// RowsEnv is what queueRows reads from the rest of the tab.
type RowsEnv struct {
	Header        Header
	MaxParallel   int
	LabelWidth    int
	HarnessLabels map[string]string
	WaitingLabels map[string]string
	CodexRows     map[string]bool
	HasQuery      bool
	SidebarActive bool
	Suggested     func(epic tickets.Epic, t tickets.Ticket) bool
}

// BuildEntriesCached returns buildQueueEntries' result, reusing the
// previous build when none of its inputs changed since the last render.
// View() (queue_view.go) must call this on every single render, and the
// spinner re-renders it constantly — so without this cache a running Queue tab
// redoes the full tree rebuild (icons, labels, per-ticket wave/order
// computation) every frame.
func (r Rows) BuildEntriesCached(env RowsEnv) []tree.Entry[Node] {
	if r.entriesCache == nil {
		return r.BuildEntries(env)
	}
	collapsed := r.Tree.CollapsedIDs()
	c := r.entriesCache
	if c.entries != nil &&
		c.hideComplete == r.HideComplete &&
		c.project == r.ProjectFilter &&
		c.labelWidth == env.LabelWidth &&
		reflect.DeepEqual(c.epics, r.Epics) &&
		reflect.DeepEqual(c.summaries, env.Header.Summaries) &&
		reflect.DeepEqual(c.checked, r.Checked) &&
		reflect.DeepEqual(c.collapsed, collapsed) {
		return c.entries
	}
	entries := r.BuildEntries(env)
	c.epics = r.Epics
	c.summaries = env.Header.Summaries
	c.checked = r.Checked
	c.hideComplete = r.HideComplete
	c.project = r.ProjectFilter
	c.labelWidth = env.LabelWidth
	c.collapsed = collapsed
	c.entries = entries
	return entries
}

// SelectFirstRow/selectLastRow implement "gg"/"G": jump the queue selection
// to the first/last row.
func (r *Rows) SelectFirstRow() {
	if len(r.Tree.Entries()) == 0 {
		return
	}
	r.Tree.SetSelectedIndex(0)
	r.Tree.SkipUnselectable(1)
}

func (r *Rows) SelectLastRow() {
	n := len(r.Tree.Entries())
	if n == 0 {
		return
	}
	r.Tree.SetSelectedIndex(n - 1)
	r.Tree.SkipUnselectable(-1)
}

// BuildEntries flattens m.epics into the Queue tab's full tree.Entry
// list: each epic with at least one candidate ticket contributes a blank
// separator, its status/context-window header lines, an optional plan-error
// line, and its own candidate tickets nested by Parent/Children — mirroring
// buildSidebarEntries (model_data.go) one level down. Reuses
// epicWaves/epicRowOrder/filterDoneTickets/attachQueueAncestors to
// compute each epic's plan-ordered candidate tickets, then wraps them into
// queueNode tree entries. Called on every
// queueEpicsLoadedMsg/hideComplete toggle/collapse mutation via
// clampSelected.
func (r Rows) BuildEntries(env RowsEnv) []tree.Entry[Node] {
	candidates := r.Candidates
	if candidates == nil {
		candidates = make(map[string]bool, len(r.Checked))
	}
	for path := range r.Checked {
		candidates[path] = true
	}

	var roots []Node
	childrenOf := map[string][]Node{}

	type shownEpic struct {
		epic    tickets.Epic
		ordered []tickets.Ticket
		planErr error
	}
	var shown []shownEpic
	var shownEpics []tickets.Epic
	for _, epic := range r.Epics {
		if r.ProjectFilter != "" && projectOf(epic.Name) != r.ProjectFilter {
			continue
		}
		waves, planErr := epicWaves(epic, candidates, env.MaxParallel)
		ordered := epicRowOrder(epic, waves, candidates)
		if r.HideComplete {
			ordered = filterDoneTickets(epic, ordered)
		}
		if len(ordered) == 0 {
			continue
		}
		shown = append(shown, shownEpic{epic, ordered, planErr})
		shownEpics = append(shownEpics, epic)
	}
	// Decided here, not at draw time, because the bar line is a real entry
	// whose presence changes the tree's height; the cache keys on the width.
	layout := env.Header.EpicHeaderLayoutFor(shownEpics, env.LabelWidth)

	for _, s := range shown {
		epic, ordered, planErr := s.epic, s.ordered, s.planErr

		roots = append(roots, Node{Kind: NodeEpicSeparator, Epic: epic})
		roots = append(roots, Node{Kind: NodeEpicName, Epic: epic, Layout: layout})
		if layout != EpicLayoutWide && env.Header.Summaries[epic.Name].Total > 0 {
			roots = append(roots, Node{Kind: NodeEpicBar, Epic: epic, Layout: layout})
		}
		roots = append(roots, Node{Kind: NodeEpicStatusLine, Epic: epic})
		roots = append(roots, Node{Kind: NodeEpicSpacer, Epic: epic})
		if planErr != nil {
			roots = append(roots, Node{Kind: NodeEpicError, Epic: epic, Err: planErr})
		}

		visible := make(map[string]bool, len(ordered))
		for _, t := range ordered {
			visible[t.Path] = true
		}
		byIdentifier := make(map[string]tickets.Ticket, len(epic.Tickets))
		for _, t := range epic.Tickets {
			if t.Identifier != "" {
				byIdentifier[t.Identifier] = t
			}
		}
		injected := map[string]bool{}
		for _, t := range ordered {
			row := Row{Epic: epic, Ticket: t, Actionable: true}
			node := Node{Kind: NodeTicket, Epic: epic, Ticket: row}
			parentPath := attachQueueAncestors(t, epic, visible, injected, byIdentifier, childrenOf, &roots)
			if parentPath != "" {
				childrenOf[parentPath] = append(childrenOf[parentPath], node)
			} else {
				roots = append(roots, node)
			}
		}
	}

	// tree.BuildEntriesFromValues computes each Entry's own HasChildren/Expanded
	// from childrenFn/collapsed, but that's only visible on the returned
	// tree.Entry — nothing feeds it back into queueNode.ticket, which
	// renderQueueTicketRow reads its fold triangle from (mirroring the
	// sidebar's rowFromEntry, model_data.go, one level down). Populate it here,
	// now that childrenOf is complete for every epic.
	collapsed := r.Tree.CollapsedIDs()
	setFoldState := func(n *Node) {
		if n.Kind != NodeTicket {
			return
		}
		n.Ticket.HasChildren = len(childrenOf[n.Ticket.Ticket.Path]) > 0
		n.Ticket.Expanded = n.Ticket.HasChildren && !collapsed[n.Ticket.Ticket.Path]
	}
	for i := range roots {
		setFoldState(&roots[i])
	}
	for path := range childrenOf {
		for i := range childrenOf[path] {
			setFoldState(&childrenOf[path][i])
		}
	}

	idFn := func(n Node) string {
		switch n.Kind {
		case NodeEpicSeparator:
			return "epic:" + n.Epic.Name + ":sep"
		case NodeEpicName:
			return "epic:" + n.Epic.Name + ":status"
		case NodeEpicBar:
			return "epic:" + n.Epic.Name + ":bar"
		case NodeEpicStatusLine:
			return "epic:" + n.Epic.Name + ":statusline"
		case NodeEpicSpacer:
			return "epic:" + n.Epic.Name + ":spacer"
		case NodeEpicError:
			return "epic:" + n.Epic.Name + ":err"
		default:
			return n.Ticket.Ticket.Path
		}
	}
	childrenFn := func(n Node) []Node {
		if n.Kind != NodeTicket {
			return nil
		}
		return childrenOf[n.Ticket.Ticket.Path]
	}

	entries := tree.BuildEntriesFromValues(roots, idFn, childrenFn, collapsed)
	for i := range entries {
		if entries[i].Value.Kind != NodeTicket {
			continue
		}
		if line, ok := r.queueTicketReasonLine(env, entries[i].Value.Ticket); ok {
			entries[i].Body = []string{line}
		}
	}
	return entries
}

// projectOf is the project part of a "project:epic" or "project:epic/id" name.
func projectOf(name string) string {
	project, _, _ := strings.Cut(name, ":")
	return project
}

// NextProject steps the filter through "" (all) and each project in epics, in
// name order.
func NextProject(epics []tickets.Epic, current string) string {
	var projects []string
	for _, e := range epics {
		projects = append(projects, projectOf(e.Name))
	}
	slices.Sort(projects)
	projects = slices.Compact(projects)
	if current == "" {
		if len(projects) == 0 {
			return ""
		}
		return projects[0]
	}
	if i := slices.Index(projects, current); i >= 0 && i+1 < len(projects) {
		return projects[i+1]
	}
	return ""
}

// filterDoneTickets drops a done ticket from ordered before nesting
// (buildQueueEntries) rather than after, so a done parent doesn't strand its
// children — nearestVisibleQueueAncestor reattaches them to the nearest
// surviving ancestor instead of losing them.
func filterDoneTickets(epic tickets.Epic, ordered []tickets.Ticket) []tickets.Ticket {
	filtered := make([]tickets.Ticket, 0, len(ordered))
	for _, t := range ordered {
		if rowfmt.TicketStatus(epic, t).Terminal() {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

// attachQueueAncestors walks t's Parent chain (ticket 03's schema field)
// looking for the nearest ancestor already in visible, mirroring the Tickets
// tab's nearestVisibleAncestor (ticket 09, model_data.go) one level down —
// keyed by Ticket.Path/Identifier rather than an epic.Tickets index since
// queueRow doesn't carry one. Unlike a plain lookup, any ancestor found along
// the way that ISN'T a scheduling candidate gets injected into roots/
// childrenOf as a dimmed (actionable: false) row of its own instead of being
// skipped (ticket 08) — otherwise t would get promoted to root and the
// parent-child connection the tree exists to show would be lost. injected
// dedupes across tickets sharing the same non-candidate ancestor within an
// epic, and visible is extended in place so a later call sees an ancestor
// this call just injected as already visible. Returns "" once the chain runs
// out, hits a Parent token with no matching ticket in the epic, or would loop
// (guarded via seen) — same fallback as before: t is promoted to root.
func attachQueueAncestors(t tickets.Ticket, epic tickets.Epic, visible map[string]bool, injected map[string]bool, byIdentifier map[string]tickets.Ticket, childrenOf map[string][]Node, roots *[]Node) string {
	seen := map[string]bool{t.Path: true}
	var chain []tickets.Ticket // non-candidate ancestors to inject, nearest first
	cur := t
	attachPoint := ""
	for cur.Parent != nil {
		parent, ok := byIdentifier[*cur.Parent]
		if !ok || seen[parent.Path] {
			break
		}
		if visible[parent.Path] {
			attachPoint = parent.Path
			break
		}
		chain = append(chain, parent)
		seen[parent.Path] = true
		cur = parent
	}

	// Inject farthest-first so each new dimmed row's own parent is already
	// attached (either an existing visible ancestor or an injected one from
	// an earlier iteration of this same loop) before it's linked in.
	parentPath := attachPoint
	for _, ancestor := range slices.Backward(chain) {
		if !injected[ancestor.Path] {
			node := Node{
				Kind:   NodeTicket,
				Epic:   epic,
				Ticket: Row{Epic: epic, Ticket: ancestor, Actionable: false},
			}
			if parentPath != "" {
				childrenOf[parentPath] = append(childrenOf[parentPath], node)
			} else {
				*roots = append(*roots, node)
			}
			injected[ancestor.Path] = true
			visible[ancestor.Path] = true
		}
		parentPath = ancestor.Path
	}
	return parentPath
}

// epicRowOrder flattens waves (blockers before dependents, ties in ticket
// number order per PlanWaves) into the epic's Queue row order. When waves is
// nil — epicWaves errored, e.g. a dependency cycle — it falls back to plain
// ticket-number order so the epic's candidates still render instead of
// vanishing from the list.
func epicRowOrder(epic tickets.Epic, waves [][]tickets.Ticket, candidates map[string]bool) []tickets.Ticket {
	if waves == nil {
		var fallback []tickets.Ticket
		for _, idx := range rowfmt.SortedTicketIndexes(epic) {
			t := epic.Tickets[idx]
			if candidates[t.Path] {
				fallback = append(fallback, t)
			}
		}
		return fallback
	}
	var out []tickets.Ticket
	for _, wave := range waves {
		out = append(out, wave...)
	}
	return out
}

// epicWaves resolves candidates (the checked-ticket paths within epic) into a
// schedule.RunScope and hands off to schedule.PlanWaves, the same planner
// Run uses to claim tickets — see buildQueueEntries.
func epicWaves(epic tickets.Epic, candidates map[string]bool, maxParallel int) ([][]tickets.Ticket, error) {
	var ids []string
	for _, idx := range rowfmt.SortedTicketIndexes(epic) {
		t := epic.Tickets[idx]
		if candidates[t.Path] {
			ids = append(ids, t.DisplayNumber())
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	scope, err := schedule.ResolveRunScope(epic, ids)
	if err != nil {
		return nil, err
	}
	return schedule.PlanWaves(epic, scope, maxParallel)
}

// SelectFocusEpic selects the header of the epic at address (its Path) once the
// rows hold it, and returns the address still waiting: "" when done, address
// itself when the next delivery has to try again.
func (r *Rows) SelectFocusEpic(address string) string {
	if address == "" {
		return ""
	}
	for i, e := range r.Tree.Entries() {
		if e.Value.Kind == NodeEpicName && e.Value.Epic.Path == address {
			r.Tree.SetSelectedIndex(i)
			return ""
		}
	}
	return address
}
