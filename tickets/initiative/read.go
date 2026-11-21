package initiative

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/tickets"
)

// Derived Milestone states. Nothing stores them: Read works them out from the
// epics each time.
const (
	MilestoneSketch       = "sketch"
	MilestoneDesigning    = "designing"
	MilestoneImplementing = "implementing"
	MilestoneMerged       = "merged"
	MilestoneGated        = "gated"
	// MilestoneBroken is a Milestone whose Design epic is missing from the
	// store and that has no gate note.
	MilestoneBroken = "broken"
)

// Initiative states.
const (
	StateActive   = "active"
	StateStopped  = "stopped"
	StateFinished = "finished"
)

// Kinds of Problem.
const (
	ProblemUnparsedMilestone    = "unparsed-milestone"
	ProblemMissingDesign        = "missing-design"
	ProblemMismatchedDesign     = "mismatched-design"
	ProblemUnknownMilestone     = "unknown-milestone"
	ProblemUnresolvedInitiative = "unresolved-initiative"
)

// MilestoneView is one parsed Milestone plus what the epics say about it. A
// sketch or broken Milestone has no ImplEpic; a broken one keeps Design as
// plain text only.
type MilestoneView struct {
	Slug, Name, Goal string
	Number           int // 1-based, in list order
	Design           string
	ImplEpic         string // "<design>-impl", empty without a live design link
	State            string
	Broken           bool
}

// NamesSlug reports whether the Milestone's slug is named elsewhere, by a
// Design link or a gate note. Such a slug can no longer change.
func (m MilestoneView) NamesSlug() bool { return m.Design != "" || m.State == MilestoneGated }

// UnparsedLine is a line that did not parse, kept verbatim for display.
type UnparsedLine struct {
	Section string
	Raw     string
}

// Problem is one link problem; Message names it.
type Problem struct {
	Kind      string
	Milestone string
	Design    string
	Message   string
}

// View is everything derived from one Initiative epic.
type View struct {
	Slug       string
	Milestones []MilestoneView
	Unparsed   []UnparsedLine
	State      string
	Done       int    // gated Milestones
	Total      int    // parsed Milestones
	Current    string // slug of the first Milestone not gated, "" when all are
	Problems   []Problem
	// Orphans are map epics whose initiative: and milestone: match a
	// Milestone that links no Design.
	Orphans []string
}

// Read derives the View of an Initiative epic. live and archived are the
// project's epics; an archived epic counts as existing. It does no IO.
func Read(init tickets.Epic, live, archived []tickets.Epic) View {
	body := ParseBody(init.Body)
	v := View{Slug: init.Name, State: StateActive}
	find := func(name string) (tickets.Epic, bool) { return findEpic(name, live, archived) }

	gated, lastOutcome := gateInfo(body.GateNotes)
	v.collectUnparsed(body)

	for _, e := range body.Milestones {
		if e.Unparsed() {
			continue
		}
		m := e.Value
		mv := MilestoneView{Slug: m.Slug, Name: m.Name, Goal: m.Goal, Number: len(v.Milestones) + 1, Design: m.Design, State: MilestoneSketch}
		if m.Design != "" {
			if _, ok := find(m.Design); !ok {
				mv.Broken = true
				mv.State = MilestoneBroken
				v.Problems = append(v.Problems, problemMissingDesign(m.Slug, m.Design))
			} else {
				mv.ImplEpic = m.Design + tickets.ImplEpicSuffix
				mv.State = MilestoneDesigning
				if impl, ok := find(mv.ImplEpic); ok {
					mv.State = MilestoneImplementing
					if !impl.MergedAt.IsZero() {
						mv.State = MilestoneMerged
					}
				}
			}
		}
		if gated[m.Slug] {
			mv.State = MilestoneGated
			v.Done++
		}
		v.Milestones = append(v.Milestones, mv)
	}
	v.Total = len(v.Milestones)
	for _, m := range v.Milestones {
		if m.State != MilestoneGated {
			v.Current = m.Slug
			break
		}
	}
	switch lastOutcome {
	case OutcomeStop:
		v.State = StateStopped
	case OutcomeDone:
		v.State = StateFinished
	}

	for _, e := range body.Milestones {
		if e.Unparsed() {
			v.Problems = append(v.Problems, Problem{
				Kind:    ProblemUnparsedMilestone,
				Message: fmt.Sprintf("Milestones line does not parse and hides its entry: %q", e.Raw),
			})
		}
	}
	v.checkMaps(live, archived)
	return v
}

func (v *View) collectUnparsed(b Body) {
	add := func(section string, raw string) { v.Unparsed = append(v.Unparsed, UnparsedLine{section, raw}) }
	for _, e := range b.Milestones {
		if e.Unparsed() {
			add(Milestones, e.Raw)
		}
	}
	for _, e := range b.Assumptions {
		if e.Unparsed() {
			add(Assumptions, e.Raw)
		}
	}
	for _, e := range b.Deferred {
		if e.Unparsed() {
			add(Deferred, e.Raw)
		}
	}
	for _, e := range b.GateNotes {
		if e.Unparsed() {
			add(GateNotes, e.Raw)
		}
	}
}

// checkMaps finds the maps pointing at this Initiative: orphans, unknown
// Milestones and Designs linked from a different Milestone.
func (v *View) checkMaps(live, archived []tickets.Epic) {
	byslug := map[string]MilestoneView{}
	for _, m := range v.Milestones {
		byslug[m.Slug] = m
	}
	for _, e := range append(append([]tickets.Epic{}, live...), archived...) {
		if !e.IsMap || e.Initiative != v.Slug {
			continue
		}
		m, ok := byslug[e.Milestone]
		switch {
		case !ok:
			v.Problems = append(v.Problems, Problem{
				Kind: ProblemUnknownMilestone, Milestone: e.Milestone, Design: e.Name,
				Message: fmt.Sprintf("Design %s names Milestone %q, which is not in the Milestones list", e.Name, e.Milestone),
			})
		case m.Design == "":
			v.Orphans = append(v.Orphans, e.Name)
		case m.Design != e.Name:
			v.Problems = append(v.Problems, Problem{
				Kind: ProblemMismatchedDesign, Milestone: m.Slug, Design: e.Name,
				Message: fmt.Sprintf("Design %s names Milestone %q, which links Design %s", e.Name, m.Slug, m.Design),
			})
		}
	}
}

func problemMissingDesign(milestone, design string) Problem {
	return Problem{
		Kind: ProblemMissingDesign, Milestone: milestone, Design: design,
		Message: fmt.Sprintf("Milestone %q links Design %q, which is missing from the store", milestone, design),
	}
}

// UnresolvedInitiatives reports each map whose initiative: names no
// Initiative epic among the given epics (live and archived together).
func UnresolvedInitiatives(all []tickets.Epic) []Problem {
	have := map[string]bool{}
	for _, e := range all {
		if e.IsInitiative {
			have[e.Name] = true
		}
	}
	var out []Problem
	for _, e := range all {
		if e.IsMap && e.Initiative != "" && !have[e.Initiative] {
			out = append(out, Problem{
				Kind: ProblemUnresolvedInitiative, Milestone: e.Milestone, Design: e.Name,
				Message: fmt.Sprintf("Design %s names Initiative %q, which does not exist", e.Name, e.Initiative),
			})
		}
	}
	return out
}

// BackLink is the result of looking up the Milestone a Design, or its -impl
// epic, belongs to. Problem is set when the epic claims a link that does not
// hold; an epic with no initiative: at all has neither.
type BackLink struct {
	Milestone  int // 1-based; 0 means no back-link
	Initiative string
	Problem    *Problem
}

// BackLinkFor returns the back-link of a Design (a map with initiative: and
// milestone:) or of its <design>-impl epic. all holds live and archived epics
// together.
func BackLinkFor(e tickets.Epic, all []tickets.Epic) BackLink {
	design := e
	if !e.IsMap && strings.HasSuffix(e.Name, tickets.ImplEpicSuffix) {
		name := strings.TrimSuffix(e.Name, tickets.ImplEpicSuffix)
		d, ok := findEpic(name, all, nil)
		if !ok {
			return BackLink{}
		}
		design = d
	}
	if !design.IsMap || design.Initiative == "" {
		return BackLink{}
	}
	fail := func(kind, format string, args ...any) BackLink {
		return BackLink{Problem: &Problem{Kind: kind, Milestone: design.Milestone, Design: design.Name, Message: fmt.Sprintf(format, args...)}}
	}
	init, ok := findEpic(design.Initiative, all, nil)
	if !ok || !init.IsInitiative {
		return fail(ProblemUnresolvedInitiative, "Design %s names Initiative %q, which does not exist", design.Name, design.Initiative)
	}
	body := ParseBody(init.Body)
	n := 0
	for _, entry := range body.Milestones {
		if entry.Unparsed() {
			continue
		}
		n++
		if entry.Value.Slug != design.Milestone {
			continue
		}
		if entry.Value.Design != design.Name {
			return fail(ProblemMismatchedDesign, "Design %s names Milestone %q, which links Design %q", design.Name, design.Milestone, entry.Value.Design)
		}
		return BackLink{Milestone: n, Initiative: init.Name}
	}
	for _, entry := range body.Milestones {
		if entry.Unparsed() {
			return fail(ProblemUnparsedMilestone, "Milestone %q is not found and a Milestones line does not parse and may hide it", design.Milestone)
		}
	}
	return fail(ProblemUnknownMilestone, "Design %s names Milestone %q, which is not in the Milestones list", design.Name, design.Milestone)
}

// gateInfo returns the slugs that have a gate note and the outcome of the last
// note.
func gateInfo(lines []Entry[GateLine]) (map[string]bool, string) {
	gated := map[string]bool{}
	var last, current string
	for _, e := range lines {
		if e.Unparsed() {
			continue
		}
		switch e.Value.Kind {
		case GateHeading:
			current = e.Value.Slug
			gated[current] = true
			last = ""
		case GateOutcome:
			if current != "" {
				last = e.Value.Outcome
			}
		}
	}
	return gated, last
}

func findEpic(name string, sets ...[]tickets.Epic) (tickets.Epic, bool) {
	for _, set := range sets {
		for _, e := range set {
			if e.Name == name {
				return e, true
			}
		}
	}
	return tickets.Epic{}, false
}
