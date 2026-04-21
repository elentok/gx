package stage

import (
	"image/color"
	"strings"

	"github.com/elentok/gx/git"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) renderBranchCommitsPane(width, height int) string {
	innerW := maxInt(1, width-2)
	innerH := maxInt(1, height-2)

	lines := make([]string, 0, innerH)
	if len(m.branchCommits) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(catSubtle).Render("no commits since main"))
	} else {
		for i, commit := range m.branchCommits {
			lines = append(lines, m.renderBranchCommitCard(commit, innerW)...)
			if i < len(m.branchCommits)-1 {
				lines = append(lines, "")
			}
			if len(lines) >= innerH {
				break
			}
		}
	}

	for len(lines) < innerH {
		lines = append(lines, "")
	}

	return m.renderPanelWithBorderTitle(width, height, "Commits", "", lines[:innerH], false, sectionUnstaged)
}

func (m Model) renderBranchCommitCard(commit branchCommitRow, width int) []string {
	subjectStyle := lipgloss.NewStyle().Foreground(m.branchCommitColor(commit.class))
	metaStyle := lipgloss.NewStyle().Foreground(catSubtle).Faint(true).Italic(true)

	subjectLines := wrapPlainText(commit.subject, width)
	if len(subjectLines) == 0 {
		subjectLines = []string{""}
	}

	lines := make([]string, 0, len(subjectLines)+1)
	for _, line := range subjectLines {
		lines = append(lines, subjectStyle.Render(line))
	}
	lines = append(lines, metaStyle.Render(humanizeOrUnknown(commit.date)+", "+commit.hash))
	return lines
}

func (m Model) branchCommitColor(class git.BranchHistoryClass) color.Color {
	switch class {
	case git.BranchHistoryLocalOnly:
		return catGreen
	case git.BranchHistoryRemoteOnly:
		return catRed
	default:
		return catText
	}
}

func wrapPlainText(text string, width int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if width <= 1 {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	lines := make([]string, 0, 2)
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if ansi.StringWidth(candidate) <= width {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = word
	}
	lines = append(lines, current)
	return lines
}
