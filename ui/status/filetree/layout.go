package filetree

import (
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tree"
)

// MinPaneHeight is the smallest outer height at which the pane keeps its info lines.
const MinPaneHeight = 5

func (p Pane) Title(maxWidth int, v View) string {
	prefix := "Changes in "
	if summary := p.BranchSummaryTitleSuffix(maxWidth-ansi.StringWidth(prefix), v.NerdIcons); summary != "" {
		return prefix + summary
	}
	return "Filetree"
}

func (p Pane) RequiredPaneWidth(height int, v View) int {
	required := ansi.StringWidth(" " + p.Title(math.MaxInt32, v) + " ")
	if w := p.requiredWidth(height, v); w > required {
		required = w
	}
	return required + 2
}

// InnerHeight returns the number of file rows actually rendered
// inside the filetree pane at the given outer height. It mirrors
// the Model's reservation of InfoLines() (e.g. the
// "worktree: X" line shown for bare-repo checkouts), so callers that clamp
// scroll/selection via fileTreeModel.SetVisibleHeight never diverge from what
// is actually rendered.
func (p Pane) InnerHeight(height int, v View) int {
	info := p.InfoLines(v)
	contentHeight := height - len(info)
	if contentHeight < MinPaneHeight {
		contentHeight = height
	}
	return max(1, contentHeight-2)
}

// InfoLines shows which worktree is active, but only when the repo
// actually uses multiple worktrees (the .bare technique) — for a plain
// single-checkout clone the worktree name adds no information.
func (p Pane) InfoLines(v View) []string {
	if !v.IsBareRepo {
		return nil
	}
	name := ui.WorktreeName(v.WorktreeRoot)
	if name == "" {
		return nil
	}
	icon := ""
	if v.NerdIcons {
		icon = ui.Icons(true).Worktree + " "
	}
	return []string{ui.StyleMuted.Faint(true).Render(icon + "worktree: " + name)}
}

func (p Pane) ComputeSearchMatches(query string) []search.Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []search.Match{}
	}
	var matches []search.Match
	for i, entry := range p.Model.Entries() {
		text := strings.ToLower(entrySearchText(entry))
		if strings.Contains(text, q) {
			matches = append(matches, search.Match{DataIndex: i})
		}
	}
	return matches
}

func entrySearchText(entry tree.Entry[stage.StageFileStatus]) string {
	name := entry.DisplayName
	if !entry.HasChildren && entry.Value.IsRenamed() && entry.Value.RenameFrom != "" {
		name = entry.Value.RenameFrom + " -> " + entry.ID
	}
	if entry.HasChildren {
		return name + "/"
	}
	return name
}
