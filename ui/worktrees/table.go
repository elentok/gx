package worktrees

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

var styleMainBranch = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

// sortedWorktrees returns a copy of wts with the main branch worktree first.
func sortedWorktrees(wts []git.Worktree, mainBranch string) []git.Worktree {
	if mainBranch == "" {
		return wts
	}
	out := make([]git.Worktree, 0, len(wts))
	var main *git.Worktree
	for i := range wts {
		if wts[i].Branch == mainBranch {
			main = &wts[i]
		} else {
			out = append(out, wts[i])
		}
	}
	if main != nil {
		out = append([]git.Worktree{*main}, out...)
	}
	return out
}

// tableStyles holds the styles configured in newTable so our custom renderer
// can use them without needing access to the unexported table.Model.styles field.
var tableStyles table.Styles

func newTable() table.Model {
	t := table.New(table.WithFocused(true))

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ui.ColorBorder).
		BorderBottom(true).
		Bold(true)
	s.Selected = ui.StyleRowHighlight
	t.SetStyles(s)
	tableStyles = s

	return t
}

func resizeTable(t *table.Model, width, height int) {
	// Account for default table cell left/right padding (2 chars per column)
	// plus inter-column spaces to avoid overflow/wrapping.
	const (
		cols       = 4
		separators = cols - 1
		padding    = cols * 2
	)
	usable := width - separators - padding
	if usable < 20 {
		usable = 20
	}

	dirtyW := 5
	baseW := 4
	statusW := int(float64(usable) * 0.20)
	if statusW < 8 {
		statusW = 8
	}
	nameW := usable - dirtyW - baseW - statusW
	if nameW < 8 {
		nameW = 8
	}
	t.SetColumns([]table.Column{
		{Title: "Worktree", Width: nameW},
		{Title: "Dirty", Width: dirtyW},
		{Title: "Base", Width: baseW},
		{Title: "Status", Width: statusW},
	})
	t.SetWidth(width)
	t.SetHeight(height)
}

// tableView renders the table using ansi.Truncate instead of the
// runewidth.Truncate used internally by bubbles/table. This allows cell values
// to contain arbitrary ANSI escape sequences (e.g. lipgloss highlights) without
// column-alignment corruption, because ansi.Truncate is ANSI-aware and will
// never cut through an escape sequence.
func tableView(t table.Model) string {
	return headersView(t) + "\n" + rowsView(t)
}

func headersView(t table.Model) string {
	cols := t.Columns()
	renderCols := make([]ui.FixedColumn, 0, len(cols))
	for _, col := range cols {
		if col.Width <= 0 {
			continue
		}
		renderCols = append(renderCols, ui.FixedColumn{
			Text:  col.Title,
			Width: col.Width,
			Style: tableStyles.Header,
		})
	}
	return ui.RenderFixedColumns(renderCols)
}

func rowsView(t table.Model) string {
	rows := t.Rows()
	cols := t.Columns()
	cursor := t.Cursor()
	height := t.Height()

	start := cursor - height/2
	if start > len(rows)-height {
		start = len(rows) - height
	}
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > len(rows) {
		end = len(rows)
	}

	rendered := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		rendered = append(rendered, renderRow(rows[i], cols, i == cursor))
	}
	return strings.Join(rendered, "\n")
}

func renderRow(row table.Row, cols []table.Column, selected bool) string {
	renderCols := make([]ui.FixedColumn, 0, len(cols))
	for i, col := range cols {
		if col.Width <= 0 {
			continue
		}
		value := ""
		if i < len(row) {
			value = row[i]
		}
		renderCols = append(renderCols, ui.FixedColumn{
			Text:  value,
			Width: col.Width,
			Style: tableStyles.Cell,
		})
	}
	rowStr := ui.RenderFixedColumns(renderCols)
	if selected {
		return ui.RenderRowHighlight(rowStr)
	}
	return rowStr
}

// buildRows builds the table rows, applying search highlighting when a query
// is active. Since tableView uses ansi.Truncate (which is ANSI-aware), cell
// values may contain arbitrary lipgloss styles without any pre-truncation.
func (m Model) buildRows() []table.Row {
	ic := icons(m.settings.UseNerdFontIcons)
	rows := make([]table.Row, len(m.worktrees))
	for i, wt := range m.worktrees {
		isSelected := i == m.table.Cursor()
		isMain := wt.Branch == m.repo.MainBranch
		nameCol := worktreeCell(wt.Name, wt.Branch, ic, isMain, isSelected)
		if m.searchQuery != "" && !isSelected {
			nameCol = highlightMatch(nameCol, m.searchQuery)
		}
		rows[i] = table.Row{
			nameCol,
			dirtyCell(m.dirties[wt.Path], ic, isSelected),
			baseCell(m.baseStatus[wt.Branch], ic, wt.Branch == m.repo.MainBranch, isSelected),
			statusCell(m.statuses[wt.Branch], ic, isSelected, m.settings.UseNerdFontIcons),
		}
	}
	return rows
}

var styleSearchHighlight = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)

// highlightMatch wraps the first occurrence of query (case-insensitive) in
// text with a yellow bold lipgloss style.
func highlightMatch(text, query string) string {
	lower := strings.ToLower(text)
	lq := strings.ToLower(query)
	idx := strings.Index(lower, lq)
	if idx < 0 {
		return text
	}
	return text[:idx] + styleSearchHighlight.Render(text[idx:idx+len(query)]) + text[idx+len(query):]
}

func worktreeCell(name, branch string, ic uiIcons, isMain, _ bool) string {
	prefix := ic.worktreePrefix
	if isMain && ic.mainPrefix != "" {
		prefix = ic.mainPrefix
	}
	text := prefix + name

	if branch != "" && branch != name {
		branchSuffix := "(" + ic.branchPrefix + branch + ")"
		if isMain {
			return styleMainBranch.Render(text + " " + branchSuffix)
		} else {
			text += " " + ui.StyleDim.Render(branchSuffix)
		}
		return text
	}

	if isMain {
		return styleMainBranch.Render(text)
	}
	return text
}

func dirtyCell(d dirtyState, ic uiIcons, _ bool) string {
	switch {
	case d.hasModified && d.hasUntracked:
		return "M?"
	case d.hasModified:
		return "M"
	case d.hasUntracked:
		return "?"
	}
	return ui.StyleDim.Render(ic.dash)
}

func baseCell(rebased *bool, ic uiIcons, isMainBranch bool, _ bool) string {
	if isMainBranch {
		return ui.StyleDim.Render(ic.dash)
	}
	if rebased == nil {
		return "" // not yet loaded
	}
	if *rebased {
		return ui.StyleStatusSynced.Render(ic.checkmark)
	}
	return ui.StyleStatusDiverged.Render(ic.x)
}

func statusCell(s git.SyncStatus, ic uiIcons, _ bool, useNerdFontIcons bool) string {
	label := ic.dash
	switch s.Name {
	case git.StatusSame:
		label = "synced"
	case git.StatusAhead, git.StatusBehind, git.StatusDiverged:
		label = s.Pretty()
	}
	if useNerdFontIcons {
		label = strings.ReplaceAll(label, "ahead", ic.ahead)
		label = strings.ReplaceAll(label, "behind", ic.behind)
	}
	switch s.Name {
	case git.StatusSame:
		return ui.StyleStatusSynced.Render(label)
	case git.StatusAhead:
		return ui.StyleStatusAhead.Render(label)
	case git.StatusBehind:
		return ui.StyleStatusBehind.Render(label)
	case git.StatusDiverged:
		return ui.StyleStatusDiverged.Render(label)
	default:
		return ui.StyleStatusUnknown.Render(label)
	}
}
