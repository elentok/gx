package components

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Item is a single selectable entry in a Checklist.
type Item struct {
	Label   string // display text
	Value   string // value returned by Checked() (e.g. a file path)
	Checked bool
}

// Checklist is an interactive multi-select list with j/k navigation,
// space to toggle, and a to toggle all.
type Checklist struct {
	Items  []Item
	Cursor int
}

var (
	clChecked   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	clUnchecked = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	clCursor    = lipgloss.NewStyle().Bold(true)
)

// NewChecklist creates a checklist with all items checked by default.
func NewChecklist(items []Item) Checklist {
	return Checklist{Items: items}
}

// Update processes a key string and returns an updated Checklist.
func (c Checklist) Update(key string) Checklist {
	switch key {
	case "j", "down":
		if c.Cursor < len(c.Items)-1 {
			c.Cursor++
		}
	case "k", "up":
		if c.Cursor > 0 {
			c.Cursor--
		}
	case " ", "space":
		if len(c.Items) > 0 {
			items := make([]Item, len(c.Items))
			copy(items, c.Items)
			items[c.Cursor].Checked = !items[c.Cursor].Checked
			c.Items = items
		}
	case "a":
		checked := 0
		for _, item := range c.Items {
			if item.Checked {
				checked++
			}
		}
		target := checked < len(c.Items) // if not all checked, check all; else uncheck all
		items := make([]Item, len(c.Items))
		copy(items, c.Items)
		for i := range items {
			items[i].Checked = target
		}
		c.Items = items
	}
	return c
}

// Checked returns the Value of every checked item.
func (c Checklist) Checked() []string {
	var out []string
	for _, item := range c.Items {
		if item.Checked {
			out = append(out, item.Value)
		}
	}
	return out
}

// View renders the visible slice of the list constrained to width × height.
func (c Checklist) View(width, height int) string {
	if len(c.Items) == 0 {
		return clUnchecked.Render("  (no files)")
	}

	// Scroll offset: keep cursor in view
	offset := 0
	if c.Cursor >= height {
		offset = c.Cursor - height + 1
	}

	var lines []string
	for i := offset; i < len(c.Items) && i < offset+height; i++ {
		item := c.Items[i]
		cur := "  "
		if i == c.Cursor {
			cur = "> "
		}
		var box string
		if item.Checked {
			box = clChecked.Render("[x]")
		} else {
			box = clUnchecked.Render("[ ]")
		}
		label := item.Label
		maxLabel := width - 7 // "> [x] " = 6 chars + separator space
		if maxLabel > 3 && len(label) > maxLabel {
			label = label[:maxLabel-1] + "…"
		}
		line := cur + box + " " + label
		if i == c.Cursor {
			line = clCursor.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
