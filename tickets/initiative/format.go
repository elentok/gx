// Package initiative is the only definition of the Initiative body formats:
// the Milestones, Assumptions, Deferred and Gate notes sections of an
// Initiative epic. It works on strings and does no file IO; callers read and
// write the body through the epic section writers.
//
// A line that does not match its format parses as an unparsed Entry holding
// the exact original text, and renders back verbatim in the same place, so a
// hand-edited line survives any change to its neighbours.
package initiative

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

// Section headings of an Initiative body, in body order.
const (
	InitialPrompt = "Initial prompt"
	Destination   = "Destination"
	Milestones    = "Milestones"
	Assumptions   = "Assumptions"
	Deferred      = "Deferred"
	GateNotes     = "Gate notes"
)

// Assumption states.
const (
	StateUnknown   = "unknown"
	StateConfirmed = "confirmed"
	StateBroken    = "broken"
)

// Deferred states.
const (
	DeferredOpen    = "open"
	DeferredTaken   = "taken"
	DeferredDropped = "dropped"
)

// Gate outcomes.
const (
	OutcomeContinue = "continue"
	OutcomeReshape  = "reshape"
	OutcomeStop     = "stop"
	OutcomeDone     = "done"
)

// Unplaced is the Deferred `to:` value for an item no Milestone owns yet.
const Unplaced = "unplaced"

const sep = " | "

var (
	slugRe     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	assumeIDRe = regexp.MustCompile(`^A([1-9][0-9]*)$`)

	assumptionStates = []string{StateUnknown, StateConfirmed, StateBroken}
	deferredStates   = []string{DeferredOpen, DeferredTaken, DeferredDropped}
	outcomes         = []string{OutcomeContinue, OutcomeReshape, OutcomeStop, OutcomeDone}
)

// Entry is one non-blank line of a section: a typed Value, or, when Raw is
// set, the exact original text of a line that did not match its format.
type Entry[T any] struct {
	Value T
	Raw   string
}

// Unparsed reports whether the entry holds a line that did not parse.
func (e Entry[T]) Unparsed() bool { return e.Raw != "" }

// Parsed wraps v as a typed entry.
func Parsed[T any](v T) Entry[T] { return Entry[T]{Value: v} }

// Milestone is one line of ## Milestones. Design is the slug of its Design
// once started, else empty.
type Milestone struct {
	Slug, Name, Goal, Design string
}

// Assumption is one line of ## Assumptions.
type Assumption struct {
	N        int
	Text     string
	TestedBy string
	State    string
	Evidence string // "-" when there is none
}

// NewAssumption returns a fresh unknown Assumption with no evidence.
func NewAssumption(n int, text, testedBy string) Assumption {
	return Assumption{N: n, Text: text, TestedBy: testedBy, State: StateUnknown, Evidence: "-"}
}

// DeferredItem is one line of ## Deferred.
type DeferredItem struct {
	Item, To, Why, From, State string
}

// GateKind says which part of a gate note a GateLine is.
type GateKind int

const (
	GateHeading    GateKind = iota // "### <slug>"
	GateOutcome                    // "outcome: <outcome>"
	GateAssumption                 // "A<n> | <state> | <evidence>"
	GateTaught                     // "taught: <text>"
	GatePlacement                  // "placed: <text>"
)

// GateLine is one typed line of ## Gate notes. A gate note is a GateHeading
// line followed by the other kinds until the next heading.
type GateLine struct {
	Kind     GateKind
	Slug     string // GateHeading
	Outcome  string // GateOutcome
	N        int    // GateAssumption
	State    string // GateAssumption
	Evidence string // GateAssumption
	Text     string // GateTaught, GatePlacement
}

// NextAssumptionNumber is one past the highest parsed A<n>, or 1 for none.
func NextAssumptionNumber(es []Entry[Assumption]) int {
	highest := 0
	for _, e := range es {
		if !e.Unparsed() {
			highest = max(highest, e.Value.N)
		}
	}
	return highest + 1
}

// ValidateOutcome refuses a Gate outcome that is not one of the known four.
func ValidateOutcome(outcome string) error { return checkEnum("outcome", outcome, outcomes) }

// AssumptionName is the "A<n>" an Assumption is addressed by.
func AssumptionName(n int) string { return "A" + strconv.Itoa(n) }

// FindDeferred returns the index of the parsed deferred item named item. It
// refuses an item that appears twice.
func FindDeferred(es []Entry[DeferredItem], item string) (int, error) {
	idx := -1
	for i, e := range es {
		if e.Unparsed() || e.Value.Item != item {
			continue
		}
		if idx >= 0 {
			return 0, fmt.Errorf("deferred item %q is ambiguous: it appears more than once", item)
		}
		idx = i
	}
	if idx < 0 {
		return 0, fmt.Errorf("no deferred item %q", item)
	}
	return idx, nil
}

// FindAssumption returns the index of the parsed Assumption addressed as name
// ("A<n>").
func FindAssumption(es []Entry[Assumption], name string) (int, error) {
	for i, e := range es {
		if !e.Unparsed() && AssumptionName(e.Value.N) == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("no Assumption %q", name)
}

func checkField(name, v string) error {
	switch {
	case v == "":
		return fmt.Errorf("%s: must not be empty", name)
	case strings.ContainsAny(v, "|\n\r"):
		return fmt.Errorf("%s: must not contain | or a newline", name)
	case v != strings.TrimSpace(v):
		return fmt.Errorf("%s: must not start or end with whitespace", name)
	}
	return nil
}

func checkSlug(name, v string) error {
	if err := checkField(name, v); err != nil {
		return err
	}
	if !slugRe.MatchString(v) {
		return fmt.Errorf("%s: %q is not a valid slug", name, v)
	}
	return nil
}

func checkEnum(name, v string, allowed []string) error {
	if slices.Contains(allowed, v) {
		return nil
	}
	return fmt.Errorf("%s: %q is not one of %s", name, v, strings.Join(allowed, ", "))
}

func checkFields(fields ...func() error) error {
	for _, f := range fields {
		if err := f(); err != nil {
			return err
		}
	}
	return nil
}

func fieldCheck(name, v string) func() error { return func() error { return checkField(name, v) } }
func slugCheck(name, v string) func() error  { return func() error { return checkSlug(name, v) } }
func enumCheck(name, v string, allowed []string) func() error {
	return func() error { return checkEnum(name, v, allowed) }
}

// RenderMilestone returns the canonical line of m.
func RenderMilestone(m Milestone) (string, error) {
	checks := []func() error{slugCheck("slug", m.Slug), fieldCheck("name", m.Name), fieldCheck("goal", m.Goal)}
	if m.Design != "" {
		checks = append(checks, slugCheck("design", m.Design))
	}
	if err := checkFields(checks...); err != nil {
		return "", err
	}
	line := "- " + m.Slug + sep + m.Name + sep + "goal: " + m.Goal
	if m.Design != "" {
		line += sep + "design: " + m.Design
	}
	return line, nil
}

func parseMilestone(line string) (Milestone, bool) {
	parts, ok := dashParts(line)
	if !ok || len(parts) < 3 || len(parts) > 4 {
		return Milestone{}, false
	}
	goal, ok := strings.CutPrefix(parts[2], "goal: ")
	if !ok {
		return Milestone{}, false
	}
	m := Milestone{Slug: parts[0], Name: parts[1], Goal: goal}
	if len(parts) == 4 {
		if m.Design, ok = strings.CutPrefix(parts[3], "design: "); !ok {
			return Milestone{}, false
		}
	}
	return m, true
}

// RenderAssumption returns the canonical line of a.
func RenderAssumption(a Assumption) (string, error) {
	if a.N < 1 {
		return "", fmt.Errorf("number: must be at least 1")
	}
	if err := checkFields(fieldCheck("text", a.Text), slugCheck("tested by", a.TestedBy),
		enumCheck("state", a.State, assumptionStates), fieldCheck("evidence", a.Evidence)); err != nil {
		return "", err
	}
	return "- A" + strconv.Itoa(a.N) + sep + a.Text + sep + "tested by: " + a.TestedBy +
		sep + "state: " + a.State + sep + "evidence: " + a.Evidence, nil
}

func parseAssumption(line string) (Assumption, bool) {
	parts, ok := dashParts(line)
	if !ok || len(parts) != 5 {
		return Assumption{}, false
	}
	n, ok := parseAssumeID(parts[0])
	if !ok {
		return Assumption{}, false
	}
	v, ok := cutFields(parts[2:], "tested by: ", "state: ", "evidence: ")
	if !ok {
		return Assumption{}, false
	}
	return Assumption{N: n, Text: parts[1], TestedBy: v[0], State: v[1], Evidence: v[2]}, true
}

// parseAssumeID reads the n of "A<n>".
func parseAssumeID(s string) (int, bool) {
	id := assumeIDRe.FindStringSubmatch(s)
	if id == nil {
		return 0, false
	}
	n, err := strconv.Atoi(id[1])
	return n, err == nil
}

// cutFields strips each prefix from the part at the same position. It fails
// when a part lacks its prefix.
func cutFields(parts []string, prefixes ...string) ([]string, bool) {
	out := make([]string, len(prefixes))
	for i, prefix := range prefixes {
		v, ok := strings.CutPrefix(parts[i], prefix)
		if !ok {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

// RenderDeferred returns the canonical line of d.
func RenderDeferred(d DeferredItem) (string, error) {
	toCheck := slugCheck("to", d.To)
	if d.To == Unplaced {
		toCheck = fieldCheck("to", d.To)
	}
	if err := checkFields(fieldCheck("item", d.Item), toCheck, fieldCheck("why", d.Why),
		fieldCheck("from", d.From), enumCheck("state", d.State, deferredStates)); err != nil {
		return "", err
	}
	return "- " + d.Item + sep + "to: " + d.To + sep + "why: " + d.Why + sep + "from: " + d.From + sep + "state: " + d.State, nil
}

func parseDeferred(line string) (DeferredItem, bool) {
	parts, ok := dashParts(line)
	if !ok || len(parts) != 5 {
		return DeferredItem{}, false
	}
	v, ok := cutFields(parts[1:], "to: ", "why: ", "from: ", "state: ")
	if !ok {
		return DeferredItem{}, false
	}
	return DeferredItem{Item: parts[0], To: v[0], Why: v[1], From: v[2], State: v[3]}, true
}

// RenderGateLine returns the canonical line of g.
func RenderGateLine(g GateLine) (string, error) {
	switch g.Kind {
	case GateHeading:
		if err := checkSlug("slug", g.Slug); err != nil {
			return "", err
		}
		return "### " + g.Slug, nil
	case GateOutcome:
		if err := checkEnum("outcome", g.Outcome, outcomes); err != nil {
			return "", err
		}
		return "outcome: " + g.Outcome, nil
	case GateAssumption:
		if g.N < 1 {
			return "", fmt.Errorf("number: must be at least 1")
		}
		if err := checkFields(enumCheck("state", g.State, assumptionStates), fieldCheck("evidence", g.Evidence)); err != nil {
			return "", err
		}
		return "A" + strconv.Itoa(g.N) + sep + g.State + sep + g.Evidence, nil
	case GateTaught, GatePlacement:
		if err := checkField("text", g.Text); err != nil {
			return "", err
		}
		if g.Kind == GateTaught {
			return "taught: " + g.Text, nil
		}
		return "placed: " + g.Text, nil
	}
	return "", fmt.Errorf("kind: unknown gate line kind %d", g.Kind)
}

func parseGateLine(line string) (GateLine, bool) {
	if s, ok := strings.CutPrefix(line, "### "); ok {
		return GateLine{Kind: GateHeading, Slug: s}, true
	}
	if s, ok := strings.CutPrefix(line, "outcome: "); ok {
		return GateLine{Kind: GateOutcome, Outcome: s}, true
	}
	if s, ok := strings.CutPrefix(line, "taught: "); ok {
		return GateLine{Kind: GateTaught, Text: s}, true
	}
	if s, ok := strings.CutPrefix(line, "placed: "); ok {
		return GateLine{Kind: GatePlacement, Text: s}, true
	}
	parts := strings.Split(line, sep)
	if len(parts) != 3 {
		return GateLine{}, false
	}
	n, ok := parseAssumeID(parts[0])
	if !ok {
		return GateLine{}, false
	}
	return GateLine{Kind: GateAssumption, N: n, State: parts[1], Evidence: parts[2]}, true
}

// dashParts strips the "- " bullet and splits a line on " | ".
func dashParts(line string) ([]string, bool) {
	rest, ok := strings.CutPrefix(line, "- ")
	if !ok {
		return nil, false
	}
	return strings.Split(rest, sep), true
}

// parseLines types every non-blank line of text. A line becomes typed only
// when it parses and renders back to exactly itself.
func parseLines[T any](text string, parse func(string) (T, bool), render func(T) (string, error)) []Entry[T] {
	var out []Entry[T]
	for line := range strings.SplitSeq(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if v, ok := parse(line); ok {
			if got, err := render(v); err == nil && got == line {
				out = append(out, Parsed(v))
				continue
			}
		}
		out = append(out, Entry[T]{Raw: line})
	}
	return out
}

func renderLines[T any](es []Entry[T], render func(T) (string, error), blankBefore func(T) bool) (string, error) {
	var b strings.Builder
	for i, e := range es {
		line := e.Raw
		if !e.Unparsed() {
			var err error
			if line, err = render(e.Value); err != nil {
				return "", fmt.Errorf("line %d: %w", i+1, err)
			}
			if i > 0 && blankBefore != nil && blankBefore(e.Value) {
				b.WriteString("\n")
			}
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// RenderMilestones renders the text of ## Milestones.
func RenderMilestones(es []Entry[Milestone]) (string, error) {
	return renderLines(es, RenderMilestone, nil)
}

// RenderAssumptions renders the text of ## Assumptions, refusing a repeated A<n>.
func RenderAssumptions(es []Entry[Assumption]) (string, error) {
	seen := map[int]bool{}
	for _, e := range es {
		if e.Unparsed() {
			continue
		}
		if seen[e.Value.N] {
			return "", fmt.Errorf("number: A%d is used twice", e.Value.N)
		}
		seen[e.Value.N] = true
	}
	return renderLines(es, RenderAssumption, nil)
}

// RenderDeferredItems renders the text of ## Deferred.
func RenderDeferredItems(es []Entry[DeferredItem]) (string, error) {
	return renderLines(es, RenderDeferred, nil)
}

// RenderGateNotes renders the text of ## Gate notes, with a blank line before
// each gate note after the first.
func RenderGateNotes(es []Entry[GateLine]) (string, error) {
	return renderLines(es, RenderGateLine, func(g GateLine) bool { return g.Kind == GateHeading })
}

// Body is an Initiative epic body: the four structured sections parsed, the
// rest kept as the original text.
type Body struct {
	raw         string
	Milestones  []Entry[Milestone]
	Assumptions []Entry[Assumption]
	Deferred    []Entry[DeferredItem]
	GateNotes   []Entry[GateLine]
}

// ParseBody parses the structured sections of a whole epic body. A missing
// section parses as empty.
func ParseBody(body string) Body {
	return Body{
		raw:         body,
		Milestones:  parseLines(schema.Section(body, Milestones), parseMilestone, RenderMilestone),
		Assumptions: parseLines(schema.Section(body, Assumptions), parseAssumption, RenderAssumption),
		Deferred:    parseLines(schema.Section(body, Deferred), parseDeferred, RenderDeferred),
		GateNotes:   parseLines(schema.Section(body, GateNotes), parseGateLine, RenderGateLine),
	}
}

// SetDestination replaces the whole ## Destination section with text. The
// Destination is free prose with no line format, so no line needs protecting.
func (b *Body) SetDestination(text string) {
	b.raw = schema.SetSection(b.raw, Destination, text)
}

// Render writes the sections back into the original body. Other sections, and
// structured sections that did not change, are left as they were.
func (b Body) Render() (string, error) {
	milestones, err := RenderMilestones(b.Milestones)
	if err != nil {
		return "", fmt.Errorf("%s: %w", Milestones, err)
	}
	assumptions, err := RenderAssumptions(b.Assumptions)
	if err != nil {
		return "", fmt.Errorf("%s: %w", Assumptions, err)
	}
	deferred, err := RenderDeferredItems(b.Deferred)
	if err != nil {
		return "", fmt.Errorf("%s: %w", Deferred, err)
	}
	gates, err := RenderGateNotes(b.GateNotes)
	if err != nil {
		return "", fmt.Errorf("%s: %w", GateNotes, err)
	}
	out := b.raw
	for _, s := range []struct{ heading, text string }{
		{Milestones, milestones}, {Assumptions, assumptions}, {Deferred, deferred}, {GateNotes, gates},
	} {
		if s.text == schema.Section(out, s.heading) {
			continue
		}
		out = schema.SetSection(out, s.heading, s.text)
	}
	return out, nil
}
