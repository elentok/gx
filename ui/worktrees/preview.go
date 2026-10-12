package worktrees

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"

	"charm.land/lipgloss/v2"
	humanize "github.com/dustin/go-humanize"
)

// previewBase is the selected worktree's standing against trunk.
type previewBase struct {
	trunk   string // empty when unresolved
	problem string // shown in place of the Base section while trunk is empty
	isTrunk bool
	rebased *bool // nil while loading
	ahead   int
	behind  int
}

func renderPreviewContent(wt *git.Worktree, upstream string, headCommit git.Commit, aheadCommits, behindCommits []git.Commit, base previewBase, changes []git.Change, spinnerView string, useNerdFontIcons bool) string {
	if wt == nil {
		return ui.StyleDim.Render("  no worktree selected")
	}

	var b strings.Builder
	ic := icons(useNerdFontIcons)

	titleLine := ui.StyleBold.Render(ic.worktreeTitle)
	if spinnerView != "" {
		titleLine += "  " + spinnerView
	}
	b.WriteString(titleLine)
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(wt.Name)
	b.WriteString("\n")
	if headCommit.Hash != "" {
		b.WriteString("  ")
		b.WriteString(ui.StyleDim.Render(headCommit.Hash))
		b.WriteString("  ")
		b.WriteString(headCommit.Subject)
		b.WriteString("\n")
		if !headCommit.Date.IsZero() {
			b.WriteString("  ")
			b.WriteString(ui.StyleDim.Render(headCommit.Date.Format("2006-01-02 15:04:05") + "  (" + humanize.Time(headCommit.Date) + ")"))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	if !base.isTrunk {
		b.WriteString("\n")
		b.WriteString(ui.StyleBold.Render(ic.baseTitle))
		b.WriteString("\n\n")
		switch {
		case base.trunk == "":
			b.WriteString(ui.StyleStatusBehind.Render("  "+base.problem) + "\n")
		case base.rebased == nil:
			b.WriteString(ui.StyleDim.Render("  loading…") + "\n")
		case *base.rebased:
			b.WriteString(ui.StyleStatusSynced.Render("  "+ic.checkmark+" rebased on "+base.trunk) + "\n")
		default:
			b.WriteString(ui.StyleStatusDiverged.Render("  "+ic.x+" needs rebase on "+base.trunk) + "\n")
		}
		if base.trunk != "" {
			b.WriteString(ui.StyleDim.Render(fmt.Sprintf("  %d ahead, %d behind %s", base.ahead, base.behind, base.trunk)) + "\n")
		}
	}

	b.WriteString("\n")
	if upstream == "" {
		b.WriteString(ui.StyleDim.Render("  no remote tracking branch") + "\n")
		b.WriteString("  " + ui.StyleTitle.Render("t") + " " + ui.StyleHint.Render("track") + " " + ui.StyleDim.Render("origin/<branch>") + "\n")
	} else {
		b.WriteString(ui.StyleBold.Render(ic.aheadTitle))
		b.WriteString("\n\n")
		if len(aheadCommits) == 0 {
			b.WriteString(ui.StyleDim.Render("  none") + "\n")
		} else {
			for _, c := range aheadCommits {
				b.WriteString("  ")
				b.WriteString(ui.StyleDim.Render(c.Hash))
				b.WriteString("  ")
				b.WriteString(c.Subject)
				b.WriteString("\n")
			}
		}

		b.WriteString("\n")
		b.WriteString(ui.StyleBold.Render(ic.behindTitle))
		b.WriteString("\n\n")
		if len(behindCommits) == 0 {
			b.WriteString(ui.StyleDim.Render("  none") + "\n")
		} else {
			for _, c := range behindCommits {
				b.WriteString("  ")
				b.WriteString(ui.StyleDim.Render(c.Hash))
				b.WriteString("  ")
				b.WriteString(c.Subject)
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(ui.StyleBold.Render(ic.changesTitle))
	b.WriteString("\n\n")
	if len(changes) == 0 {
		b.WriteString(ui.StyleDim.Render("  clean") + "\n")
	} else {
		for _, c := range changes {
			b.WriteString("  ")
			b.WriteString(changeKindStyle(c.Kind).Render(string(c.Kind)))
			b.WriteString("  ")
			b.WriteString(c.Path)
			b.WriteString("\n")
		}
	}

	return b.String()
}

func changeKindStyle(k git.ChangeKind) lipgloss.Style {
	switch k {
	case git.ChangeAdded:
		return ui.StyleStatusSynced
	case git.ChangeDeleted:
		return ui.StyleStatusDiverged
	case git.ChangeModified:
		return ui.StyleStatusBehind
	default:
		return ui.StyleDim
	}
}
