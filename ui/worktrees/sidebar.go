package worktrees

import (
	"strings"

	"gx/git"
	"gx/ui"

	"charm.land/lipgloss/v2"
	humanize "github.com/dustin/go-humanize"
)

func renderSidebarContent(wt *git.Worktree, upstream string, headCommit git.Commit, aheadCommits, behindCommits []git.Commit, rebasedOnMain *bool, isMainBranch bool, changes []git.Change, spinnerView string, useNerdFontIcons bool) string {
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

	if !isMainBranch {
		b.WriteString("\n")
		b.WriteString(ui.StyleBold.Render(ic.baseTitle))
		b.WriteString("\n\n")
		switch {
		case rebasedOnMain == nil:
			b.WriteString(ui.StyleDim.Render("  loading…") + "\n")
		case *rebasedOnMain:
			b.WriteString(ui.StyleStatusSynced.Render("  "+ic.checkmark+" rebased on main") + "\n")
		default:
			b.WriteString(ui.StyleStatusDiverged.Render("  "+ic.x+" needs rebase on main") + "\n")
		}
	}

	b.WriteString("\n")
	if upstream == "" {
		b.WriteString(ui.StyleDim.Render("  no remote tracking branch") + "\n")
		b.WriteString("  " + ui.RenderInlineBindings(keys.Track) + " " + ui.StyleDim.Render("origin/<branch>") + "\n")
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
