package diffarea

import (
	"fmt"
	"image/color"

	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/diffview/diffrender"

	"charm.land/lipgloss/v2"
)

const (
	collapsedSectionHeight = 3
	emptyMessage           = "No file selected"
)

// PaneInput carries the parent state a render needs: the area itself owns
// only the diff sections.
type PaneInput struct {
	// DiffFocused is true while keyboard focus is on the diff pane.
	DiffFocused bool
	// NerdFont selects nerd-font icons for the header indicators.
	NerdFont bool
	// File is the displayed path of the selected file; empty when none is selected.
	File string
	// BinaryLines appends the body of a binary diff section.
	BinaryLines func(lines []string, bodyH, innerW int) []string
}

// Title is the section's panel title.
func (s Section) Title() string {
	if s == SectionStaged {
		return "Staged"
	}
	return "Unstaged"
}

// DisplayedPath is the path shown in a section title, with the rename source.
func DisplayedPath(file stage.StageFileStatus) string {
	if file.IsRenamed() && file.RenameFrom != "" {
		return file.RenameFrom + " -> " + file.Path
	}
	return file.Path
}

// PaneHeights splits total rows into the expanded and collapsed section heights.
func PaneHeights(total int) (expanded, collapsed int) {
	if total <= 0 {
		return 0, 0
	}
	collapsed = collapsedSectionHeight
	if total < collapsed+3 {
		collapsed = max(1, total-3)
	}
	if collapsed < 0 {
		collapsed = 0
	}
	expanded = total - collapsed
	if expanded <= 0 {
		expanded = total
		collapsed = 0
	}
	return expanded, collapsed
}

func (d *Model) Render(width, height int, in PaneInput) string {
	if height <= 0 || width <= 0 {
		return ""
	}

	expandedH, collapsedH := PaneHeights(height - 1) // seam row
	seam := ui.RenderSeamRow(width, ui.SeamColor)
	if d.ActiveSection == SectionStaged {
		top := d.RenderSection(width, collapsedH, SectionUnstaged, in)
		bottom := d.RenderSection(width, expandedH, SectionStaged, in)
		return lipgloss.JoinVertical(lipgloss.Left, top, seam, bottom)
	}

	top := d.RenderSection(width, expandedH, SectionUnstaged, in)
	bottom := d.RenderSection(width, collapsedH, SectionStaged, in)
	return lipgloss.JoinVertical(lipgloss.Left, top, seam, bottom)
}

func (d *Model) RenderSection(width, height int, section Section, in PaneInput) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	innerW := max(1, width-2)
	innerH := max(1, height-2)

	activeSection := in.DiffFocused && d.ActiveSection == section
	collapsed := !activeSection && height <= collapsedSectionHeight

	bodyH := innerH
	if bodyH < 0 {
		bodyH = 0
	}

	title := section.Title()
	diffviewModel := d.SectionModel(section)
	diff := diffviewModel.DataRef()
	accent := ui.ColorOrange
	if section == SectionStaged {
		accent = ui.ColorGreen
	}
	diffviewModel.Viewport().SetHeight(max(0, bodyH))
	diffviewModel.Viewport().SetWidth(innerW)

	titleText := d.sectionPaneTitle(title, section, !collapsed, in.File)
	if si := diffrender.ParseSymlinkDiffInfo(diff.Parsed); si.IsSymlink {
		if label := si.TitleLabel(); label != "" {
			titleText += " " + label
		}
	}
	if d.Fullscreen {
		titleText += " [fullscreen]"
	}
	rightTitleText := d.headerOptions(diffviewModel, section, activeSection, in, accent)

	lines := make([]string, 0, bodyH)
	if len(diff.ViewLines) == 0 && diffcore.HasBinaryDiff(diff.Parsed) {
		lines = in.BinaryLines(lines, bodyH, innerW)
	}
	if len(lines) == 0 && bodyH > 0 {
		if placeholder := d.placeholder(section, collapsed, in.File != ""); placeholder != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorSubtle).Render(placeholder))
		}
	}

	if len(lines) == 0 {
		lines = diffviewModel.RenderRows(bodyH, activeSection, diffview.RenderOpts{
			AccentColor: accent,
			InnerWidth:  innerW,
			SearchMatch: func(displayIdx int) (bool, bool) {
				return diffviewModel.SearchMatchAt(displayIdx)
			},
			SearchQuery: diffviewModel.Search().Query(),
		})
	}
	for len(lines) < bodyH {
		lines = append(lines, "")
	}
	return d.renderPanel(width, height, titleText, rightTitleText, lines, activeSection, section)
}

func (d *Model) renderPanel(width, height int, title, rightTitle string, lines []string, active bool, section Section) string {
	highlightMoved := d.Flash.Active && d.Flash.Section == section
	titleColor := ui.ColorOrange
	if section == SectionStaged {
		titleColor = ui.ColorGreen
	}
	accent := color.Color(nil)
	if active || highlightMoved {
		accent = titleColor
	}
	opts := ui.PanelOptionsFor(width, height, title, rightTitle, lines, active || highlightMoved, titleColor, accent, false)
	opts.RightTitleStyled = true
	return ui.RenderPanel(opts)
}

// headerOptions builds the pre-styled right side of a diff panel
// header. Only the focused panel shows the diff options; the others keep just
// their scroll % and search counter.
func (d *Model) headerOptions(diffviewModel *diffview.Model, section Section, focused bool, in PaneInput, accent color.Color) string {
	dim := lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	search := d.SearchCounter(section, in.DiffFocused, in.NerdFont)
	if !focused {
		if text := diffview.JoinDot(diffviewModel.ScrollPercentText(), search); text != "" {
			return dim.Render(text)
		}
		return ""
	}
	context := fmt.Sprintf("±%d", d.ContextLines())
	out := dim.Render(diffview.JoinDot(d.NavModeLabel(), context, diffviewModel.ScrollPercentText(), search))
	if d.wrap {
		wrap := "wrap"
		if icon := ui.Icons(in.NerdFont).Wrap; icon != "" {
			wrap = icon + " " + wrap
		}
		out = lipgloss.NewStyle().Foreground(accent).Render(wrap) + dim.Render(" · ") + out
	}
	if diffviewModel.DataRef().VisualActive {
		out = ui.RenderChip("VISUAL", accent) + " " + out
	}
	return out
}

func (d *Model) sectionPaneTitle(title string, section Section, expanded bool, file string) string {
	if !expanded || file == "" {
		if !d.SectionHasContent(section) {
			return title + " (empty)"
		}
		return title
	}
	return title + ": " + file
}

func (d *Model) placeholder(section Section, collapsed, hasFile bool) string {
	if !hasFile {
		return emptyMessage
	}
	if !d.SectionHasContent(section) {
		if collapsed {
			return "empty"
		}
		return "No changes"
	}
	return ""
}

func (d Model) SectionHasContent(section Section) bool {
	data := d.SectionModel(section).Data()
	return data.HasContent()
}

func (d Model) NavModeLabel() string {
	if d.navMode == diffview.NavModeLine {
		return "line"
	}
	return "hunk"
}
