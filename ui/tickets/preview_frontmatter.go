package tickets

import (
	"strconv"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

// frontmatterLabelStyle renders a frontmatter field's prettified label at
// the top of a ticket preview (renderFrontmatterBlock), muted so it recedes
// behind the value next to it.
var frontmatterLabelStyle = ui.StyleMuted

// frontmatterLabelOverrides names frontmatter fields whose prettified label
// isn't just prettifyFieldName's default underscore-to-space, capitalize-
// first-word transform - e.g. "actual_context_window" drops "actual"
// entirely (both the actual and expected variants read the same in the
// preview, since only one is ever populated on a given ticket).
var frontmatterLabelOverrides = map[string]string{
	"actual_context_window": "Context window",
}

// prettifyFieldName turns an on-disk snake_case frontmatter key (see
// tickets/schema/frontmatter.go's ticketYAML) into a human-readable label:
// underscores become spaces and only the first word is capitalized (e.g.
// "blocked_by" -> "Blocked by"), matching frontmatterLabelOverrides' entries
// where the default transform doesn't already read naturally.
func prettifyFieldName(name string) string {
	if label, ok := frontmatterLabelOverrides[name]; ok {
		return label
	}
	label := strings.ReplaceAll(name, "_", " ")
	if label == "" {
		return label
	}
	return strings.ToUpper(label[:1]) + label[1:]
}

// frontmatterField is one "key: value" row of renderFrontmatterBlock's
// output; key is the on-disk snake_case name (prettified via
// prettifyFieldName at render time) and value is its already-formatted
// display string.
type frontmatterField struct {
	key   string
	value string
}

// ticketFrontmatterFields lists t's populated frontmatter fields in a fixed
// display order, skipping any that are empty/zero - a ticket predating a
// field (e.g. no compactions recorded yet) simply omits that row rather than
// showing a blank one.
func ticketFrontmatterFields(t tickets.Ticket, status tickets.RenderedStatus) []frontmatterField {
	var fields []frontmatterField
	add := func(key, value string) {
		if value == "" {
			return
		}
		fields = append(fields, frontmatterField{key: key, value: value})
	}

	add("status", status.Word())
	add("type", t.Type)
	add("blocked_by", strings.Join(t.BlockedBy, ", "))
	add("split", strings.Join(t.Split, ", "))
	if t.Parent != nil {
		add("split_from", *t.Parent)
	}
	if t.ActualContextWindow > 0 {
		add("actual_context_window", formatTokenCount(t.ActualContextWindow))
	}
	if t.ElapsedTime > 0 {
		add("elapsed_time", formatElapsed(t.ElapsedTime))
	}
	if t.Compactions > 0 {
		add("compactions", strconv.Itoa(t.Compactions))
	}
	if t.Commitless {
		add("commitless", "true")
	}
	return fields
}

// renderFrontmatterBlock renders a ticket's frontmatter as prettified
// "Label: value" lines, one per field - the preview's replacement for its
// old synthesized header+meta lines (see renderTicketPreview's doc comment).
func renderFrontmatterBlock(t tickets.Ticket, status tickets.RenderedStatus) string {
	fields := ticketFrontmatterFields(t, status)
	lines := make([]string, len(fields))
	for i, f := range fields {
		lines[i] = "  " + frontmatterLabelStyle.Render(prettifyFieldName(f.key)+":") + " " + f.value
	}
	return strings.Join(lines, "\n")
}
